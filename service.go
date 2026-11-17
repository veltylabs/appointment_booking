package appointmentbooking

import (
	"webtyp.com/crud"
	"webtyp.com/events"
	"webtyp.com/model"
	"webtyp.com/time"
)

var _ SchedulingService = (*Module)(nil)

// ///////////////////////////////////////////////////////////////////////////
// Configuración de servicio y de calendario
// ///////////////////////////////////////////////////////////////////////////

// CreateEmployeeServiceConfig registers that a professional performs a service, with its own duration/price override.
func (m *Module) CreateEmployeeServiceConfig(cfg EmployeeServiceConfig) (EmployeeServiceConfig, error) {
	if cfg.TenantId == "" {
		cfg.TenantId = m.tenantID
	}
	if err := m.own(cfg.TenantId); err != nil {
		return EmployeeServiceConfig{}, err
	}
	if cfg.Id == "" {
		cfg.Id = m.ids.NewID()
	}
	if err := checkDuration(cfg.DurationMin); err != nil {
		return EmployeeServiceConfig{}, err
	}
	if err := m.store.Create(&cfg); err != nil {
		return EmployeeServiceConfig{}, err
	}
	return cfg, nil
}

// GetEmployeeServiceConfig reads one by id.
func (m *Module) GetEmployeeServiceConfig(id string) (EmployeeServiceConfig, error) {
	return m.repoOn(m.store).GetEmployeeServiceConfig(id)
}

// ListEmployeeServiceConfigByStaff lists every service a professional performs, active or not — the editing screen
// needs to show and reactivate a disabled one, not just the active set.
func (m *Module) ListEmployeeServiceConfigByStaff(tenantId, staffId string) ([]EmployeeServiceConfig, error) {
	if err := m.own(tenantId); err != nil {
		return nil, nil
	}
	return m.repoOn(m.store).ListEmployeeServiceConfigByStaff(staffId)
}

// UpdateEmployeeServiceConfig writes the full record.
func (m *Module) UpdateEmployeeServiceConfig(cfg EmployeeServiceConfig) error {
	if err := m.own(cfg.TenantId); err != nil {
		return err
	}
	if err := checkDuration(cfg.DurationMin); err != nil {
		return err
	}
	return m.store.UpdateFields(&cfg, cfg.Id, []string{
		EmployeeServiceConfig_.StaffId, EmployeeServiceConfig_.ServiceId, EmployeeServiceConfig_.DurationMin, EmployeeServiceConfig_.BufferMin,
		EmployeeServiceConfig_.PriceOverride, EmployeeServiceConfig_.PaymentRequired, EmployeeServiceConfig_.IsActive})
}

// checkDuration rechaza una duración vacía: con 0 minutos el generador de horas no avanzaría nunca.
func checkDuration(minutes int64) error {
	if minutes < 1 {
		return &model.FieldError{Field: EmployeeServiceConfig_.DurationMin, Err: ErrInvalidDuration}
	}
	return nil
}

func (m *Module) GetCalendarConfig(tenantId, staffId string) (WorkCalendarConfig, error) {
	if err := m.own(tenantId); err != nil {
		return WorkCalendarConfig{}, ErrCalendarConfigNotFound
	}
	return m.requireCalendarConfig(m.repoOn(m.store), staffId)
}

// UpsertCalendarConfig crea el calendario del profesional o actualiza el que ya tiene (uno por profesional).
func (m *Module) UpsertCalendarConfig(cfg WorkCalendarConfig) error {
	if err := m.own(cfg.TenantId); err != nil {
		return err
	}
	if cfg.MaxOverbookPerDay < 0 {
		return ErrMissingArgs
	}
	return m.store.Atomic(func(tx crud.Tx) error {
		existing, err := m.repoOn(tx).GetCalendarConfig(cfg.StaffId)
		if isNotFound(err) {
			cfg.Id = m.ids.NewID()
			return tx.Create(&cfg)
		}
		if err != nil {
			return err
		}
		cfg.Id = existing.Id
		return tx.UpdateFields(&cfg, cfg.Id, []string{WorkCalendarConfig_.IsActive, WorkCalendarConfig_.MaxOverbookPerDay})
	})
}

func (m *Module) requireCalendarConfig(r *Repository, staffId string) (WorkCalendarConfig, error) {
	cfg, err := r.GetCalendarConfig(staffId)
	if err != nil {
		if isNotFound(err) {
			return WorkCalendarConfig{}, ErrCalendarConfigNotFound
		}
		return WorkCalendarConfig{}, err
	}
	return cfg, nil
}

// GetDayLimits proxies Deps.Hours para que el editor acote sus propios controles llamando a este módulo, no al
// calendario institucional directamente (§6.2).
func (m *Module) GetDayLimits(date time.Unix) (time.DayLimits, error) {
	return m.limitsForDay(date)
}

