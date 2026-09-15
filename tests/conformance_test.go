package tests

import (
	"testing"

	ab "github.com/veltylabs/appointment_booking"
	"webtyp.com/json"
	"webtyp.com/router/mock"
	"webtyp.com/view"
)

func TestViewConformance(t *testing.T) {
	// Create mock results to be returned by list_reservations_by_staff
	r1 := &ab.Reservation{
		Id:              "res-1",
		TenantId:        "t1",
		LocalStringDate: "2026-03-04",
		LocalStringTime: "14:30",
		Status:          ab.StatusPending,
	}
	r2 := &ab.Reservation{
		Id:              "res-2",
		TenantId:        "t1",
		LocalStringDate: "2026-03-04",
		LocalStringTime: "15:00",
		Status:          ab.StatusConfirmed,
	}

	list := ab.ReservationList{r1, r2}

	var body []byte
	if err := json.Encode(&list, &body); err != nil {
		t.Fatalf("encode canned list: %v", err)
	}

	caller := &mock.Caller{CannedResult: body}

	pres := ab.NewView(caller, "t1", "staff-1")

	// 1. Title test
	if pres.Title() != "Reservas" {
		t.Fatalf("expected title 'Reservas', got %s", pres.Title())
	}

	// 2. Reload / list projection test
	var rerr error
	pres.Reload(func(err error) { rerr = err })
	if rerr != nil {
		t.Fatalf("Reload failed: %v", rerr)
	}

	items := pres.Items()
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}

	if items[0].ID != "res-1" || items[0].Label != "2026-03-04 14:30" || items[0].Description != ab.StatusPending {
		t.Fatalf("unexpected first item: %+v", items[0])
	}

	if items[1].ID != "res-2" || items[1].Label != "2026-03-04 15:00" || items[1].Description != ab.StatusConfirmed {
		t.Fatalf("unexpected second item: %+v", items[1])
	}

	// 3. Select / fill test
	selected := pres.Select("res-2")
	if selected == nil {
		t.Fatalf("expected selected model not to be nil")
	}
	gotRes2 := selected.(*ab.Reservation)
	if gotRes2.Id != "res-2" || gotRes2.Status != ab.StatusConfirmed {
		t.Fatalf("unexpected selected model: %+v", gotRes2)
	}

	if pres.Selected() != "res-2" {
		t.Fatalf("expected Selected() to return res-2, got %s", pres.Selected())
	}

	// 4. Save and delete capabilities must be disabled by design — Saver/Deleter are capabilities
	// the renderer discovers by type assertion (view.Presenter doc comment), not CanSave()/CanDelete().
	if _, ok := pres.(view.Saver); ok {
		t.Fatalf("expected no Saver capability")
	}

	if _, ok := pres.(view.Deleter); ok {
		t.Fatalf("expected no Deleter capability")
	}
}
