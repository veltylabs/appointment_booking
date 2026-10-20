package tests

import (
	"testing"

	wfmt "webtyp.com/fmt"
	"webtyp.com/model"
	"webtyp.com/orm"
	"webtyp.com/storage"
	"webtyp.com/storage/mem"
	tinytime "webtyp.com/time"

	ab "github.com/veltylabs/appointment_booking"
)

var errStorage = wfmt.Err("error simulado")

// staffCard es un profesional del doble de staff_manager.
type staffCard struct {
	id, name, specialty string
	active              bool
}

// fakeStaff responde StaffExists/StaffCard desde una lista (como staff_manager:
// un id ausente es (false, nil) en StaffExists y un error en StaffCard).
type fakeStaff struct {
	cards     []staffCard
	existsErr error
	cardErr   error
}

func (f *fakeStaff) find(id string) (staffCard, bool) {
	for _, c := range f.cards {
		if c.id == id {
			return c, true
		}
	}
	return staffCard{}, false
}

func (f *fakeStaff) StaffExists(tenantId, staffId string) (bool, error) {
	if f.existsErr != nil {
		return false, f.existsErr
	}
	_, ok := f.find(staffId)
	return ok, nil
}

func (f *fakeStaff) StaffCard(tenantId, staffId string) (string, string, bool, error) {
	if f.cardErr != nil {
		return "", "", false, f.cardErr
	}
	c, ok := f.find(staffId)
	if !ok {
		return "", "", false, wfmt.Err("staff not found")
	}
	return c.name, c.specialty, c.active, nil
}

// serviceItem es un servicio del doble de service_catalog.
type serviceItem struct {
	id, name, currency string
	price              float64
	active             bool
}

type fakeCatalog struct {
	items     []serviceItem
	existsErr error
	cardErr   error
}

func (f *fakeCatalog) find(id string) (serviceItem, bool) {
	for _, it := range f.items {
		if it.id == id {
			return it, true
		}
	}
	return serviceItem{}, false
}

func (f *fakeCatalog) ServiceExists(tenantId, serviceId string) (bool, error) {
	if f.existsErr != nil {
		return false, f.existsErr
	}
	_, ok := f.find(serviceId)
	return ok, nil
}

func (f *fakeCatalog) ServiceCard(tenantId, serviceId string) (string, float64, string, bool, error) {
	if f.cardErr != nil {
		return "", 0, "", false, f.cardErr
	}
	it, ok := f.find(serviceId)
	if !ok {
		return "", 0, "", false, wfmt.Err("item not found")
	}
	return it.name, it.price, it.currency, it.active, nil
}

// fakePayments es el PaymentReader. onPaid corre antes de responder (para
// simular otro proceso que cambia la reserva al mismo tiempo).
type fakePayments struct {
	amount   int64
	currency string
	err      error
	onPaid   func()
}

func (f *fakePayments) PaidAmount(tenantId, paymentId, referenceModule, referenceId string) (int64, string, error) {
	if fn := f.onPaid; fn != nil {
		f.onPaid = nil
		fn()
	}
	return f.amount, f.currency, f.err
}

// failConn envuelve storage/mem y hace fallar una tabla y acción a pedido,
// después de dejar pasar skip coincidencias.
type failConn struct {
	storage.Conn
	table  string
	action storage.Action
	skip   int
	seen   int
}

func (f *failConn) Compile(q storage.Query, m model.Model) (storage.Plan, error) {
	if f.table != "" && q.Table == f.table && q.Action == f.action {
		f.seen++
		if f.seen > f.skip {
			return storage.Plan{}, errStorage
		}
	}
	return f.Conn.Compile(q, m)
}

func (f *failConn) BeginTx() (storage.TxBoundExecutor, error) {
	return f.Conn.(storage.TxExecutor).BeginTx()
}

func (f *failConn) failOn(table string, action storage.Action, skip int) {
	f.table, f.action, f.skip, f.seen = table, action, skip, 0
}

const (
	bTenant  = "t1"
	bService = "srv1"
	tblCfg   = "employee_service_config"
	tblRes   = "reservation"
)

// bookDay es un lunes futuro; las horas libres van de 09:00 a 17:00 UTC.
var bookDay = Date(2031, 1, 6, 0, 0, 0, 0)

type bookEnv struct {
	m       *ab.Module
	repo    *ab.Repository
	db      *orm.DB
	conn    *failConn
	staff   *fakeStaff
	catalog *fakeCatalog
	pay     *fakePayments
	pub     *MockEventPublisher
	dir     *MockDirectoryReader
	bounds  *errBounds
}

// errBounds es un BoundsReader que no limita, salvo cuando err no es nil.
type errBounds struct{ err error }

