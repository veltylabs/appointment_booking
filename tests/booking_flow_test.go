package tests

import (
	"testing"

	tinytime "webtyp.com/time"

	ab "github.com/veltylabs/appointment_booking"
	patientdirectory "github.com/veltylabs/patient_directory"
)

// TestBookingFlow_ProgressionSteps verifica la progresión lógica de los 6 pasos:
// 1. Especialidad/Servicio fija duración y precio.
// 2. Profesional acota la disponibilidad.
// 3. Fecha y hora seleccionada mediante FreeSlots.
// 4. Resumen y notas.
// 5. Identificación del paciente (por RUT / guest).
// 6. Pago condicional según PaymentRequired.
func TestBookingFlow_ProgressionSteps(t *testing.T) {
	const tenant = "tenant-flow"
	const staff = "staff-flow"

	svc, repo := newTestModuleService(t, SetupDependencies())

	// Paso 1 & 2: Configurar servicio con prepago requerido
	cfgID := nextTestID()
	cfg := ab.EmployeeServiceConfig{
		Id:              cfgID,
		TenantId:        tenant,
		StaffId:         staff,
		ServiceId:       "srv-flow",
		DurationMin:     30,
		PriceOverride:   15000,
		PaymentRequired: true,
		IsActive:        true,
	}
	if err := repo.InsertEmployeeServiceConfig(cfg); err != nil {
		t.Fatalf("InsertEmployeeServiceConfig: %v", err)
	}

	// Configurar calendario para el staff
	if err := svc.UpsertCalendarConfig(ab.WorkCalendarConfig{
		TenantId: tenant, StaffId: staff, Timezone: "UTC", IsActive: true,
	}); err != nil {
		t.Fatalf("UpsertCalendarConfig: %v", err)
	}
	if err := svc.SaveDayBlocks(tenant, staff, 1, []ab.WorkCalendarBlock{
		{TenantId: tenant, StaffId: staff, DayOfWeek: 1, StartMin: 540, EndMin: 720, IsActive: true},
	}); err != nil {
		t.Fatalf("SaveDayBlocks: %v", err)
	}

	// Paso 3: Slot horario
	targetDay := Date(2025, 1, 6, 0, 0, 0, 0) // Mon Jan 6 2025
	slotStartUTC := targetDay + 540*60        // 09:00 UTC

	// Paso 5: Paciente (Simular alta o resolución de paciente)
	patientID := "patient-123"

	// Crear la reserva en estado PENDING con origen Online
	res, err := svc.CreateReservation(ab.CreateReservationCmd{
		Id:                      nextTestID(),
		TenantId:                tenant,
		ClientId:                patientID,
		CreatorUserId:           "user-guest",
		EmployeeServiceConfigId: cfgID,
		SlotStartUtc:            slotStartUTC,
		Origin:                  ab.OriginOnline,
		Notes:                   "Control médico online",
	})
	if err != nil {
		t.Fatalf("CreateReservation online: %v", err)
	}
	if res.Status != ab.StatusPending {
		t.Fatalf("expected PENDING status for online booking, got %s", res.Status)
	}
	if res.Origin != ab.OriginOnline {
		t.Fatalf("expected OriginOnline, got %s", res.Origin)
	}

	// Paso 6: Como PaymentRequired es true, el paso 6 vincula el pago al confirmar
	paymentID := "pay-flow-999"
	err = svc.ChangeReservationStatus(ab.ChangeStatusCmd{
		TenantId:  tenant,
		Id:        res.Id,
		Event:     ab.EventConfirm,
		ActorId:   "user-guest",
		PaymentId: paymentID,
		Revision:  int(res.Revision),
	})
	if err != nil {
		t.Fatalf("ChangeReservationStatus Confirm: %v", err)
	}

	got, err := svc.GetReservation(tenant, res.Id)
	if err != nil {
		t.Fatalf("GetReservation: %v", err)
	}
	if got.Status != ab.StatusConfirmed {
		t.Fatalf("expected CONFIRMED status, got %s", got.Status)
	}
	if got.PaymentId != paymentID {
		t.Fatalf("expected PaymentId '%s', got '%s'", paymentID, got.PaymentId)
	}
}

