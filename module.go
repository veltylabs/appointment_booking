package appointmentbooking

import (
	"webtyp.com/events"
	"webtyp.com/fmt"
	"webtyp.com/model"
	"webtyp.com/orm"
	"webtyp.com/svg"
	"webtyp.com/time"
)

// domainError is the concrete type of this package's sentinel errors. Code
// compares them by asserting this type and comparing the value: == between two
// error values compiles, under TinyGo, to runtime.interfaceEqual, which pulls
// internal/reflectlite into the wasm binary.
const (
	ErrCalendarConfigExists      model.Conflict = "this professional already has a calendar"
	ErrStatusRequired            model.Invalid  = "a reservation update must send its status"
	ErrOnlyStatusChanges         model.Invalid  = "only the status of a reservation can change"
	ErrInvalidDuration           model.Invalid  = "the service duration must be at least 1 minute"
	ErrCalendarConfigNotFound    model.NotFound = "no calendar config for this professional"
	ErrSlotTaken                 model.Conflict = "slot taken"
	ErrMissingArgs               model.Invalid  = "missing args"
	ErrInvalidBlock              model.Invalid  = "appointment_booking: start_min must be < end_min and both within 0..1439"
	ErrBlocksOverlap             model.Invalid  = "appointment_booking: two blocks of the same day overlap"
	ErrBlockOutsideBusinessHours model.Invalid  = "appointment_booking: block falls outside the establishment's opening hours"
	ErrBlockOnClosedDay          model.Invalid  = "appointment_booking: the establishment is closed on that date"
	ErrNoServiceConfig           model.Invalid  = "appointment_booking: FormConfig.ServiceConfigId is required to book — the professional has no service configured"
	ErrIncompleteSlot            model.Invalid  = "appointment_booking: a booking needs both a day and an hour"
	ErrIdTaken                   model.Invalid  = "id already used"
	ErrImportStatus              model.Invalid  = "an imported reservation must be confirmed, cancelled, completed or no-show"
	ErrOverbookNotAllowed        model.Invalid  = "overbooking not allowed for this professional and day"
	ErrSlotNotInSchedule         model.Invalid  = "appointment_booking: slot is not within professional's scheduled blocks"
	ErrClientNotFound            model.NotFound = "unknown client"
	ErrStaffNotFound             model.NotFound = "unknown professional"
	ErrServiceNotFound           model.NotFound = "unknown service"
)

// Tipos de excepción de calendario (valor de WorkCalendarException.ExceptionType).
const (
	ExcHoliday      = "HOLIDAY"
	ExcSpecialHours = "SPECIAL_HOURS"
	ExcBlocked      = "BLOCKED"
)

// iconAppointmentBooking es la referencia al ícono de marca del módulo — solo el nombre llega al
// binario wasm; la geometría se declara en svg.go (registrado por IconSvg en svg.go).
const iconAppointmentBooking = svg.Icon("appointment-booking-module")

// Eventos de dominio emitidos por este módulo.
const (
	EventReservationCreated     = "appointment.reservation.created"
	EventReservationConfirmed   = "appointment.reservation.confirmed"
	EventReservationCancelled   = "appointment.reservation.cancelled"
	EventReservationCompleted   = "appointment.reservation.completed"
	EventReservationNoShow      = "appointment.reservation.no_show"
	EventReservationExpired     = "appointment.reservation.expired"
	EventReservationRescheduled = "appointment.reservation.rescheduled"
	// EventReservationConflicted se emite UNA VEZ POR reserva cuando un cambio
	// vino de la edición del propio profesional (§8.2) — el notificador llega al
	// paciente sin sondeo. El caso establishment-wide (feriado/cierre que golpea
	// a decenas de profesionales) publica SOLO el summary por staff y el
	// consumidor lee ListConflictingReservations (§8.6).
	EventReservationConflicted = "appointment.reservation.conflicted"
	// EventScheduleChanged se emite al mutar la agenda de un profesional o
	// recomputar conflictos tras un cambio del establecimiento. Un consumidor lo
	// usa para recalcular disponibilidad / notificar (p. ej. reservation recarga
	// sus huecos libres). Solo se emite cuando el cambio PUSO reservas en
	// conflicto (CU-19: sin afectados ⇒ silencio).
	EventScheduleChanged = "appointment.schedule.changed"
)

// ScheduleChangedPayload es el payload tipado del evento EventScheduleChanged.
// Implementa model.Encodable (lo que events.Event.Payload exige) y también
// model.Decodable, para que un broker que cruza un cable real (webtyp/sse en
// mjosefa-cms) pueda serializarlo; el broker in-proc de la demo entrega el
// puntero concreto sin codificar.
type ScheduleChangedPayload struct {
	TenantId string
	StaffId  string
	// FromDate/ToDate bound what changed, so a consumer recomputes a range
	// instead of everything. 0/0 means "unknown, recompute all".
	FromDate int64
	ToDate   int64
	// ConflictCount is how many reservations the change put in conflict.
	// Zero means nobody needs to be told (CU-19).
	ConflictCount int
}