// ///////////////////////////////////////////////////////////////////////////
// Bloques y excepciones de la agenda
// ///////////////////////////////////////////////////////////////////////////

// SaveDayBlocks replaces EVERY weekly block of that weekday with the ones given. A whole-day replace, not a
// per-row upsert: partial edits are what let the stored set drift out of step with what the editor is showing.
func (m *Module) SaveDayBlocks(tenantId, staffId string, dayOfWeek int, blocks []WorkCalendarBlock) error {
	if err := m.own(tenantId); err != nil {
		return err
	}
	if dayOfWeek < 0 || dayOfWeek > 6 {
		return ErrInvalidBlock
	}
	if err := validateBlocks(blocks); err != nil {
		return err
	}
	for i := range blocks {
		blocks[i].TenantId = tenantId
		blocks[i].StaffId = staffId
		blocks[i].DayOfWeek = int64(dayOfWeek)
		blocks[i].SpecificDate = 0
		blocks[i].IsActive = true
	}
	limits, err := m.limitsForWeekday(dayOfWeek)
	if err != nil {
		return err
	}
	if err := validateBlockSet(limits, blocks); err != nil {
		return err
	}
	from := time.MidnightUTC(time.NowUnix())
	return m.editSchedule(staffId, from, forwardHorizon(from), func(r *Repository) error {
		if _, err := m.requireCalendarConfig(r, staffId); err != nil {
			return err
		}
		return r.ReplaceWeekdayBlocks(staffId, dayOfWeek, blocks)
	})
}

// SaveDateBlocks replaces the dated blocks of ONE date, so a single marked day can diverge from the common window
// it was created with (CU-11).
func (m *Module) SaveDateBlocks(tenantId, staffId string, date int64, blocks []WorkCalendarBlock) error {
	if err := m.own(tenantId); err != nil {
		return err
	}
	if err := validateBlocks(blocks); err != nil {
		return err
	}
	dUnix := time.Unix(date)
	for i := range blocks {
		blocks[i].TenantId = tenantId
		blocks[i].StaffId = staffId
		blocks[i].DayOfWeek = int64(time.WeekdayUTC(dUnix))
		blocks[i].SpecificDate = dUnix
		blocks[i].IsActive = true
	}
	limits, err := m.limitsForDay(dUnix)
	if err != nil {
		return err
	}
	if err := validateBlockSet(limits, blocks); err != nil {
		return err
	}
	return m.editSchedule(staffId, dUnix, dUnix, func(r *Repository) error {
		if _, err := m.requireCalendarConfig(r, staffId); err != nil {
			return err
		}
		return r.ReplaceDateBlocks(staffId, date, blocks)
	})
}

// MarkWorkingDays writes ONE dated block per date, all with the same window — the bulk gesture behind "mark the
// days I work over the next 6 months" (CU-10/CU-11). Re-marking a date replaces its blocks.
func (m *Module) MarkWorkingDays(tenantId, staffId string, dates []int64, startMin, endMin int) error {
	if err := m.own(tenantId); err != nil {
		return err
	}
	if startMin < 0 || startMin >= endMin || endMin > 1440 {
		return ErrInvalidBlock
	}
	if len(dates) == 0 {
		return nil
	}
	from, to := dateRange(dates)
	for _, d := range dates {
		limits, err := m.limitsForDay(time.Unix(d))
		if err != nil {
			return err
		}
		if err := checkBlockAgainstBounds(limits, WorkCalendarBlock{StartMin: int64(startMin), EndMin: int64(endMin)}); err != nil {
			return err
		}
	}
	return m.editSchedule(staffId, time.Unix(from), time.Unix(to), func(r *Repository) error {
		if _, err := m.requireCalendarConfig(r, staffId); err != nil {
			return err
		}
		for _, d := range dates {
			dUnix := time.Unix(d)
			if err := r.ReplaceDateBlocks(staffId, d, []WorkCalendarBlock{{
				TenantId: tenantId, StaffId: staffId, DayOfWeek: int64(time.WeekdayUTC(dUnix)),
				SpecificDate: dUnix, StartMin: int64(startMin), EndMin: int64(endMin), IsActive: true,
			}}); err != nil {
				return err
			}
		}
		return nil
	})
}

// UnmarkWorkingDays deletes every dated block on those dates, returning the days to whatever the weekly template
// says — or to unworked (CU-15).
func (m *Module) UnmarkWorkingDays(tenantId, staffId string, dates []int64) error {
	if err := m.own(tenantId); err != nil {
		return err
	}
	if len(dates) == 0 {
		return nil
	}
	from, to := dateRange(dates)
	return m.editSchedule(staffId, time.Unix(from), time.Unix(to), func(r *Repository) error {
		if _, err := m.requireCalendarConfig(r, staffId); err != nil {
			return err
		}
		for _, d := range dates {
			if err := r.DeleteDateBlocks(staffId, d); err != nil {
				return err
			}
		}
		return nil
	})
}

