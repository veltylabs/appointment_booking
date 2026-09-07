package appointmentbooking

import (
	"webtyp.com/events"
	"webtyp.com/fmt"
	"webtyp.com/model"
	"webtyp.com/orm"
	"webtyp.com/svg"
	tinytime "webtyp.com/time"
)

var (
	ErrCalendarConfigNotFound = fmt.Err("calendar", "config", "not", "found")
	ErrSlotTaken              = fmt.Err("slot", "taken")
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
	// EventScheduleChanged se emite al mutar la agenda de un profesional
	// (upsert semanal, alta/baja de excepción). Un consumidor lo usa para
	// recalcular disponibilidad (p. ej. reservation recarga sus huecos libres).
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
}

func (p *ScheduleChangedPayload) IsNil() bool { return p == nil }

func (p *ScheduleChangedPayload) EncodeFields(w model.FieldWriter) {
	w.String("tenant_id", p.TenantId)
	w.String("staff_id", p.StaffId)
}

func (p *ScheduleChangedPayload) DecodeFields(r model.FieldReader) {
	p.TenantId, _ = r.String("tenant_id")
	p.StaffId, _ = r.String("staff_id")
}

// StaffReader verifica que un miembro del staff existe y pertenece al tenant.
type StaffReader interface {
	StaffExists(tenantId, staffId string) (bool, error)
}

// CatalogReader verifica que un servicio existe y pertenece al tenant.
type CatalogReader interface {
	ServiceExists(tenantId, serviceId string) (bool, error)
}

// DirectoryReader verifica que un cliente existe y pertenece al tenant.
type DirectoryReader interface {
	ClientExists(tenantId, clientId string) (bool, error)
}

type SchedulingService interface {
	// Gestión de calendario
	UpsertCalendarConfig(cfg WorkCalendarConfig) error
	UpsertWeeklyCalendar(cal WorkCalendarWeekly) error
	AddException(exc WorkCalendarException) error
	RemoveException(tenantId, exceptionId string) error
	ListWeeklyCalendar(tenantId, staffId string) ([]WorkCalendarWeekly, error)
	ListExceptions(tenantId, staffId string, from, to int64) ([]WorkCalendarException, error)

	// Disponibilidad
	ListAvailability(tenantId, staffId, configId string, from, to int64) ([]TimeSlot, error)

	// Reservas
	CreateReservation(cmd CreateReservationCmd) (Reservation, error)
	GetReservation(tenantId, id string) (Reservation, error)
	ListReservationsByStaff(tenantId, staffId string, from, to int64) ([]Reservation, error)
	ListReservationsByClient(tenantId, clientId string) ([]Reservation, error)
	ChangeReservationStatus(cmd ChangeStatusCmd) error
	ExpirePendingReservations(tenantId string, before int64) (int, error)
}

type CreateReservationCmd struct {
	TenantId                string
	ClientId                string
	CreatorUserId           string
	EmployeeServiceConfigId string
	SlotStartUtc            int64
	Notes                   string
	RescheduledFromId       string
}

type ChangeStatusCmd struct {
	TenantId  string
	Id        string
	Event     string
	ActorId   string
	PaymentId string
	Revision  int
}

type Module struct {
	db        *orm.DB
	repo      *Repository
	ids       model.IDGenerator
	staff     StaffReader
	catalog   CatalogReader
	directory DirectoryReader
	pub       events.Publisher
}

type Deps struct {
	Staff     StaffReader
	Catalog   CatalogReader
	Directory DirectoryReader
	IDs       model.IDGenerator // requerido
	Publisher events.Publisher  // opcional — nil desactiva
}

func New(db *orm.DB, deps Deps) (*Module, error) {
	if deps.IDs == nil {
		return nil, fmt.Err("appointment_booking: Deps.IDs is required")
	}
	repo, err := NewRepository(db, deps.IDs)
	if err != nil {
		return nil, err
	}

	return &Module{
		db:        db,
		repo:      repo,
		ids:       deps.IDs,
		staff:     deps.Staff,
		catalog:   deps.Catalog,
		directory: deps.Directory,
		pub:       deps.Publisher,
	}, nil
}

var _ SchedulingService = (*Module)(nil)

func (m *Module) UpsertCalendarConfig(cfg WorkCalendarConfig) error {
	return m.repo.UpsertCalendarConfig(cfg)
}

