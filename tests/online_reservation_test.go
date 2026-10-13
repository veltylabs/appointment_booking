package tests

import (
	"testing"

	ab "github.com/veltylabs/appointment_booking"
)

func TestOnline_NoPaymentsUnavailable(t *testing.T) {
	e := buildBookEnv(t, false)
	cfg := e.config(t, "s1", 0, false)
	_, err := e.m.CreateOnlineReservation(ab.CreateReservationCmd{Id: "r1", TenantId: bTenant, ClientId: "c1", EmployeeServiceConfigId: cfg, SlotStartUtc: bookDay + 9*3600})
	wantErrIs(t, err, ab.ErrPaymentsUnavailable)
}

func TestOnline_CreatesPendingOnlineWithHold(t *testing.T) {
	e := newBookEnv(t)
	r := e.online(t, "r1", e.config(t, "s1", 0, false), "c1", 9)
	now := bookNow()
	if r.Status != ab.StatusPending || r.Origin != ab.OriginOnline || r.HoldUntil < now+ab.OnlineHoldSeconds-5 || r.HoldUntil > now+ab.OnlineHoldSeconds {
		t.Fatalf("reservation = %+v", r)
	}
}

func TestOnline_TooManyHolds(t *testing.T) {
	e := newBookEnv(t)
	cfg := e.config(t, "s1", 0, false)
	e.online(t, "r1", cfg, "c1", 9)
	e.online(t, "r2", cfg, "c1", 10)
	_, err := e.m.CreateOnlineReservation(ab.CreateReservationCmd{Id: "r3", TenantId: bTenant, ClientId: "c1", EmployeeServiceConfigId: cfg, SlotStartUtc: bookDay + 11*3600})
	wantErrIs(t, err, ab.ErrTooManyHolds)

	// una retención vencida ya no cuenta
	e.patch(t, "r1", func(r *ab.Reservation) { r.HoldUntil = bookNow() - 1 })
	e.online(t, "r3", cfg, "c1", 11)
	// ni una reserva de mesón del mismo paciente
	e.reserve(t, "r4", cfg, "c2", 12, ab.OriginCounter)
	e.online(t, "r5", cfg, "c2", 13)
}

func TestOnline_SlotTakenPropagates(t *testing.T) {
	e := newBookEnv(t)
	cfg := e.config(t, "s1", 0, false)
	e.reserve(t, "r1", cfg, "c1", 9, ab.OriginCounter)
	_, err := e.m.CreateOnlineReservation(ab.CreateReservationCmd{Id: "r2", TenantId: bTenant, ClientId: "c2", EmployeeServiceConfigId: cfg, SlotStartUtc: bookDay + 9*3600})
	wantErrIs(t, err, ab.ErrSlotTaken)
}
