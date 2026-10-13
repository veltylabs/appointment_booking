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

	db.Create(&ab.WorkCalendarBlock{
		TenantId: "t1", StaffId: "s1", DayOfWeek: 3, SpecificDate: 0,
		StartMin: 540, EndMin: 1020, IsActive: true,
	})

	// 1700000000 UTC is Wednesday, 22:13:20 UTC. Wait! 1700000000 UTC is:
	// 1700000000 / 60 = 28333333 minutes since epoch.
	// Wait, is 1700000000 within work hours (540 to 1020 minutes from midnight)?
	// Let's check: 1700000000 UTC midnight:
	// We can choose a target date and SlotStartUtc explicitly inside work hours:
	// Let's use 1736154000 (Jan 6, 2025 is Monday, 09:00 UTC) which we already proved works in setup_test.go.
	// 1736154000 is Monday. Let's seed DayOfWeek: 1 (Monday).

	db.Create(&ab.WorkCalendarBlock{
		TenantId: "t1", StaffId: "s1", DayOfWeek: 1, SpecificDate: 0,
		StartMin: 540, EndMin: 1020, IsActive: true,
	})

	body := []byte(`{"id":"res-ops-1","tenant_id":"t1","client_id":"c1","creator_user_id":"u1",` +
		`"employee_service_config_id":"` + cfg.Id + `","slot_start_utc":1736154000,"origin":"` + ab.OriginCounter + `"}`)

	ok := &mock.Context{InBody: body}
	ok.SetUserID("u1")
	reg.Invoke("OP", "/"+ab.OpCreateReservation, ok)
	if ok.Status != 0 && ok.Status != 200 {
		t.Fatalf("primera reserva: status %d, body=%s", ok.Status, ok.ResponseBody())
	}

	bodyTaken := []byte(`{"id":"res-ops-2","tenant_id":"t1","client_id":"c1","creator_user_id":"u1",` +
		`"employee_service_config_id":"` + cfg.Id + `","slot_start_utc":1736154000,"origin":"` + ab.OriginCounter + `"}`)

	taken := &mock.Context{InBody: bodyTaken}
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

// A reservation for a patient the directory does not know is refused with 404 and says why. It used
// to answer 500 with an empty body: the error was untyped, so writeError fell to its last branch,
// and a client (rpc, a sync engine's conflict tray) could not tell what was wrong.
func TestMountOperations_CreateReservation_UnknownClient(t *testing.T) {
	db := orm.New(mem.New())
	deps := SetupDependencies()
	deps.Directory = &MockDirectoryReader{Exists: false}
	m, err := ab.New(db, deps)
	if err != nil {
		t.Fatalf("ab.New: %v", err)
	}
	reg := &mock.Router{}
	reg.Configure(mock.Config{
		Authorize: func(userID string, r model.Resource, a model.Action) bool { return true },
	})
	m.MountOperations(reg)

	cfg := ab.EmployeeServiceConfig{TenantId: "t1", StaffId: "s1", ServiceId: "srv1", DurationMin: 60, IsActive: true}
	db.Create(&cfg)

	body := []byte(`{"id":"res-unknown-client","tenant_id":"t1","client_id":"nobody","creator_user_id":"u1",` +
		`"employee_service_config_id":"` + cfg.Id + `","slot_start_utc":1736154000,"origin":"` + ab.OriginCounter + `"}`)
	ctx := &mock.Context{InBody: body}
	ctx.SetUserID("u1")
	reg.Invoke("OP", "/"+ab.OpCreateReservation, ctx)
	if ctx.Status != 404 || string(ctx.ResponseBody()) != ab.ErrClientNotFound.Error() {
		t.Fatalf("unknown client: got %d %q, want 404 %q", ctx.Status, ctx.ResponseBody(), ab.ErrClientNotFound.Error())
	}

	if _, err := m.CreateReservation(ab.CreateReservationCmd{Id: "res-unknown-2", TenantId: "t1", ClientId: "nobody",
		EmployeeServiceConfigId: cfg.Id, SlotStartUtc: 1736154000, Origin: ab.OriginCounter}); err != ab.ErrClientNotFound {
		t.Fatalf("CreateReservation: err = %v, want ErrClientNotFound", err)
	}
}

// Every refusal says why. A 400 used to go out with an empty body on a bad payload or a failed
// validation, so a client (rpc, mjosefa-cms tests, a sync engine's conflict tray) saw "rpc: 400"
// and had to guess which field was wrong.
func TestMountOperations_EveryRefusalHasAText(t *testing.T) {
	m, err := ab.New(orm.New(mem.New()), SetupDependencies())
	if err != nil {
		t.Fatalf("ab.New: %v", err)
	}
	reg := &mock.Router{}
	reg.Configure(mock.Config{
		Authorize: func(string, model.Resource, model.Action) bool { return true },
	})
	m.MountOperations(reg)

	for _, body := range []string{`{`, `{}`} {
		for _, r := range reg.Routes() {
			ctx := &mock.Context{InBody: []byte(body)}
			ctx.SetUserID("u1")
			reg.Invoke(r.Method, r.Path, ctx)
			if ctx.Status >= 400 && len(ctx.ResponseBody()) == 0 {
				t.Errorf("%s with %s: %d and no text", r.Path, body, ctx.Status)
			}
		}
	}
}

// A create_reservation without its id is refused with the validation's own text.
func TestMountOperations_CreateReservation_InvalidArgsSaysWhy(t *testing.T) {
	m, err := ab.New(orm.New(mem.New()), SetupDependencies())
	if err != nil {
		t.Fatalf("ab.New: %v", err)
	}
	reg := &mock.Router{}
	reg.Configure(mock.Config{
		Authorize: func(string, model.Resource, model.Action) bool { return true },
	})
	m.MountOperations(reg)

	args := ab.CreateReservationArgs{TenantId: "t1", ClientId: "c1", EmployeeServiceConfigId: "cfg1",
		SlotStartUtc: 1736154000, Origin: ab.OriginCounter}
	want := args.Validate(model.ActionCreate)
	if want == nil {
		t.Fatal("args without id must not validate")
	}
	body := []byte(`{"tenant_id":"t1","client_id":"c1","employee_service_config_id":"cfg1",` +
		`"slot_start_utc":1736154000,"origin":"` + ab.OriginCounter + `"}`)
	ctx := &mock.Context{InBody: body}
	ctx.SetUserID("u1")
	reg.Invoke("OP", "/"+ab.OpCreateReservation, ctx)
	if ctx.Status != 400 || string(ctx.ResponseBody()) != want.Error() {
		t.Fatalf("got %d %q, want 400 %q", ctx.Status, ctx.ResponseBody(), want.Error())
	}
}
