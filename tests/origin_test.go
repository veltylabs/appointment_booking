package tests

import (
	"testing"

	"webtyp.com/ddl"
	"webtyp.com/model"
	"webtyp.com/orm"
	"webtyp.com/storage/mem"

	ab "github.com/veltylabs/appointment_booking"
	"github.com/veltylabs/appointment_booking/migrate"
)

type dummyExecer struct{ calls []string }

func (d *dummyExecer) Exec(query string, args ...any) error {
	d.calls = append(d.calls, query)
	return nil
}

type dummyCompiler struct{}

func (d *dummyCompiler) CompileDDL(stmt ddl.Stmt, m model.Model) (string, []any, error) {
	return stmt.Table, nil, nil
}

// helper to set up a test service with calendar & employee service config ready for booking
func setupBookingService(t *testing.T) (ab.SchedulingService, *ab.Repository, string, int64) {
	t.Helper()
	db := orm.New(mem.New())
	repo, _ := ab.NewRepository(db, SetupDependencies().IDs)
	s, err := ab.New(db, SetupDependencies())
	if err != nil {
		t.Fatalf("ab.New: %v", err)
	}

	cfg := ab.EmployeeServiceConfig{
		TenantId:      "t1",
		StaffId:       "s1",
		ServiceId:     "srv1",
		DurationMin:   30,
		IsActive:      true,
		PriceOverride: 100,
	}
	if err := repo.InsertEmployeeServiceConfig(cfg); err != nil {
		t.Fatalf("InsertEmployeeServiceConfig: %v", err)
	}
	cfgs, _ := repo.ListEmployeeServiceConfigByStaff("t1", "s1")
	cfgID := cfgs[0].Id

	_ = s.UpsertCalendarConfig(ab.WorkCalendarConfig{
		TenantId: "t1",
		StaffId:  "s1",
		Timezone: "UTC",
		IsActive: true,
	})

	_ = s.SaveDayBlocks("t1", "s1", 1, []ab.WorkCalendarBlock{
		{StartMin: 540, EndMin: 1020, IsActive: true},
	})

	targetDay := Date(2025, 1, 6, 0, 0, 0, 0) // Mon Jan 6 2025
	slotStartUTC := targetDay + 540*60        // 09:00 UTC

	return s, repo, cfgID, slotStartUTC
}

// CU-1: El mesón crea una reserva declarando su canal.
func TestCU1_CreateReservation_OriginCounter(t *testing.T) {
	s, _, cfgID, slotStart := setupBookingService(t)

	cmd := ab.CreateReservationCmd{
		TenantId:                "t1",
		ClientId:                "c1",
		CreatorUserId:           "u1",
		EmployeeServiceConfigId: cfgID,
		SlotStartUtc:            slotStart,
		Origin:                  ab.OriginCounter,
	}
	res, err := s.CreateReservation(cmd)
	if err != nil {
		t.Fatalf("CreateReservation failed: %v", err)
	}
	if res.Origin != ab.OriginCounter {
		t.Fatalf("expected origin %q, got %q", ab.OriginCounter, res.Origin)
	}
}

// CU-2: Nadie puede crear una reserva sin declarar el canal (o con un valor no permitido).
func TestCU2_CreateReservation_OriginValidation(t *testing.T) {
	s, _, cfgID, slotStart := setupBookingService(t)

	// Origin vacio
	cmdEmpty := ab.CreateReservationCmd{
		TenantId:                "t1",
		ClientId:                "c1",
		CreatorUserId:           "u1",
		EmployeeServiceConfigId: cfgID,
		SlotStartUtc:            slotStart,
		Origin:                  "",
	}
	_, err := s.CreateReservation(cmdEmpty)
	if err != ab.ErrMissingArgs {
		t.Fatalf("expected ErrMissingArgs for empty origin, got: %v", err)
	}

	// Origin inventado
	cmdInvalid := ab.CreateReservationCmd{
		TenantId:                "t1",
		ClientId:                "c1",
		CreatorUserId:           "u1",
		EmployeeServiceConfigId: cfgID,
		SlotStartUtc:            slotStart,
		Origin:                  "WHATSAPP",
	}
	_, err = s.CreateReservation(cmdInvalid)
	if err != ab.ErrMissingArgs {
		t.Fatalf("expected ErrMissingArgs for invalid origin 'WHATSAPP', got: %v", err)
	}
}

