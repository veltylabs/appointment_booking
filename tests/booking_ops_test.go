package tests

import (
	"testing"

	wfmt "webtyp.com/fmt"
	"webtyp.com/json"
	"webtyp.com/model"
	"webtyp.com/router"
	"webtyp.com/router/mock"

	ab "github.com/veltylabs/appointment_booking"
)

func bookingRouter(e *bookEnv) *mock.Router {
	r := &mock.Router{}
	r.Configure(mock.Config{
		Authn: func(next router.HandlerFunc) router.HandlerFunc {
			return func(c router.Context) { c.SetUserID("staff"); next(c) }
		},
		Authorize: func(string, model.Resource, model.Action) bool { return true },
	})
	e.m.MountOperations(r)
	return r
}

func invoke(r *mock.Router, op, body string) (int, string) {
	ctx := &mock.Context{InMethod: "OP", InPath: "/" + op, InBody: []byte(body)}
	r.Invoke("OP", "/"+op, ctx)
	if ctx.Status == 0 {
		return 200, string(ctx.ResponseBody())
	}
	return ctx.Status, string(ctx.ResponseBody())
}

func jsonOf(t *testing.T, v model.Encodable) string {
	t.Helper()
	var out []byte
	if err := json.Encode(v, &out); err != nil {
		t.Fatal(err)
	}
	return string(out)
}

func TestOps_NewOpsRequires(t *testing.T) {
	r := bookingRouter(newBookEnv(t))
	want := []struct {
		op, resource string
		action       model.Action
	}{
		{ab.OpListBookableServices, ab.ResourceBookingCatalog, model.Read},
		{ab.OpListServiceAvailability, ab.ResourceBookingCatalog, model.Read},
		{ab.OpCreateOnlineReservation, ab.ResourceReservationOnline, model.Create},
	}
	for _, w := range want {
		found := false
		for _, info := range r.Routes() {
			if info.Path == "/"+w.op {
				found = true
				if string(info.Resource) != w.resource || info.Action != w.action || info.Args == nil {
					t.Errorf("%s: %s %v", w.op, info.Resource, info.Action)
				}
			}
		}
		if !found {
			t.Errorf("%s not registered", w.op)
		}
	}
}

func TestOps_NewOpsHappyPaths(t *testing.T) {
	e := newBookEnv(t)
	cfg := e.config(t, "s1", 0, false)
	r := bookingRouter(e)
	if st, body := invoke(r, ab.OpListBookableServices, jsonOf(t, &ab.ListBookableServicesArgs{TenantId: bTenant})); st != 200 || !wfmt.Contains(body, "Dra. Ana") {
		t.Errorf("list_bookable_services: %d %s", st, body)
	}
	if st, body := invoke(r, ab.OpListServiceAvailability, jsonOf(t, &ab.ListServiceAvailabilityArgs{TenantId: bTenant, ServiceId: bService, From: bookDay, To: bookDay})); st != 200 || !wfmt.Contains(body, "start_utc") {
		t.Errorf("list_service_availability: %d %s", st, body)
	}
	args := &ab.CreateOnlineReservationArgs{TenantId: bTenant, Id: "r1", ClientId: "c1", EmployeeServiceConfigId: cfg, SlotStartUtc: bookDay + 9*3600, Notes: "dolor"}
	if st, body := invoke(r, ab.OpCreateOnlineReservation, jsonOf(t, args)); st != 200 || !wfmt.Contains(body, ab.OriginOnline) {
		t.Errorf("create_online_reservation: %d %s", st, body)
	}
}

func TestOps_NewOpsBadInput(t *testing.T) {
	r := bookingRouter(newBookEnv(t))
	for _, op := range []string{ab.OpListBookableServices, ab.OpListServiceAvailability, ab.OpCreateOnlineReservation} {
		if st, _ := invoke(r, op, `{`); st != 400 {
			t.Errorf("%s broken body: %d", op, st)
		}
		if st, _ := invoke(r, op, `{}`); st != 400 {
			t.Errorf("%s missing args: %d", op, st)
		}
	}
}

func TestOps_NewErrorsStatus(t *testing.T) {
	e := newBookEnv(t)
	cfg := e.config(t, "s1", 0, false)
	r := bookingRouter(e)
	e.staff.cardErr = errStorage
	if st, _ := invoke(r, ab.OpListBookableServices, jsonOf(t, &ab.ListBookableServicesArgs{TenantId: bTenant})); st != 500 {
		t.Errorf("list_bookable_services error: %d", st)
	}
	if st, _ := invoke(r, ab.OpListServiceAvailability, jsonOf(t, &ab.ListServiceAvailabilityArgs{TenantId: bTenant, ServiceId: bService, From: bookDay, To: bookDay})); st != 500 {
		t.Errorf("list_service_availability error: %d", st)
	}
	e.staff.cardErr = nil
	e.online(t, "h1", cfg, "c1", 9)
	e.online(t, "h2", cfg, "c1", 10)
	args := &ab.CreateOnlineReservationArgs{TenantId: bTenant, Id: "h3", ClientId: "c1", EmployeeServiceConfigId: cfg, SlotStartUtc: bookDay + 11*3600}
	if st, body := invoke(r, ab.OpCreateOnlineReservation, jsonOf(t, args)); st != 409 || body == "" {
		t.Errorf("too many holds: %d %q", st, body)
	}

	cases := []struct {
		name string
		env  *bookEnv
		code int
	}{
		{"payments unavailable", buildBookEnv(t, false), 500},
	}
	for _, c := range cases {
		cfg := c.env.config(t, "s1", 0, false)
		args := &ab.CreateOnlineReservationArgs{TenantId: bTenant, Id: "x", ClientId: "c1", EmployeeServiceConfigId: cfg, SlotStartUtc: bookDay + 9*3600}
		if st, _ := invoke(bookingRouter(c.env), ab.OpCreateOnlineReservation, jsonOf(t, args)); st != c.code {
			t.Errorf("%s: %d, want %d", c.name, st, c.code)
		}
	}
}
