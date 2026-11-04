package tests

import (
	"testing"

	tinytime "webtyp.com/time"

	ab "github.com/veltylabs/appointment_booking"
)

func TestImportReservationPast(t *testing.T) {
	e := newBookEnv(t)
	// inactive config, time in 2019
	cfg, _ := e.m.CreateEmployeeServiceConfig(ab.EmployeeServiceConfig{TenantId: bTenant, StaffId: "s1", ServiceId: bService, DurationMin: 30, IsActive: false})

	pastTime := tinytime.Date(2019, 10, 15, 14, 30, 0)

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
	if got.LocalStringDate != "2019-10-15" {
		t.Errorf("date = %s, want 2019-10-15", got.LocalStringDate)
	}
}

func TestImportReservationTwice(t *testing.T) {
	e := newBookEnv(t)
	cfg, _ := e.m.CreateEmployeeServiceConfig(ab.EmployeeServiceConfig{TenantId: bTenant, StaffId: "s1", ServiceId: bService, DurationMin: 30, IsActive: true})

	t1 := int64(tinytime.Date(2023, 1, 1, 10, 0, 0))
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