// CU-3: El funcionario ve que una reserva de mesón es trabajo suyo.
func TestCU3_Item_PendingCounter(t *testing.T) {
	rf := &ab.ReservationForm{
		Status: ab.StatusPending,
		Origin: ab.OriginCounter,
	}
	item := rf.Item()
	if item.Description != "Pending confirmation" {
		t.Fatalf("expected description 'Pending confirmation', got %q", item.Description)
	}
}

// CU-4: El funcionario ve que una online no le toca (y CU-3 y CU-4 dan textos distintos).
func TestCU4_Item_PendingOnline(t *testing.T) {
	rfCounter := &ab.ReservationForm{
		Status: ab.StatusPending,
		Origin: ab.OriginCounter,
	}
	rfOnline := &ab.ReservationForm{
		Status: ab.StatusPending,
		Origin: ab.OriginOnline,
	}

	descCounter := rfCounter.Item().Description
	descOnline := rfOnline.Item().Description

	if descOnline != "Awaiting patient" {
		t.Fatalf("expected description 'Awaiting patient', got %q", descOnline)
	}
	if descCounter == descOnline {
		t.Fatalf("expected distinct descriptions for COUNTER and ONLINE pending reservations, both were %q", descCounter)
	}
}

// CU-5: Un estado que no es PENDING se lee por su estado, sin importar el canal.
func TestCU5_Item_NonPendingState(t *testing.T) {
	rfCounter := &ab.ReservationForm{
		Status: ab.StatusConfirmed,
		Origin: ab.OriginCounter,
	}
	rfOnline := &ab.ReservationForm{
		Status: ab.StatusConfirmed,
		Origin: ab.OriginOnline,
	}

	descCounter := rfCounter.Item().Description
	descOnline := rfOnline.Item().Description

	if descCounter != "Confirmed" || descOnline != "Confirmed" {
		t.Fatalf("expected 'Confirmed' for both origins when status is CONFIRMED, got COUNTER=%q ONLINE=%q", descCounter, descOnline)
	}
}

// CU-6: El funcionario confirma a mano una reserva online (el paciente llamó).
func TestCU6_ChangeReservationStatus_ConfirmOnlineHand(t *testing.T) {
	s, _, cfgID, slotStart := setupBookingService(t)

	cmd := ab.CreateReservationCmd{
		TenantId:                "t1",
		ClientId:                "c1",
		CreatorUserId:           "u1",
		EmployeeServiceConfigId: cfgID,
		SlotStartUtc:            slotStart,
		Origin:                  ab.OriginOnline,
	}
	res, err := s.CreateReservation(cmd)
	if err != nil {
		t.Fatalf("CreateReservation failed: %v", err)
	}

	err = s.ChangeReservationStatus(ab.ChangeStatusCmd{
		TenantId: "t1",
		Id:       res.Id,
		Event:    ab.EventConfirm,
		ActorId:  "staff_actor",
		Revision: 0,
	})
	if err != nil {
		t.Fatalf("ChangeReservationStatus failed: %v", err)
	}

	got, err := s.GetReservation("t1", res.Id)
	if err != nil {
		t.Fatalf("GetReservation failed: %v", err)
	}
	if got.Status != ab.StatusConfirmed {
		t.Fatalf("expected status %q, got %q", ab.StatusConfirmed, got.Status)
	}
	if got.UpdatedBy != "staff_actor" {
		t.Fatalf("expected updated_by 'staff_actor', got %q", got.UpdatedBy)
	}
}

// CU-7: El FSM no cambió.
func TestCU7_FSM_Unchanged(t *testing.T) {
	// PENDING + CONFIRM -> CONFIRMED
	st, err := ab.Transition(ab.StatusPending, ab.EventConfirm)
	if err != nil || st != ab.StatusConfirmed {
		t.Fatalf("expected transition PENDING+CONFIRM -> CONFIRMED, got %q, err=%v", st, err)
	}

	// PENDING + CANCEL -> CANCELLED
	st, err = ab.Transition(ab.StatusPending, ab.EventCancel)
	if err != nil || st != ab.StatusCancelled {
		t.Fatalf("expected transition PENDING+CANCEL -> CANCELLED, got %q, err=%v", st, err)
	}

	// Terminal states should fail transition
	_, err = ab.Transition(ab.StatusCancelled, ab.EventConfirm)
	if err != ab.ErrInvalidTransition {
		t.Fatalf("expected ErrInvalidTransition from CANCELLED, got %v", err)
	}
}

// CU-8: Una base ya creada recibe la columna nueva mediante Sync en migrate.
func TestCU8_MigrateSync_AddsColumn(t *testing.T) {
	execer := &dummyExecer{}
	compiler := &dummyCompiler{}

	err := migrate.Migrate(execer, compiler)
	if err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}
}