func dateRange(dates []int64) (from, to int64) {
	from, to = dates[0], dates[0]
	for _, d := range dates {
		if d < from {
			from = d
		}
		if d > to {
			to = d
		}
	}
	return from, to
}

func (m *Module) ListBlocks(tenantId, staffId string) ([]WorkCalendarBlock, error) {
	if err := m.own(tenantId); err != nil {
		return nil, nil
	}
	return m.repoOn(m.store).ListBlocks(staffId)
}

func (m *Module) ListExceptions(tenantId, staffId string, from, to time.Unix) ([]WorkCalendarException, error) {
	if err := m.own(tenantId); err != nil {
		return nil, nil
	}
	return m.repoOn(m.store).ListExceptions(staffId, from, to)
}

func (m *Module) AddException(exc WorkCalendarException) error {
	if err := m.own(exc.TenantId); err != nil {
		return err
	}
	if exc.Id == "" {
		exc.Id = m.ids.NewID()
	}
	from := time.MidnightUTC(exc.SpecificDate)
	return m.editSchedule(exc.StaffId, from, from, func(r *Repository) error { return r.tx.Create(&exc) })
}

func (m *Module) RemoveException(tenantId, exceptionId string) error {
	if err := m.own(tenantId); err != nil {
		return err
	}
	exc, err := m.repoOn(m.store).GetException(exceptionId)
	if err != nil {
		return err
	}
	from := time.MidnightUTC(exc.SpecificDate)
	return m.editSchedule(exc.StaffId, from, from, func(r *Repository) error {
		return r.tx.Delete(&WorkCalendarException{}, exceptionId)
	})
}

// editSchedule es el camino de las EDICIONES DEL PROFESIONAL (§8.2): escribe el cambio y recalcula los conflictos
// del rango afectado de UN profesional en la misma transacción, y después avisa (CU-19: sin afectados, silencio).
func (m *Module) editSchedule(staffId string, from, to time.Unix, write func(r *Repository) error) error {
	var result recomputeResult
	err := m.store.Atomic(func(tx crud.Tx) error {
		r := m.repoOn(tx)
		if err := write(r); err != nil {
			return err
		}
		var err error
		result, err = m.recomputeConflictsFor(r, from, to, staffId, scheduleChange{})
		return err
	})
	if err != nil {
		return err
	}
	m.announceEdit(result, from, to)
	return nil
}

// ///////////////////////////////////////////////////////////////////////////
// Disponibilidad
// ///////////////////////////////////////////////////////////////////////////

func (m *Module) ListAvailability(tenantId, staffId, configId string, from, to time.Unix) ([]TimeSlot, error) {
	if err := m.own(tenantId); err != nil {
		return nil, err
	}
	return m.availability(m.repoOn(m.store), staffId, configId, from, to)
}

func (m *Module) availability(r *Repository, staffId, configId string, from, to time.Unix) ([]TimeSlot, error) {
	cfg, err := m.requireCalendarConfig(r, staffId)
	if err != nil {
		return nil, err
	}
	if !cfg.IsActive {
		return []TimeSlot{}, nil
	}
	blocks, err := r.ListBlocks(staffId)
	if err != nil {
		return nil, err
	}
	exceptions, err := r.ListExceptions(staffId, from, to)
	if err != nil {
		return nil, err
	}
	fromUtc := m.at(from, 0)
	_, toUtc, _ := m.zone.Day(dateOf(to))
	reservations, err := r.ListReservationsByStaff(staffId, fromUtc, toUtc)
	if err != nil {
		return nil, err
	}
	activeReservations := []Reservation{}
	for _, res := range reservations {
		if holdsSlot(res.Status) {
			activeReservations = append(activeReservations, res)
		}
	}
	empSvcCfg, err := r.GetEmployeeServiceConfig(configId)
	if err != nil {
		return nil, err
	}
	if !empSvcCfg.IsActive {
		return []TimeSlot{}, nil
	}
	return m.scheduleSlots(from, to, blocks, exceptions, cfg, empSvcCfg, activeReservations)
}

// ///////////////////////////////////////////////////////////////////////////
// Reservas
// ///////////////////////////////////////////////////////////////////////////

type CreateReservationCmd struct {
	Id                      string
	TenantId                string
	ClientId                string
	CreatorUserId           string
	EmployeeServiceConfigId string
	SlotTimeUtc            time.Unix
	Notes                   string
	RescheduledFromId       string
	Origin                  string
	HoldUntil               time.Unix
}

type CreateOverbookingCmd = CreateReservationCmd

type ChangeStatusCmd struct {
	TenantId  string
	Id        string
	Event     string
	ActorId   string
	PaymentId string
	Revision  int
}

