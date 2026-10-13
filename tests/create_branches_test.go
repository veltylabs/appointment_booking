package tests

import (
	"testing"

	"webtyp.com/storage"

	ab "github.com/veltylabs/appointment_booking"
)

// creator es CreateReservation o CreateOverbooking: comparten las mismas
// validaciones de entrada, que se prueban una vez para cada una.
type creator struct {
	name string
	call func(e *bookEnv, cmd ab.CreateReservationCmd) (ab.Reservation, error)
	// prepare deja todo listo para que la función llegue a escribir (para el
	// sobrecupo: tope 1 y la hora ya tomada).
	prepare func(t *testing.T, e *bookEnv, cfg string)
}

var creators = []creator{
	{
		name:    "CreateReservation",
		call:    func(e *bookEnv, c ab.CreateReservationCmd) (ab.Reservation, error) { return e.m.CreateReservation(c) },
		prepare: func(*testing.T, *bookEnv, string) {},
	},
	{
		name: "CreateOverbooking",
		call: func(e *bookEnv, c ab.CreateReservationCmd) (ab.Reservation, error) { return e.m.CreateOverbooking(c) },
		prepare: func(t *testing.T, e *bookEnv, cfg string) {
			if err := e.m.UpsertCalendarConfig(ab.WorkCalendarConfig{TenantId: bTenant, StaffId: "s1", Timezone: "UTC", IsActive: true, MaxOverbookPerDay: 1}); err != nil {
				t.Fatal(err)
			}
			e.reserve(t, "taken", cfg, "c0", 9, ab.OriginCounter)
		},
	},
}

func cmd9(id, cfg string) ab.CreateReservationCmd {
	return ab.CreateReservationCmd{Id: id, TenantId: bTenant, ClientId: "c1", EmployeeServiceConfigId: cfg, SlotStartUtc: bookDay + 9*3600, Origin: ab.OriginCounter}
}

func TestCreate_ValidationBranches(t *testing.T) {
	for _, c := range creators {
		t.Run(c.name, func(t *testing.T) {
			type tc struct {
				name  string
				setup func(e *bookEnv, cmd *ab.CreateReservationCmd)
				want  error // nil = cualquier error no nulo
			}
			cases := []tc{
				{"invalid origin", func(e *bookEnv, cmd *ab.CreateReservationCmd) { cmd.Origin = "PHONE" }, ab.ErrMissingArgs},
				{"id lookup storage error", func(e *bookEnv, cmd *ab.CreateReservationCmd) { e.conn.failOn(tblRes, storage.ActionReadOne, 0) }, errStorage},
				{"config storage error", func(e *bookEnv, cmd *ab.CreateReservationCmd) { e.conn.failOn(tblCfg, storage.ActionReadOne, 0) }, errStorage},
				{"config of another tenant", func(e *bookEnv, cmd *ab.CreateReservationCmd) { cmd.TenantId = "t2" }, ab.ErrNotFound},
				{"directory error", func(e *bookEnv, cmd *ab.CreateReservationCmd) { e.dir.Err = errStorage }, errStorage},
				{"client missing", func(e *bookEnv, cmd *ab.CreateReservationCmd) { e.dir.Exists = false }, ab.ErrClientNotFound},
				{"staff error", func(e *bookEnv, cmd *ab.CreateReservationCmd) { e.staff.existsErr = errStorage }, errStorage},
				{"staff missing", func(e *bookEnv, cmd *ab.CreateReservationCmd) { e.staff.cards = nil }, nil},
				{"service error", func(e *bookEnv, cmd *ab.CreateReservationCmd) { e.catalog.existsErr = errStorage }, errStorage},
				{"service missing", func(e *bookEnv, cmd *ab.CreateReservationCmd) { e.catalog.items = nil }, nil},
				{"calendar storage error", func(e *bookEnv, cmd *ab.CreateReservationCmd) {
					e.conn.failOn(ab.WorkCalendarConfigModel.Name, storage.ActionReadOne, 0)
				}, errStorage},
				{"insert error", func(e *bookEnv, cmd *ab.CreateReservationCmd) { e.conn.failOn(tblRes, storage.ActionCreate, 0) }, errStorage},
				{"reschedule of an unknown reservation", func(e *bookEnv, cmd *ab.CreateReservationCmd) { cmd.RescheduledFromId = "nope" }, ab.ErrNotFound},
			}
			for _, k := range cases {
				e := newBookEnv(t)
				cfg := e.config(t, "s1", 0, false)
				c.prepare(t, e, cfg)
				cmd := cmd9("new", cfg)
				k.setup(e, &cmd)
				_, err := c.call(e, cmd)
				if err == nil || (k.want != nil && err != k.want) {
					t.Errorf("%s: err = %v, want %v", k.name, err, k.want)
				}
			}
		})
	}
}

