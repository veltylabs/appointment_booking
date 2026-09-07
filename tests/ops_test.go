package tests

import (
	"testing"

	"webtyp.com/model"
	"webtyp.com/orm"
	"webtyp.com/router/mock"
	"webtyp.com/storage/mem"
	ab "github.com/veltylabs/appointment_booking"
)

func TestMountOperations_CreateReservation_SlotTaken(t *testing.T) {
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

	// Seeding config & calendar config for staff
	cfg := ab.EmployeeServiceConfig{
		TenantId:    "t1",
		StaffId:     "s1",
		ServiceId:   "srv1",
		DurationMin: 60,
		IsActive:    true,
	}
	db.Create(&cfg)

	db.Create(&ab.WorkCalendarConfig{
		TenantId: "t1",
		StaffId:  "s1",
		Timezone: "UTC",
		IsActive: true,
	})

	db.Create(&ab.WorkCalendarWeekly{
		TenantId:   "t1",
		StaffId:    "s1",
		DayOfWeek:  3, // Wednesday, same as 1700000000 UTC which is Wednesday
		WorkStart:  540,
		WorkFinish: 1020,
		IsActive:   true,
	})

	// 1700000000 UTC is Wednesday, 22:13:20 UTC. Wait! 1700000000 UTC is:
	// 1700000000 / 60 = 28333333 minutes since epoch.
	// Wait, is 1700000000 within work hours (540 to 1020 minutes from midnight)?
	// Let's check: 1700000000 UTC midnight:
	// We can choose a target date and SlotStartUtc explicitly inside work hours:
	// Let's use 1736154000 (Jan 6, 2025 is Monday, 09:00 UTC) which we already proved works in setup_test.go.
	// 1736154000 is Monday. Let's seed DayOfWeek: 1 (Monday).

	db.Create(&ab.WorkCalendarWeekly{
		TenantId:   "t1",
		StaffId:    "s1",
		DayOfWeek:  1, // Monday
		WorkStart:  540,
		WorkFinish: 1020,
		IsActive:   true,
	})

	body := []byte(`{"tenant_id":"t1","client_id":"c1","creator_user_id":"u1",` +
		`"employee_service_config_id":"` + cfg.Id + `","slot_start_utc":1736154000}`)

	ok := &mock.Context{InBody: body}
	ok.SetUserID("u1")
	reg.Invoke("OP", "/"+ab.OpCreateReservation, ok)
	if ok.Status != 0 && ok.Status != 200 {
		t.Fatalf("primera reserva: status %d, body=%s", ok.Status, ok.ResponseBody())
	}

	taken := &mock.Context{InBody: body}
	taken.SetUserID("u1")
	reg.Invoke("OP", "/"+ab.OpCreateReservation, taken)
	if taken.Status != 409 {
		t.Fatalf("slot tomado: se esperaba 409, got %d", taken.Status)
	}
}

func TestMountOperations_RBAC_Deny(t *testing.T) {
	db := orm.New(mem.New())
	m, err := ab.New(db, SetupDependencies())
	if err != nil {
		t.Fatalf("ab.New: %v", err)
	}

	reg := &mock.Router{}
	reg.Configure(mock.Config{
		Authorize: func(userID string, r model.Resource, a model.Action) bool { return false }, // denega todo
	})
	m.MountOperations(reg)

	body := []byte(`{"tenant_id":"t1","client_id":"c1"}`)
	ctx := &mock.Context{InBody: body}
	ctx.SetUserID("u1")
	reg.Invoke("OP", "/"+ab.OpGetReservation, ctx)
	if ctx.Status != 403 {
		t.Fatalf("esperaba 403 por denegación de RBAC, got %d", ctx.Status)
	}
}
