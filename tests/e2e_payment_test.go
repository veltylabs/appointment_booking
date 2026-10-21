package tests

import (
	"testing"

	"webtyp.com/events"
	"webtyp.com/events/mock"
	"webtyp.com/orm"
	"webtyp.com/storage/mem"
	tinytime "webtyp.com/time"

	ab "github.com/veltylabs/appointment_booking"
	pm "github.com/veltylabs/payment_manager"
	"github.com/veltylabs/payment_manager/sandbox"
)

// chargeViaBooking resuelve el ciclo de construcción: payment_manager pregunta
// el monto a appointment_booking, que se construye después con pm como PaymentReader.
type chargeViaBooking struct{ booking **ab.Module }

func (c chargeViaBooking) Charge(tenantId, referenceId string) (int64, string, bool, error) {
	return (*c.booking).Charge(tenantId, referenceId)
}

type circuit struct {
	ab      *ab.Module
	pm      *pm.Module
	gw      *sandbox.Gateway
	cfg     string
	refunds int
}

// newCircuit cablea los dos módulos como lo hará la raíz de composición: el
// evento payment.paid confirma la reserva; si ya no se puede, se devuelve el pago.
func newCircuit(t *testing.T, paymentRequired bool) *circuit {
	t.Helper()
	db := orm.New(mem.New())
	broker := &mock.Broker{}
	c := &circuit{gw: sandbox.New("https://webpay.example/pay")}
	var booking *ab.Module
	var err error
	c.pm, err = pm.New(db, pm.Deps{Publisher: broker, Gateway: c.gw, ReturnURL: "https://clinic.example/return",
		Charges: []pm.ChargeSource{{Module: ab.ModelName, Reader: chargeViaBooking{&booking}}}})
	if err != nil {
		t.Fatal(err)
	}
	staff := &fakeStaff{cards: []staffCard{{"s1", "Dra. Ana", "Medicina General", true}}}
	catalog := &fakeCatalog{items: []serviceItem{{bService, "Consulta", "CLP", 25000, true}}}
	booking, err = ab.New(db, ab.Deps{Staff: staff, Catalog: catalog, Directory: &MockDirectoryReader{Exists: true}, IDs: &fakeIDs{}, Publisher: broker, Payments: c.pm})
	if err != nil {
		t.Fatal(err)
	}
	c.ab = booking
	broker.Subscribe(pm.EventPaymentPaid, func(ev events.Event) {
		p := ev.Payload.(*pm.Payment)
		if p.ReferenceModule != ab.ModelName {
			return
		}
		switch err := c.ab.ConfirmPaidReservation(p.TenantId, p.ReferenceId, p.Id); err {
		case nil:
		case ab.ErrReservationNotPayable:
			if _, err := c.pm.RefundPayment(pm.RefundPaymentArgs{TenantId: p.TenantId, Id: "refund-" + p.Id, PaymentId: p.Id, Amount: p.Amount, Reason: "reservation expired before payment", Method: pm.MethodCash}); err != nil {
				t.Errorf("refund: %v", err)
			}
			c.refunds++
		default:
			t.Errorf("ConfirmPaidReservation: %v", err)
		}
	})
	cfg, err := c.ab.CreateEmployeeServiceConfig(ab.EmployeeServiceConfig{TenantId: bTenant, StaffId: "s1", ServiceId: bService, DurationMin: 30, PaymentRequired: paymentRequired, IsActive: true})
	if err != nil {
		t.Fatal(err)
	}
	c.cfg = cfg.Id
	ensureCalendarConfig(t, c.ab, bTenant, "s1")
	seedWeekdayBlock(t, c.ab, bTenant, "s1", int(tinytime.Weekday(bookDay)), defaultBlocks())
	return c
}

func (c *circuit) payOnline(t *testing.T, reservationID, paymentID string) {
	t.Helper()
	started, err := c.pm.StartOnlinePayment(pm.StartOnlinePaymentArgs{TenantId: bTenant, Id: paymentID, ReferenceModule: ab.ModelName, ReferenceId: reservationID})
	if err != nil {
		t.Fatalf("StartOnlinePayment: %v", err)
	}
	c.gw.Approve(started.Token)
	if _, err := c.pm.CompleteOnlinePayment(pm.OnlineReturn{Token: started.Token}); err != nil {
		t.Fatalf("CompleteOnlinePayment: %v", err)
	}
}

