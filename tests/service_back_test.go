package tests

import (
	"testing"

	"webtyp.com/orm"
	"webtyp.com/storage/mem"
	ab "github.com/veltylabs/appointment_booking"
)

func TestService_Back(t *testing.T) {
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

	// Run pure tests first on mem
	t.Run("PureTests", func(t *testing.T) {
		RunServicePureTests(t, svc, repo, db)
		RunServiceValidationTests(t, svc, repo, db, deps)
		RunAvailabilityTests(t, svc, repo, db)
	})

	// Run integration/concurrency specific tests
	t.Run("Integration_Concurrency", func(t *testing.T) {
		// Setup config
		cfg := ab.EmployeeServiceConfig{
			TenantId:      "t99",
			StaffId:       "s99",
			ServiceId:     "srv99",
			DurationMin:   30,
			IsActive:      true,
			PriceOverride: 100,
		}
		repo.InsertEmployeeServiceConfig(cfg)
		cfgs, _ := repo.ListEmployeeServiceConfigByStaff("t99", "s99")
		cfgID := cfgs[0].Id

		s := svc
		s.UpsertCalendarConfig(ab.WorkCalendarConfig{
			TenantId: "t99",
			StaffId:  "s99",
			Timezone: "UTC",
			IsActive: true,
		})
		s.SaveDayBlocks("t99", "s99", 4, []ab.WorkCalendarBlock{
			{StartMin: 540, EndMin: 600, IsActive: true},
		})

		targetDay := Date(2025, 1, 9, 0, 0, 0, 0) // Jan 9, 2025 is Thursday
		slotStartUTC := targetDay + 540*60

		res, err := s.CreateReservation(ab.CreateReservationCmd{
			TenantId:                "t99",
			ClientId:                "c1",
			CreatorUserId:           "u1",
			EmployeeServiceConfigId: cfgID,
			SlotStartUtc:            slotStartUTC,
		})
		if err != nil {
			t.Fatalf("CreateReservation: %v", err)
		}

		// Test Conflict / Revision System
		err1 := s.ChangeReservationStatus(ab.ChangeStatusCmd{
			TenantId:  "t99",
			Id:        res.Id,
			Event:     ab.EventConfirm,
			ActorId:   "u1",
			PaymentId: "pay1",
			Revision:  0, // Correct revision
		})
		if err1 != nil {
			t.Fatalf("First change should succeed, got: %v", err1)
		}

		err2 := s.ChangeReservationStatus(ab.ChangeStatusCmd{
			TenantId: "t99",
			Id:       res.Id,
			Event:    ab.EventCancel,
			ActorId:  "u1",
			Revision: 0, // Wrong revision, should be 1
		})
		if err2 != ab.ErrConflict {
			t.Fatalf("Second change should fail with ab.ErrConflict, got: %v", err2)
		}

		// Verify event publisher received events
		pub := deps.Publisher.(*MockEventPublisher)
		foundCreated := false
		foundConfirmed := false
		for _, e := range pub.PublishedEvents {
			if e == ab.EventReservationCreated {
				foundCreated = true
			}
			if e == ab.EventReservationConfirmed {
				foundConfirmed = true
			}
		}
		if !foundCreated {
			t.Fatalf("expected ab.EventReservationCreated to be published")
		}
		if !foundConfirmed {
			t.Fatalf("expected ab.EventReservationConfirmed to be published")
		}
	})
}
