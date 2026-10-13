package tests

import (
	"testing"

	ab "github.com/veltylabs/appointment_booking"
)

func TestConfirmPaid_NoPaymentsUnavailable(t *testing.T) {
	err := buildBookEnv(t, false).m.ConfirmPaidReservation(bTenant, "r1", "p1")
	wantErrIs(t, err, ab.ErrPaymentsUnavailable)
}

func TestConfirmPaid_OtherTenantNotFound(t *testing.T) {
	e := newBookEnv(t)
	e.reserve(t, "r1", e.config(t, "s1", 0, false), "c1", 9, ab.OriginCounter)
	wantErrIs(t, e.m.ConfirmPaidReservation("t2", "r1", "p1"), ab.ErrNotFound)
}

func TestConfirmPaid_PendingBecomesConfirmedAndHoldCleared(t *testing.T) {
	e := newBookEnv(t)
	before := e.online(t, "r1", e.config(t, "s1", 0, false), "c1", 9)
	// pagó en los últimos minutos de la retención: igual vale
	e.patch(t, "r1", func(r *ab.Reservation) { r.HoldUntil = bookNow() + 60 })
	if err := e.m.ConfirmPaidReservation(bTenant, "r1", "p1"); err != nil {
		t.Fatal(err)
	}
	r := e.get(t, "r1")
	if r.Status != ab.StatusConfirmed || r.PaymentId != "p1" || r.HoldUntil != 0 || r.Revision != before.Revision+1 || r.UpdatedBy != ab.ActorPayments {
		t.Fatalf("reservation = %+v", r)
	}
}

func TestConfirmPaid_ConfirmedGetsPaymentId(t *testing.T) {
	e := newBookEnv(t)
	r := e.reserve(t, "r1", e.config(t, "s1", 0, false), "c1", 9, ab.OriginCounter)
	if err := e.m.ChangeReservationStatus(ab.ChangeStatusCmd{TenantId: bTenant, Id: "r1", Event: ab.EventConfirm, Revision: int(r.Revision)}); err != nil {
		t.Fatal(err)
	}
	if err := e.m.ConfirmPaidReservation(bTenant, "r1", "p1"); err != nil {
		t.Fatal(err)
	}
	if got := e.get(t, "r1"); got.Status != ab.StatusConfirmed || got.PaymentId != "p1" {
		t.Fatalf("reservation = %+v", got)
	}
}

func TestConfirmPaid_IdempotentSamePayment(t *testing.T) {
	e := newBookEnv(t)
	e.reserve(t, "r1", e.config(t, "s1", 0, false), "c1", 9, ab.OriginCounter)
	if err := e.m.ConfirmPaidReservation(bTenant, "r1", "p1"); err != nil {
		t.Fatal(err)
	}
	events := len(e.pub.PublishedEvents)
	if err := e.m.ConfirmPaidReservation(bTenant, "r1", "p1"); err != nil {
		t.Fatalf("repeat: %v", err)
	}
	if len(e.pub.PublishedEvents) != events {
		t.Fatal("a repeated confirmation publishes nothing")
	}
}

func TestConfirmPaid_OtherPaymentAlreadyPaid(t *testing.T) {
	e := newBookEnv(t)
	e.reserve(t, "r1", e.config(t, "s1", 0, false), "c1", 9, ab.OriginCounter)
	if err := e.m.ConfirmPaidReservation(bTenant, "r1", "p1"); err != nil {
		t.Fatal(err)
	}
	wantErrIs(t, e.m.ConfirmPaidReservation(bTenant, "r1", "p2"), ab.ErrAlreadyPaid)
}

func TestConfirmPaid_UnverifiedOrShortPaymentRejected(t *testing.T) {
	e := newBookEnv(t)
	e.reserve(t, "r1", e.config(t, "s1", 0, false), "c1", 9, ab.OriginCounter)
	e.pay.err = errStorage
	wantErrIs(t, e.m.ConfirmPaidReservation(bTenant, "r1", "p1"), ab.ErrPaymentNotVerified)
	e.pay.err = nil
	e.pay.amount = 24999
	wantErrIs(t, e.m.ConfirmPaidReservation(bTenant, "r1", "p1"), ab.ErrPaymentNotVerified)
	e.pay.amount = 25000
	e.pay.currency = "USD"
	wantErrIs(t, e.m.ConfirmPaidReservation(bTenant, "r1", "p1"), ab.ErrPaymentNotVerified)
}

func TestConfirmPaid_UnsupportedCurrency(t *testing.T) {
	e := newBookEnv(t)
	e.reserve(t, "r1", e.config(t, "s1", 0, false), "c1", 9, ab.OriginCounter)
	e.patch(t, "r1", func(r *ab.Reservation) { r.CurrencySnapshot = "EUR" })
	wantErrIs(t, e.m.ConfirmPaidReservation(bTenant, "r1", "p1"), ab.ErrUnsupportedCurrency)
}

func TestConfirmPaid_ExpiredNotPayable(t *testing.T) {
	e := newBookEnv(t)
	r := e.reserve(t, "r1", e.config(t, "s1", 0, false), "c1", 9, ab.OriginCounter)
	if err := e.m.ChangeReservationStatus(ab.ChangeStatusCmd{TenantId: bTenant, Id: "r1", Event: ab.EventExpire, Revision: int(r.Revision)}); err != nil {
		t.Fatal(err)
	}
	wantErrIs(t, e.m.ConfirmPaidReservation(bTenant, "r1", "p1"), ab.ErrReservationNotPayable)
}

// Otro proceso cambia la reserva mientras se verifica el pago.
func TestConfirmPaid_RevisionConflict(t *testing.T) {
	e := newBookEnv(t)
	r := e.reserve(t, "r1", e.config(t, "s1", 0, false), "c1", 9, ab.OriginCounter)
	e.pay.onPaid = func() {
		if err := e.m.ChangeReservationStatus(ab.ChangeStatusCmd{TenantId: bTenant, Id: "r1", Event: ab.EventConfirm, Revision: int(r.Revision)}); err != nil {
			t.Fatalf("concurrent confirm: %v", err)
		}
	}
	wantErrIs(t, e.m.ConfirmPaidReservation(bTenant, "r1", "p1"), ab.ErrConflict)
	if got := e.get(t, "r1"); got.PaymentId != "" {
		t.Fatalf("nothing written expected, got %+v", got)
	}
}

func TestConfirmPaid_PublishesConfirmation(t *testing.T) {
	e := newBookEnv(t)
	e.reserve(t, "r1", e.config(t, "s1", 0, false), "c1", 9, ab.OriginCounter)
	if err := e.m.ConfirmPaidReservation(bTenant, "r1", "p1"); err != nil {
		t.Fatal(err)
	}
	got := e.pub.PublishedEvents
	if got[len(got)-1] != ab.EventReservationConfirmed {
		t.Fatalf("events = %v", got)
	}
}

func TestConfirmPaid_WithoutPublisher(t *testing.T) {
	e := newBookEnv(t)
	cfg := e.config(t, "s1", 0, false)
	e.reserve(t, "r1", cfg, "c1", 9, ab.OriginCounter)
	quiet, err := ab.New(e.db, ab.Deps{Staff: e.staff, Catalog: e.catalog, Directory: &MockDirectoryReader{Exists: true}, IDs: &fakeIDs{}, Payments: e.pay})
	if err != nil {
		t.Fatal(err)
	}
	if err := quiet.ConfirmPaidReservation(bTenant, "r1", "p1"); err != nil {
		t.Fatalf("a nil Publisher must not break the confirmation: %v", err)
	}
}
