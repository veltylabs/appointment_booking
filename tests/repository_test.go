package tests

import (
	"testing"

	"webtyp.com/orm"
	"webtyp.com/storage/mem"
	ab "github.com/veltylabs/appointment_booking"
)

func newTestRepo(t *testing.T) *ab.Repository {
	t.Helper()
	db := orm.New(mem.New())
	repo, err := ab.NewRepository(db, &fakeIDs{})
	if err != nil {
		t.Fatalf("ab.NewRepository failed: %v", err)
	}
	return repo
}

func TestInsertGet_EmployeeServiceConfig(t *testing.T) {
	repo := newTestRepo(t)

	cfg := ab.EmployeeServiceConfig{
		TenantId:    "t1",
		StaffId:     "s1",
		ServiceId:   "srv1",
		DurationMin: 30,
		IsActive:    true,
	}

	err := repo.InsertEmployeeServiceConfig(cfg)
	if err != nil {
		t.Fatalf("InsertEmployeeServiceConfig failed: %v", err)
	}

	cfgs, err := repo.ListEmployeeServiceConfigByStaff("t1", "s1")
	if err != nil {
		t.Fatalf("ListEmployeeServiceConfigByStaff failed: %v", err)
	}
	if len(cfgs) != 1 {
		t.Fatalf("Expected 1 config, got %d", len(cfgs))
	}

	id := cfgs[0].Id
	got, err := repo.GetEmployeeServiceConfig(id)
	if err != nil {
		t.Fatalf("GetEmployeeServiceConfig failed: %v", err)
	}

	if got.TenantId != "t1" || got.StaffId != "s1" || got.ServiceId != "srv1" || got.DurationMin != 30 {
		t.Fatalf("Got unexpected config: %+v", got)
	}
}

func TestListEmployeeServiceConfigByStaff(t *testing.T) {
	repo := newTestRepo(t)

	repo.InsertEmployeeServiceConfig(ab.EmployeeServiceConfig{TenantId: "t1", StaffId: "s1", ServiceId: "srv1"})
	repo.InsertEmployeeServiceConfig(ab.EmployeeServiceConfig{TenantId: "t1", StaffId: "s1", ServiceId: "srv2"})
	repo.InsertEmployeeServiceConfig(ab.EmployeeServiceConfig{TenantId: "t1", StaffId: "s2", ServiceId: "srv3"})
	repo.InsertEmployeeServiceConfig(ab.EmployeeServiceConfig{TenantId: "t2", StaffId: "s1", ServiceId: "srv1"})

	cfgs, err := repo.ListEmployeeServiceConfigByStaff("t1", "s1")
	if err != nil {
		t.Fatalf("ListEmployeeServiceConfigByStaff failed: %v", err)
	}

	if len(cfgs) != 2 {
		t.Fatalf("Expected 2 configs for t1/s1, got %d", len(cfgs))
	}
}

func TestGetEmployeeServiceConfig_NotFound(t *testing.T) {
	repo := newTestRepo(t)

	_, err := repo.GetEmployeeServiceConfig("non-existent")
	if err != ab.ErrNotFound {
		t.Fatalf("Expected ab.ErrNotFound, got %v", err)
	}
}

func TestUpsertCalendarConfig_CreateAndUpdate(t *testing.T) {
	repo := newTestRepo(t)

	// Create
	cfg1 := ab.WorkCalendarConfig{
		TenantId: "t1",
		StaffId:  "s1",
		Timezone: "America/Santiago",
	}
	err := repo.UpsertCalendarConfig(cfg1)
	if err != nil {
		t.Fatalf("UpsertCalendarConfig (create) failed: %v", err)
	}

	got1, err := repo.GetCalendarConfig("t1", "s1")
	if err != nil {
		t.Fatalf("GetCalendarConfig failed: %v", err)
	}
	if got1.Timezone != "America/Santiago" {
		t.Fatalf("Expected timezone America/Santiago, got %s", got1.Timezone)
	}

	// Update
	cfg2 := ab.WorkCalendarConfig{
		TenantId: "t1",
		StaffId:  "s1",
		Timezone: "America/New_York",
	}
	err = repo.UpsertCalendarConfig(cfg2)
	if err != nil {
		t.Fatalf("UpsertCalendarConfig (update) failed: %v", err)
	}

	got2, err := repo.GetCalendarConfig("t1", "s1")
	if err != nil {
		t.Fatalf("GetCalendarConfig failed: %v", err)
	}
	if got2.Timezone != "America/New_York" {
		t.Fatalf("Expected timezone America/New_York, got %s", got2.Timezone)
	}
	if got2.Id != got1.Id {
		t.Fatalf("Expected ID to be preserved across upsert, old=%s new=%s", got1.Id, got2.Id)
	}
}