// reservationOf arma la fila de una orden de reserva; prepareReservation la completa y la valida.
func reservationOf(cmd CreateReservationCmd) Reservation {
	return Reservation{
		Id: cmd.Id, TenantId: cmd.TenantId, ClientId: cmd.ClientId, EmployeeServiceConfigId: cmd.EmployeeServiceConfigId,
		ReservationTime: int64(cmd.SlotTimeUtc), HoldUntil: cmd.HoldUntil, Origin: cmd.Origin,
		RescheduledFromId: cmd.RescheduledFromId, Notes: cmd.Notes, CreatorUserId: cmd.CreatorUserId,
	}
}

// prepareReservation completa y valida una reserva nueva: la configuración activa, el cliente, el profesional y
// el servicio existen, la hora es un inicio de horario habilitado (libre, o de sobrecupo dentro del tope del día) y
// congela precio y duración. No escribe: la regla de crud o CreateReservation la guardan.
// checkClient = false deja pasar un cliente aún sin resolver (la reserva online resuelve al paciente después).
func (m *Module) prepareReservation(r *Repository, res *Reservation, overbook, checkClient bool) error {
	if res.Id == "" || res.ClientId == "" {
		return ErrMissingArgs
	}
	if res.Origin != OriginCounter && res.Origin != OriginOnline {
		return ErrMissingArgs
	}
	empSvcCfg, err := r.GetEmployeeServiceConfig(res.EmployeeServiceConfigId)
	if err != nil {
		return err
	}
	if !empSvcCfg.IsActive {
		return ErrNotFound
	}
	if checkClient {
		_, _, _, _, patientExists, err := m.patients.GetPatient(m.tenantID, res.ClientId)
		if err != nil {
			return err
		}
		if !patientExists {
			return ErrClientNotFound
		}
	}
	staffExists, err := m.staff.StaffExists(m.tenantID, empSvcCfg.StaffId)
	if err != nil {
		return err
	}
	if !staffExists {
		return ErrStaffNotFound
	}
	serviceExists, err := m.catalog.ServiceExists(m.tenantID, empSvcCfg.ServiceId)
	if err != nil {
		return err
	}
	if !serviceExists {
		return ErrServiceNotFound
	}
	slot := time.Unix(res.ReservationTime)
	if overbook {
		err = m.checkOverbookSlot(r, empSvcCfg, slot)
	} else {
		err = m.checkFreeSlot(r, empSvcCfg, slot)
	}
	if err != nil {
		return err
	}
	price, currency, err := m.priceFor(empSvcCfg)
	if err != nil {
		return err
	}
	res.StaffIdsnapshot = empSvcCfg.StaffId
	res.ServiceIdsnapshot = empSvcCfg.ServiceId
	res.DurationMinSnapshot = empSvcCfg.DurationMin
	res.PriceSnapshot = price
	res.CurrencySnapshot = currency
	res.Status = StatusPending
	res.Overbooked = overbook
	res.UpdatedAt = time.NowNano()
	res.UpdatedBy = res.CreatorUserId
	res.Revision = 1
	return nil
}

// checkFreeSlot exige que la hora sea el inicio de un horario libre (no choca con otra reserva de esta transacción).
func (m *Module) checkFreeSlot(r *Repository, cfg EmployeeServiceConfig, slot time.Unix) error {
	targetDay := m.dayKey(slot)
	slots, err := m.availability(r, cfg.StaffId, cfg.Id, targetDay-time.Day, targetDay+time.Day)
	if err != nil {
		return err
	}
	for _, s := range slots {
		if s.StartUtc == slot {
			return nil
		}
	}
	return ErrSlotTaken
}

// checkOverbookSlot exige que la hora sea el inicio de un horario habilitado de la agenda (esté libre u ocupado) y
// que el día todavía tenga cupo de sobrecupo (cerrado si MaxOverbookPerDay == 0).
func (m *Module) checkOverbookSlot(r *Repository, cfg EmployeeServiceConfig, slot time.Unix) error {
	targetDay := m.dayKey(slot)
	fromDay, toDay := targetDay-time.Day, targetDay+time.Day
	calCfg, err := m.requireCalendarConfig(r, cfg.StaffId)
	if err != nil {
		return err
	}
	if !calCfg.IsActive {
		return ErrSlotNotInSchedule
	}
	blocks, err := r.ListBlocks(cfg.StaffId)
	if err != nil {
		return err
	}
	exceptions, err := r.ListExceptions(cfg.StaffId, fromDay, toDay)
	if err != nil {
		return err
	}
	slots, err := m.scheduleSlots(fromDay, toDay, blocks, exceptions, calCfg, cfg, nil)
	if err != nil {
		return err
	}
	matches := false
	for _, s := range slots {
		if s.StartUtc == slot {
			matches = true
			break
		}
	}
	if !matches {
		return ErrSlotNotInSchedule
	}
	if calCfg.MaxOverbookPerDay == 0 {
		return ErrOverbookNotAllowed
	}
	fromUtc := m.at(targetDay, 0)
	_, toUtc, _ := m.zone.Day(dateOf(targetDay))
	dayReservations, err := r.ListReservationsByStaff(cfg.StaffId, fromUtc, toUtc)
	if err != nil {
		return err
	}
	overbooked := int64(0)
	for _, res := range dayReservations {
		if res.Overbooked && holdsSlot(res.Status) {
			overbooked++
		}
	}
	if overbooked >= calCfg.MaxOverbookPerDay {
		return ErrOverbookNotAllowed
	}
	return nil
}

