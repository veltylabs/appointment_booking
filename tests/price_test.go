package tests

import (
	"testing"

	ab "github.com/veltylabs/appointment_booking"
)

func TestPrice_CatalogErrorPropagates(t *testing.T) {
	e := newBookEnv(t)
	cfg := e.config(t, "s1", 0, false)
	e.catalog.cardErr = errStorage
	_, err := e.m.CreateReservation(ab.CreateReservationCmd{Id: "r1", TenantId: bTenant, ClientId: "c1", EmployeeServiceConfigId: cfg, SlotStartUtc: bookDay + 9*3600, Origin: ab.OriginCounter})
	wantErrIs(t, err, errStorage)
}

func TestPrice_OverrideWins(t *testing.T) {
	e := newBookEnv(t)
	r := e.reserve(t, "r1", e.config(t, "s1", 18000, false), "c1", 9, ab.OriginCounter)
	if r.PriceSnapshot != 18000 {
		t.Fatalf("price = %v, want the override", r.PriceSnapshot)
	}
}

func TestPrice_ListPriceWhenNoOverride(t *testing.T) {
	e := newBookEnv(t)
	r := e.reserve(t, "r1", e.config(t, "s1", 0, false), "c1", 9, ab.OriginCounter)
	if r.PriceSnapshot != 25000 {
		t.Fatalf("price = %v, want the catalog list price", r.PriceSnapshot)
	}
}

func TestPrice_CurrencyFromCatalogOrCLP(t *testing.T) {
	e := newBookEnv(t)
	cfg := e.config(t, "s1", 30, false)
	e.catalog.items[0].currency = "USD"
	if r := e.reserve(t, "r1", cfg, "c1", 9, ab.OriginCounter); r.CurrencySnapshot != "USD" {
		t.Fatalf("currency = %s, want the catalog's even with an override", r.CurrencySnapshot)
	}
	e.catalog.items[0].currency = ""
	if r := e.reserve(t, "r2", cfg, "c2", 10, ab.OriginCounter); r.CurrencySnapshot != "CLP" {
		t.Fatalf("currency = %s, want CLP when the catalog has none", r.CurrencySnapshot)
	}
}

func TestPrice_OverbookingUsesPriceFor(t *testing.T) {
	e := newBookEnv(t)
	cfg := e.config(t, "s1", 0, false)
	if err := e.m.UpsertCalendarConfig(ab.WorkCalendarConfig{TenantId: bTenant, StaffId: "s1", Timezone: "UTC", IsActive: true, MaxOverbookPerDay: 2}); err != nil {
		t.Fatal(err)
	}
	e.reserve(t, "r1", cfg, "c1", 9, ab.OriginCounter)
	ob, err := e.m.CreateOverbooking(ab.CreateReservationCmd{Id: "r2", TenantId: bTenant, ClientId: "c2", EmployeeServiceConfigId: cfg, SlotStartUtc: bookDay + 9*3600, Origin: ab.OriginCounter})
	if err != nil || ob.PriceSnapshot != 25000 || ob.CurrencySnapshot != "CLP" {
		t.Fatalf("overbooking = %v %s, %v", ob.PriceSnapshot, ob.CurrencySnapshot, err)
	}
	e.catalog.cardErr = errStorage
	_, err = e.m.CreateOverbooking(ab.CreateReservationCmd{Id: "r3", TenantId: bTenant, ClientId: "c3", EmployeeServiceConfigId: cfg, SlotStartUtc: bookDay + 9*3600, Origin: ab.OriginCounter})
	wantErrIs(t, err, errStorage)
}
