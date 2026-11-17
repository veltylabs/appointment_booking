package tests

import (
	"testing"

	ab "github.com/veltylabs/appointment_booking"
	"webtyp.com/model"
)

func TestAvailability_ExcludesBookedSlot(t *testing.T) {
	e := newEnv(t)
	e.schedule(0)
	free, err := e.mod.ListAvailability("t1", "s1", "cfg1", e.monday, e.monday)
	if err != nil {
		t.Fatal(err)
	}
	if len(free) != 16 { // 09:00–17:00 in 30-minute slots
		t.Fatalf("free slots = %d, want 16", len(free))
	}
	if _, err := e.book("r1", 540); err != nil {
		t.Fatal(err)
	}
	after, err := e.mod.ListAvailability("t1", "s1", "cfg1", e.monday, e.monday)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != 15 {
		t.Fatalf("free slots after booking = %d, want 15", len(after))
	}
	for _, s := range after {
		if s.StartUtc == e.at(540) {
			t.Fatal("the booked slot is still offered")
		}
	}
}

func TestBlockCreate_RejectsOverlap(t *testing.T) {
	e := newEnv(t)
	e.schedule(0)
	err := e.create(&ab.WorkCalendarBlock{Id: "b2", StaffId: "s1", DayOfWeek: 1, StartMin: 600, EndMin: 700}, &ab.WorkCalendarBlock{})
	if err == nil {
		t.Fatal("a block overlapping b1 was accepted")
	}
}

func TestBookableServices_PriceOverrideWins(t *testing.T) {
	e := newEnv(t)
	cfg := &ab.EmployeeServiceConfig{Id: "cfg1", StaffId: "s1", ServiceId: "v1", DurationMin: 30, IsActive: true, PriceOverride: 250}
	e.must(e.create(cfg, &ab.EmployeeServiceConfig{}))
	list, err := e.mod.ListBookableServices("t1", "")
	if err != nil || len(list) != 1 {
		t.Fatalf("list = %v, %v", list, err)
	}
	if list[0].Price != 250 || list[0].Currency != "CLP" {
		t.Fatalf("price = %v %s, want the override 250 CLP", list[0].Price, list[0].Currency)
	}
}

func TestCharge_PendingReservationIsPayable(t *testing.T) {
	e := newEnv(t)
	e.schedule(0)
	if _, err := e.book("r1", 540); err != nil {
		t.Fatal(err)
	}
	amount, currency, payable, err := e.mod.Charge("t1", "r1")
	if err != nil || !payable || amount != 100 || currency != "CLP" {
		t.Fatalf("charge = %d %s payable %v err %v", amount, currency, payable, err)
	}
}

func TestConfirmPaidReservation_IsIdempotent(t *testing.T) {
	e := newEnv(t)
	e.schedule(0)
	if _, err := e.book("r1", 540); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := e.mod.ConfirmPaidReservation("t1", "r1", "pay1"); err != nil {
			t.Fatalf("confirm #%d: %v", i+1, err)
		}
	}
	got := e.reservation("r1")
	if got.Status != ab.StatusConfirmed || got.PaymentId != "pay1" {
		t.Fatalf("reservation = %s payment %q", got.Status, got.PaymentId)
	}
	if err := e.mod.ConfirmPaidReservation("t1", "r1", "pay2"); model.KindOf(err) != model.ErrorInvalid {
		t.Fatalf("a second payment: %v, want ErrAlreadyPaid", err)
	}
}

func TestImportReservation_TwiceLeavesOneRow(t *testing.T) {
	e := newEnv(t)
	e.schedule(0)
	row := ab.Reservation{Id: "h1", TenantId: "t1", ClientId: "c1", EmployeeServiceConfigId: "cfg1", ReservationTime: int64(e.at(540)),
		Status: ab.StatusCompleted, Origin: ab.OriginCounter}
	for i := 0; i < 2; i++ {
		if _, err := e.mod.ImportReservation(row); err != nil {
			t.Fatalf("import #%d: %v", i+1, err)
		}
	}
	if n := len(e.reservations()); n != 1 {
		t.Fatalf("rows = %d, want 1", n)
	}
	row.Status = ab.StatusPending
	if _, err := e.mod.ImportReservation(row); err == nil {
		t.Fatal("an imported PENDING reservation was accepted")
	}
}

func TestTenantIsolation_OtherTenantRejected(t *testing.T) {
	e := newEnv(t)
	e.schedule(0)
	if _, err := e.mod.GetReservation("other", "r1"); err == nil {
		t.Fatal("a reservation was read through another tenant")
	}
}