// rescheduleOriginal marca como RESCHEDULED la reserva que la nueva reemplaza y la devuelve (para avisar).
func (m *Module) rescheduleOriginal(r *Repository, res *Reservation) (*Reservation, error) {
	if res.RescheduledFromId == "" {
		return nil, nil
	}
	orig, err := r.GetReservation(res.RescheduledFromId)
	if err != nil {
		return nil, err
	}
	if _, err := Transition(orig.Status, EventReschedule); err != nil {
		return nil, err
	}
	if err := r.UpdateReservationStatus(orig.Id, StatusRescheduled, res.CreatorUserId, res.UpdatedAt, orig.Revision); err != nil {
		return nil, err
	}
	orig.Status = StatusRescheduled
	return &orig, nil
}

func (m *Module) CreateReservation(cmd CreateReservationCmd) (Reservation, error) {
	return m.createReservation(cmd, false)
}

func (m *Module) CreateOverbooking(cmd CreateReservationCmd) (Reservation, error) {
	return m.createReservation(cmd, true)
}

func (m *Module) createReservation(cmd CreateReservationCmd, overbook bool) (Reservation, error) {
	if err := m.own(cmd.TenantId); err != nil {
		return Reservation{}, err
	}
	if cmd.Id == "" || cmd.ClientId == "" {
		return Reservation{}, ErrMissingArgs
	}
	var res Reservation
	var original *Reservation
	created := false
	err := m.store.Atomic(func(tx crud.Tx) error {
		r := m.repoOn(tx)
		existing, err := r.GetReservation(cmd.Id)
		if err == nil {
			if existing.ClientId == cmd.ClientId {
				res = existing // el mismo pedido otra vez: devuelve lo guardado, sin anunciarlo de nuevo
				return nil
			}
			return ErrIdTaken
		} else if !isNotFound(err) {
			return err
		}
		res = reservationOf(cmd)
		if err := m.prepareReservation(r, &res, overbook, true); err != nil {
			return err
		}
		if original, err = m.rescheduleOriginal(r, &res); err != nil {
			return err
		}
		created = true
		return tx.Create(&res)
	})
	if err != nil {
		return Reservation{}, err
	}
	if m.pub != nil && created {
		m.pub.Publish(events.Event{Topic: EventReservationCreated, Payload: &res})
		if original != nil {
			m.pub.Publish(events.Event{Topic: EventReservationRescheduled, Payload: original})
		}
	}
	return res, nil
}

func (m *Module) GetReservation(tenantId, id string) (Reservation, error) {
	if err := m.own(tenantId); err != nil {
		return Reservation{}, err
	}
	return m.repoOn(m.store).GetReservation(id)
}

func (m *Module) ListReservationsByStaff(tenantId, staffId string, from, to time.Unix) ([]Reservation, error) {
	if err := m.own(tenantId); err != nil {
		return nil, nil
	}
	fromUtc := m.at(from, 0)
	_, toUtc, _ := m.zone.Day(dateOf(to))
	return m.repoOn(m.store).ListReservationsByStaff(staffId, fromUtc, toUtc)
}

func (m *Module) ListReservationsByClient(tenantId, clientId string) ([]Reservation, error) {
	if err := m.own(tenantId); err != nil {
		return nil, nil
	}
	return m.repoOn(m.store).ListReservationsByClient(clientId)
}

func (m *Module) ChangeReservationStatus(cmd ChangeStatusCmd) error {
	if err := m.own(cmd.TenantId); err != nil {
		return err
	}
	var updated Reservation
	err := m.store.Atomic(func(tx crud.Tx) error {
		r := m.repoOn(tx)
		current, err := r.GetReservation(cmd.Id)
		if err != nil {
			return err
		}
		nextState, err := Transition(current.Status, cmd.Event)
		if err != nil {
			return err
		}
		if cmd.Event == EventConfirm && cmd.PaymentId != "" {
			if current.Revision != int64(cmd.Revision) {
				return ErrConflict
			}
			current.Status, current.UpdatedBy, current.UpdatedAt, current.PaymentId = nextState, cmd.ActorId, time.NowNano(), cmd.PaymentId
			current.Revision++
			updated = current
			return tx.UpdateFields(&current, current.Id, []string{Reservation_.Status, Reservation_.UpdatedBy, Reservation_.UpdatedAt, Reservation_.PaymentId, Reservation_.Revision})
		}
		if err := r.UpdateReservationStatus(cmd.Id, nextState, cmd.ActorId, time.NowNano(), int64(cmd.Revision)); err != nil {
			return err
		}
		updated, err = r.GetReservation(cmd.Id)
		return err
	})
	if err != nil {
		return err
	}
	if domainEvent := domainEventOf(cmd.Event); m.pub != nil && domainEvent != "" {
		m.pub.Publish(events.Event{Topic: domainEvent, Payload: &updated})
	}
	return nil
}