func (m *Module) UpsertWeeklyCalendar(cal WorkCalendarWeekly) error {
	// Primero hay que verificar si CalendarConfig existe
	_, err := m.repo.GetCalendarConfig(cal.TenantId, cal.StaffId)
	if err != nil {
		if err == ErrNotFound {
			return ErrCalendarConfigNotFound
		}
		return err
	}

	if err := m.repo.UpsertWeeklyCalendar(cal); err != nil {
		return err
	}
	if m.pub != nil {
		m.pub.Publish(events.Event{Topic: EventScheduleChanged, Payload: &ScheduleChangedPayload{TenantId: cal.TenantId, StaffId: cal.StaffId}})
	}
	return nil
}

func (m *Module) AddException(exc WorkCalendarException) error {
	if err := m.repo.InsertException(exc); err != nil {
		return err
	}
	if m.pub != nil {
		m.pub.Publish(events.Event{Topic: EventScheduleChanged, Payload: &ScheduleChangedPayload{TenantId: exc.TenantId, StaffId: exc.StaffId}})
	}
	return nil
}

func (m *Module) RemoveException(tenantId, exceptionId string) error {
	exc, err := m.repo.GetException(tenantId, exceptionId)
	if err != nil {
		return err
	}
	if err := m.repo.DeleteException(tenantId, exceptionId); err != nil {
		return err
	}
	if m.pub != nil {
		m.pub.Publish(events.Event{Topic: EventScheduleChanged, Payload: &ScheduleChangedPayload{TenantId: tenantId, StaffId: exc.StaffId}})
	}
	return nil
}

func (m *Module) ListWeeklyCalendar(tenantId, staffId string) ([]WorkCalendarWeekly, error) {
	return m.repo.ListWeeklyCalendar(tenantId, staffId)
}

func (m *Module) ListExceptions(tenantId, staffId string, from, to int64) ([]WorkCalendarException, error) {
	return m.repo.ListExceptions(tenantId, staffId, from, to)
}

// LocalIntToUnixUTC interpreta localInt como minutos desde la medianoche en la fecha dada (medianoche UTC) en la zona horaria (tz) especificada.
func LocalIntToUnixUTC(date int64, localInt int, tz string) int64 {
	return tinytime.LocalMinutesToUnixUTC(date, localInt, tz)
}

// weeklyForDay busca el horario semanal activo para un día de la semana dado — scan lineal sobre
// como mucho 7 filas, ver la nota en ListAvailability.
func weeklyForDay(weeklies []WorkCalendarWeekly, dow int) (WorkCalendarWeekly, bool) {
	for _, w := range weeklies {
		if int(w.DayOfWeek) == dow {
			return w, true
		}
	}
	return WorkCalendarWeekly{}, false
}

