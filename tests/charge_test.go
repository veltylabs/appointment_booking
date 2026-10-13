package tests

import (
	"testing"

	ab "github.com/veltylabs/appointment_booking"
)

func TestCharge_OtherTenantNotFound(t *testing.T) {
	e := newBookEnv(t)
	e.reserve(t, "r1", e.config(t, "s1", 0, false), "c1", 9, ab.OriginCounter)
	_, _, _, err := e.m.Charge("t2", "r1")
	wantErrIs(t, err, ab.ErrNotFound)
}

func TestCharge_NotPayableStates(t *testing.T) {
	e := newBookEnv(t)
	cfg := e.config(t, "s1", 0, false)
	cancelled := e.reserve(t, "r1", cfg, "c1", 9, ab.OriginCounter)
	if err := e.m.ChangeReservationStatus(ab.ChangeStatusCmd{TenantId: bTenant, Id: "r1", Event: ab.EventCancel, Revision: int(cancelled.Revision)}); err != nil {
		t.Fatal(err)
	}
	e.reserve(t, "r2", cfg, "c2", 10, ab.OriginCounter)
	e.patch(t, "r2", func(r *ab.Reservation) { r.PaymentId = "p1" })
	e.reserve(t, "r3", cfg, "c3", 11, ab.OriginCounter)
	e.patch(t, "r3", func(r *ab.Reservation) { r.PriceSnapshot = 0 })
	for _, id := range []string{"r1", "r2", "r3"} {
		if _, _, payable, err := e.m.Charge(bTenant, id); err != nil || payable {
			t.Errorf("%s: payable=%v err=%v, want not payable", id, payable, err)
		}
	}
}

func TestCharge_CounterPayable(t *testing.T) {
	e := newBookEnv(t)
	e.reserve(t, "r1", e.config(t, "s1", 0, false), "c1", 9, ab.OriginCounter)
	amount, currency, payable, err := e.m.Charge(bTenant, "r1")
	if err != nil || !payable || amount != 25000 || currency != "CLP" {
		t.Fatalf("got %d %s %v %v", amount, currency, payable, err)
	}
}

func TestCharge_OnlineNeedsPaymentWindow(t *testing.T) {
	e := newBookEnv(t)
	e.online(t, "r1", e.config(t, "s1", 0, false), "c1", 9)
	if _, _, payable, _ := e.m.Charge(bTenant, "r1"); !payable {
		t.Fatal("a fresh hold is payable")
	}
	e.patch(t, "r1", func(r *ab.Reservation) { r.HoldUntil = bookNow() + ab.PaymentWindowSeconds - 30 })
	if _, _, payable, _ := e.m.Charge(bTenant, "r1"); payable {
		t.Fatal("less than PaymentWindowSeconds left: not payable")
	}
}

func TestCharge_MinorUnitsByCurrency(t *testing.T) {
	e := newBookEnv(t)
	cfg := e.config(t, "s1", 0, false)
	e.reserve(t, "r1", cfg, "c1", 9, ab.OriginCounter)
	e.patch(t, "r1", func(r *ab.Reservation) { r.PriceSnapshot = 12.5; r.CurrencySnapshot = "USD" })
	if amount, currency, _, err := e.m.Charge(bTenant, "r1"); err != nil || amount != 1250 || currency != "USD" {
		t.Fatalf("USD: %d %s %v", amount, currency, err)
	}
	e.patch(t, "r1", func(r *ab.Reservation) { r.CurrencySnapshot = "EUR" })
	_, _, _, err := e.m.Charge(bTenant, "r1")
	wantErrIs(t, err, ab.ErrUnsupportedCurrency)
}