func domainEventOf(event string) string {
	switch event {
	case EventConfirm:
		return EventReservationConfirmed
	case EventCancel:
		return EventReservationCancelled
	case EventComplete:
		return EventReservationCompleted
	case EventNoShow:
		return EventReservationNoShow
	case EventExpire:
		return EventReservationExpired
	}
	return ""
}

// userEventTo busca el evento de usuario que lleva una reserva de from a to (la regla de Update recibe el estado
// nuevo, no el evento). CONFLICT y RESOLVE los pone solo la recomputación y RESCHEDULE nace al crear la nueva.
func userEventTo(from, to string) (string, bool) {
	for _, t := range transitions {
		switch t.Event {
		case EventConfirm, EventCancel, EventComplete, EventNoShow, EventExpire:
			if t.From == from && t.To == to {
				return t.Event, true
			}
		}
	}
	return "", false
}

// ///////////////////////////////////////////////////////////////////////////
// Conflictos (§8)
// ///////////////////////////////////////////////////////////////////////////

// scheduleChange es el cambio de agenda que una regla de crud todavía no escribió cuando pide el recálculo: crud
// guarda la fila después de la regla, así que el recálculo la suma (o quita) a lo que ya está guardado.
type scheduleChange struct {
	staffId         string
	addBlock        *WorkCalendarBlock
	dropBlockId     string
	addException    *WorkCalendarException
	dropExceptionId string
}

func (m *Module) loadSchedule(r *Repository, staffId string, from, to time.Unix, ch scheduleChange) (staffSchedule, error) {
	cfg, err := r.GetCalendarConfig(staffId)
	if err != nil {
		return staffSchedule{}, err
	}
	blocks, err := r.ListBlocks(staffId)
	if err != nil {
		return staffSchedule{}, err
	}
	exceptions, err := r.ListExceptions(staffId, from, to)
	if err != nil {
		return staffSchedule{}, err
	}
	if ch.staffId == staffId {
		kept := blocks[:0:0]
		for _, b := range blocks {
			if b.Id != ch.dropBlockId {
				kept = append(kept, b)
			}
		}
		blocks = kept
		if ch.addBlock != nil {
			blocks = append(blocks, *ch.addBlock)
		}
		keptExc := exceptions[:0:0]
		for _, e := range exceptions {
			if e.Id != ch.dropExceptionId {
				keptExc = append(keptExc, e)
			}
		}
		exceptions = keptExc
		if ch.addException != nil && ch.addException.SpecificDate >= from && ch.addException.SpecificDate <= to {
			exceptions = append(exceptions, *ch.addException)
		}
	}
	return staffSchedule{staffId: staffId, cfg: cfg, blocks: blocks, exceptions: exceptions}, nil
}

// recomputeConflictsFor re-evalúa todas las reservas futuras de [from, to] contra la agenda ACTUAL y los bounds,
// marcando o limpiando CONFLICTED según la respuesta. onlyStaff != "" restringe a un profesional (el caso de las
// ediciones de su propia agenda); "" evalúa a todo el tenant (el caso del establecimiento). Idempotente: llamarlo
// dos veces sin cambios intermedios no cambia nada y no produce eventos (CU-19/CU-29).
func (m *Module) recomputeConflictsFor(r *Repository, from, to time.Unix, onlyStaff string, ch scheduleChange) (recomputeResult, error) {
	if to == 0 {
		to = forwardHorizon(from)
	}
	fromUtc := m.at(from, 0)
	_, toUtc, _ := m.zone.Day(dateOf(to))
	rows, err := r.ListReservationsByTenantRange(fromUtc, toUtc)
	if err != nil {
		return recomputeResult{}, err
	}
	type staffReservs struct {
		staffId string
		rsv     []Reservation
	}
	var groups []staffReservs
	for _, row := range rows {
		if !conflictedCandidate(row) {
			continue
		}
		if onlyStaff != "" && row.StaffIdsnapshot != onlyStaff {
			continue
		}
		found := false
		for i := range groups {
			if groups[i].staffId == row.StaffIdsnapshot {
				groups[i].rsv = append(groups[i].rsv, row)
				found = true
				break
			}
		}
		if !found {
			groups = append(groups, staffReservs{staffId: row.StaffIdsnapshot, rsv: []Reservation{row}})
		}
	}

	var result recomputeResult
	for _, g := range groups {
		sched, err := m.loadSchedule(r, g.staffId, from, to, ch)
		if err != nil {
			return recomputeResult{}, err
		}
		sc := staffChangeCount{staffId: g.staffId}
		for _, res := range g.rsv {
			limits, err := m.limitsForDay(m.dayKey(time.Unix(res.ReservationTime)))
			if err != nil {
				return recomputeResult{}, err
			}
			fits, _ := m.checkReservation(res, sched.blocks, sched.exceptions, sched.cfg, limits)
			if fits {
				if res.Status == StatusConflicted {
					if err := m.clearConflict(r, res); err != nil {
						return recomputeResult{}, err
					}
					sc.cleared++
					result.totalChanged++
				}
				continue
			}
			if res.Status != StatusConflicted {
				if err := m.markConflict(r, res); err != nil {
					return recomputeResult{}, err
				}
				sc.entered++
				result.totalChanged++
				result.changes = append(result.changes, conflictEntry{reservation: res})
			}
		}
		if sc.entered > 0 || sc.cleared > 0 {
			result.staffCounts = append(result.staffCounts, sc)
		}
	}
	return result, nil
}