func (b *errBounds) GetDayBounds(int64) (tinytime.DayBounds, error) {
	return tinytime.Unbounded(), b.err
}

func (b *errBounds) GetWeekdayBounds(int) (tinytime.DayBounds, error) {
	return tinytime.Unbounded(), nil
}

func newBookEnv(t *testing.T) *bookEnv {
	t.Helper()
	return buildBookEnv(t, true)
}

func buildBookEnv(t *testing.T, withPayments bool) *bookEnv {
	t.Helper()
	e := &bookEnv{
		conn:    &failConn{Conn: mem.New()},
		staff:   &fakeStaff{cards: []staffCard{{"s1", "Dra. Ana", "Medicina General", true}, {"s2", "Dr. Beto", "Medicina General", true}}},
		catalog: &fakeCatalog{items: []serviceItem{{bService, "Consulta", pmCurrencyCLP, 25000, true}}},
		pay:     &fakePayments{amount: 25000, currency: pmCurrencyCLP},
		pub:     &MockEventPublisher{},
		dir:     &MockDirectoryReader{Exists: true},
		bounds:  &errBounds{},
	}
	e.db = orm.New(e.conn)
	deps := ab.Deps{Staff: e.staff, Catalog: e.catalog, Directory: e.dir, IDs: &fakeIDs{}, Publisher: e.pub, Bounds: e.bounds}
	if withPayments {
		deps.Payments = e.pay
	}
	var err error
	if e.repo, err = ab.NewRepository(e.db, deps.IDs); err != nil {
		t.Fatal(err)
	}
	if e.m, err = ab.New(e.db, deps); err != nil {
		t.Fatal(err)
	}
	return e
}

const pmCurrencyCLP = "CLP"

// config crea la configuración staff × bService con su agenda 09–17 del bookDay.
func (e *bookEnv) config(t *testing.T, staff string, override float64, paymentRequired bool) string {
	t.Helper()
	cfg, err := e.m.CreateEmployeeServiceConfig(ab.EmployeeServiceConfig{TenantId: bTenant, StaffId: staff, ServiceId: bService, DurationMin: 30, PriceOverride: override, PaymentRequired: paymentRequired, IsActive: true})
	if err != nil {
		t.Fatalf("CreateEmployeeServiceConfig: %v", err)
	}
	ensureCalendarConfig(t, e.m, bTenant, staff)
	seedWeekdayBlock(t, e.m, bTenant, staff, int(tinytime.Weekday(bookDay)), defaultBlocks())
	return cfg.Id
}

func (e *bookEnv) reserve(t *testing.T, id, cfgID, client string, hour int64, origin string) ab.Reservation {
	t.Helper()
	r, err := e.m.CreateReservation(ab.CreateReservationCmd{Id: id, TenantId: bTenant, ClientId: client, EmployeeServiceConfigId: cfgID, SlotStartUtc: bookDay + hour*3600, Origin: origin})
	if err != nil {
		t.Fatalf("CreateReservation: %v", err)
	}
	return r
}

func (e *bookEnv) online(t *testing.T, id, cfgID, client string, hour int64) ab.Reservation {
	t.Helper()
	r, err := e.m.CreateOnlineReservation(ab.CreateReservationCmd{Id: id, TenantId: bTenant, ClientId: client, EmployeeServiceConfigId: cfgID, SlotStartUtc: bookDay + hour*3600})
	if err != nil {
		t.Fatalf("CreateOnlineReservation: %v", err)
	}
	return r
}

func (e *bookEnv) get(t *testing.T, id string) ab.Reservation {
	t.Helper()
	r, err := e.m.GetReservation(bTenant, id)
	if err != nil {
		t.Fatalf("GetReservation(%s): %v", id, err)
	}
	return r
}

// patch cambia una reserva guardada (preparación de la prueba).
func (e *bookEnv) patch(t *testing.T, id string, change func(*ab.Reservation)) {
	t.Helper()
	r := e.get(t, id)
	change(&r)
	if err := e.db.Update(&r, orm.Eq(ab.Reservation_.Id, id), orm.Eq(ab.Reservation_.TenantId, bTenant)); err != nil {
		t.Fatalf("patch: %v", err)
	}
}

func bookNow() int64 { return tinytime.Now() / 1_000_000_000 }

func wantErrIs(t *testing.T, got, want error) {
	t.Helper()
	if got != want {
		t.Fatalf("err = %v, want %v", got, want)
	}
}

func cfgFor(staff, service string) ab.EmployeeServiceConfig {
	return ab.EmployeeServiceConfig{TenantId: bTenant, StaffId: staff, ServiceId: service, DurationMin: 30, IsActive: true}
}