func TestE2E_OnlineHoldPayConfirm(t *testing.T) {
	c := newCircuit(t, true)
	r, err := c.ab.CreateOnlineReservation(ab.CreateReservationCmd{Id: "r1", TenantId: bTenant, ClientId: "c1", EmployeeServiceConfigId: c.cfg, SlotStartUtc: bookDay + 9*3600})
	if err != nil {
		t.Fatal(err)
	}
	c.payOnline(t, r.Id, "pay1")
	got, _ := c.ab.GetReservation(bTenant, r.Id)
	if got.Status != ab.StatusConfirmed || got.PaymentId != "pay1" || got.HoldUntil != 0 {
		t.Fatalf("reservation = %+v", got)
	}
}

func TestE2E_CounterPayConfirm(t *testing.T) {
	c := newCircuit(t, true)
	r, err := c.ab.CreateReservation(ab.CreateReservationCmd{Id: "r1", TenantId: bTenant, ClientId: "c1", EmployeeServiceConfigId: c.cfg, SlotStartUtc: bookDay + 9*3600, Origin: ab.OriginCounter})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.pm.CreateCounterPayment(pm.CreateCounterPaymentArgs{TenantId: bTenant, Id: "pay1", ReferenceModule: ab.ModelName, ReferenceId: r.Id, Method: pm.MethodDebit}); err != nil {
		t.Fatal(err)
	}
	if got, _ := c.ab.GetReservation(bTenant, r.Id); got.Status != ab.StatusConfirmed || got.PaymentId != "pay1" {
		t.Fatalf("reservation = %+v", got)
	}
}

func TestE2E_PaidAfterHoldExpiredIsRefunded(t *testing.T) {
	c := newCircuit(t, true)
	r, err := c.ab.CreateOnlineReservation(ab.CreateReservationCmd{Id: "r1", TenantId: bTenant, ClientId: "c1", EmployeeServiceConfigId: c.cfg, SlotStartUtc: bookDay + 9*3600})
	if err != nil {
		t.Fatal(err)
	}
	started, err := c.pm.StartOnlinePayment(pm.StartOnlinePaymentArgs{TenantId: bTenant, Id: "pay1", ReferenceModule: ab.ModelName, ReferenceId: r.Id})
	if err != nil {
		t.Fatal(err)
	}
	// La retención vence mientras el paciente está en Webpay.
	if n, err := c.ab.ExpirePendingReservations(bTenant, tinytime.NowUnix()+ab.OnlineHoldSeconds+1); err != nil || n != 1 {
		t.Fatalf("expire: %d, %v", n, err)
	}
	c.gw.Approve(started.Token)
	if _, err := c.pm.CompleteOnlinePayment(pm.OnlineReturn{Token: started.Token}); err != nil {
		t.Fatal(err)
	}
	if c.refunds != 1 {
		t.Fatalf("refunds = %d, want 1", c.refunds)
	}
	if p, _ := c.pm.GetPayment(bTenant, "pay1"); p.Status != pm.StatusRefunded {
		t.Fatalf("payment status = %s, want REFUNDED", p.Status)
	}
}

// Los contratos se cumplen sin que el paquete raíz importe al otro.
var (
	_ pm.ChargeReader  = (*ab.Module)(nil)
	_ ab.PaymentReader = (*pm.Module)(nil)
)

func TestContract_ModuleSatisfiesChargeReader(t *testing.T) {
	var reader pm.ChargeReader = (*ab.Module)(nil)
	if reader == nil {
		t.Fatal("unreachable: the assignment compiles")
	}
}

func TestContract_PaymentManagerSatisfiesPaymentReader(t *testing.T) {
	var reader ab.PaymentReader = (*pm.Module)(nil)
	if reader == nil {
		t.Fatal("unreachable: the assignment compiles")
	}
}
