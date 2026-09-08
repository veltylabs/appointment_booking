package tests

import (
	"testing"

	"webtyp.com/orm"
	tinytime "webtyp.com/time"
	ab "github.com/veltylabs/appointment_booking"
)

func contains(s, substr string) bool {
	if len(substr) == 0 {
		return true
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func RunServiceValidationTests(t *testing.T, s ab.SchedulingService, repo *ab.Repository, db *orm.DB, deps ab.Deps) {
	t.Run("UC-01_CreateReservation_InactiveConfig", func(t *testing.T) {
		cfg := ab.EmployeeServiceConfig{
			TenantId:    "t_uc01",
			StaffId:     "s_uc01",
			ServiceId:   "srv_uc01",
			DurationMin: 30,
			IsActive:    false, // inactive
		}
		repo.InsertEmployeeServiceConfig(cfg)
		cfgs, _ := repo.ListEmployeeServiceConfigByStaff("t_uc01", "s_uc01")
		cfgID := cfgs[0].Id

		_, err := s.CreateReservation(ab.CreateReservationCmd{
			TenantId:                "t_uc01",
			ClientId:                "c1",
			CreatorUserId:           "u1",
			EmployeeServiceConfigId: cfgID,
			SlotStartUtc:            1000,
		})
		if err != ab.ErrNotFound {
			t.Fatalf("expected ab.ErrNotFound, got: %v", err)
		}
	})

	t.Run("UC-02_CreateReservation_StaffNotFound", func(t *testing.T) {
		cfg := ab.EmployeeServiceConfig{
			TenantId:    "t_uc02",
			StaffId:     "s_uc02",
			ServiceId:   "srv_uc02",
			DurationMin: 30,
			IsActive:    true,
		}
		repo.InsertEmployeeServiceConfig(cfg)
		cfgs, _ := repo.ListEmployeeServiceConfigByStaff("t_uc02", "s_uc02")
		cfgID := cfgs[0].Id

		mockStaff := deps.Staff.(*MockStaffReader)
		mockStaff.Exists = false
		defer func() { mockStaff.Exists = true }()

		_, err := s.CreateReservation(ab.CreateReservationCmd{
			TenantId:                "t_uc02",
			ClientId:                "c1",
			CreatorUserId:           "u1",
			EmployeeServiceConfigId: cfgID,
			SlotStartUtc:            1000,
		})
		if err == nil || !contains(err.Error(), "staff not found") {
			t.Fatalf("expected 'staff not found' error, got: %v", err)
		}
	})

	t.Run("UC-03_CreateReservation_ServiceNotFound", func(t *testing.T) {
		cfg := ab.EmployeeServiceConfig{
			TenantId:    "t_uc03",
			StaffId:     "s_uc03",
			ServiceId:   "srv_uc03",
			DurationMin: 30,
			IsActive:    true,
		}
		repo.InsertEmployeeServiceConfig(cfg)
		cfgs, _ := repo.ListEmployeeServiceConfigByStaff("t_uc03", "s_uc03")
		cfgID := cfgs[0].Id

		mockCatalog := deps.Catalog.(*MockCatalogReader)
		mockCatalog.Exists = false
		defer func() { mockCatalog.Exists = true }()

		_, err := s.CreateReservation(ab.CreateReservationCmd{
			TenantId:                "t_uc03",
			ClientId:                "c1",
			CreatorUserId:           "u1",
			EmployeeServiceConfigId: cfgID,
			SlotStartUtc:            1000,
		})
		if err == nil || !contains(err.Error(), "service not found") {
			t.Fatalf("expected 'service not found' error, got: %v", err)
		}
	})

	t.Run("UC-04_CreateReservation_ClientNotFound", func(t *testing.T) {
		cfg := ab.EmployeeServiceConfig{
			TenantId:    "t_uc04",
			StaffId:     "s_uc04",
			ServiceId:   "srv_uc04",
			DurationMin: 30,
			IsActive:    true,
		}
		repo.InsertEmployeeServiceConfig(cfg)
		cfgs, _ := repo.ListEmployeeServiceConfigByStaff("t_uc04", "s_uc04")
		cfgID := cfgs[0].Id

		mockDirectory := deps.Directory.(*MockDirectoryReader)
		mockDirectory.Exists = false
		defer func() { mockDirectory.Exists = true }()

		_, err := s.CreateReservation(ab.CreateReservationCmd{
			TenantId:                "t_uc04",
			ClientId:                "c1",
			CreatorUserId:           "u1",
			EmployeeServiceConfigId: cfgID,
			SlotStartUtc:            1000,
		})
		if err == nil || !contains(err.Error(), "client not found") {
			t.Fatalf("expected 'client not found' error, got: %v", err)
		}
	})

	setupValidConfig := func(tenant, staff string, slotStart int64) string {
		cfg := ab.EmployeeServiceConfig{
			TenantId:    tenant,
			StaffId:     staff,
			ServiceId:   "srv1",
			DurationMin: 30,
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
			{StartMin: 0, EndMin: 1440, IsActive: true}, // all day
		})
		return cfgID
	}

	t.Run("UC-05_ChangeReservationStatus_Cancel_FromPending", func(t *testing.T) {
		slot := Date(2025, 2, 1, 10, 0, 0, 0)
		cfgID := setupValidConfig("t_uc05", "s_uc05", slot)

		res, err := s.CreateReservation(ab.CreateReservationCmd{
			TenantId:                "t_uc05",
			ClientId:                "c1",
			CreatorUserId:           "u1",
			EmployeeServiceConfigId: cfgID,
			SlotStartUtc:            slot,
		})
		if err != nil {
			t.Fatalf("CreateReservation: %v", err)
		}

		mockPub := deps.Publisher.(*MockEventPublisher)
		mockPub.PublishedEvents = nil // reset

		err = s.ChangeReservationStatus(ab.ChangeStatusCmd{
			TenantId: "t_uc05",
			Id:       res.Id,
			Event:    ab.EventCancel,
			ActorId:  "u1",
			Revision: 0,
		})
		if err != nil {
			t.Fatalf("ChangeReservationStatus: %v", err)
		}

		got, _ := s.GetReservation("t_uc05", res.Id)
		if got.Status != ab.StatusCancelled {
			t.Fatalf("expected CANCELLED, got %s", got.Status)
		}

		foundCancel := false
		for _, e := range mockPub.PublishedEvents {
			if e == ab.EventReservationCancelled {
				foundCancel = true
				break
			}
		}
		if !foundCancel {
			t.Fatalf("expected EventReservationCancelled")
		}
	})

	t.Run("UC-06_ChangeReservationStatus_Complete_FromConfirmed", func(t *testing.T) {
		slot := Date(2025, 2, 2, 10, 0, 0, 0)
		cfgID := setupValidConfig("t_uc06", "s_uc06", slot)

		res, _ := s.CreateReservation(ab.CreateReservationCmd{
			TenantId:                "t_uc06",
			ClientId:                "c1",
			CreatorUserId:           "u1",
			EmployeeServiceConfigId: cfgID,
			SlotStartUtc:            slot,
		})

		s.ChangeReservationStatus(ab.ChangeStatusCmd{
			TenantId: "t_uc06",
			Id:       res.Id,
			Event:    ab.EventConfirm,
			ActorId:  "u1",
			Revision: 0,
		})

		mockPub := deps.Publisher.(*MockEventPublisher)
		mockPub.PublishedEvents = nil

		err := s.ChangeReservationStatus(ab.ChangeStatusCmd{
			TenantId: "t_uc06",
			Id:       res.Id,
			Event:    ab.EventComplete,
			ActorId:  "u1",
			Revision: 1,
		})
		if err != nil {
			t.Fatalf("ChangeReservationStatus Complete: %v", err)
		}

		got, _ := s.GetReservation("t_uc06", res.Id)
		if got.Status != ab.StatusCompleted {
			t.Fatalf("expected COMPLETED, got %s", got.Status)
		}

		foundComplete := false
		for _, e := range mockPub.PublishedEvents {
			if e == ab.EventReservationCompleted {
				foundComplete = true
				break
			}
		}
		if !foundComplete {
			t.Fatalf("expected EventReservationCompleted")
		}
	})

	t.Run("UC-07_CreateReservation_Reschedule", func(t *testing.T) {
		slot1 := Date(2025, 2, 3, 10, 0, 0, 0)
		slot2 := Date(2025, 2, 3, 11, 0, 0, 0)
		cfgID := setupValidConfig("t_uc07", "s_uc07", slot1)

		res1, _ := s.CreateReservation(ab.CreateReservationCmd{
			TenantId:                "t_uc07",
			ClientId:                "c1",
			CreatorUserId:           "u1",
			EmployeeServiceConfigId: cfgID,
			SlotStartUtc:            slot1,
		})

		mockPub := deps.Publisher.(*MockEventPublisher)
		mockPub.PublishedEvents = nil

		res2, err := s.CreateReservation(ab.CreateReservationCmd{
			TenantId:                "t_uc07",
			ClientId:                "c1",
			CreatorUserId:           "u1",
			EmployeeServiceConfigId: cfgID,
			SlotStartUtc:            slot2,
			RescheduledFromId:       res1.Id,
		})
		if err != nil {
			t.Fatalf("CreateReservation reschedule: %v", err)
		}

		if res2.Status != ab.StatusPending {
			t.Fatalf("expected new res PENDING, got %s", res2.Status)
		}

		gotOrig, _ := s.GetReservation("t_uc07", res1.Id)
		if gotOrig.Status != ab.StatusRescheduled {
			t.Fatalf("expected orig res RESCHEDULED, got %s", gotOrig.Status)
		}

		foundCreated, foundResched := false, false
		for _, e := range mockPub.PublishedEvents {
			if e == ab.EventReservationCreated {
				foundCreated = true
			}
			if e == ab.EventReservationRescheduled {
				foundResched = true
			}
		}
		if !foundCreated || !foundResched {
			t.Fatalf("expected both Created and Rescheduled events, got %+v", mockPub.PublishedEvents)
		}
	})

	t.Run("UC-13_GetReservation_CrossTenantIsolation", func(t *testing.T) {
		slot := Date(2025, 2, 4, 10, 0, 0, 0)
		cfgID := setupValidConfig("T1", "s_uc13", slot)

		res, _ := s.CreateReservation(ab.CreateReservationCmd{
			TenantId:                "T1",
			ClientId:                "c1",
			CreatorUserId:           "u1",
			EmployeeServiceConfigId: cfgID,
			SlotStartUtc:            slot,
		})

		_, err := s.GetReservation("T2", res.Id)
		if err != ab.ErrNotFound {
			t.Fatalf("expected ErrNotFound for cross tenant Get, got %v", err)
		}
	})

	t.Run("UC-14_ChangeReservationStatus_CrossTenantIsolation", func(t *testing.T) {
		slot := Date(2025, 2, 5, 10, 0, 0, 0)
		cfgID := setupValidConfig("T1", "s_uc14", slot)

		res, _ := s.CreateReservation(ab.CreateReservationCmd{
			TenantId:                "T1",
			ClientId:                "c1",
			CreatorUserId:           "u1",
			EmployeeServiceConfigId: cfgID,
			SlotStartUtc:            slot,
		})

		err := s.ChangeReservationStatus(ab.ChangeStatusCmd{
			TenantId: "T2",
			Id:       res.Id,
			Event:    ab.EventCancel,
			ActorId:  "u1",
			Revision: 0,
		})
		if err != ab.ErrNotFound {
			t.Fatalf("expected ErrNotFound for cross tenant ChangeStatus, got %v", err)
		}
	})

	t.Run("UC-12_SaveDayBlocks_CalendarConfigNotFound", func(t *testing.T) {
		err := s.SaveDayBlocks("t_uc12", "non_existent", 1, []ab.WorkCalendarBlock{
			{StartMin: 540, EndMin: 1020, IsActive: true},
		})
		if err != ab.ErrCalendarConfigNotFound {
			t.Fatalf("expected ErrCalendarConfigNotFound, got %v", err)
		}
	})

	t.Run("UC-18_ChangeReservationStatus_ConfirmWithPaymentID", func(t *testing.T) {
		slot := Date(2025, 2, 6, 10, 0, 0, 0)
		cfgID := setupValidConfig("t_uc18", "s_uc18", slot)

		res, _ := s.CreateReservation(ab.CreateReservationCmd{
			TenantId:                "t_uc18",
			ClientId:                "c1",
			CreatorUserId:           "u1",
			EmployeeServiceConfigId: cfgID,
			SlotStartUtc:            slot,
		})

		err := s.ChangeReservationStatus(ab.ChangeStatusCmd{
			TenantId:  "t_uc18",
			Id:        res.Id,
			Event:     ab.EventConfirm,
			ActorId:   "u1",
			PaymentId: "pay_123",
			Revision:  0,
		})
		if err != nil {
			t.Fatalf("ChangeReservationStatus: %v", err)
		}

		got, _ := s.GetReservation("t_uc18", res.Id)
		if got.PaymentId != "pay_123" {
			t.Fatalf("expected PaymentID 'pay_123', got '%s'", got.PaymentId)
		}
	})

	t.Run("UC-19_ListReservationsByStaff_ViaService", func(t *testing.T) {
		slot1 := Date(2025, 2, 7, 10, 0, 0, 0)
		slot2 := Date(2025, 2, 7, 11, 0, 0, 0)
		cfgID := setupValidConfig("t_uc19", "s_uc19", slot1)

		s.CreateReservation(ab.CreateReservationCmd{
			TenantId:                "t_uc19",
			ClientId:                "c1",
			CreatorUserId:           "u1",
			EmployeeServiceConfigId: cfgID,
			SlotStartUtc:            slot1,
		})
		for i := 0; i < 1000000; i++ {} // busy wait alternative to avoid nanosecond collision
		s.CreateReservation(ab.CreateReservationCmd{
			TenantId:                "t_uc19",
			ClientId:                "c2",
			CreatorUserId:           "u2",
			EmployeeServiceConfigId: cfgID,
			SlotStartUtc:            slot2,
		})

		from := Date(2025, 2, 7, 0, 0, 0, 0)
		to := Date(2025, 2, 8, 0, 0, 0, 0)

		res, err := s.ListReservationsByStaff("t_uc19", "s_uc19", from, to)
		if err != nil {
			t.Fatalf("ListReservationsByStaff: %v", err)
		}
		if len(res) != 2 {
			t.Fatalf("expected 2 reservations, got %d", len(res))
		}
		if res[0].StaffIdsnapshot != "s_uc19" || res[1].StaffIdsnapshot != "s_uc19" {
			t.Fatalf("expected staffID s_uc19")
		}
	})

	t.Run("UC-20_ExpirePendingReservations_NothingToExpire", func(t *testing.T) {
		slot := Date(2025, 2, 8, 10, 0, 0, 0)
		cfgID := setupValidConfig("t_uc20", "s_uc20", slot)

		s.CreateReservation(ab.CreateReservationCmd{
			TenantId:                "t_uc20",
			ClientId:                "c1",
			CreatorUserId:           "u1",
			EmployeeServiceConfigId: cfgID,
			SlotStartUtc:            slot,
		})

		// try to expire before the reservation
		count, err := s.ExpirePendingReservations("t_uc20", slot-3600)
		if err != nil {
			t.Fatalf("ExpirePendingReservations: %v", err)
		}
		if count != 0 {
			t.Fatalf("expected 0 expired, got %d", count)
		}
	})
}
