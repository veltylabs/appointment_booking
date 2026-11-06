package tests

import (
	"testing"

	"webtyp.com/model"
	"webtyp.com/time"

	ab "github.com/veltylabs/appointment_booking"
)

func TestImportReservationPast(t *testing.T) {
	e := newBookEnv(t)
	// inactive config, time in 2019
	cfg, _ := e.m.CreateEmployeeServiceConfig(ab.EmployeeServiceConfig{TenantId: bTenant, StaffId: "s1", ServiceId: bService, DurationMin: 30, IsActive: false})

	pastTime := time.DateUTC(2019, 10, 15, 14, 30, 0)

	r := ab.Reservation{
		Id: "imp1", TenantId: bTenant, ClientId: "c1",
		EmployeeServiceConfigId: cfg.Id, ReservationTime: int64(pastTime),
		Status: ab.StatusCompleted, Origin: ab.OriginCounter,
	}

	got, err := e.m.ImportReservation(r)
	if err != nil {
		t.Fatalf("ImportReservation: %v", err)
	}

	if got.Status != ab.StatusCompleted {
		t.Errorf("status = %s, want %s", got.Status, ab.StatusCompleted)
	}
	if got.ReservationTime != int64(pastTime) {
		t.Errorf("time = %d, want %d", got.ReservationTime, int64(pastTime))
	}
}

func TestImportReservationTwice(t *testing.T) {
	e := newBookEnv(t)
	cfg, _ := e.m.CreateEmployeeServiceConfig(ab.EmployeeServiceConfig{TenantId: bTenant, StaffId: "s1", ServiceId: bService, DurationMin: 30, IsActive: true})

	t1 := int64(time.DateUTC(2023, 1, 1, 10, 0, 0))
	r1 := ab.Reservation{
		Id: "imp2", TenantId: bTenant, ClientId: "c1",
		EmployeeServiceConfigId: cfg.Id, ReservationTime: t1,
		Status: ab.StatusConfirmed, Origin: ab.OriginOnline,
		Notes: "first",
	}

	_, err := e.m.ImportReservation(r1)
	if err != nil {
		t.Fatal(err)
	}

	r2 := r1
	r2.Notes = "second"

	got, err := e.m.ImportReservation(r2)
	if err != nil {
		t.Fatal(err)
	}

	if got.Notes != "second" {
		t.Errorf("notes = %s, want second", got.Notes)
	}
	if got.Revision != 1 {
		t.Errorf("revision = %d, want 1", got.Revision)
	}
}

func TestImportReservationNoSlotNeeded(t *testing.T) {
	e := newBookEnv(t)
	// professional with no calendar blocks
	cfg, _ := e.m.CreateEmployeeServiceConfig(ab.EmployeeServiceConfig{TenantId: bTenant, StaffId: "s2", ServiceId: bService, DurationMin: 30, IsActive: true})

	r := ab.Reservation{
		Id: "imp3", TenantId: bTenant, ClientId: "c1",
		EmployeeServiceConfigId: cfg.Id, ReservationTime: int64(bookDay),
		Status: ab.StatusConfirmed, Origin: ab.OriginCounter,
	}

	_, err := e.m.ImportReservation(r)
	if err != nil {
		t.Errorf("expected to store without slot needed, got %v", err)
	}
}

func TestImportReservationBadStatus(t *testing.T) {
	e := newBookEnv(t)
	cfg, _ := e.m.CreateEmployeeServiceConfig(ab.EmployeeServiceConfig{TenantId: bTenant, StaffId: "s1", ServiceId: bService, DurationMin: 30, IsActive: true})

	r := ab.Reservation{
		Id: "imp4", TenantId: bTenant, ClientId: "c1",
		EmployeeServiceConfigId: cfg.Id, ReservationTime: int64(bookDay),
		Status: ab.StatusPending, Origin: ab.OriginCounter,
	}

	_, err := e.m.ImportReservation(r)
	if err != ab.ErrImportStatus {
		t.Errorf("err = %v, want ErrImportStatus", err)
	}
}