func (m *Module) ListAvailability(tenantId, staffId, configId string, from, to int64) ([]TimeSlot, error) {
	// 1. Cargar WorkCalendarConfig
	cfg, err := m.repo.GetCalendarConfig(tenantId, staffId)
	if err != nil {
		if err == ErrNotFound {
			return nil, ErrCalendarConfigNotFound
		}
		return nil, err
	}
	if !cfg.IsActive {
		return []TimeSlot{}, nil
	}

	// 2. Cargar WorkCalendarWeekly
	weeklies, err := m.repo.ListWeeklyCalendar(tenantId, staffId)
	if err != nil {
		return nil, err
	}
	// Slice, no map — la regla "cero map" de AGENTS.md no tiene excepciones; a lo sumo 7 filas
	// (una por día de la semana), un scan lineal no cuesta nada medible.
	activeWeeklies := make([]WorkCalendarWeekly, 0, len(weeklies))
	for _, w := range weeklies {
		if w.IsActive {
			activeWeeklies = append(activeWeeklies, w)
		}
	}

	// 3. Cargar WorkCalendarException
	exceptions, err := m.repo.ListExceptions(tenantId, staffId, from, to)
	if err != nil {
		return nil, err
	}
	// Sin agrupar en un map — el filtro por fecha se hace inline con un scan lineal sobre
	// `exceptions` dentro del loop de días (ver más abajo). Las excepciones son un conjunto
	// disperso por diseño (feriados, horarios especiales); nunca lo suficientemente grande
	// para que un scan lineal importe.

	// 4. Cargar Reservations existentes
	reservations, err := m.repo.ListReservationsByStaff(tenantId, staffId, from, to)
	if err != nil {
		return nil, err
	}
	activeReservations := []Reservation{}
	for _, r := range reservations {
		if r.Status != StatusCancelled && r.Status != StatusRescheduled && r.Status != StatusExpired {
			activeReservations = append(activeReservations, r)
		}
	}

	// 5. Cargar EmployeeServiceConfig
	empSvcCfg, err := m.repo.GetEmployeeServiceConfig(configId)
	if err != nil {
		return nil, err
	}
	if !empSvcCfg.IsActive || empSvcCfg.TenantId != tenantId {
		return []TimeSlot{}, nil
	}

	durationMin := int(empSvcCfg.DurationMin)
	bufferMin := int(empSvcCfg.BufferMin)

	slots := []TimeSlot{}

	// 6. Para cada día D en [from, to] (asumiendo que from y to son timestamps de medianoche UTC)
	// Incrementamos de a 1 día (86400 segundos)
	for d := from; d <= to; d += 86400 {
		dow := tinytime.Weekday(d)

		weekly, hasWeekly := weeklyForDay(activeWeeklies, dow)
		if !hasWeekly {
			continue // saltar el día
		}

		workStartUTC := LocalIntToUnixUTC(d, int(weekly.WorkStart), cfg.Timezone)
		workFinishUTC := LocalIntToUnixUTC(d, int(weekly.WorkFinish), cfg.Timezone)
		breakStartUTC := LocalIntToUnixUTC(d, int(weekly.BreakStart), cfg.Timezone)
		breakFinishUTC := LocalIntToUnixUTC(d, int(weekly.BreakFinish), cfg.Timezone)

		hasBreak := weekly.BreakStart > 0 || weekly.BreakFinish > 0

		// Aplicar excepciones
		var dayExceptions []WorkCalendarException
		for _, e := range exceptions {
			if e.SpecificDate == d {
				dayExceptions = append(dayExceptions, e)
			}
		}
		isHoliday := false

		// Prioridad: HOLIDAY > SPECIAL_HOURS > BLOCKED
		var specialHours *WorkCalendarException
		var blockedExcs []WorkCalendarException

		for _, e := range dayExceptions {
			eCopy := e
			if e.ExceptionType == ExcHoliday {
				isHoliday = true
			} else if e.ExceptionType == ExcSpecialHours {
				if specialHours == nil {
					specialHours = &eCopy
				}
			} else if e.ExceptionType == ExcBlocked {
				blockedExcs = append(blockedExcs, e)
			}
		}

		if isHoliday {
			continue
		}

		if specialHours != nil {
			workStartUTC = LocalIntToUnixUTC(d, int(specialHours.StartTime), cfg.Timezone)
			workFinishUTC = LocalIntToUnixUTC(d, int(specialHours.EndTime), cfg.Timezone)
			hasBreak = false // break interval removed
		}

		// Generar slots
		curr := workStartUTC
		for {
			end := curr + int64(durationMin*60)
			endWithBuffer := end + int64(bufferMin*60)

			if endWithBuffer > workFinishUTC {
				break
			}

			// Verificar break
			if hasBreak {
				if !(end <= breakStartUTC || curr >= breakFinishUTC) {
					// saltar al final del break para permitir slots después del break
					curr = breakFinishUTC
					continue
				}
			}

			// Verificar excepciones bloqueadas
			isBlocked := false
			var blockedEnd int64
			for _, b := range blockedExcs {
				bStart := LocalIntToUnixUTC(d, int(b.StartTime), cfg.Timezone)
				bEnd := LocalIntToUnixUTC(d, int(b.EndTime), cfg.Timezone)
				// Verificación de solapamiento
				if curr < bEnd && endWithBuffer > bStart {
					isBlocked = true
					blockedEnd = bEnd
					break
				}
			}
			if isBlocked {
				curr = blockedEnd // advance past block
				continue
			}

			// Verificar reservas existentes
			hasOverlap := false
			var resEnd int64
			for _, r := range activeReservations {
				rStart := r.ReservationTime
				rEnd := rStart + int64(r.DurationMinSnapshot*60)
				// Verificación de solapamiento
				if curr < rEnd && endWithBuffer > rStart {
					hasOverlap = true
					resEnd = rEnd
					break
				}
			}

			if hasOverlap {
				curr = resEnd
			} else {
				slots = append(slots, TimeSlot{StartUtc: curr, EndUtc: end})
				curr += int64(durationMin * 60)
			}
		}
	}

	return slots, nil
}

