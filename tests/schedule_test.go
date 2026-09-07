package tests

import (
	"testing"

	"webtyp.com/events"
	"webtyp.com/events/mock"
	"webtyp.com/orm"
	"webtyp.com/router/loopback"
	"webtyp.com/storage/mem"
	ab "github.com/veltylabs/appointment_booking"
)

func seedCalendar(t *testing.T, m *ab.Module, tenantId, staffId string) {
	t.Helper()
	if err := m.UpsertCalendarConfig(ab.WorkCalendarConfig{
		TenantId: tenantId, StaffId: staffId, Timezone: "UTC", IsActive: true,
	}); err != nil {
		t.Fatalf("UpsertCalendarConfig: %v", err)
	}
}

func newModuleWithBroker(t *testing.T) (*ab.Module, *mock.Broker) {
	t.Helper()
	db := orm.New(mem.New())
	broker := &mock.Broker{}
	deps := SetupDependencies()
	deps.Publisher = broker
	m, err := ab.New(db, deps)
	if err != nil {
		t.Fatalf("ab.New: %v", err)
	}
	return m, broker
}

func TestListWeeklyOp_ReturnsRows(t *testing.T) {
	m, _ := newModuleWithBroker(t)
	seedCalendar(t, m, "t1", "s1")

	days := []int64{1, 2, 3}
	for _, d := range days {
		if err := m.UpsertWeeklyCalendar(ab.WorkCalendarWeekly{
			TenantId: "t1", StaffId: "s1", DayOfWeek: d,
			WorkStart: 540, WorkFinish: 1020, IsActive: true,
		}); err != nil {
			t.Fatalf("UpsertWeeklyCalendar day %d: %v", d, err)
		}
	}

	caller := loopback.New(m)
	out := &ab.WorkCalendarWeeklyList{}
	var got error
	caller.Call(ab.OpListWeeklyCalendar, &ab.ListWeeklyCalendarArgs{TenantId: "t1", StaffId: "s1"}, out, func(err error) { got = err })
	if got != nil {
		t.Fatalf("Call: %v", got)
	}
	if out.Len() != 3 {
		t.Fatalf("expected 3 rows, got %d", out.Len())
	}
}

func TestListExceptionsOp_RangeFilter(t *testing.T) {
	m, _ := newModuleWithBroker(t)
	seedCalendar(t, m, "t1", "s1")

	from, to := int64(1767139200), int64(1767225600) // Dec 31 2025 – Jan 1 2026 (UTC)
	outOfRange := int64(1767312000)                  // Jan 2 2026
	for _, date := range []int64{from, to, outOfRange} {
		if err := m.AddException(ab.WorkCalendarException{
			TenantId: "t1", StaffId: "s1", SpecificDate: date,
			ExceptionType: ab.ExcBlocked, StartTime: 0, EndTime: 1439,
		}); err != nil {
			t.Fatalf("AddException: %v", err)
		}
	}

	caller := loopback.New(m)
	out := &ab.WorkCalendarExceptionList{}
	var got error
	caller.Call(ab.OpListExceptions, &ab.ListExceptionsArgs{TenantId: "t1", StaffId: "s1", From: from, To: to}, out, func(err error) { got = err })
	if got != nil {
		t.Fatalf("Call: %v", got)
	}
	if out.Len() != 2 {
		t.Fatalf("expected 2 exceptions in range, got %d", out.Len())
	}
}

func TestUpsertWeekly_PublishesScheduleChanged(t *testing.T) {
	m, broker := newModuleWithBroker(t)
	seedCalendar(t, m, "t1", "s1")

	var payload *ab.ScheduleChangedPayload
	broker.Subscribe(ab.EventScheduleChanged, func(e events.Event) {
		payload = e.Payload.(*ab.ScheduleChangedPayload)
	})

	if err := m.UpsertWeeklyCalendar(ab.WorkCalendarWeekly{
		TenantId: "t1", StaffId: "s1", DayOfWeek: 1,
		WorkStart: 540, WorkFinish: 1020, IsActive: true,
	}); err != nil {
		t.Fatalf("UpsertWeeklyCalendar: %v", err)
	}
	if payload == nil {
		t.Fatal("expected schedule.changed event, got none")
	}
	if payload.StaffId != "s1" || payload.TenantId != "t1" {
		t.Fatalf("unexpected payload: %+v", payload)
	}
}

