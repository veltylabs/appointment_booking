package tests

import (
	"testing"

	ab "github.com/veltylabs/appointment_booking"
	"webtyp.com/crud"
	"webtyp.com/fmt"
	"webtyp.com/model"
)

func (e *env) online(id string, minutes int) (ab.OnlineReservation, error) {
	var out ab.OnlineReservation
	err := e.create(&ab.OnlineReservation{Id: id, EmployeeServiceConfigId: "cfg1", SlotTimeUtc: e.at(minutes),
		DocumentNumber: "12.345.678-5", Name: "Juan Pérez", Email: "j@example.com", ReturnTo: "back"}, &out)
	return out, err
}

func (e *env) onlineRow(id string) ab.OnlineReservation {
	e.t.Helper()
	var list ab.OnlineReservationList
	e.must(e.do(op("online_reservation", model.Read), &crud.ReadArgs{ID: id}, &list))
	if len(list) != 1 {
		e.t.Fatalf("online_reservation %s: %d rows", id, len(list))
	}
	return *list[0]
}

func TestOnline_HoldThenProcess(t *testing.T) {
	e := newEnv(t)
	e.schedule(0)
	if _, err := e.online("on1", 540); err != nil {
		t.Fatalf("online: %v", err)
	}
	row := e.onlineRow("on1")
	if row.PatientId == "" || row.PaymentUrl == "" || row.Status != ab.OnlineDone {
		t.Fatalf("row = patient %q url %q status %q", row.PatientId, row.PaymentUrl, row.Status)
	}
	if e.patients.created != 1 || e.payments.started != 1 {
		t.Fatalf("patients created %d, payments started %d, want 1 and 1", e.patients.created, e.payments.started)
	}
	if len(e.queue.done) != 1 {
		t.Fatalf("queue.done = %v, want one id", e.queue.done)
	}
	held := e.reservation(row.ReservationId)
	if held.Status != ab.StatusPending || held.Origin != ab.OriginOnline || held.HoldUntil == 0 {
		t.Fatalf("held reservation = %s %s hold %d", held.Status, held.Origin, held.HoldUntil)
	}
	if held.ClientId != row.PatientId {
		t.Fatalf("reservation client %q, want the resolved patient %q", held.ClientId, row.PatientId)
	}
	if _, err := e.book("r2", 540); err == nil {
		t.Fatal("the held slot was booked again")
	}
}

func TestOnline_WithoutPaymentsPort(t *testing.T) {
	e := newEnv(t, withoutPayments())
	e.schedule(0)
	if _, err := e.online("on1", 540); err != nil {
		t.Fatalf("online: %v", err)
	}
	row := e.onlineRow("on1")
	if row.Status != ab.OnlineDone || row.PatientId == "" || row.PaymentUrl != "" {
		t.Fatalf("row = status %q patient %q url %q", row.Status, row.PatientId, row.PaymentUrl)
	}
}

func TestOnline_PaymentFailureKeepsHold(t *testing.T) {
	e := newEnv(t)
	e.schedule(0)
	e.payments.fail = fmt.Err("gateway down")
	e.online("on1", 540) // the request reports the failed call; what matters is what stays
	if len(e.queue.failed) != 1 {
		t.Fatalf("queue.failed = %v, want the reason once", e.queue.failed)
	}
	row := e.onlineRow("on1")
	if row.Status != ab.OnlinePending {
		t.Fatalf("status = %q, want pending", row.Status)
	}
	if got := e.reservation(row.ReservationId).Status; got != ab.StatusPending {
		t.Fatalf("held reservation = %s", got)
	}
	if _, err := e.book("r2", 540); err == nil {
		t.Fatal("the slot was freed by a failed payment start")
	}
}