func (p *ScheduleChangedPayload) IsNil() bool { return p == nil }

func (p *ScheduleChangedPayload) EncodeFields(w model.FieldWriter) {
	w.String("tenant_id", p.TenantId)
	w.String("staff_id", p.StaffId)
	w.Int("from_date", p.FromDate)
	w.Int("to_date", p.ToDate)
	w.Int("conflict_count", int64(p.ConflictCount))
}

func (p *ScheduleChangedPayload) DecodeFields(r model.FieldReader) {
	p.TenantId, _ = r.String("tenant_id")
	p.StaffId, _ = r.String("staff_id")
	p.FromDate, _ = r.Int("from_date")
	p.ToDate, _ = r.Int("to_date")
	if v, ok := r.Int("conflict_count"); ok {
		p.ConflictCount = int(v)
	}
}

type Module struct {
	db       *orm.DB
	store    *orm.Store
	ids      model.IDGenerator
	staff    StaffReader
	catalog  CatalogReader
	patients PatientDirectory
	pub      events.Publisher
	hours    OpeningHours
	payments PaymentGateway
	zone     time.Zone
	tenantID string
}

type Deps struct {
	Staff     StaffReader
	Catalog   CatalogReader
	Patients  PatientDirectory
	Hours     OpeningHours // opcional: nil = time.Unlimited()
	Payments  PaymentGateway
	IDs       model.IDGenerator // requerido
	Publisher events.Publisher  // opcional — nil desactiva
	Zone      time.Zone         // obligatoria
	TenantID  string            // requerido: el módulo atiende a un solo tenant
}

func depErr(name string) error {
	return fmt.Err("appointment_booking: Deps."+name, "is required")
}

// New conecta el módulo a un *orm.DB ya conectado; se asume que el esquema ya existe —consulte el submódulo migrate.
func New(db *orm.DB, deps Deps) (*Module, error) {
	if deps.IDs == nil {
		return nil, depErr("IDs")
	}
	if deps.Staff == nil {
		return nil, depErr("Staff")
	}
	if deps.Catalog == nil {
		return nil, depErr("Catalog")
	}
	if deps.Patients == nil {
		return nil, depErr("Patients")
	}
	if deps.Zone.Name() == "" {
		return nil, depErr("Zone")
	}
	if deps.TenantID == "" {
		return nil, depErr("TenantID")
	}
	return &Module{
		db:       db,
		store:    orm.TenantStore(db, deps.TenantID, deps.IDs),
		ids:      deps.IDs,
		staff:    deps.Staff,
		catalog:  deps.Catalog,
		patients: deps.Patients,
		pub:      deps.Publisher,
		hours:    deps.Hours,
		payments: deps.Payments,
		zone:     deps.Zone,
		tenantID: deps.TenantID,
	}, nil
}

// own responde ErrNotFound cuando tenantId no es el tenant de este módulo: un módulo atiende a un solo tenant.
func (m *Module) own(tenantId string) error {
	if tenantId != m.tenantID {
		return ErrNotFound
	}
	return nil
}

func (m *Module) Zone() time.Zone { return m.zone }

func (m *Module) dayKey(u time.Unix) time.Unix {
	d, _ := time.ParseDateUTC(m.zone.Date(u))
	return d
}

func dateOf(dayKey time.Unix) string {
	y, mo, d, _, _, _ := time.CivilUTC(dayKey)
	return fmt.Sprintf("%04d-%02d-%02d", y, mo, d)
}

func (m *Module) at(dayKey time.Unix, minutes int) time.Unix {
	if minutes >= 1440 {
		extraDays := minutes / 1440
		remMin := minutes % 1440
		u, _ := m.zone.At(dateOf(dayKey+time.Unix(extraDays*86400)), remMin)
		return u
	}
	u, _ := m.zone.At(dateOf(dayKey), minutes)
	return u
}

// limitsForDay es el ÚNICO punto del módulo que consulta OpeningHours para una
// fecha concreta. Un solo nil-check de m.hours en todo el módulo (regla §6.1):
// nil = Unlimited, y casi todo el código que viene después ya siempre tiene limits.
func (m *Module) limitsForDay(d time.Unix) (time.DayLimits, error) {
	if m.hours == nil {
		return time.Unlimited(), nil
	}
	return m.hours.DayLimits(d)
}

// limitsForWeekday es similar a limitsForDay pero consulta OpeningHours
// por el horario regular de un día de la semana (0 = Sunday … 6 = Saturday),
// ignorando excepciones por fecha.
func (m *Module) limitsForWeekday(dow int) (time.DayLimits, error) {
	if m.hours == nil {
		return time.Unlimited(), nil
	}
	return m.hours.WeekdayLimits(dow)
}


