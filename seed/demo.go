//go:build wasm

package seed

import (
	"webtyp.com/crud"
	"webtyp.com/ddl"
	. "webtyp.com/dom"
	"webtyp.com/events/inprocess"
	"webtyp.com/fmt"
	. "webtyp.com/html"
	"webtyp.com/orm"
	"webtyp.com/storage/mem"
	"webtyp.com/time"
	"webtyp.com/unixid"
	"webtyp.com/view"

	ab "github.com/veltylabs/appointment_booking"
	"github.com/veltylabs/appointment_booking/migrate"
	"github.com/veltylabs/appointment_booking/ui"
)

const demoTenantID = "demo"

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

// demoPatientStep reemplaza la pantalla de patient_directory: elige siempre al mismo paciente de la demo.
type demoPatientStep struct{}

func (demoPatientStep) Render(choose func(id, name string)) *Element {
	return Div().Child(
		P().Text("Juan Pérez · 12.345.678-5"),
		Button().Attr("type", "button").Text("Elegir paciente").OnClick(func(Event) { choose("pat-1", "Juan Pérez") }),
	)
}

func (demoPatientStep) Reset() {}

// demoPaymentStep reemplaza la pantalla de payment_manager: registra un pago de demostración.
type demoPaymentStep struct{}

func (demoPaymentStep) Render(reservationID string, paid func(paymentID string)) *Element {
	return Div().Child(
		P().Text("Cobro de la reserva "+reservationID),
		Button().Attr("type", "button").Text("Registrar pago").OnClick(func(Event) { paid("pay-demo") }),
	)
}

// Demo crea una demostración ejecutable del módulo appointment_booking sobre una base en memoria con datos semilla.
// Los pasos de paciente y cobro son dobles: este módulo no importa a los módulos que los implementan.
//
// Ejemplo: demo, err := seed.Demo()
func Demo() (view.Demo, error) {
	ids, err := unixid.NewUnixID()
	if err != nil {
		return view.Demo{}, err
	}
	zone, err := time.LoadZone("America/Santiago")
	if err != nil {
		return view.Demo{}, err
	}
	mod, err := ab.New(orm.New(mem.New()), ab.Deps{
		Publisher: &inprocess.Broker{},
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
		return view.Demo{}, err
	}
	if _, err := Load(mod, demoTenantID, LoadOptions{
		Staff:      []StaffSeed{{ID: "staff-1", Name: "Dra. Soto", Specialty: "Pediatría"}},
		Services:   []ServiceSeed{{ID: "svc-1", Specialty: "Pediatría"}},
		PatientIDs: []string{"pat-1", "pat-2"},
	}); err != nil {
		return view.Demo{}, err
	}
	return view.Demo{
		Label:      ui.NavLabel,
		Timezone:   "America/Santiago",
		ActingAs:   "demo-user-ana",
		Modules:    []crud.Module{mod},
		Migrations: []ddl.Step{{Name: ab.Definition.Name, Run: migrate.Migrate}},
		Screens: func(d view.BrowserDeps) []view.Screen {
			return ui.Screens(d, ui.Steps{Patient: demoPatientStep{}, Payment: demoPaymentStep{}})
		},
	}, nil
}
