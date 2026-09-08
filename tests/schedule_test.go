package tests

import (
	"testing"

	"webtyp.com/events"
	"webtyp.com/events/mock"
	"webtyp.com/orm"
	"webtyp.com/router/loopback"
	"webtyp.com/storage/mem"
	tinytime "webtyp.com/time"
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

func TestListBlocksOp_ReturnsRows(t *testing.T) {
	m, _ := newModuleWithBroker(t)
	seedCalendar(t, m, "t1", "s1")

	days := []int{1, 2, 3}
	for _, d := range days {
		if err := m.SaveDayBlocks("t1", "s1", d, []ab.WorkCalendarBlock{
			{StartMin: 540, EndMin: 1020, IsActive: true},
		}); err != nil {
			t.Fatalf("SaveDayBlocks day %d: %v", d, err)
		}
	}

	caller := loopback.New(m)
	out := &ab.WorkCalendarBlockList{}
	var got error
	caller.Call(ab.OpListBlocks, &ab.ListBlocksArgs{TenantId: "t1", StaffId: "s1"}, out, func(err error) { got = err })
	if got != nil {
		t.Fatalf("Call: %v", got)
	}
	if out.Len() != 3 {
		t.Fatalf("expected 3 blocks, got %d", out.Len())
	}
}

func TestGetDayBoundsOp_NilBoundsMeansUnbounded(t *testing.T) {
	m, _ := newModuleWithBroker(t)

	caller := loopback.New(m)
	out := &ab.DayBoundsResult{}
	var got error
	caller.Call(ab.OpGetDayBounds, &ab.GetDayBoundsArgs{TenantId: "t1", Date: 1767139200}, out, func(err error) { got = err })
	if got != nil {
		t.Fatalf("Call: %v", got)
	}
	if !out.Open || out.OpenMin != 0 || out.CloseMin != 1440 {
		t.Fatalf("expected unbounded bounds when Deps.Bounds is nil, got %+v", out)
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

func TestSaveDayBlocks_NoAffectedReservationsIsSilent(t *testing.T) {
	// CU-19 — cambiar la agenda sin reservas afectadas no publica nada.
	m, broker := newModuleWithBroker(t)
	seedCalendar(t, m, "t1", "s1")

	var count int
	broker.Subscribe(ab.EventScheduleChanged, func(e events.Event) { count++ })
	broker.Subscribe(ab.EventReservationConflicted, func(e events.Event) { count++ })

	if err := m.SaveDayBlocks("t1", "s1", 1, []ab.WorkCalendarBlock{
		{StartMin: 540, EndMin: 1020, IsActive: true},
	}); err != nil {
		t.Fatalf("SaveDayBlocks: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected no events without affected reservations, got %d", count)
	}
}

func TestAddExceptionHoliday_ConflictsAndPublishes(t *testing.T) {
	broker := &mock.Broker{}
	deps := SetupDependencies()
	deps.Publisher = broker
	m, repo, _ := newTestModule(t, deps)
	seedCalendar(t, m, "t1", "s1")
	d := Date(2027, 6, 7, 0, 0, 0, 0)
	seedWeekdayBlock(t, m, "t1", "s1", int(tinytime.Weekday(d)), defaultBlocks())
	cfgID := seedEmployeeConfig(t, repo, "t1", "s1", 60)
	createBooked(t, m, "t1", "s1", cfgID, d+10*3600)

	var sched *ab.ScheduleChangedPayload
	broker.Subscribe(ab.EventScheduleChanged, func(e events.Event) {
		sched = e.Payload.(*ab.ScheduleChangedPayload)
	})

	if err := m.AddException(ab.WorkCalendarException{
		TenantId: "t1", StaffId: "s1", SpecificDate: d, ExceptionType: ab.ExcHoliday,
	}); err != nil {
		t.Fatalf("AddException: %v", err)
	}
	if sched == nil || sched.StaffId != "s1" {
		t.Fatalf("expected schedule.changed with staff s1, got %+v", sched)
	}
	if sched.ConflictCount != 1 {
		t.Fatalf("expected ConflictCount 1, got %d", sched.ConflictCount)
	}
}

func TestRemoveExceptionHoliday_RestoresAndIsSilentOnNewConflicts(t *testing.T) {
	broker := &mock.Broker{}
	deps := SetupDependencies()
	deps.Publisher = broker
	m, repo, _ := newTestModule(t, deps)
	seedCalendar(t, m, "t1", "s1")
	d := Date(2027, 6, 8, 0, 0, 0, 0)
	seedWeekdayBlock(t, m, "t1", "s1", int(tinytime.Weekday(d)), defaultBlocks())
	cfgID := seedEmployeeConfig(t, repo, "t1", "s1", 60)
	createBooked(t, m, "t1", "s1", cfgID, d+10*3600)

	if err := m.AddException(ab.WorkCalendarException{
		TenantId: "t1", StaffId: "s1", SpecificDate: d, ExceptionType: ab.ExcHoliday,
	}); err != nil {
		t.Fatalf("AddException: %v", err)
	}
	excs, err := m.ListExceptions("t1", "s1", d, d)
	if err != nil || len(excs) != 1 {
		t.Fatalf("ListExceptions after add: %v, %d", err, len(excs))
	}

	var count int
	broker.Subscribe(ab.EventScheduleChanged, func(e events.Event) { count++ })

	if err := m.RemoveException("t1", excs[0].Id); err != nil {
		t.Fatalf("RemoveException: %v", err)
	}
	got, _ := m.GetReservation("t1", firstReservation(t, m))
	if got.Status != ab.StatusConfirmed {
		t.Fatalf("expected the reservation restored to CONFIRMED, got %s", got.Status)
	}
	if count != 0 {
		t.Fatalf("removing a holiday that resolves conflicts does not put anyone in conflict; expected 0 events, got %d", count)
	}
}

func firstReservation(t *testing.T, m *ab.Module) string {
	t.Helper()
	rows, err := m.ListReservationsByStaff("t1", "s1", 0, Date(2030, 1, 1, 0, 0, 0, 0))
	if err != nil || len(rows) == 0 {
		t.Fatalf("ListReservationsByStaff: %v, %d", err, len(rows))
	}
	return rows[0].Id
}

func TestScheduleClient_RoundTrip(t *testing.T) {
	m, _ := newModuleWithBroker(t)
	seedCalendar(t, m, "t1", "s1")

	caller := loopback.New(m)
	cl := ab.NewScheduleClient(caller, "t1", "s1")

	var saveErr error
	cl.SaveDayBlocks(5, []ab.WorkCalendarBlock{
		{StartMin: 540, EndMin: 1020, IsActive: true},
	}, func(err error) { saveErr = err })
	if saveErr != nil {
		t.Fatalf("SaveDayBlocks: %v", saveErr)
	}

	var rows []ab.WorkCalendarBlock
	cl.Blocks(func(r []ab.WorkCalendarBlock, err error) {
		rows, saveErr = r, err
	})
	if saveErr != nil {
		t.Fatalf("Blocks: %v", saveErr)
	}
	if len(rows) != 1 || rows[0].DayOfWeek != 5 || rows[0].StartMin != 540 {
		t.Fatalf("unexpected blocks: %+v", rows)
	}
}

func TestRemoveException_UnknownIsNotFound(t *testing.T) {
	m, _ := newModuleWithBroker(t)
	if err := m.RemoveException("t1", "nope"); err != ab.ErrNotFound {
		t.Fatalf("expected ab.ErrNotFound, got %v", err)
	}
}