func TestImportReservationUnknownClient(t *testing.T) {
	e := newBookEnv(t)
	cfg, _ := e.m.CreateEmployeeServiceConfig(ab.EmployeeServiceConfig{TenantId: bTenant, StaffId: "s1", ServiceId: bService, DurationMin: 30, IsActive: true})

	// client port says no
	e.dir.Exists = false

	r := ab.Reservation{
		Id: "imp5", TenantId: bTenant, ClientId: "c_unknown",
		EmployeeServiceConfigId: cfg.Id, ReservationTime: int64(bookDay),
		Status: ab.StatusConfirmed, Origin: ab.OriginCounter,
	}

	_, err := e.m.ImportReservation(r)
	if err != ab.ErrClientNotFound {
		t.Errorf("err = %v, want ErrClientNotFound", err)
	}
}

func TestImportReservationOtherTenant(t *testing.T) {
	e := newBookEnv(t)
	cfg1, _ := e.m.CreateEmployeeServiceConfig(ab.EmployeeServiceConfig{TenantId: bTenant, StaffId: "s1", ServiceId: bService, DurationMin: 30, IsActive: true})

	r1 := ab.Reservation{
		Id: "imp6", TenantId: bTenant, ClientId: "c1",
		EmployeeServiceConfigId: cfg1.Id, ReservationTime: int64(bookDay),
		Status: ab.StatusConfirmed, Origin: ab.OriginCounter,
	}
	_, err := e.m.ImportReservation(r1)
	if err != nil {
		t.Fatal(err)
	}

	cfg2, _ := e.m.CreateEmployeeServiceConfig(ab.EmployeeServiceConfig{TenantId: "t2", StaffId: "s1", ServiceId: bService, DurationMin: 30, IsActive: true})

	r2 := ab.Reservation{
		Id: "imp6", TenantId: "t2", ClientId: "c1",
		EmployeeServiceConfigId: cfg2.Id, ReservationTime: int64(bookDay),
		Status: ab.StatusConfirmed, Origin: ab.OriginCounter,
	}

	_, err = e.m.ImportReservation(r2)
	if err != ab.ErrIdTaken {
		t.Errorf("err = %v, want ErrIdTaken", err)
	}
}

func TestImportReservationKeepsGivenDuration(t *testing.T) {
	e := newBookEnv(t)
	cfg, _ := e.m.CreateEmployeeServiceConfig(ab.EmployeeServiceConfig{TenantId: bTenant, StaffId: "s1", ServiceId: bService, DurationMin: 30, IsActive: true})

	r := ab.Reservation{
		Id: "imp7", TenantId: bTenant, ClientId: "c1",
		EmployeeServiceConfigId: cfg.Id, ReservationTime: int64(bookDay),
		Status: ab.StatusConfirmed, Origin: ab.OriginCounter,
		DurationMinSnapshot: 45,
	}

	got, err := e.m.ImportReservation(r)
	if err != nil {
		t.Fatal(err)
	}

	if got.DurationMinSnapshot != 45 {
		t.Errorf("duration = %d, want 45", got.DurationMinSnapshot)
	}
}

// El instante UTC se conserva tal cual: es la verdad única de la reserva.
func TestImportReservationKeepsInstant(t *testing.T) {
	e := newBookEnv(t)
	cfg, _ := e.m.CreateEmployeeServiceConfig(ab.EmployeeServiceConfig{TenantId: bTenant, StaffId: "s1", ServiceId: bService, DurationMin: 30, IsActive: false})
	wantTime := int64(time.DateUTC(2019, 7, 16, 1, 30, 0))
	r := ab.Reservation{
		Id: "imp-local", TenantId: bTenant, ClientId: "c1",
		EmployeeServiceConfigId: cfg.Id, ReservationTime: wantTime,
		Status: ab.StatusCompleted, Origin: ab.OriginCounter,
	}
	got, err := e.m.ImportReservation(r)
	if err != nil {
		t.Fatalf("ImportReservation: %v", err)
	}
	if got.ReservationTime != wantTime {
		t.Errorf("time = %d, want %d", got.ReservationTime, wantTime)
	}
	scl, _ := time.LoadZone("America/Santiago")
	dateStr := scl.Date(time.Unix(got.ReservationTime))
	clockStr := scl.Clock(time.Unix(got.ReservationTime))
	if dateStr != "2019-07-15" || clockStr != "21:30" {
		t.Errorf("derived local = %s %s, want 2019-07-15 21:30", dateStr, clockStr)
	}
}

