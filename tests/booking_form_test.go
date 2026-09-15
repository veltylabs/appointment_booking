package tests

import (
	"errors"
	"testing"

	"webtyp.com/json"
	"webtyp.com/model"
	"webtyp.com/orm"
	"webtyp.com/router/mock"
	"webtyp.com/storage/mem"
	tinytime "webtyp.com/time"
	"webtyp.com/view"

	ab "github.com/veltylabs/appointment_booking"
)

type loopbackCaller struct {
	reg *mock.Router
}

func (l loopbackCaller) Call(op string, in model.Encodable, out model.Decodable, cb func(error)) {
	var body []byte
	if err := json.Encode(in, &body); err != nil {
		cb(err)
		return
	}
	ctx := &mock.Context{InBody: body}
	ctx.SetUserID("test-user")
	l.reg.Invoke("OP", "/"+op, ctx)

	if ctx.Status != 0 && ctx.Status != 200 {
		cb(errors.New(string(ctx.ResponseBody())))
		return
	}
	if out != nil {
		respBody := ctx.ResponseBody()
		if err := json.Decode(respBody, out); err != nil {
			cb(err)
			return
		}
	}
	cb(nil)
}

func (l loopbackCaller) Dispatch(op string, in model.Encodable) {
	l.Call(op, in, nil, func(err error) {})
}

func setupBookingFormTestEnv(t *testing.T) (*mock.Router, *ab.EmployeeServiceConfig) {
	db := orm.New(mem.New())
	m, err := ab.New(db, SetupDependencies())
	if err != nil {
		t.Fatalf("ab.New: %v", err)
	}

	reg := &mock.Router{}
	reg.Configure(mock.Config{
		Authorize: func(userID string, r model.Resource, a model.Action) bool { return true },
	})
	m.MountOperations(reg)

	cfg := ab.EmployeeServiceConfig{
		Id:          "esc1",
		TenantId:    "t1",
		StaffId:     "s1",
		ServiceId:   "srv1",
		DurationMin: 60,
		IsActive:    true,
	}
	if err := db.Create(&cfg); err != nil {
		t.Fatalf("Create EmployeeServiceConfig: %v", err)
	}

	if err := db.Create(&ab.WorkCalendarConfig{
		TenantId: "t1",
		StaffId:  "s1",
		Timezone: "UTC",
		IsActive: true,
	}); err != nil {
		t.Fatalf("Create WorkCalendarConfig: %v", err)
	}

	// Monday (DayOfWeek 1): 09:00 (540) - 17:00 (1020)
	if err := db.Create(&ab.WorkCalendarBlock{
		TenantId: "t1", StaffId: "s1", DayOfWeek: 1, SpecificDate: 0,
		StartMin: 540, EndMin: 1020, IsActive: true,
	}); err != nil {
		t.Fatalf("Create WorkCalendarBlock: %v", err)
	}

	return reg, &cfg
}

func TestFormView_ListsScopedReservations(t *testing.T) {
	reg, cfg := setupBookingFormTestEnv(t)
	caller := loopbackCaller{reg: reg}

	// Create a reservation via caller first
	// 1736154000 = Mon Jan 06 2025 09:00:00 UTC
	createRes := ab.CreateReservationArgs{
		TenantId:                "t1",
		ClientId:                "c1",
		CreatorUserId:           "u1",
		EmployeeServiceConfigId: cfg.Id,
		SlotStartUtc:            1736154000,
		Notes:                   "First booking",
	}
	var res ab.Reservation
	caller.Call(ab.OpCreateReservation, &createRes, &res, func(err error) {
		if err != nil {
			t.Fatalf("OpCreateReservation failed: %v", err)
		}
	})

	formCfg := ab.FormConfig{
		TenantId:        "t1",
		StaffId:         "s1",
		ServiceConfigId: cfg.Id,
		Timezone:        "UTC",
		From:            1736121600, // Jan 6, 2025 midnight UTC
		To:              1736208000,
	}

	pv := ab.NewFormView(caller, formCfg)
	var rerr error
	pv.Reload(func(err error) { rerr = err })
	if rerr != nil {
		t.Fatalf("Reload failed: %v", rerr)
	}

	items := pv.Items()
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	item := items[0]
	if item.ID != res.Id {
		t.Errorf("expected ID %q, got %q", res.Id, item.ID)
	}
	if item.LeadMain != res.LocalStringTime {
		t.Errorf("expected LeadMain %q, got %q", res.LocalStringTime, item.LeadMain)
	}
	if item.Label != "c1" {
		t.Errorf("expected Label %q, got %q", "c1", item.Label)
	}
	if item.Description != ab.StatusPending {
		t.Errorf("expected Status %q, got %q", ab.StatusPending, item.Description)
	}
}

func TestFormView_ListEmptyWithoutStaff(t *testing.T) {
	reg, cfg := setupBookingFormTestEnv(t)
	caller := loopbackCaller{reg: reg}

	formCfg := ab.FormConfig{
		TenantId:        "t1",
		StaffId:         "", // empty staff id
		ServiceConfigId: cfg.Id,
		Timezone:        "UTC",
	}

	pv := ab.NewFormView(caller, formCfg)
	var rerr error
	pv.Reload(func(err error) { rerr = err })
	if rerr != nil {
		t.Fatalf("Reload failed: %v", rerr)
	}

	items := pv.Items()
	if len(items) != 0 {
		t.Fatalf("expected 0 items, got %d", len(items))
	}
}