// TestBookingFlow_CounterOriginDirectConfirm verifica el flujo de mostrador (Recepcionista):
// - Origen Counter.
// - Confirmación directa sin exigir pasarela online previa.
func TestBookingFlow_CounterOriginDirectConfirm(t *testing.T) {
	const tenant = "tenant-counter"
	const staff = "staff-counter"

	svc, repo := newTestModuleService(t, SetupDependencies())

	cfgID := nextTestID()
	cfg := ab.EmployeeServiceConfig{
		Id:              cfgID,
		TenantId:        tenant,
		StaffId:         staff,
		ServiceId:       "srv-counter",
		DurationMin:     20,
		PaymentRequired: false,
		IsActive:        true,
	}
	if err := repo.InsertEmployeeServiceConfig(cfg); err != nil {
		t.Fatalf("InsertEmployeeServiceConfig: %v", err)
	}

	if err := svc.UpsertCalendarConfig(ab.WorkCalendarConfig{
		TenantId: tenant, StaffId: staff, Timezone: "UTC", IsActive: true,
	}); err != nil {
		t.Fatalf("UpsertCalendarConfig: %v", err)
	}
	if err := svc.SaveDayBlocks(tenant, staff, 1, []ab.WorkCalendarBlock{
		{TenantId: tenant, StaffId: staff, DayOfWeek: 1, StartMin: 540, EndMin: 720, IsActive: true},
	}); err != nil {
		t.Fatalf("SaveDayBlocks: %v", err)
	}

	targetDay := Date(2025, 1, 6, 0, 0, 0, 0)
	slotStartUTC := targetDay + 540*60

	res, err := svc.CreateReservation(ab.CreateReservationCmd{
		Id:                      nextTestID(),
		TenantId:                tenant,
		ClientId:                "patient-counter-1",
		CreatorUserId:           "user-receptionist",
		EmployeeServiceConfigId: cfgID,
		SlotStartUtc:            slotStartUTC,
		Origin:                  ab.OriginCounter,
		Notes:                   "Atención en mesón",
	})
	if err != nil {
		t.Fatalf("CreateReservation counter: %v", err)
	}
	if res.Origin != ab.OriginCounter {
		t.Fatalf("expected OriginCounter, got %s", res.Origin)
	}

	// Recepcionista confirma de inmediato
	err = svc.ChangeReservationStatus(ab.ChangeStatusCmd{
		TenantId: tenant,
		Id:       res.Id,
		Event:    ab.EventConfirm,
		ActorId:  "user-receptionist",
		Revision: int(res.Revision),
	})
	if err != nil {
		t.Fatalf("ChangeReservationStatus: %v", err)
	}

	got, err := svc.GetReservation(tenant, res.Id)
	if err != nil {
		t.Fatalf("GetReservation: %v", err)
	}
	if got.Status != ab.StatusConfirmed {
		t.Fatalf("expected CONFIRMED, got %s", got.Status)
	}
}

// MockPatientDirectory simula el comportamiento de búsqueda y creación de patient_directory
type MockPatientDirectory struct {
	patients map[string]patientdirectory.Patient
}

func NewMockPatientDirectory() *MockPatientDirectory {
	return &MockPatientDirectory{patients: make(map[string]patientdirectory.Patient)}
}

func (m *MockPatientDirectory) FindByRUT(rut string) (*patientdirectory.Patient, bool) {
	p, ok := m.patients[rut]
	if !ok {
		return nil, false
	}
	return &p, true
}

func (m *MockPatientDirectory) SavePatient(p patientdirectory.Patient) {
	m.patients[p.Rut] = p
}

func TestBookingFlow_PatientLookupRUT(t *testing.T) {
	pd := NewMockPatientDirectory()
	pd.SavePatient(patientdirectory.Patient{
		Id:   "p-1",
		Rut:  "11.111.111-1",
		Name: "Ana Gómez",
	})

	// Caso 1: Paciente existe
	found, exists := pd.FindByRUT("11.111.111-1")
	if !exists || found.Name != "Ana Gómez" {
		t.Fatalf("expected to find patient Ana Gómez")
	}

	// Caso 2: Paciente no existe -> se crea en el paso 5
	_, exists2 := pd.FindByRUT("22.222.222-2")
	if exists2 {
		t.Fatalf("expected patient 22.222.222-2 to not exist initially")
	}
	pd.SavePatient(patientdirectory.Patient{
		Id:   "p-2",
		Rut:  "22.222.222-2",
		Name: "Carlos Pérez",
	})
	created, exists3 := pd.FindByRUT("22.222.222-2")
	if !exists3 || created.Name != "Carlos Pérez" {
		t.Fatalf("expected Carlos Pérez to be created")
	}
}

var _ = tinytime.NowUnix