func TestGetCalendarConfig_NotFound(t *testing.T) {
	repo := newTestRepo(t)

	_, err := repo.GetCalendarConfig("t1", "s1")
	if err != ab.ErrNotFound {
		t.Fatalf("Expected ab.ErrNotFound, got %v", err)
	}
}

func TestReplaceWeekdayBlocks_WholeDayReplace(t *testing.T) {
	repo := newTestRepo(t)

	// Create: dos bloques para el lunes.
	err := repo.ReplaceWeekdayBlocks("t1", "s1", 1, []ab.WorkCalendarBlock{
		{TenantId: "t1", StaffId: "s1", DayOfWeek: 1, SpecificDate: 0, StartMin: 540, EndMin: 720, IsActive: true},
		{TenantId: "t1", StaffId: "s1", DayOfWeek: 1, SpecificDate: 0, StartMin: 780, EndMin: 1020, IsActive: true},
	})
	if err != nil {
		t.Fatalf("ReplaceWeekdayBlocks (create) failed: %v", err)
	}

	blocks, err := repo.ListBlocks("t1", "s1")
	if err != nil {
		t.Fatalf("ListBlocks failed: %v", err)
	}
	if len(blocks) != 2 {
		t.Fatalf("expected 2 blocks, got %d", len(blocks))
	}

	// Replace: un solo bloque — el reemplazo es del DÍA completo, no un upsert por fila.
	err = repo.ReplaceWeekdayBlocks("t1", "s1", 1, []ab.WorkCalendarBlock{
		{TenantId: "t1", StaffId: "s1", DayOfWeek: 1, SpecificDate: 0, StartMin: 600, EndMin: 900, IsActive: true},
	})
	if err != nil {
		t.Fatalf("ReplaceWeekdayBlocks (update) failed: %v", err)
	}

	blocks2, err := repo.ListBlocks("t1", "s1")
	if err != nil {
		t.Fatalf("ListBlocks failed: %v", err)
	}
	if len(blocks2) != 1 || blocks2[0].StartMin != 600 || blocks2[0].EndMin != 900 {
		t.Fatalf("expected single block 600-900 after replace, got %+v", blocks2)
	}

	// Un reemplazo del martes no toca al lunes.
	err = repo.ReplaceWeekdayBlocks("t1", "s1", 2, []ab.WorkCalendarBlock{
		{TenantId: "t1", StaffId: "s1", DayOfWeek: 2, SpecificDate: 0, StartMin: 540, EndMin: 720, IsActive: true},
	})
	if err != nil {
		t.Fatalf("ReplaceWeekdayBlocks (tuesday) failed: %v", err)
	}
	blocks3, _ := repo.ListBlocks("t1", "s1")
	if len(blocks3) != 2 {
		t.Fatalf("expected 2 blocks (monday + tuesday), got %d", len(blocks3))
	}
}

func TestReplaceAndDeleteDateBlocks(t *testing.T) {
	repo := newTestRepo(t)

	day := Date(2027, 6, 7, 0, 0, 0, 0)
	err := repo.ReplaceDateBlocks("t1", "s1", day, []ab.WorkCalendarBlock{
		{TenantId: "t1", StaffId: "s1", SpecificDate: day, StartMin: 540, EndMin: 720, IsActive: true},
	})
	if err != nil {
		t.Fatalf("ReplaceDateBlocks failed: %v", err)
	}

	blocks, err := repo.ListBlocks("t1", "s1")
	if err != nil {
		t.Fatalf("ListBlocks failed: %v", err)
	}
	if len(blocks) != 1 || blocks[0].SpecificDate != day {
		t.Fatalf("expected 1 dated block, got %+v", blocks)
	}

	err = repo.DeleteDateBlocks("t1", "s1", day)
	if err != nil {
		t.Fatalf("DeleteDateBlocks failed: %v", err)
	}

	blocks2, _ := repo.ListBlocks("t1", "s1")
	if len(blocks2) != 0 {
		t.Fatalf("expected 0 blocks after delete, got %d", len(blocks2))
	}
}

