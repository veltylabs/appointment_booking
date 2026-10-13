package tests

import (
	"testing"

	ab "github.com/veltylabs/appointment_booking"
)

func TestExpire_PastAppointmentExpires(t *testing.T) {
	e := newBookEnv(t)
	e.reserve(t, "r1", e.config(t, "s1", 0, false), "c1", 9, ab.OriginCounter)
	n, err := e.m.ExpirePendingReservations(bTenant, bookDay+10*3600)
	if err != nil || n != 1 || e.get(t, "r1").Status != ab.StatusExpired {
		t.Fatalf("n=%d err=%v status=%s", n, err, e.get(t, "r1").Status)
	}
}

func TestExpire_LapsedHoldExpires(t *testing.T) {
	e := newBookEnv(t)
	e.online(t, "r1", e.config(t, "s1", 0, false), "c1", 9)
	n, err := e.m.ExpirePendingReservations(bTenant, bookNow()+ab.OnlineHoldSeconds+1)
	if err != nil || n != 1 || e.get(t, "r1").Status != ab.StatusExpired {
		t.Fatalf("n=%d err=%v", n, err)
	}
}

func TestExpire_LiveHoldAndCounterPendingKept(t *testing.T) {
	e := newBookEnv(t)
	cfg := e.config(t, "s1", 0, false)
	e.online(t, "r1", cfg, "c1", 9)
	e.reserve(t, "r2", cfg, "c2", 10, ab.OriginCounter)
	n, err := e.m.ExpirePendingReservations(bTenant, bookNow())
	if err != nil || n != 0 {
		t.Fatalf("n=%d err=%v", n, err)
	}
}
