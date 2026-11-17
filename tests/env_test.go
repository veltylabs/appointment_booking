package tests

import (
	"testing"

	ab "github.com/veltylabs/appointment_booking"
	"webtyp.com/crud"
	"webtyp.com/model"
	"webtyp.com/orm"
	"webtyp.com/router"
	"webtyp.com/storage/mem"
	"webtyp.com/time"
)

// fakePayments is the payments port: it records what the module asks and can be told to fail.
type fakePayments struct {
	started int
	fail    error
}

func (p *fakePayments) PaidAmount(string, string, string, string) (int64, string, error) {
	return 100, "CLP", nil
}
func (p *fakePayments) ChargeDetail(string, string, string) (bool, bool, int64, string, error) {
	return true, false, 100, "CLP", nil
}
func (p *fakePayments) RegisterCounterPayment(_, id, _, _, _, _ string) (string, error) {
	return id, nil
}
func (p *fakePayments) StartTerminalPayment(_, id, _, _, _ string) (string, error) { return id, nil }
func (p *fakePayments) RefreshTerminalPayment(string, string) (string, error)      { return "paid", nil }
func (p *fakePayments) StartOnlinePayment(string, string, string, string, string) (string, string, string, error) {
	p.started++
	if p.fail != nil {
		return "", "", "", p.fail
	}
	return "https://pay.example/go", "", "", nil
}

// countingPatients wraps the directory mock and counts what the online call asks of it.
type countingPatients struct {
	MockDirectoryReader
	found, created int
}

func (p *countingPatients) FindPatient(tenantID, doc string) (string, string, string, string, bool, error) {
	p.found++
	return "", "", "", "", false, nil
}

func (p *countingPatients) CreatePatient(tenantID, id, doc, name, phone, email string) (string, error) {
	p.created++
	return "pat-" + id, nil
}

// env is one module over memory, reached through the loopback exactly as a screen reaches it.
type env struct {
	t        *testing.T
	mod      *ab.Module
	queue    *callQueue
	events   *MockEventPublisher
	patients *countingPatients
	payments *fakePayments
	caller   router.Caller
	monday   time.Unix // a Monday inside the conflict horizon, midnight UTC
}

type envOption func(*ab.Deps, *env)

// withoutPayments leaves the payments port unset.
func withoutPayments() envOption { return func(d *ab.Deps, e *env) { d.Payments, e.payments = nil, nil } }

func newEnv(t *testing.T, opts ...envOption) *env {
	t.Helper()
	e := &env{t: t, queue: &callQueue{}, events: &MockEventPublisher{}, patients: &countingPatients{}, payments: &fakePayments{}}
	e.patients.Exists = true
	deps := ab.Deps{
		Staff:     &MockStaffReader{Exists: true, Name: "Dra. Soto", Specialty: "gen", Active: true},
		Catalog:   &MockCatalogReader{Exists: true, Name: "Consulta", Price: 100, Currency: "CLP", Active: true},
		Patients:  e.patients,
		Payments:  e.payments,
		IDs:       &fakeIDs{},
		Publisher: e.events,
		Zone:      zoneUTC,
		TenantID:  "t1",
	}
	for _, o := range opts {
		o(&deps, e)
	}
	mod, err := ab.New(orm.New(mem.New()), deps)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	e.mod = mod
	e.caller = crud.LoopbackActingAsWithQueue(nil, "u1", e.queue, mod)
	e.monday = time.MidnightUTC(time.NowUnix()) + 7*time.Day
	for time.WeekdayUTC(e.monday) != 1 {
		e.monday += time.Day
	}
	return e
}

func op(entity string, action model.Action) router.Operation {
	return router.Operation{Module: ab.ModelName, Entity: entity, Action: action}
}

func (e *env) do(o router.Operation, args model.Encodable, out model.Decodable) error {
	var err error
	e.caller.Call(o, args, out, func(got error) { err = got })
	return err
}

func (e *env) create(rec model.Model, out model.Decodable) error {
	return e.do(op(rec.ModelName(), model.Create), &crud.CreateArgs{Records: []model.Model{rec}}, out)
}

func (e *env) must(err error) {
	e.t.Helper()
	if err != nil {
		e.t.Fatal(err)
	}
}

// schedule gives staff "s1" one service (30 min, 100 CLP), a calendar that allows `overbook` extra bookings a day and
// a Monday block 09:00–17:00. It returns the service configuration id.
func (e *env) schedule(overbook int64) string {
	e.t.Helper()
	cfg := &ab.EmployeeServiceConfig{Id: "cfg1", StaffId: "s1", ServiceId: "v1", DurationMin: 30, IsActive: true}
	e.must(e.create(cfg, &ab.EmployeeServiceConfig{}))
	e.must(e.create(&ab.WorkCalendarConfig{Id: "wc1", StaffId: "s1", IsActive: true, MaxOverbookPerDay: overbook}, &ab.WorkCalendarConfig{}))
	e.must(e.create(&ab.WorkCalendarBlock{Id: "b1", StaffId: "s1", DayOfWeek: 1, StartMin: 540, EndMin: 1020}, &ab.WorkCalendarBlock{}))
	return cfg.Id
}

// at is the start of the slot `hhmm` minutes after midnight on the test Monday.
func (e *env) at(minutes int) time.Unix { return e.monday + time.Unix(minutes*60) }

func (e *env) book(id string, minutes int) (ab.Reservation, error) {
	var out ab.Reservation
	err := e.create(&ab.Reservation{Id: id, ClientId: "c-" + id, EmployeeServiceConfigId: "cfg1", ReservationTime: int64(e.at(minutes))}, &out)
	return out, err
}

// reservation reads one reservation row by id.
func (e *env) reservation(id string) ab.Reservation {
	e.t.Helper()
	var list ab.ReservationList
	e.must(e.do(op("reservation", model.Read), &crud.ReadArgs{ID: id}, &list))
	if len(list) != 1 {
		e.t.Fatalf("reservation %s: %d rows", id, len(list))
	}
	return *list[0]
}

func (e *env) reservations() ab.ReservationList {
	e.t.Helper()
	var list ab.ReservationList
	e.must(e.do(op("reservation", model.Read), &crud.ReadArgs{Limit: crud.MaxPage}, &list))
	return list
}