func TestInsertGet_Reservation(t *testing.T) {
	repo := newTestRepo(t)

	res := ab.Reservation{
		TenantId: "t1",
		ClientId: "c1",
		Status:   ab.StatusPending,
	}

	err := repo.InsertReservation(&res)
	if err != nil {
		t.Fatalf("InsertReservation failed: %v", err)
	}

	resList, err := repo.ListReservationsByClient("t1", "c1")
	if err != nil || len(resList) != 1 {
		t.Fatalf("Failed to list reservations: %v", err)
	}

	id := resList[0].Id
	got, err := repo.GetReservation(id)
	if err != nil {
		t.Fatalf("GetReservation failed: %v", err)
	}

	if got.TenantId != "t1" || got.ClientId != "c1" || got.Status != ab.StatusPending || got.Revision != 0 {
		t.Fatalf("Got unexpected reservation: %+v", got)
	}
}

func TestListReservationsByStaff(t *testing.T) {
	repo := newTestRepo(t)

	repo.InsertReservation(&ab.Reservation{Id: "t1", TenantId: "t1", StaffIdsnapshot: "s1", ReservationDate: 100})
	for i := 0; i < 1000000; i++ {} // avoid conflict
	repo.InsertReservation(&ab.Reservation{Id: "t2", TenantId: "t1", StaffIdsnapshot: "s1", ReservationDate: 200})
	for i := 0; i < 1000000; i++ {} // avoid conflict
	repo.InsertReservation(&ab.Reservation{Id: "t3", TenantId: "t1", StaffIdsnapshot: "s1", ReservationDate: 300})
	for i := 0; i < 1000000; i++ {} // avoid conflict
	repo.InsertReservation(&ab.Reservation{Id: "t4", TenantId: "t1", StaffIdsnapshot: "s2", ReservationDate: 200})

	res, err := repo.ListReservationsByStaff("t1", "s1", 150, 250)
	if err != nil {
		t.Fatalf("ListReservationsByStaff failed: %v", err)
	}

	if len(res) != 1 || res[0].ReservationDate != 200 {
		t.Fatalf("Expected 1 reservation with date 200, got %v", res)
	}
}

func TestUpdateReservationStatus_OK(t *testing.T) {
	repo := newTestRepo(t)

	repo.InsertReservation(&ab.Reservation{TenantId: "t1", ClientId: "c1", Status: ab.StatusPending})
	resList, _ := repo.ListReservationsByClient("t1", "c1")
	id := resList[0].Id

	err := repo.UpdateReservationStatus(id, ab.StatusConfirmed, "u1", 12345, 0)
	if err != nil {
		t.Fatalf("UpdateReservationStatus failed: %v", err)
	}

	got, _ := repo.GetReservation(id)
	if got.Status != ab.StatusConfirmed || got.UpdatedBy != "u1" || got.UpdatedAt != 12345 || got.Revision != 1 {
		t.Fatalf("Got unexpected reservation state: %+v", got)
	}
}

func TestUpdateReservationStatus_Conflict(t *testing.T) {
	repo := newTestRepo(t)

	repo.InsertReservation(&ab.Reservation{TenantId: "t1", ClientId: "c1", Status: ab.StatusPending})
	resList, _ := repo.ListReservationsByClient("t1", "c1")
	id := resList[0].Id

	// Provide wrong revision
	err := repo.UpdateReservationStatus(id, ab.StatusConfirmed, "u1", 12345, 99)
	if err != ab.ErrConflict {
		t.Fatalf("Expected ab.ErrConflict, got %v", err)
	}
}

func TestInsertListDeleteException(t *testing.T) {
	repo := newTestRepo(t)

	exc := ab.WorkCalendarException{
		TenantId:     "t1",
		StaffId:      "s1",
		SpecificDate: 100,
	}

	err := repo.InsertException(exc)
	if err != nil {
		t.Fatalf("InsertException failed: %v", err)
	}

	excs, err := repo.ListExceptions("t1", "s1", 50, 150)
	if err != nil {
		t.Fatalf("ListExceptions failed: %v", err)
	}
	if len(excs) != 1 {
		t.Fatalf("Expected 1 exception, got %d", len(excs))
	}
	id := excs[0].Id

	err = repo.DeleteException("t1", id)
	if err != nil {
		t.Fatalf("DeleteException failed: %v", err)
	}

	excs2, _ := repo.ListExceptions("t1", "s1", 50, 150)
	if len(excs2) != 0 {
		t.Fatalf("Expected 0 exceptions after delete, got %d", len(excs2))
	}
}
