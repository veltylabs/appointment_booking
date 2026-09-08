package tests

import (
	"testing"

	"webtyp.com/orm"
	"webtyp.com/storage/mem"
	ab "github.com/veltylabs/appointment_booking"
)

func TestTenantIsolation(t *testing.T) {
	db := orm.New(mem.New())
	deps := SetupDependencies()
	repo, err := ab.NewRepository(db, deps.IDs)
	if err != nil {
		t.Fatalf("ab.NewRepository: %v", err)
	}
	svc, err := ab.New(db, deps)
	if err != nil {
		t.Fatalf("ab.New: %v", err)
	}

	// 1. Seed Tenant A ("TA") and Tenant B ("TB") employee configurations and working calendars.
	cfgA := ab.EmployeeServiceConfig{
		TenantId:    "TA",
		StaffId:     "staff_A",
		ServiceId:   "srv_A",
		DurationMin: 30,
		IsActive:    true,
	}
	repo.InsertEmployeeServiceConfig(cfgA)
	cfgsA, _ := repo.ListEmployeeServiceConfigByStaff("TA", "staff_A")
	cfgAId := cfgsA[0].Id

	svc.UpsertCalendarConfig(ab.WorkCalendarConfig{
		TenantId: "TA",
		StaffId:  "staff_A",
		Timezone: "UTC",
		IsActive: true,
	})
	svc.SaveDayBlocks("TA", "staff_A", 1, []ab.WorkCalendarBlock{
		{StartMin: 540, EndMin: 1020, IsActive: true},
	})

	slotA := Date(2025, 1, 6, 10, 0, 0, 0) // Monday, 10:00 UTC

	// Create reservation for Tenant A
	resA, err := svc.CreateReservation(ab.CreateReservationCmd{
		TenantId:                "TA",
		ClientId:                "client_A",
		CreatorUserId:           "user_A",
		EmployeeServiceConfigId: cfgAId,
		SlotStartUtc:            slotA,
	})
	if err != nil {
		t.Fatalf("CreateReservation TA: %v", err)
	}

	// 2. Test GetReservation isolation
	_, err = svc.GetReservation("TB", resA.Id)
	if err != ab.ErrNotFound {
		t.Fatalf("expected GetReservation cross-tenant to return ErrNotFound, got %v", err)
	}

	// 3. Test ChangeReservationStatus isolation
	err = svc.ChangeReservationStatus(ab.ChangeStatusCmd{
		TenantId: "TB",
		Id:       resA.Id,
		Event:    ab.EventCancel,
		ActorId:  "user_B",
		Revision: 0,
	})
	if err != ab.ErrNotFound {
		t.Fatalf("expected ChangeReservationStatus cross-tenant to return ErrNotFound, got %v", err)
	}

	// Verify resA status remains PENDING and was not cancelled
	gotResA, err := svc.GetReservation("TA", resA.Id)
	if err != nil {
		t.Fatalf("GetReservation: %v", err)
	}
	if gotResA.Status != ab.StatusPending {
		t.Fatalf("expected status to remain PENDING, got %s", gotResA.Status)
	}

	// 4. Test RemoveException isolation
	excA := ab.WorkCalendarException{
		TenantId:      "TA",
		StaffId:       "staff_A",
		ExceptionType: "HOLIDAY",
		SpecificDate:  Date(2025, 1, 6, 0, 0, 0, 0),
	}
	err = svc.AddException(excA)
	if err != nil {
		t.Fatalf("AddException: %v", err)
	}
	excs, err := repo.ListExceptions("TA", "staff_A", Date(2025, 1, 6, 0, 0, 0, 0), Date(2025, 1, 6, 0, 0, 0, 0))
	if err != nil || len(excs) != 1 {
		t.Fatalf("expected 1 exception, got %v", excs)
	}
	excAId := excs[0].Id

	// Try to remove exception using Tenant B
	err = svc.RemoveException("TB", excAId)
	// Since RemoveException is directly mapped to DeleteException which uses tenant condition in WHERE clause:
	// If TB tries to delete it, it won't affect TA's exception row. Let's verify.
	excsAfter, err := repo.ListExceptions("TA", "staff_A", Date(2025, 1, 6, 0, 0, 0, 0), Date(2025, 1, 6, 0, 0, 0, 0))
	if err != nil || len(excsAfter) != 1 {
		t.Fatalf("expected TA's exception to survive TB delete attempt, got %v", excsAfter)
	}
}