func (m *Module) CreateReservation(cmd CreateReservationCmd) (Reservation, error) {
	// 1. Cargar EmployeeServiceConfig
	empSvcCfg, err := m.repo.GetEmployeeServiceConfig(cmd.EmployeeServiceConfigId)
	if err != nil {
		return Reservation{}, err
	}
	if !empSvcCfg.IsActive || empSvcCfg.TenantId != cmd.TenantId {
		return Reservation{}, ErrNotFound
	}

	// 2. Validar cliente
	clientExists, err := m.directory.ClientExists(cmd.TenantId, cmd.ClientId)
	if err != nil {
		return Reservation{}, err
	}
	if !clientExists {
		return Reservation{}, fmt.Err("client", "not", "found")
	}

	// 3. Validar staff
	staffExists, err := m.staff.StaffExists(cmd.TenantId, empSvcCfg.StaffId)
	if err != nil {
		return Reservation{}, err
	}
	if !staffExists {
		return Reservation{}, fmt.Err("staff", "not", "found")
	}

	// 4. Validar servicio
	serviceExists, err := m.catalog.ServiceExists(cmd.TenantId, empSvcCfg.ServiceId)
	if err != nil {
		return Reservation{}, err
	}
	if !serviceExists {
		return Reservation{}, fmt.Err("service", "not", "found")
	}

	// 5. Verificar disponibilidad
	// Obtener disponibilidad para el día objetivo (medianoche UTC)
	targetDay := tinytime.MidnightUTC(cmd.SlotStartUtc)

	// Ampliar la búsqueda un día a cada lado para cubrir diferencias de límite de zona horaria
	fromDay := targetDay - 86400
	toDay := targetDay + 86400

	slots, err := m.ListAvailability(cmd.TenantId, empSvcCfg.StaffId, empSvcCfg.Id, fromDay, toDay)
	if err != nil {
		return Reservation{}, err
	}

	isAvailable := false
	for _, slot := range slots {
		if slot.StartUtc == cmd.SlotStartUtc {
			isAvailable = true
			break
		}
	}
	if !isAvailable {
		return Reservation{}, ErrSlotTaken
	}

	var newReservation Reservation
	var originalReservation *Reservation

	err = m.db.Tx(func(tx *orm.DB) error {
		now := tinytime.Now()

		newReservation = Reservation{
			TenantId:                cmd.TenantId,
			ClientId:                cmd.ClientId,
			CreatorUserId:           cmd.CreatorUserId,
			EmployeeServiceConfigId: cmd.EmployeeServiceConfigId,
			StaffIdsnapshot:         empSvcCfg.StaffId,
			ServiceIdsnapshot:       empSvcCfg.ServiceId,
			DurationMinSnapshot:     empSvcCfg.DurationMin,
			PriceSnapshot:           empSvcCfg.PriceOverride,
			CurrencySnapshot:        "CLP", // default
			ReservationDate:         targetDay,
			ReservationTime:         cmd.SlotStartUtc,
			LocalStringDate:         tinytime.FormatDate(cmd.SlotStartUtc * 1000000000),
			LocalStringTime:         tinytime.FormatTime(cmd.SlotStartUtc * 1000000000),
			Status:                  StatusPending,
			RescheduledFromId:       cmd.RescheduledFromId,
			Notes:                   cmd.Notes,
			UpdatedAt:               now,
			UpdatedBy:               cmd.CreatorUserId, // Usar CreatorUserId como ActorID al momento de la creación
			Revision:                0,
		}

		if cmd.RescheduledFromId != "" {
			orig, err := m.repo.GetReservationTx(tx, cmd.TenantId, cmd.RescheduledFromId)
			if err != nil {
				return err
			}
			originalReservation = &orig

			_, err = Transition(orig.Status, EventReschedule)
			if err != nil {
				return err
			}

			err = m.repo.UpdateReservationStatusTx(tx, orig.Id, StatusRescheduled, cmd.CreatorUserId, now, orig.Revision)
			if err != nil {
				return err
			}
		}

		newReservation.Id = m.ids.NewID()

		// Hacer un insert dentro de la tx en vez de repo.InsertReservation, que usa db.Create
		err = tx.Create(&newReservation)
		if err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		return Reservation{}, err
	}

	if m.pub != nil {
		m.pub.Publish(events.Event{Topic: EventReservationCreated, Payload: &newReservation})
		if originalReservation != nil {
			m.pub.Publish(events.Event{Topic: EventReservationRescheduled, Payload: originalReservation})
		}
	}

	return newReservation, nil
}

func (m *Module) GetReservation(tenantId, id string) (Reservation, error) {
	res, err := m.repo.GetReservation(id)
	if err != nil {
		return Reservation{}, err
	}
	if res.TenantId != tenantId {
		return Reservation{}, ErrNotFound
	}
	return res, nil
}

func (m *Module) ListReservationsByStaff(tenantId, staffId string, from, to int64) ([]Reservation, error) {
	return m.repo.ListReservationsByStaff(tenantId, staffId, from, to)
}

func (m *Module) ListReservationsByClient(tenantId, clientId string) ([]Reservation, error) {
	return m.repo.ListReservationsByClient(tenantId, clientId)
}

func (m *Module) ChangeReservationStatus(cmd ChangeStatusCmd) error {
	current, err := m.repo.GetReservation(cmd.Id)
	if err != nil {
		return err
	}
	if current.TenantId != cmd.TenantId {
		return ErrNotFound
	}

	nextState, err := Transition(current.Status, cmd.Event)
	if err != nil {
		return err
	}

	err = m.db.Tx(func(tx *orm.DB) error {
		now := tinytime.Now()

		if cmd.Event == EventConfirm && cmd.PaymentId != "" {
			got := &Reservation{}
			qb := tx.Query(got).Where(Reservation_.Id).Eq(cmd.Id)
			gotRes, err := ReadOneReservation(qb, got)
			if err != nil {
				return err
			}
			if gotRes.Revision != int64(cmd.Revision) {
				return ErrConflict
			}
			gotRes.Status = nextState
			gotRes.UpdatedBy = cmd.ActorId
			gotRes.UpdatedAt = now
			gotRes.PaymentId = cmd.PaymentId
			gotRes.Revision++
			return tx.Update(gotRes, orm.Eq(Reservation_.Id, gotRes.Id), orm.Eq(Reservation_.TenantId, gotRes.TenantId))
		}

		return m.repo.UpdateReservationStatusTx(tx, cmd.Id, nextState, cmd.ActorId, now, int64(cmd.Revision))
	})

	if err != nil {
		return err
	}

	var domainEvent string
	switch cmd.Event {
	case EventConfirm:
		domainEvent = EventReservationConfirmed
	case EventCancel:
		domainEvent = EventReservationCancelled
	case EventComplete:
		domainEvent = EventReservationCompleted
	case EventNoShow:
		domainEvent = EventReservationNoShow
	case EventExpire:
		domainEvent = EventReservationExpired
	}

	if m.pub != nil && domainEvent != "" {
		// obtener actualizado
		updated, _ := m.repo.GetReservation(cmd.Id)
		m.pub.Publish(events.Event{Topic: domainEvent, Payload: &updated})
	}

	return nil
}

func (m *Module) ExpirePendingReservations(tenantId string, before int64) (int, error) {
	proxy := &Reservation{}
	qb := m.db.Query(proxy).
		Where(Reservation_.TenantId).Eq(tenantId).
		Where(Reservation_.Status).Eq(StatusPending).
		Where(Reservation_.ReservationTime).Lt(before)

	rows, err := ReadAllReservation(qb)
	if err != nil {
		if err == orm.ErrNotFound {
			return 0, nil
		}
		return 0, err
	}

	expiredCount := 0
	for _, row := range rows {
		err := m.ChangeReservationStatus(ChangeStatusCmd{
			TenantId: tenantId,
			Id:       row.Id,
			Event:    EventExpire,
			ActorId:  "system",
			Revision: int(row.Revision),
		})
		if err != nil {
			return expiredCount, err
		}
		expiredCount++
	}

	return expiredCount, nil
}
