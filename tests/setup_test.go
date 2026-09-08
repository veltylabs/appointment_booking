package tests

import (
	"testing"

	"webtyp.com/fmt"
	"webtyp.com/orm"
	tinytime "webtyp.com/time"
	ab "github.com/veltylabs/appointment_booking"
)

func pad2(n int) string {
	s := fmt.Convert(n).String()
	if len(s) < 2 {
		return "0" + s
	}
	return s
}

func Date(year, month, day, hour, min, sec, ms int) int64 {
	dateStr := fmt.Convert(year).String() + "-" + pad2(month) + "-" + pad2(day)
	timeStr := pad2(hour) + ":" + pad2(min) + ":" + pad2(sec)
	nano, err := tinytime.ParseDateTime(dateStr, timeStr)
	if err != nil {
		nano, _ = tinytime.ParseDate(dateStr)
	}
	return nano / 1000000000
}

// RunServicePureTests tests generic logic of the service (Availability, FSM changes, CreateReservation)
// without depending on standard lib SQLite, so it can run on WASM.
func RunServicePureTests(t *testing.T, s ab.SchedulingService, repo *ab.Repository, db *orm.DB) {
	t.Run("CreateReservation_Success", func(t *testing.T) {
		// Insert active ab.EmployeeServiceConfig
		cfg := ab.EmployeeServiceConfig{
			TenantId:      "t1",
			StaffId:       "s1",
			ServiceId:     "srv1",
			DurationMin:   30,
			BufferMin:     0,
			IsActive:      true,
			PriceOverride: 100,
		}
		if err := repo.InsertEmployeeServiceConfig(cfg); err != nil {
			t.Fatalf("InsertEmployeeServiceConfig: %v", err)
		}
		// find auto-generated ID
		cfgs, _ := repo.ListEmployeeServiceConfigByStaff("t1", "s1")
		cfgID := cfgs[0].Id

		// Create Calendar Config
		if err := s.UpsertCalendarConfig(ab.WorkCalendarConfig{
			TenantId: "t1",
			StaffId:  "s1",
			Timezone: "UTC",
			IsActive: true,
		}); err != nil {
			t.Fatalf("UpsertCalendarConfig: %v", err)
		}

		// Weekly calendar: Monday 09:00–17:00
		if err := s.SaveDayBlocks("t1", "s1", 1, []ab.WorkCalendarBlock{
			{StartMin: 540, EndMin: 1020, IsActive: true},
		}); err != nil {
			t.Fatalf("SaveDayBlocks: %v", err)
		}

		targetDay := Date(2025, 1, 6, 0, 0, 0, 0) // Jan 6, 2025 is Monday
		slotStartUTC := targetDay + 540*60 // 09:00 UTC

		// Test ListAvailability
		slots, err := s.ListAvailability("t1", "s1", cfgID, targetDay, targetDay)
		if err != nil {
			t.Fatalf("ListAvailability: %v", err)
		}
		if len(slots) == 0 {
			t.Fatalf("expected some slots")
		}

		cmd := ab.CreateReservationCmd{
			TenantId:                "t1",
			ClientId:                "c1",
			CreatorUserId:           "u1",
			EmployeeServiceConfigId: cfgID,
			SlotStartUtc:            slotStartUTC,
			Notes:                   "Test note",
		}
		res, err := s.CreateReservation(cmd)
		if err != nil {
			t.Fatalf("CreateReservation: %v", err)
		}

		if res.Status != ab.StatusPending {
			t.Fatalf("expected status pending, got %s", res.Status)
		}
		if res.Notes != "Test note" {
			t.Fatalf("expected notes to match")
		}

		// ChangeStatus
		err = s.ChangeReservationStatus(ab.ChangeStatusCmd{
			TenantId: "t1",
			Id:       res.Id,
			Event:    ab.EventConfirm,
			ActorId:  "u1",
			Revision: 0,
		})
		if err != nil {
			t.Fatalf("ChangeReservationStatus (Confirm): %v", err)
		}

		got, err := s.GetReservation("t1", res.Id)
		if err != nil {
			t.Fatalf("GetReservation: %v", err)
		}
		if got.Status != ab.StatusConfirmed {
			t.Fatalf("expected status confirmed, got %s", got.Status)
		}

		// ChangeStatus (NoShow)
		err = s.ChangeReservationStatus(ab.ChangeStatusCmd{
			TenantId: "t1",
			Id:       res.Id,
			Event:    ab.EventNoShow,
			ActorId:  "u1",
			Revision: 1,
		})
		if err != nil {
			t.Fatalf("ChangeReservationStatus (NoShow): %v", err)
		}
	})

	t.Run("CreateReservation_SlotTaken", func(t *testing.T) {
		// Insert active ab.EmployeeServiceConfig
		cfg := ab.EmployeeServiceConfig{
			TenantId:      "t2",
			StaffId:       "s2",
			ServiceId:     "srv2",
			DurationMin:   60,
			BufferMin:     0,
			IsActive:      true,
			PriceOverride: 100,
		}
		if err := repo.InsertEmployeeServiceConfig(cfg); err != nil {
			t.Fatalf("InsertEmployeeServiceConfig: %v", err)
		}
		cfgs, _ := repo.ListEmployeeServiceConfigByStaff("t2", "s2")
		cfgID := cfgs[0].Id

		// Create Calendar Config
		s.UpsertCalendarConfig(ab.WorkCalendarConfig{
			TenantId: "t2",
			StaffId:  "s2",
			Timezone: "UTC",
			IsActive: true,
		})

		s.SaveDayBlocks("t2", "s2", 2, []ab.WorkCalendarBlock{
			{StartMin: 540, EndMin: 600, IsActive: true}, // 09:00 to 10:00 - exactly 1 hour
		})

		targetDay := Date(2025, 1, 7, 0, 0, 0, 0) // Jan 7, 2025 is Tuesday
		slotStartUTC := targetDay + 540*60 // 09:00 UTC

		cmd := ab.CreateReservationCmd{
			TenantId:                "t2",
			ClientId:                "c1",
			CreatorUserId:           "u1",
			EmployeeServiceConfigId: cfgID,
			SlotStartUtc:            slotStartUTC,
		}

		_, err := s.CreateReservation(cmd)
		if err != nil {
			t.Fatalf("first CreateReservation should succeed, got: %v", err)
		}

		// Second reservation on same slot
		_, err = s.CreateReservation(cmd)
		if err != ab.ErrSlotTaken {
			t.Fatalf("expected ab.ErrSlotTaken, got: %v", err)
		}
	})

	t.Run("ExpirePendingReservations", func(t *testing.T) {
		cfg := ab.EmployeeServiceConfig{
			TenantId:    "t3",
			StaffId:     "s3",
			ServiceId:   "srv3",
			DurationMin: 30,
			IsActive:    true,
		}
		repo.InsertEmployeeServiceConfig(cfg)
		cfgs, _ := repo.ListEmployeeServiceConfigByStaff("t3", "s3")
		cfgID := cfgs[0].Id

		s.UpsertCalendarConfig(ab.WorkCalendarConfig{
			TenantId: "t3",
			StaffId:  "s3",
			Timezone: "UTC",
			IsActive: true,
		})

		s.SaveDayBlocks("t3", "s3", 3, []ab.WorkCalendarBlock{
			{StartMin: 540, EndMin: 600, IsActive: true},
		})

		targetDay := Date(2025, 1, 8, 0, 0, 0, 0) // Jan 8, 2025 is Wednesday
		slotStartUTC := targetDay + 540*60 // 09:00 UTC

		cmd := ab.CreateReservationCmd{
			TenantId:                "t3",
			ClientId:                "c1",
			CreatorUserId:           "u1",
			EmployeeServiceConfigId: cfgID,
			SlotStartUtc:            slotStartUTC,
		}

		res, err := s.CreateReservation(cmd)
		if err != nil {
			t.Fatalf("CreateReservation: %v", err)
		}

		// Expire everything before slotStartUTC + 1 hour
		count, err := s.ExpirePendingReservations("t3", slotStartUTC + 3600)
		if err != nil {
			t.Fatalf("ExpirePendingReservations: %v", err)
		}
		if count != 1 {
			t.Fatalf("expected 1 expired reservation, got %d", count)
		}

		got, _ := s.GetReservation("t3", res.Id)
		if got.Status != ab.StatusExpired {
			t.Fatalf("expected expired status, got %s", got.Status)
		}
	})
}
