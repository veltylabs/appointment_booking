package tests

import (
	"testing"

	"webtyp.com/orm"
	tinytime "webtyp.com/time"
	ab "github.com/veltylabs/appointment_booking"
)

func RunAvailabilityTests(t *testing.T, s ab.SchedulingService, repo *ab.Repository, db *orm.DB) {
	setupValidConfig := func(tenant, staff string, slotStart int64) string {
		cfg := ab.EmployeeServiceConfig{
			TenantId:    tenant,
			StaffId:     staff,
			ServiceId:   "srv1",
			DurationMin: 60,
			IsActive:    true,
		}
		repo.InsertEmployeeServiceConfig(cfg)
		cfgs, _ := repo.ListEmployeeServiceConfigByStaff(tenant, staff)
		cfgID := cfgs[0].Id

		s.UpsertCalendarConfig(ab.WorkCalendarConfig{
			TenantId: tenant,
			StaffId:  staff,
			Timezone: "UTC",
			IsActive: true,
		})

		dow := tinytime.Weekday(slotStart)

		s.SaveDayBlocks(tenant, staff, dow, []ab.WorkCalendarBlock{
			{StartMin: 540, EndMin: 1020, IsActive: true}, // 09:00–17:00
		})
		return cfgID
	}

	t.Run("UC-08_ListAvailability_HolidayException", func(t *testing.T) {
		slot := Date(2025, 3, 1, 10, 0, 0, 0)
		cfgID := setupValidConfig("t_uc08", "s_uc08", slot)

		dayStart := Date(2025, 3, 1, 0, 0, 0, 0)
		s.AddException(ab.WorkCalendarException{
			TenantId:      "t_uc08",
			StaffId:       "s_uc08",
			ExceptionType: "HOLIDAY",
			SpecificDate:  dayStart,
		})

		slots, err := s.ListAvailability("t_uc08", "s_uc08", cfgID, dayStart, dayStart+86400)
		if err != nil {
			t.Fatalf("ListAvailability: %v", err)
		}
		if len(slots) != 0 {
			t.Fatalf("expected 0 slots on holiday, got %d", len(slots))
		}
	})

	t.Run("UC-09_ListAvailability_BlockedException", func(t *testing.T) {
		slot := Date(2025, 3, 2, 10, 0, 0, 0)
		cfgID := setupValidConfig("t_uc09", "s_uc09", slot)

		dayStart := Date(2025, 3, 2, 0, 0, 0, 0)
		s.AddException(ab.WorkCalendarException{
			TenantId:      "t_uc09",
			StaffId:       "s_uc09",
			ExceptionType: "BLOCKED",
			SpecificDate:  dayStart,
			StartTime:     600, // 10:00
			EndTime:       720, // 12:00
		})

		slots, err := s.ListAvailability("t_uc09", "s_uc09", cfgID, dayStart, dayStart+86400)
		if err != nil {
			t.Fatalf("ListAvailability: %v", err)
		}

		for _, s := range slots {
			startLocal := (s.StartUtc - dayStart) / 60
			endLocal := (s.EndUtc - dayStart) / 60
			// blocked from 600 to 720
			if startLocal < 720 && endLocal > 600 {
				t.Fatalf("slot %v overlaps blocked time (600-720)", s)
			}
		}
	})

	t.Run("UC-10_ListAvailability_SpecialHoursException", func(t *testing.T) {
		slot := Date(2025, 3, 3, 10, 0, 0, 0)
		cfgID := setupValidConfig("t_uc10", "s_uc10", slot)

		dayStart := Date(2025, 3, 3, 0, 0, 0, 0)
		s.AddException(ab.WorkCalendarException{
			TenantId:      "t_uc10",
			StaffId:       "s_uc10",
			ExceptionType: "SPECIAL_HOURS",
			SpecificDate:  dayStart,
			StartTime:     480, // 08:00
			EndTime:       600, // 10:00
		})

		slots, err := s.ListAvailability("t_uc10", "s_uc10", cfgID, dayStart, dayStart+86400)
		if err != nil {
			t.Fatalf("ListAvailability: %v", err)
		}

		if len(slots) != 2 { // 8:00-9:00, 9:00-10:00
			t.Fatalf("expected 2 slots in special hours, got %d", len(slots))
		}
		for _, s := range slots {
			startLocal := (s.StartUtc - dayStart) / 60
			endLocal := (s.EndUtc - dayStart) / 60
			if startLocal < 480 || endLocal > 600 {
				t.Fatalf("slot %v outside special hours (480-600)", s)
			}
		}
	})

	t.Run("UC-11_ListAvailability_BreakTime", func(t *testing.T) {
		slot := Date(2025, 3, 4, 10, 0, 0, 0)
		cfgID := setupValidConfig("t_uc11", "s_uc11", slot)

		dayStart := Date(2025, 3, 4, 0, 0, 0, 0)

		// Un día con DOS bloques: la comida es el GAP entre ellos (CU-09).
		s.SaveDayBlocks("t_uc11", "s_uc11", tinytime.Weekday(slot), []ab.WorkCalendarBlock{
			{StartMin: 540, EndMin: 720, IsActive: true},  // 09:00–12:00
			{StartMin: 780, EndMin: 1020, IsActive: true}, // 13:00–17:00
		})

		slots, err := s.ListAvailability("t_uc11", "s_uc11", cfgID, dayStart, dayStart+86400)
		if err != nil {
			t.Fatalf("ListAvailability: %v", err)
		}

		for _, s := range slots {
			startLocal := (s.StartUtc - dayStart) / 60
			endLocal := (s.EndUtc - dayStart) / 60
			if startLocal < 780 && endLocal > 720 {
				t.Fatalf("slot %v overlaps break time (720-780)", s)
			}
		}
	})

	t.Run("UC-15_ListAvailability_NoCalendarConfig", func(t *testing.T) {
		_, err := s.ListAvailability("t_uc15", "non_existent_staff", "cfg_id", 0, 1000)
		if err != ab.ErrCalendarConfigNotFound {
			t.Fatalf("expected ab.ErrCalendarConfigNotFound, got: %v", err)
		}
	})

	t.Run("UC-16_ListAvailability_InactiveCalendarConfig", func(t *testing.T) {
		slot := Date(2025, 3, 6, 10, 0, 0, 0)
		cfgID := setupValidConfig("t_uc16", "s_uc16", slot)

		s.UpsertCalendarConfig(ab.WorkCalendarConfig{
			TenantId: "t_uc16",
			StaffId:  "s_uc16",
			Timezone: "UTC",
			IsActive: false, // inactive
		})

		dayStart := Date(2025, 3, 6, 0, 0, 0, 0)
		slots, err := s.ListAvailability("t_uc16", "s_uc16", cfgID, dayStart, dayStart+86400)
		if err != nil {
			t.Fatalf("ListAvailability: %v", err)
		}
		if len(slots) != 0 {
			t.Fatalf("expected 0 slots for inactive calendar config, got %d", len(slots))
		}
	})

	t.Run("UC-17_CreateReservation_SlotOutsideWorkHours", func(t *testing.T) {
		slot := Date(2025, 3, 7, 10, 0, 0, 0) // 10:00 UTC (Work hours 09:00 - 17:00)
		cfgID := setupValidConfig("t_uc17", "s_uc17", slot)

		dayStart := Date(2025, 3, 7, 0, 0, 0, 0)
		midnightSlot := dayStart // 00:00 UTC, which is outside 09:00 - 17:00

		_, err := s.CreateReservation(ab.CreateReservationCmd{
			TenantId:                "t_uc17",
			ClientId:                "c1",
			CreatorUserId:           "u1",
			EmployeeServiceConfigId: cfgID,
			SlotStartUtc:            midnightSlot,
		})
		if err != ab.ErrSlotTaken {
			t.Fatalf("expected ab.ErrSlotTaken for slot outside work hours, got: %v", err)
		}
	})
}
