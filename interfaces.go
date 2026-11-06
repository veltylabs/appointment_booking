package appointmentbooking

import (
	"webtyp.com/fmt"
	"webtyp.com/time"
)

// StaffReader consulta existencia, ficha y opciones de profesionales clínicos.
// Ejemplo: ok, err := s.StaffExists("t1", "staff1")
type StaffReader interface {
	StaffExists(tenantId, staffId string) (bool, error)
	StaffCard(tenantId, staffId string) (name, specialty string, active bool, err error)
	OccupantOptions(tenantID string) ([]fmt.KeyValue, error)
}

// CatalogReader consulta existencia y datos comerciales de un servicio clínico.
// Ejemplo: name, price, curr, ok, err := c.ServiceCard("t1", "svc1")
type CatalogReader interface {
	ServiceExists(tenantId, serviceId string) (bool, error)
	ServiceCard(tenantId, serviceId string) (name string, price float64, currency string, active bool, err error)
}

// PatientDirectory es la parte de patient_directory que este módulo usa.
// Ejemplo: id, name, phone, email, ok, err := p.FindPatient("t1", "12.345.678-5")
type PatientDirectory interface {
	GetPatient(tenantID, id string) (name, documentNumber, phone, email string, found bool, err error)
	FindPatient(tenantID, documentNumber string) (id, name, phone, email string, found bool, err error)
	CreatePatient(tenantID, id, documentNumber, name, phone, email string) (patientID string, err error)
}

// OpeningHours informa las ventanas horarias y días cerrados del establecimiento.
// Ejemplo: lim, err := h.DayLimits(date)
type OpeningHours interface {
	DayLimits(date time.Unix) (time.DayLimits, error)
	WeekdayLimits(dayOfWeek int) (time.DayLimits, error)
	ClosedDates(from, to time.Unix) (holidays, closures []string, err error)
}

// PaymentGateway verifica y procesa cobros asociados a una reserva.
// Ejemplo: amt, curr, err := p.PaidAmount("t1", "pay1", "appointment_booking", "res1")
type PaymentGateway interface {
	PaidAmount(tenantId, paymentId, referenceModule, referenceId string) (amount int64, currency string, err error)
	ChargeDetail(tenantID, referenceModule, referenceID string) (payable, terminal bool, amount int64, currency string, err error)
	RegisterCounterPayment(tenantID, id, referenceModule, referenceID, method, actorID string) (paymentID string, err error)
	StartTerminalPayment(tenantID, id, referenceModule, referenceID, method string) (paymentID string, err error)
	RefreshTerminalPayment(tenantID, paymentID string) (status string, err error)
	StartOnlinePayment(tenantID, id, referenceModule, referenceID, returnTo string) (redirectURL, tokenField, token string, err error)
}

// SchedulingService define las capacidades del módulo de reservas y calendarios.
type SchedulingService interface {
	// Configuración de servicio de empleado
	CreateEmployeeServiceConfig(cfg EmployeeServiceConfig) (EmployeeServiceConfig, error)
	GetEmployeeServiceConfig(id string) (EmployeeServiceConfig, error)
	ListEmployeeServiceConfigByStaff(tenantId, staffId string) ([]EmployeeServiceConfig, error)
	UpdateEmployeeServiceConfig(cfg EmployeeServiceConfig) error

	// Gestión de calendario
	GetCalendarConfig(tenantId, staffId string) (WorkCalendarConfig, error)
	UpsertCalendarConfig(cfg WorkCalendarConfig) error
	SaveDayBlocks(tenantId, staffId string, dayOfWeek int, blocks []WorkCalendarBlock) error
	ImportDayBlocks(tenantId, staffId string, dayOfWeek int, blocks []WorkCalendarBlock) error
	SaveDateBlocks(tenantId, staffId string, date int64, blocks []WorkCalendarBlock) error
	MarkWorkingDays(tenantId, staffId string, dates []int64, startMin, endMin int) error
	UnmarkWorkingDays(tenantId, staffId string, dates []int64) error
	ListBlocks(tenantId, staffId string) ([]WorkCalendarBlock, error)
	AddException(exc WorkCalendarException) error
	RemoveException(tenantId, exceptionId string) error
	ListExceptions(tenantId, staffId string, from, to time.Unix) ([]WorkCalendarException, error)
	GetDayLimits(date time.Unix) (time.DayLimits, error)

	// Disponibilidad
	ListAvailability(tenantId, staffId, configId string, from, to time.Unix) ([]TimeSlot, error)

	// Reservas
	CreateReservation(cmd CreateReservationCmd) (Reservation, error)
	CreateOverbooking(cmd CreateReservationCmd) (Reservation, error)
	GetReservation(tenantId, id string) (Reservation, error)
	ListReservationsByStaff(tenantId, staffId string, from, to time.Unix) ([]Reservation, error)
	ListReservationsByClient(tenantId, clientId string) ([]Reservation, error)
	ChangeReservationStatus(cmd ChangeStatusCmd) error
	ExpirePendingReservations(tenantId string, now time.Unix) (int, error)

	// Conflictos
	ListConflictingReservations(tenantId, staffId string, from time.Unix) ([]ConflictingReservation, error)
	RecomputeConflicts(tenantId string, from, to time.Unix) (int, error)
}

