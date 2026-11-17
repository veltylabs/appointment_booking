package tests

import (
	"testing"

	ab "github.com/veltylabs/appointment_booking"
	"webtyp.com/crud"
	"webtyp.com/model"
	"webtyp.com/router"
	"webtyp.com/time"
)

func TestMount_MatchesDefinition(t *testing.T) {
	e := newEnv(t)
	if err := crud.Mount(nil, nil, nil, e.mod); err != nil {
		t.Fatalf("crud.Mount: %v", err)
	}
}

func TestReservation_PublishesChange(t *testing.T) {
	e := newEnv(t)
	e.schedule(0)
	pub := &MockEventPublisher{}
	e.caller = crud.LoopbackActingAsWithQueue(pub, "u1", e.queue, e.mod)
	if _, err := e.book("r1", 540); err != nil {
		t.Fatal(err)
	}
	want := crud.Topic(ab.ModelName, "reservation", model.Create)
	for _, topic := range pub.PublishedEvents {
		if topic == want {
			return
		}
	}
	t.Fatalf("no %q among %v", want, pub.PublishedEvents)
}

func TestReservation_NoDoubleBooking(t *testing.T) {
	e := newEnv(t)
	e.schedule(0)
	if _, err := e.book("r1", 540); err != nil {
		t.Fatalf("first: %v", err)
	}
	if _, err := e.book("r2", 540); err == nil {
		t.Fatal("second booking of the same slot was accepted")
	}
	if n := len(e.reservations()); n != 1 {
		t.Fatalf("rows = %d, want 1", n)
	}
}

func TestReservation_CreatorFromCaller(t *testing.T) {
	e := newEnv(t)
	e.schedule(0)
	forged := &ab.Reservation{Id: "r1", ClientId: "c1", EmployeeServiceConfigId: "cfg1", ReservationTime: int64(e.at(540)), CreatorUserId: "someone-else"}
	if err := e.create(forged, &ab.Reservation{}); err == nil {
		t.Fatal("a creator sent by the client was accepted")
	}
	got, err := e.book("r2", 570)
	if err != nil {
		t.Fatal(err)
	}
	if got.CreatorUserId != "u1" {
		t.Fatalf("creator = %q, want the caller u1", got.CreatorUserId)
	}
}

func TestReservation_StatusTransitions(t *testing.T) {
	e := newEnv(t)
	e.schedule(0)
	if _, err := e.book("r1", 540); err != nil {
		t.Fatal(err)
	}
	change := func(status string) error {
		rec := &ab.Reservation{Id: "r1", Status: status}
		return e.do(op("reservation", model.Update), &crud.UpdateArgs{IDs: []string{"r1"}, Fields: []string{ab.Reservation_.Status}, Record: rec}, nil)
	}
	err := change(ab.StatusCompleted) // PENDING cannot complete
	if err == nil || err.Error() == "" {
		t.Fatal("PENDING → COMPLETED was accepted")
	}
	if got := e.reservation("r1").Status; got != ab.StatusPending {
		t.Fatalf("status after the rejected change = %s", got)
	}
	if err := change(ab.StatusConfirmed); err != nil {
		t.Fatalf("PENDING → CONFIRMED: %v", err)
	}
	if got := e.reservation("r1").Status; got != ab.StatusConfirmed {
		t.Fatalf("status = %s, want CONFIRMED", got)
	}
}

func TestOverbooking_OwnPermission(t *testing.T) {
	e := newEnv(t)
	e.schedule(1)
	if _, err := e.book("r1", 540); err != nil {
		t.Fatal(err)
	}
	if _, err := e.book("r2", 540); err == nil {
		t.Fatal("create on reservation accepted a taken slot")
	}
	var over ab.ReservationOverbooking
	err := e.create(&ab.ReservationOverbooking{Id: "o1", ClientId: "c2", EmployeeServiceConfigId: "cfg1", SlotTimeUtc: e.at(540)}, &over)
	if err != nil {
		t.Fatalf("overbooking: %v", err)
	}
	if over.ReservationId == "" {
		t.Fatal("the overbooking did not record its reservation")
	}
	if !e.reservation(over.ReservationId).Overbooked {
		t.Fatal("the reservation is not marked overbooked")
	}
	var second ab.ReservationOverbooking
	if err := e.create(&ab.ReservationOverbooking{Id: "o2", ClientId: "c3", EmployeeServiceConfigId: "cfg1", SlotTimeUtc: e.at(540)}, &second); err == nil {
		t.Fatal("overbooking beyond max_overbook_per_day was accepted")
	}
}

func TestForwardingOperationsGone(t *testing.T) {
	e := newEnv(t)
	gone := router.Operation{Module: ab.ModelName, Entity: "booking_patient", Action: model.Read}
	if err := e.do(gone, &crud.ReadArgs{}, &ab.ReservationList{}); err == nil {
		t.Fatal("booking_patient is still mounted")
	}
}

func TestExpiry_RecordsCount(t *testing.T) {
	e := newEnv(t)
	past := time.NowUnix() - time.Day
	for _, id := range []string{"p1", "p2"} {
		row := &ab.Reservation{Id: id, ClientId: "c", EmployeeServiceConfigId: "cfg1", StaffIdsnapshot: "s1", ServiceIdsnapshot: "v1",
			ReservationTime: int64(past), Status: ab.StatusPending, Origin: ab.OriginCounter, Revision: 1}
		if err := e.mod.StoreReservation().Create(row); err != nil {
			t.Fatal(err)
		}
	}
	var order ab.ReservationExpiry
	e.must(e.create(&ab.ReservationExpiry{Id: "x1"}, &order))
	if order.Expired != 2 {
		t.Fatalf("expired = %d, want 2", order.Expired)
	}
	for _, id := range []string{"p1", "p2"} {
		if got := e.reservation(id).Status; got != ab.StatusExpired {
			t.Fatalf("%s status = %s", id, got)
		}
	}
}
