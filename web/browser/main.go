package main

import (
	. "webtyp.com/dom"
	"webtyp.com/events/mock"
	"webtyp.com/fmt"
	"webtyp.com/layout/platformd"
	"webtyp.com/orm"
	"webtyp.com/router/mock"
	"webtyp.com/storage/mem"
	"webtyp.com/time"
	"webtyp.com/unixid"

	ab "github.com/veltylabs/appointment_booking"
	"github.com/veltylabs/appointment_booking/seed"
	"github.com/veltylabs/appointment_booking/ui"
)

const demoTenantID = "demo"

type demoUser struct{}

func (demoUser) UserName() string    { return "Demo" }
func (demoUser) UserAvatar() string  { return "" }
func (demoUser) UserRoles() []string { return []string{"Administrador"} }

type fakeStaffReader struct{}

func (fakeStaffReader) StaffExists(tenantId, staffId string) (bool, error) { return true, nil }
func (fakeStaffReader) StaffCard(tenantId, staffId string) (string, string, bool, error) {
	return "Dra. Soto", "Pediatría", true, nil
}
func (fakeStaffReader) OccupantOptions(tenantID string) ([]fmt.KeyValue, error) {
	return []fmt.KeyValue{{Key: "staff-1", Value: "Dra. Soto"}}, nil
}

type fakeCatalogReader struct{}

func (fakeCatalogReader) ServiceExists(tenantId, serviceId string) (bool, error) { return true, nil }
func (fakeCatalogReader) ServiceCard(tenantId, serviceId string) (string, float64, string, bool, error) {
	return "Consulta General", 35000, "CLP", true, nil
}

type fakePatientDirectory struct{}

func (fakePatientDirectory) GetPatient(tenantID, id string) (string, string, string, string, bool, error) {
	return "", "", "", "", true, nil
}

func (fakePatientDirectory) FindPatient(tenantID, documentNumber string) (string, string, string, string, bool, error) {
	if documentNumber == "12.345.678-5" || documentNumber == "12345678-5" {
		return "pat-1", "Juan Pérez", "+56911223344", "juan@example.com", true, nil
	}
	return "", "", "", "", false, nil
}

func (fakePatientDirectory) CreatePatient(tenantID, id, documentNumber, name, phone, email string) (string, error) {
	return id, nil
}

type fakeOpeningHours struct{}

func (fakeOpeningHours) DayLimits(date time.Unix) (time.DayLimits, error) {
	return time.DayLimits{Open: true, OpenMin: 480, CloseMin: 1200}, nil
}
func (fakeOpeningHours) WeekdayLimits(dayOfWeek int) (time.DayLimits, error) {
	return time.DayLimits{Open: true, OpenMin: 480, CloseMin: 1200}, nil
}
func (fakeOpeningHours) ClosedDates(from, to time.Unix) ([]string, []string, error) {
	return nil, nil, nil
}

type fakePaymentGateway struct{}

func (fakePaymentGateway) PaidAmount(tenantId, paymentId, referenceModule, referenceId string) (int64, string, error) {
	return 35000, "CLP", nil
}
func (fakePaymentGateway) ChargeDetail(tenantID, referenceModule, referenceID string) (bool, bool, int64, string, error) {
	return true, false, 35000, "CLP", nil
}
func (fakePaymentGateway) RegisterCounterPayment(tenantID, id, referenceModule, referenceID, method, actorID string) (string, error) {
	return id, nil
}
func (fakePaymentGateway) StartTerminalPayment(tenantID, id, referenceModule, referenceID, method string) (string, error) {
	return id, nil
}
func (fakePaymentGateway) RefreshTerminalPayment(tenantID, paymentID string) (string, error) {
	return "paid", nil
}
func (fakePaymentGateway) StartOnlinePayment(tenantID, id, referenceModule, referenceID, returnTo string) (string, string, string, error) {
	return "/demo-success", "", "", nil
}

func main() {
	ids, err := unixid.NewUnixID()
	if err != nil {
		panic(err)
	}
	db := orm.New(mem.New())
	broker := &mock.Broker{}

	zone, _ := time.LoadZone("America/Santiago")

	abMod, err := ab.New(db, ab.Deps{
		Publisher: broker,
		IDs:       ids,
		TenantID:  demoTenantID,
		Zone:      zone,
		Staff:     fakeStaffReader{},
		Catalog:   fakeCatalogReader{},
		Patients:  fakePatientDirectory{},
		Hours:     fakeOpeningHours{},
		Payments:  fakePaymentGateway{},
	})
	if err != nil {
		panic(err)
	}

	_, _ = seed.Load(abMod, demoTenantID, seed.LoadOptions{
		Staff: []seed.StaffSeed{
			{ID: "staff-1", Name: "Dra. Soto", Specialty: "Pediatría"},
		},
		Services: []seed.ServiceSeed{
			{ID: "svc-1", Specialty: "Pediatría"},
		},
		PatientIDs: []string{"pat-1", "pat-2"},
	})

	caller := crud.LoopbackActingAsWithQueue(demoTenantID, abMod)
	screens := ui.Screens(caller, zone, ids, demoTenantID)

	p := &platformd.Platform{
		AppName:   "Appointment Booking — demo",
		User:      demoUser{},
		DefaultID: ui.ID,
	}
	_ = screens
	Append("body", p)
	select {}
}
