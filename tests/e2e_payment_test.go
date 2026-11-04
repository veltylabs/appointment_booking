package tests

import (
	"testing"

	"webtyp.com/events/mock"
	"webtyp.com/orm"
	"webtyp.com/storage/mem"
	"webtyp.com/time"

	ab "github.com/veltylabs/appointment_booking"
)

type mockPaymentGateway struct {
	paidAmount int64
	paidCurr   string
	payable    bool
	terminal   bool
	chargeAmt  int64
	chargeCurr string
	termStatus string
	lastMethod string
	lastActor  string
	lastReturn string
}

func (m *mockPaymentGateway) PaidAmount(tenantId, paymentId, referenceModule, referenceId string) (int64, string, error) {
	return m.paidAmount, m.paidCurr, nil
}

func (m *mockPaymentGateway) ChargeDetail(tenantID, referenceModule, referenceID string) (bool, bool, int64, string, error) {
	return m.payable, m.terminal, m.chargeAmt, m.chargeCurr, nil
}

func (m *mockPaymentGateway) RegisterCounterPayment(tenantID, id, referenceModule, referenceID, method, actorID string) (string, error) {
	m.lastMethod = method
	m.lastActor = actorID
	return id, nil
}

func (m *mockPaymentGateway) StartTerminalPayment(tenantID, id, referenceModule, referenceID, method string) (string, error) {
	m.lastMethod = method
	return id, nil
}

func (m *mockPaymentGateway) RefreshTerminalPayment(tenantID, paymentID string) (string, error) {
	return m.termStatus, nil
}

func (m *mockPaymentGateway) StartOnlinePayment(tenantID, id, referenceModule, referenceID, returnTo string) (string, string, string, error) {
	m.lastReturn = returnTo
	return "https://webpay.example/pay?token=tok-1", "token_ws", "tok-1", nil
}

type circuit struct {
	ab  *ab.Module
	gw  *mockPaymentGateway
	cfg string
}

func newCircuit(t *testing.T, paymentRequired bool) *circuit {
	t.Helper()
	db := orm.New(mem.New())
	broker := &mock.Broker{}
	gw := &mockPaymentGateway{
		paidAmount: 25000,
		paidCurr:   "CLP",
		payable:    true,
		chargeAmt:  25000,
		chargeCurr: "CLP",
		termStatus: "paid",
	}

	staff := &fakeStaff{cards: []staffCard{{"s1", "Dra. Ana", "Medicina General", true}}}
	catalog := &fakeCatalog{items: []serviceItem{{bService, "Consulta", "CLP", 25000, true}}}
	zone, _ := time.LoadZone("UTC")

	booking, err := ab.New(db, ab.Deps{
		Staff:     staff,
		Catalog:   catalog,
		Patients:  &MockDirectoryReader{Exists: true},
		IDs:       &fakeIDs{},
		Publisher: broker,
		Payments:  gw,
		Zone:      zone,
		TenantID:  bTenant,
	})
	if err != nil {
		t.Fatal(err)
	}

	cfg, err := booking.CreateEmployeeServiceConfig(ab.EmployeeServiceConfig{
		TenantId:        bTenant,
		StaffId:         "s1",
		ServiceId:       bService,
		DurationMin:     30,
		PaymentRequired: paymentRequired,
		IsActive:        true,
	})
	if err != nil {
		t.Fatal(err)
	}

	ensureCalendarConfig(t, booking, bTenant, "s1")
	seedWeekdayBlock(t, booking, bTenant, "s1", int(time.WeekdayUTC(bookDay)), defaultBlocks())

	return &circuit{ab: booking, gw: gw, cfg: cfg.Id}
}

func TestE2E_OnlineHoldPayConfirm(t *testing.T) {
	c := newCircuit(t, true)
	r, err := c.ab.CreateOnlineReservation(ab.CreateReservationCmd{
		Id:                      "r1",
		TenantId:                bTenant,
		ClientId:                "c1",
		EmployeeServiceConfigId: c.cfg,
		SlotStartUtc:            bookDay + 9*3600,
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := c.ab.ConfirmPaidReservation(bTenant, r.Id, "pay1"); err != nil {
		t.Fatalf("ConfirmPaidReservation: %v", err)
	}

	got, _ := c.ab.GetReservation(bTenant, r.Id)
	if got.Status != ab.StatusConfirmed || got.PaymentId != "pay1" || got.HoldUntil != 0 {
		t.Fatalf("reservation = %+v", got)
	}
}

func TestE2E_CounterPayConfirm(t *testing.T) {
	c := newCircuit(t, true)
	r, err := c.ab.CreateReservation(ab.CreateReservationCmd{
		Id:                      "r1",
		TenantId:                bTenant,
		ClientId:                "c1",
		EmployeeServiceConfigId: c.cfg,
		SlotStartUtc:            bookDay + 9*3600,
		Origin:                  ab.OriginCounter,
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := c.ab.ConfirmPaidReservation(bTenant, r.Id, "pay1"); err != nil {
		t.Fatalf("ConfirmPaidReservation: %v", err)
	}

	if got, _ := c.ab.GetReservation(bTenant, r.Id); got.Status != ab.StatusConfirmed || got.PaymentId != "pay1" {
		t.Fatalf("reservation = %+v", got)
	}
}

func TestE2E_PaidAfterHoldExpiredIsRefunded(t *testing.T) {
	c := newCircuit(t, true)
	r, err := c.ab.CreateOnlineReservation(ab.CreateReservationCmd{
		Id:                      "r1",
		TenantId:                bTenant,
		ClientId:                "c1",
		EmployeeServiceConfigId: c.cfg,
		SlotStartUtc:            bookDay + 9*3600,
	})
	if err != nil {
		t.Fatal(err)
	}

	// La retención vence mientras el paciente está en Webpay.
	if n, err := c.ab.ExpirePendingReservations(bTenant, time.NowUnix()+ab.OnlineHoldSeconds+1); err != nil || n != 1 {
		t.Fatalf("expire: %d, %v", n, err)
	}

	err = c.ab.ConfirmPaidReservation(bTenant, r.Id, "pay1")
	if err != ab.ErrReservationNotPayable {
		t.Fatalf("expected ErrReservationNotPayable, got %v", err)
	}
}