func TestFormView_SaveCreatesReservation(t *testing.T) {
	reg, cfg := setupBookingFormTestEnv(t)
	caller := loopbackCaller{reg: reg}

	formCfg := ab.FormConfig{
		TenantId:        "t1",
		StaffId:         "s1",
		ServiceConfigId: cfg.Id,
		Timezone:        "UTC",
		From:            1736121600, // Jan 6, 2025 midnight UTC
		To:              1736208000,
		ActorId:         "u1",
	}

	pv := ab.NewFormView(caller, formCfg)
	saver, ok := pv.(view.Saver)
	if !ok {
		t.Fatalf("presenter does not implement view.Saver")
	}

	rec := &ab.ReservationForm{
		ClientId: "c1",
		Day:      "2025-01-06",
		Hour:     "09:00",
		Notes:    "Test save",
	}

	var serr error
	saver.Save([]model.Model{rec}, func(err error) { serr = err })
	if serr != nil {
		t.Fatalf("Save failed: %v", serr)
	}

	// Verify creation by listing
	var rerr error
	pv.Reload(func(err error) { rerr = err })
	if rerr != nil {
		t.Fatalf("Reload failed: %v", rerr)
	}
	items := pv.Items()
	if len(items) != 1 {
		t.Fatalf("expected 1 item after save, got %d", len(items))
	}
}

func TestFormView_SaveWithoutServiceConfig(t *testing.T) {
	reg, _ := setupBookingFormTestEnv(t)
	caller := loopbackCaller{reg: reg}

	formCfg := ab.FormConfig{
		TenantId:        "t1",
		StaffId:         "s1",
		ServiceConfigId: "", // missing service config
		Timezone:        "UTC",
	}

	pv := ab.NewFormView(caller, formCfg)
	saver := pv.(view.Saver)

	rec := &ab.ReservationForm{
		ClientId: "c1",
		Day:      "2025-01-06",
		Hour:     "09:00",
	}

	var serr error
	saver.Save([]model.Model{rec}, func(err error) { serr = err })
	if !errors.Is(serr, ab.ErrNoServiceConfig) && serr != ab.ErrNoServiceConfig {
		t.Fatalf("expected ErrNoServiceConfig, got %v", serr)
	}
}

func TestFormView_SaveWithoutHour(t *testing.T) {
	reg, cfg := setupBookingFormTestEnv(t)
	caller := loopbackCaller{reg: reg}

	formCfg := ab.FormConfig{
		TenantId:        "t1",
		StaffId:         "s1",
		ServiceConfigId: cfg.Id,
		Timezone:        "UTC",
	}

	pv := ab.NewFormView(caller, formCfg)
	saver := pv.(view.Saver)

	rec := &ab.ReservationForm{
		ClientId: "c1",
		Day:      "2025-01-06",
		Hour:     "", // missing hour
	}

	var serr error
	saver.Save([]model.Model{rec}, func(err error) { serr = err })
	if !errors.Is(serr, ab.ErrIncompleteSlot) && serr != ab.ErrIncompleteSlot {
		t.Fatalf("expected ErrIncompleteSlot, got %v", serr)
	}
}

func TestFormView_NoUpdateNoDelete(t *testing.T) {
	reg, cfg := setupBookingFormTestEnv(t)
	caller := loopbackCaller{reg: reg}

	formCfg := ab.FormConfig{
		TenantId:        "t1",
		StaffId:         "s1",
		ServiceConfigId: cfg.Id,
		Timezone:        "UTC",
	}

	pv := ab.NewFormView(caller, formCfg)

	if _, ok := pv.(view.Updater); ok {
		t.Errorf("presenter should NOT implement view.Updater")
	}
	if _, ok := pv.(view.Deleter); ok {
		t.Errorf("presenter should NOT implement view.Deleter")
	}
}

func TestDayToUnix_RoundTripsAtNegativeOffset(t *testing.T) {
	dayStr := "2026-09-08"

	// ParseDate converts "2026-09-08" -> nanoseconds
	nano, err := tinytime.ParseDate(dayStr)
	if err != nil {
		t.Fatalf("ParseDate error: %v", err)
	}
	sec := nano / 1000000000

	// FormatISO8601 returns UTC format e.g. "2026-09-08T00:00:00Z"
	iso := tinytime.FormatISO8601(sec * 1000000000)
	if len(iso) < 10 {
		t.Fatalf("unexpected ISO string: %q", iso)
	}
	formatted := iso[:10]
	if formatted != dayStr {
		t.Fatalf("expected %q, got %q", dayStr, formatted)
	}
}

func TestFreeSlots_EmptyWithoutServiceConfig(t *testing.T) {
	reg, _ := setupBookingFormTestEnv(t)
	caller := loopbackCaller{reg: reg}

	formCfg := ab.FormConfig{
		TenantId:        "t1",
		StaffId:         "s1",
		ServiceConfigId: "", // empty
		Timezone:        "UTC",
	}

	slots, err := ab.FreeSlots(caller, formCfg, "2025-01-06")
	if err != nil {
		t.Fatalf("FreeSlots unexpected error: %v", err)
	}
	if slots != nil {
		t.Fatalf("expected nil slots, got %v", slots)
	}
}
