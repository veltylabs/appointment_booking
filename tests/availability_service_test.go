package tests

import (
	"testing"

	ab "github.com/veltylabs/appointment_booking"
)

func availArgs(staff string, taken bool) ab.ListServiceAvailabilityArgs {
	return ab.ListServiceAvailabilityArgs{TenantId: bTenant, ServiceId: bService, StaffId: staff, From: bookDay, To: bookDay, IncludeTaken: taken}
}

func TestAvailability_OneProfessional(t *testing.T) {
	e := newBookEnv(t)
	e.config(t, "s1", 0, false)
	e.config(t, "s2", 0, false)
	slots, err := e.m.ListServiceAvailability(availArgs("s1", false))
	if err != nil || len(slots) != 16 {
		t.Fatalf("got %d slots, %v (09–17 in 30 min = 16)", len(slots), err)
	}
	for _, s := range slots {
		if s.StaffId != "s1" || s.Taken {
			t.Fatalf("slot %+v", s)
		}
	}
	if slots[0].StartUtc != bookDay+9*3600 {
		t.Fatalf("first slot %d", slots[0].StartUtc-bookDay)
	}
}

func TestAvailability_AnyMergesFirstWins(t *testing.T) {
	e := newBookEnv(t)
	cfg1 := e.config(t, "s1", 0, false)
	e.config(t, "s2", 0, false)
	e.reserve(t, "r1", cfg1, "c1", 9, ab.OriginCounter)
	slots, err := e.m.ListServiceAvailability(availArgs("", false))
	if err != nil || len(slots) != 16 {
		t.Fatalf("got %d, %v", len(slots), err)
	}
	// 09:00 está tomada para Dra. Ana (s1) pero libre para Dr. Beto (s2): queda s2.
	// Las demás horas las ofrecen ambos y gana el primero del orden (Dr. Beto).
	if slots[0].StartUtc != bookDay+9*3600 || slots[0].StaffId != "s2" {
		t.Fatalf("09:00 = %+v", slots[0])
	}
	for i := 1; i < len(slots); i++ {
		if slots[i].StartUtc <= slots[i-1].StartUtc {
			t.Fatal("slots must be ordered by start")
		}
	}
}

func TestAvailability_NoConfigsEmpty(t *testing.T) {
	slots, err := newBookEnv(t).m.ListServiceAvailability(availArgs("", false))
	if err != nil || len(slots) != 0 {
		t.Fatalf("got %v, %v", slots, err)
	}
}

func TestAvailability_ErrorPropagates(t *testing.T) {
	e := newBookEnv(t)
	e.config(t, "s1", 0, false)
	e.staff.cardErr = errStorage
	_, err := e.m.ListServiceAvailability(availArgs("", false))
	wantErrIs(t, err, errStorage)
}

func TestAvailability_IncludeTakenMarksTaken(t *testing.T) {
	e := newBookEnv(t)
	cfg := e.config(t, "s1", 0, false)
	e.reserve(t, "r1", cfg, "c1", 10, ab.OriginCounter)
	cancelled := e.reserve(t, "r2", cfg, "c2", 11, ab.OriginCounter)
	if err := e.m.ChangeReservationStatus(ab.ChangeStatusCmd{TenantId: bTenant, Id: cancelled.Id, Event: ab.EventCancel, Revision: int(cancelled.Revision)}); err != nil {
		t.Fatal(err)
	}
	slots, err := e.m.ListServiceAvailability(availArgs("s1", true))
	if err != nil || len(slots) != 16 {
		t.Fatalf("got %d, %v", len(slots), err)
	}
	for _, s := range slots {
		wantTaken := s.StartUtc == bookDay+10*3600
		if s.Taken != wantTaken {
			t.Fatalf("slot %d taken=%v", (s.StartUtc-bookDay)/60, s.Taken)
		}
	}
}

func TestAvailability_IncludeTakenIgnoredForAny(t *testing.T) {
	e := newBookEnv(t)
	cfg := e.config(t, "s1", 0, false)
	e.reserve(t, "r1", cfg, "c1", 10, ab.OriginCounter)
	slots, _ := e.m.ListServiceAvailability(availArgs("", true))
	for _, s := range slots {
		if s.Taken {
			t.Fatalf("with any professional, taken hours are never offered: %+v", s)
		}
	}
}