// A schedule from the previous system is kept as it was, also when it falls outside today's opening hours
// (Pa100T: Saturdays until 18:00, the clinic now closes at 12:30). The importer reports it as a notice for the front
// desk to adjust later; nothing is trimmed or dropped. Availability still offers only the opening hours.
func TestImportDayBlocksKeepsBlocksOutsideOpeningHours(t *testing.T) {
	deps := SetupDependenciesWithBounds(&MockBoundsReader{
		Default: time.DayLimits{Open: true, OpenMin: 540, CloseMin: 780}, // 09:00–13:00
	})
	s, _ := newTestModuleService(t, deps)
	ensureCalendarConfig(t, s, "t1", "s1")
	blocks := []ab.WorkCalendarBlock{{StartMin: 540, EndMin: 780}, {StartMin: 840, EndMin: 1080}} // 09–13 y 14–18

	if err := s.SaveDayBlocks("t1", "s1", 6, append([]ab.WorkCalendarBlock(nil), blocks...)); err == nil {
		t.Fatal("SaveDayBlocks (the screen) must still refuse a block outside opening hours")
	}
	if err := s.ImportDayBlocks("t1", "s1", 6, append([]ab.WorkCalendarBlock(nil), blocks...)); err != nil {
		t.Fatalf("ImportDayBlocks: %v", err)
	}
	got, err := s.ListBlocks("t1", "s1")
	if err != nil {
		t.Fatalf("ListBlocks: %v", err)
	}
	if len(got) != 2 || got[1].StartMin != 840 || got[1].EndMin != 1080 || got[1].DayOfWeek != 6 {
		t.Fatalf("expected both blocks kept as given, got %+v", got)
	}
	if err := s.ImportDayBlocks("t1", "s1", 6, append([]ab.WorkCalendarBlock(nil), blocks...)); err != nil {
		t.Fatalf("ImportDayBlocks twice: %v", err)
	}
	if again, _ := s.ListBlocks("t1", "s1"); len(again) != 2 {
		t.Fatalf("importing twice must leave the same 2 blocks, got %d", len(again))
	}
	if err := s.ImportDayBlocks("t1", "s1", 6, []ab.WorkCalendarBlock{{StartMin: 600, EndMin: 540}}); err == nil {
		t.Fatal("a block that ends before it starts is still invalid")
	}
}

// A booking from the previous system is history: it imports although its service is inactive today (the importer
// leaves a service without a price inactive until someone sets it). A service that does not exist at all is a
// typed not-found, never an untyped error the importer would take for a database failure.
func TestImportReservationInactiveService(t *testing.T) {
	e := newBookEnv(t)
	e.catalog.items = append(e.catalog.items, serviceItem{"svc-off", "Consulta sin precio", pmCurrencyCLP, 0, false})
	cfg, _ := e.m.CreateEmployeeServiceConfig(ab.EmployeeServiceConfig{TenantId: bTenant, StaffId: "s1", ServiceId: "svc-off", DurationMin: 20, IsActive: true})
	r := ab.Reservation{Id: "imp-off", TenantId: bTenant, ClientId: "c1", EmployeeServiceConfigId: cfg.Id,
		ReservationTime: int64(time.DateUTC(2022, 3, 21, 10, 0, 0)), Status: ab.StatusCompleted, Origin: ab.OriginCounter}
	if _, err := e.m.ImportReservation(r); err != nil {
		t.Fatalf("ImportReservation with an inactive service: %v", err)
	}

	gone, _ := e.m.CreateEmployeeServiceConfig(ab.EmployeeServiceConfig{TenantId: bTenant, StaffId: "s1", ServiceId: "svc-gone", DurationMin: 20})
	r.Id, r.EmployeeServiceConfigId = "imp-gone", gone.Id
	if _, err := e.m.ImportReservation(r); model.KindOf(err) != model.ErrorNotFound {
		t.Errorf("ImportReservation with a service that does not exist: want a not-found error, got %v", err)
	}
}