// markConflict recuerda de dónde venía la reserva en StatusBeforeConflict: sin él, "des-conflictear" tendría que
// adivinar entre PENDING y CONFIRMED (§8.5).
func (m *Module) markConflict(r *Repository, res Reservation) error {
	return r.UpdateReservationConflict(res.Id, StatusConflicted, res.Status, ActorSystem, time.NowNano(), res.Revision)
}

func (m *Module) clearConflict(r *Repository, res Reservation) error {
	prev := res.StatusBeforeConflict
	if prev == "" || prev == StatusConflicted {
		prev = StatusPending
	}
	return r.UpdateReservationConflict(res.Id, prev, "", ActorSystem, time.NowNano(), res.Revision)
}

func (m *Module) publishScheduleChanged(staffId string, from, to time.Unix, conflictCount int) {
	if m.pub == nil {
		return
	}
	m.pub.Publish(events.Event{Topic: EventScheduleChanged, Payload: &ScheduleChangedPayload{
		TenantId: m.tenantID, StaffId: staffId, FromDate: int64(from), ToDate: int64(to), ConflictCount: conflictCount,
	}})
}

// announceEdit avisa de una edición del profesional (§8.2): EventScheduleChanged una vez por profesional, solo si
// alguien entró en conflicto (CU-19), y EventReservationConflicted una vez por reserva (§8.6b): el número de una
// edición es chico y el notificador del paciente necesita el registro individual.
func (m *Module) announceEdit(result recomputeResult, from, to time.Unix) {
	for _, sc := range result.staffCounts {
		if sc.entered > 0 {
			m.publishScheduleChanged(sc.staffId, from, to, sc.entered)
		}
	}
	if m.pub != nil {
		for i := range result.changes {
			m.pub.Publish(events.Event{Topic: EventReservationConflicted, Payload: &result.changes[i].reservation})
		}
	}
}

// ListConflictingReservations returns future reservations that the CURRENT schedule no longer covers. from is
// normally "now". Son las que una edición de agenda o del establecimiento dejó huérfanas — el worklist de CU-17.
// Reported, never cancelled: cancelling on the patient's behalf is a decision for a person.
func (m *Module) ListConflictingReservations(tenantId, staffId string, from time.Unix) ([]ConflictingReservation, error) {
	if err := m.own(tenantId); err != nil {
		return nil, nil
	}
	r := m.repoOn(m.store)
	to := forwardHorizon(from)
	fromUtc := m.at(from, 0)
	_, toUtc, _ := m.zone.Day(dateOf(to))
	rows, err := r.ListReservationsByStaff(staffId, fromUtc, toUtc)
	if err != nil {
		return nil, err
	}
	sched, err := m.loadSchedule(r, staffId, from, to, scheduleChange{})
	if err != nil {
		return nil, err
	}
	var out []ConflictingReservation
	for _, res := range rows {
		if !conflictedCandidate(res) {
			continue
		}
		limits, err := m.limitsForDay(m.dayKey(time.Unix(res.ReservationTime)))
		if err != nil {
			return nil, err
		}
		fits, reason := m.checkReservation(res, sched.blocks, sched.exceptions, sched.cfg, limits)
		if !fits {
			out = append(out, ConflictingReservation{
				ReservationId: res.Id, StartsAt: time.Unix(res.ReservationTime), ClientId: res.ClientId, Reason: reason,
			})
		}
	}
	return out, nil
}