func TestCreate_IdTakenByAnotherClient(t *testing.T) {
	for _, c := range creators {
		e := newBookEnv(t)
		cfg := e.config(t, "s1", 0, false)
		c.prepare(t, e, cfg)
		e.reserve(t, "dup", cfg, "c9", 10, ab.OriginCounter)
		_, err := c.call(e, cmd9("dup", cfg))
		if err != ab.ErrIdTaken {
			t.Errorf("%s: err = %v", c.name, err)
		}
	}
}

func TestCreate_RescheduleMovesTheOriginal(t *testing.T) {
	for _, c := range creators {
		e := newBookEnv(t)
		cfg := e.config(t, "s1", 0, false)
		c.prepare(t, e, cfg)
		orig := e.reserve(t, "orig", cfg, "c1", 11, ab.OriginCounter)
		cmd := cmd9("moved", cfg)
		cmd.RescheduledFromId = orig.Id
		if _, err := c.call(e, cmd); err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if got := e.get(t, "orig"); got.Status != ab.StatusRescheduled {
			t.Errorf("%s: original status %s", c.name, got.Status)
		}
		last := e.pub.PublishedEvents[len(e.pub.PublishedEvents)-1]
		if last != ab.EventReservationRescheduled {
			t.Errorf("%s: last event %s", c.name, last)
		}
	}
}

func TestCreate_RescheduleOfTerminalReservationRejected(t *testing.T) {
	for _, c := range creators {
		e := newBookEnv(t)
		cfg := e.config(t, "s1", 0, false)
		c.prepare(t, e, cfg)
		orig := e.reserve(t, "orig", cfg, "c1", 11, ab.OriginCounter)
		if err := e.m.ChangeReservationStatus(ab.ChangeStatusCmd{TenantId: bTenant, Id: orig.Id, Event: ab.EventCancel, Revision: int(orig.Revision)}); err != nil {
			t.Fatal(err)
		}
		cmd := cmd9("moved", cfg)
		cmd.RescheduledFromId = orig.Id
		if _, err := c.call(e, cmd); err != ab.ErrInvalidTransition {
			t.Errorf("%s: err = %v", c.name, err)
		}
	}
}

func TestCreate_RescheduleUpdateError(t *testing.T) {
	for _, c := range creators {
		e := newBookEnv(t)
		cfg := e.config(t, "s1", 0, false)
		c.prepare(t, e, cfg)
		orig := e.reserve(t, "orig", cfg, "c1", 11, ab.OriginCounter)
		e.conn.failOn(tblRes, storage.ActionUpdate, 0)
		cmd := cmd9("moved", cfg)
		cmd.RescheduledFromId = orig.Id
		if _, err := c.call(e, cmd); err != errStorage {
			t.Errorf("%s: err = %v", c.name, err)
		}
	}
}

func TestCreateReservation_AvailabilityErrorPropagates(t *testing.T) {
	e := newBookEnv(t)
	cfg := e.config(t, "s1", 0, false)
	e.bounds.err = errStorage
	if _, err := e.m.CreateReservation(cmd9("new", cfg)); err != errStorage {
		t.Fatalf("err = %v", err)
	}
}

func TestCreateOverbooking_ScheduleBranches(t *testing.T) {
	cases := []struct {
		name  string
		setup func(t *testing.T, e *bookEnv)
		want  error
	}{
		{"inactive calendar", func(t *testing.T, e *bookEnv) {
			if err := e.m.UpsertCalendarConfig(ab.WorkCalendarConfig{TenantId: bTenant, StaffId: "s1", Timezone: "UTC", IsActive: false, MaxOverbookPerDay: 1}); err != nil {
				t.Fatal(err)
			}
		}, ab.ErrSlotNotInSchedule},
		{"blocks storage error", func(t *testing.T, e *bookEnv) { e.conn.failOn("work_calendar_block", storage.ActionReadAll, 0) }, errStorage},
		{"exceptions storage error", func(t *testing.T, e *bookEnv) { e.conn.failOn("work_calendar_exception", storage.ActionReadAll, 0) }, errStorage},
		{"bounds error", func(t *testing.T, e *bookEnv) { e.bounds.err = errStorage }, errStorage},
		{"day reservations storage error", func(t *testing.T, e *bookEnv) { e.conn.failOn(tblRes, storage.ActionReadAll, 0) }, errStorage},
	}
	for _, k := range cases {
		e := newBookEnv(t)
		cfg := e.config(t, "s1", 0, false)
		creators[1].prepare(t, e, cfg)
		k.setup(t, e)
		if _, err := e.m.CreateOverbooking(cmd9("ob", cfg)); err != k.want {
			t.Errorf("%s: err = %v, want %v", k.name, err, k.want)
		}
	}
}