func TestAddException_PublishesScheduleChanged(t *testing.T) {
	m, broker := newModuleWithBroker(t)
	seedCalendar(t, m, "t1", "s1")

	var received []*ab.ScheduleChangedPayload
	broker.Subscribe(ab.EventScheduleChanged, func(e events.Event) {
		received = append(received, e.Payload.(*ab.ScheduleChangedPayload))
	})

	if err := m.AddException(ab.WorkCalendarException{
		TenantId: "t1", StaffId: "s1", SpecificDate: 1767139200,
		ExceptionType: ab.ExcHoliday,
	}); err != nil {
		t.Fatalf("AddException: %v", err)
	}
	if len(received) != 1 || received[0].StaffId != "s1" {
		t.Fatalf("expected 1 schedule.changed with staff s1, got %+v", received)
	}
}

func TestRemoveException_PublishesScheduleChanged(t *testing.T) {
	m, broker := newModuleWithBroker(t)
	seedCalendar(t, m, "t1", "s1")

	if err := m.AddException(ab.WorkCalendarException{
		TenantId: "t1", StaffId: "s1", SpecificDate: 1767139200,
		ExceptionType: ab.ExcBlocked,
	}); err != nil {
		t.Fatalf("AddException: %v", err)
	}
	excs, err := m.ListExceptions("t1", "s1", 1767139200, 1767139200)
	if err != nil || len(excs) != 1 {
		t.Fatalf("ListExceptions after add: %v, %d", err, len(excs))
	}
	excID := excs[0].Id

	var evPayload *ab.ScheduleChangedPayload
	broker.Subscribe(ab.EventScheduleChanged, func(e events.Event) {
		evPayload = e.Payload.(*ab.ScheduleChangedPayload)
	})

	if err := m.RemoveException("t1", excID); err != nil {
		t.Fatalf("RemoveException: %v", err)
	}
	if evPayload == nil || evPayload.StaffId != "s1" {
		t.Fatalf("expected schedule.changed with the removed exception staffId, got %+v", evPayload)
	}
}

func TestScheduleClient_RoundTrip(t *testing.T) {
	m, _ := newModuleWithBroker(t)
	seedCalendar(t, m, "t1", "s1")

	caller := loopback.New(m)
	cl := ab.NewScheduleClient(caller, "t1", "s1")

	var saveErr error
	cl.SaveWeeklyRow(ab.WorkCalendarWeekly{
		TenantId: "t1", StaffId: "s1", DayOfWeek: 5,
		WorkStart: 540, WorkFinish: 1020, IsActive: true,
	}, func(err error) { saveErr = err })
	if saveErr != nil {
		t.Fatalf("SaveWeeklyRow: %v", saveErr)
	}

	var rows []ab.WorkCalendarWeekly
	cl.Weekly(func(r []ab.WorkCalendarWeekly, err error) {
		rows, saveErr = r, err
	})
	if saveErr != nil {
		t.Fatalf("Weekly: %v", saveErr)
	}
	if len(rows) != 1 || rows[0].DayOfWeek != 5 || rows[0].WorkStart != 540 {
		t.Fatalf("unexpected weekly rows: %+v", rows)
	}
}

func TestRemoveException_UnknownIsNotFound(t *testing.T) {
	m, _ := newModuleWithBroker(t)
	if err := m.RemoveException("t1", "nope"); err != ab.ErrNotFound {
		t.Fatalf("expected ab.ErrNotFound, got %v", err)
	}
}