// RecomputeConflicts re-evaluates every future reservation in [from, to] against the CURRENT schedule and bounds,
// marking or clearing CONFLICTED as the answer dictates. Returns how many reservations changed state.
//
// It is idempotent: calling it twice with no intervening change is a no-op and publishes nothing (CU-19/CU-29).
// from/to are midnight UTC seconds; to == 0 means "the module's full forward horizon".
func (m *Module) RecomputeConflicts(tenantId string, from, to time.Unix) (int, error) {
	if err := m.own(tenantId); err != nil {
		return 0, err
	}
	var result recomputeResult
	err := m.store.Atomic(func(tx crud.Tx) error {
		var err error
		result, err = m.recomputeConflictsFor(m.repoOn(tx), from, to, "", scheduleChange{})
		return err
	})
	if err != nil {
		return 0, err
	}
	m.announceEstablishment(result, from, to)
	return result.totalChanged, nil
}

// announceEstablishment avisa de un recálculo de todo el establecimiento: un feriado puede conflictar cientos de
// reservas en decenas de profesionales, así que va UN resumen por profesional y nunca un evento por reserva (§8.6).
func (m *Module) announceEstablishment(result recomputeResult, from, to time.Unix) {
	for _, sc := range result.staffCounts {
		if sc.entered > 0 {
			m.publishScheduleChanged(sc.staffId, from, to, sc.entered)
		}
	}
}

// ///////////////////////////////////////////////////////////////////////////
// Piezas compartidas por las reglas de bloques y excepciones
// ///////////////////////////////////////////////////////////////////////////

// ruleRange es el rango de fechas que un bloque cambia: un bloque semanal toca todo el horizonte desde hoy.
func ruleRange(b WorkCalendarBlock) (from, to time.Unix) {
	if b.SpecificDate > 0 {
		return b.SpecificDate, b.SpecificDate
	}
	from = time.MidnightUTC(time.NowUnix())
	return from, forwardHorizon(from)
}

// checkNewBlock valida un bloque que una regla va a guardar y lo normaliza: el rango, el calendario del
// profesional, las horas del establecimiento y que no se solape con otro bloque del mismo día. skipId es el propio
// bloque cuando se está modificando.
func (m *Module) checkNewBlock(r *Repository, b *WorkCalendarBlock, skipId string) error {
	if _, err := m.requireCalendarConfig(r, b.StaffId); err != nil {
		return err
	}
	if err := validateBlocks([]WorkCalendarBlock{*b}); err != nil {
		return err
	}
	var limits time.DayLimits
	var err error
	if b.SpecificDate > 0 {
		b.DayOfWeek = int64(time.WeekdayUTC(b.SpecificDate))
		limits, err = m.limitsForDay(b.SpecificDate)
	} else {
		if b.DayOfWeek < 0 || b.DayOfWeek > 6 {
			return ErrInvalidBlock
		}
		limits, err = m.limitsForWeekday(int(b.DayOfWeek))
	}
	if err != nil {
		return err
	}
	if err := checkBlockAgainstBounds(limits, *b); err != nil {
		return err
	}
	same, err := r.ListBlocks(b.StaffId)
	if err != nil {
		return err
	}
	for _, other := range same {
		if other.Id == skipId || other.SpecificDate != b.SpecificDate || (b.SpecificDate == 0 && other.DayOfWeek != b.DayOfWeek) {
			continue
		}
		if b.StartMin < other.EndMin && other.StartMin < b.EndMin {
			return ErrBlocksOverlap
		}
	}
	return nil
}

// recomputeForRule recalcula los conflictos que un cambio de agenda provoca, dentro de la transacción de la regla
// que lo pidió, y avisa. La regla de crud guarda su fila después: ch dice qué fila falta o sobra.
func (m *Module) recomputeForRule(r *Repository, staffId string, from, to time.Unix, ch scheduleChange) error {
	ch.staffId = staffId
	result, err := m.recomputeConflictsFor(r, from, to, staffId, ch)
	if err != nil {
		return err
	}
	m.announceEdit(result, from, to)
	return nil
}

// expireIn vence las reservas PENDING cuya hora ya pasó o cuya retención online venció (§7) dentro de r, y
// devuelve cuántas venció.
func (m *Module) expireIn(r *Repository, now time.Unix) ([]Reservation, error) {
	pending, err := r.ListReservationsByStatus(StatusPending)
	if err != nil {
		return nil, err
	}
	var expired []Reservation
	for _, res := range pending {
		pastAppointment := time.Unix(res.ReservationTime) < now // X1
		lapsedHold := res.HoldUntil > 0 && res.HoldUntil < now  // X2
		if !pastAppointment && !lapsedHold {
			continue
		}
		if err := r.UpdateReservationStatus(res.Id, StatusExpired, ActorSystem, time.NowNano(), res.Revision); err != nil {
			return nil, err
		}
		res.Status = StatusExpired
		expired = append(expired, res)
	}
	return expired, nil
}

