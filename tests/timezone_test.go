package tests

import (
	"testing"

	ab "github.com/veltylabs/appointment_booking"
	"webtyp.com/orm"
	"webtyp.com/storage/mem"
	"webtyp.com/time"
)

func TestNightBookingInItsDay(t *testing.T) {
	db := orm.New(mem.New())
	scl, err := time.LoadZone("America/Santiago")
	if err != nil {
		t.Fatalf("LoadZone: %v", err)
	}
	deps := SetupDependencies()
	deps.Zone = scl
	m, err := ab.New(db, deps)
	if err != nil {
		t.Fatalf("ab.New: %v", err)
	}

	// 2026-06-15 23:00 en Santiago (UTC-4 en invierno)
	// 23:00 Santiago = 2026-06-16 03:00 UTC.
	u, err := scl.At("2026-06-15", 23*60)
	if err != nil {
		t.Fatalf("scl.At: %v", err)
	}

	if date := m.Zone().Date(u); date != "2026-06-15" {
		t.Fatalf("Date = %s, want 2026-06-15", date)
	}
	if clock := m.Zone().Clock(u); clock != "23:00" {
		t.Fatalf("Clock = %s, want 23:00", clock)
	}
}

func TestSlotsAcrossDST(t *testing.T) {
	db := orm.New(mem.New())
	scl, err := time.LoadZone("America/Santiago")
	if err != nil {
		t.Fatalf("LoadZone: %v", err)
	}
	deps := SetupDependencies()
	deps.Zone = scl
	m, err := ab.New(db, deps)
	if err != nil {
		t.Fatalf("ab.New: %v", err)
	}

	uBefore, err := scl.At("2026-09-05", 12*60)
	if err != nil {
		t.Fatalf("scl.At before: %v", err)
	}
	uAfter, err := scl.At("2026-09-07", 12*60)
	if err != nil {
		t.Fatalf("scl.At after: %v", err)
	}

	if m.Zone().Clock(uBefore) != "12:00" || m.Zone().Date(uBefore) != "2026-09-05" {
		t.Fatalf("before DST mismatch: %s %s", m.Zone().Date(uBefore), m.Zone().Clock(uBefore))
	}
	if m.Zone().Clock(uAfter) != "12:00" || m.Zone().Date(uAfter) != "2026-09-07" {
		t.Fatalf("after DST mismatch: %s %s", m.Zone().Date(uAfter), m.Zone().Clock(uAfter))
	}
}
