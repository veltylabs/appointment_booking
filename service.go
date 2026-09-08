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
	ErrInvalidBlock           = fmt.Err("appointment_booking: start_min must be < end_min and both within 0..1439")
	ErrBlocksOverlap          = fmt.Err("appointment_booking: two blocks of the same day overlap")
	ErrBlockOutsideBusinessHours = fmt.Err("appointment_booking: block falls outside the establishment's opening hours")
	ErrBlockOnClosedDay          = fmt.Err("appointment_booking: the establishment is closed on that date")
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

// BoundsReader answers "which minutes of this date are usable at all".
//
// Declared here, not imported: a booking module must not depend on any
// particular institutional calendar. Anything that can answer the question
// satisfies it — veltylabs/business_calendar does, and so would a static
// config or a different organisation's calendar.
//
// date is midnight UTC in seconds.
type BoundsReader interface {
	GetDayBounds(date int64) (tinytime.DayBounds, error)
}

type SchedulingService interface {
	// Gestión de calendario
	UpsertCalendarConfig(cfg WorkCalendarConfig) error
	SaveDayBlocks(tenantId, staffId string, dayOfWeek int, blocks []WorkCalendarBlock) error
	SaveDateBlocks(tenantId, staffId string, date int64, blocks []WorkCalendarBlock) error
	MarkWorkingDays(tenantId, staffId string, dates []int64, startMin, endMin int) error
	UnmarkWorkingDays(tenantId, staffId string, dates []int64) error
	ListBlocks(tenantId, staffId string) ([]WorkCalendarBlock, error)
	AddException(exc WorkCalendarException) error
	RemoveException(tenantId, exceptionId string) error
	ListExceptions(tenantId, staffId string, from, to int64) ([]WorkCalendarException, error)
	GetDayBounds(date int64) (tinytime.DayBounds, error)

	// Disponibilidad
	ListAvailability(tenantId, staffId, configId string, from, to int64) ([]TimeSlot, error)

	// Reservas
	CreateReservation(cmd CreateReservationCmd) (Reservation, error)
	GetReservation(tenantId, id string) (Reservation, error)
	ListReservationsByStaff(tenantId, staffId string, from, to int64) ([]Reservation, error)
	ListReservationsByClient(tenantId, clientId string) ([]Reservation, error)
	ChangeReservationStatus(cmd ChangeStatusCmd) error
	ExpirePendingReservations(tenantId string, before int64) (int, error)

	// Conflictos
	ListConflictingReservations(tenantId, staffId string, from int64) ([]ConflictingReservation, error)
	RecomputeConflicts(tenantId string, from, to int64) (int, error)
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
	bounds    BoundsReader
}

type Deps struct {
	Staff     StaffReader
	Catalog   CatalogReader
	Directory DirectoryReader
	IDs       model.IDGenerator // requerido
	Publisher events.Publisher  // opcional — nil desactiva
	// Bounds constrains every block to the institution's usable window.
	// OPTIONAL: nil means unbounded, which is correct for an app with no
	// institution above the professional (a freelancer's booking page).
	Bounds BoundsReader
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
		bounds:    deps.Bounds,
	}, nil
}

var _ SchedulingService = (*Module)(nil)

func (m *Module) UpsertCalendarConfig(cfg WorkCalendarConfig) error {
	return m.repo.UpsertCalendarConfig(cfg)
}

func (m *Module) requireCalendarConfig(tenantId, staffId string) (WorkCalendarConfig, error) {
	cfg, err := m.repo.GetCalendarConfig(tenantId, staffId)
	if err != nil {
		if err == ErrNotFound {
			return WorkCalendarConfig{}, ErrCalendarConfigNotFound
		}
		return WorkCalendarConfig{}, err
	}
	return cfg, nil
}

// boundsForDay es el ÚNICO punto del módulo que consulta BoundsReader. Un solo
// nil-check de m.bounds en todo el módulo (regla §6.1): nil = Unbounded, y casi
// todo el código que viene después ya siempre tiene bounds.
func (m *Module) boundsForDay(d int64) (tinytime.DayBounds, error) {
	if m.bounds == nil {
		return tinytime.Unbounded(), nil
	}
	return m.bounds.GetDayBounds(d)
}

// GetDayBounds proxies Deps.Bounds para que el editor acote sus propios controles
// llamando a este módulo, no al calendario institucional directamente (§6.2).
func (m *Module) GetDayBounds(date int64) (tinytime.DayBounds, error) {
	return m.boundsForDay(date)
}

// validateBlocks aplica las constraints de rango y solapamiento a un conjunto de
// bloques de UN día. Las escribiré todas con start < end y dentro de 0..1439.
func validateBlocks(blocks []WorkCalendarBlock) error {
	for i := range blocks {
		b := &blocks[i]
		if b.StartMin < 0 || b.StartMin >= 1440 || b.EndMin <= 0 || b.EndMin > 1440 || b.StartMin >= b.EndMin {
			return ErrInvalidBlock
		}
	}
	for i := 0; i < len(blocks); i++ {
		for j := i + 1; j < len(blocks); j++ {
			a, c := &blocks[i], &blocks[j]
			if a.StartMin < c.EndMin && c.StartMin < a.EndMin {
				return ErrBlocksOverlap
			}
		}
	}
	return nil
}

// checkBlockAgainstBounds rechaza un bloque que el establecimiento no cubre:
// día cerrado (ErrBlockOnClosedDay) o ventana fuera de las horas de apertura
// (ErrBlockOutsideBusinessHours). La UI ya solo ofrece horas legales (CU-02),
// pero las ops pasan por router y la UI no puede ser la única defensa (CU-07).
func checkBlockAgainstBounds(bounds tinytime.DayBounds, b WorkCalendarBlock) error {
	if !bounds.Open {
		return ErrBlockOnClosedDay
	}
	if _, _, ok := bounds.Clamp(int(b.StartMin), int(b.EndMin)); !ok {
		return ErrBlockOutsideBusinessHours
	}
	return nil
}

func validateBlockSet(bounds tinytime.DayBounds, blocks []WorkCalendarBlock) error {
	for i := range blocks {
		if err := checkBlockAgainstBounds(bounds, blocks[i]); err != nil {
			return err
		}
	}
	return nil
}

// nextWeekday devuelve la fecha (medianoche UTC) de la próxima ocurrencia de dow
// a partir de now. Es la fecha representativa que un bloque WEEKLY se valida
// contra: un template semanal aplica a muchas fechas, y esta es la única
// elección determinista que un BoundsReader con datos iguales cubre.
func nextWeekday(now int64, dow int) int64 {
	start := tinytime.MidnightUTC(now)
	for i := 0; i < 7; i++ {
		d := start + int64(i*86400)
		if tinytime.Weekday(d) == dow {
			return d
		}
	}
	return start
}

// unixNowSeconds es Now() en segundos epoch — MidnightUTC/Weekday/etc. del
// paquete webtyp.com/time operan en segundos, mientras que Now() entrega nanos.
func unixNowSeconds() int64 { return tinytime.Now() / 1000000000 }

// ///////////////////////////////////////////////////////////////////////////
// Stage 4 — write API
// ///////////////////////////////////////////////////////////////////////////

// SaveDayBlocks replaces EVERY weekly block of that weekday with the ones
// given. A whole-day replace, not a per-row upsert: partial edits are what let
// the stored set drift out of step with what the editor is showing.
func (m *Module) SaveDayBlocks(tenantId, staffId string, dayOfWeek int, blocks []WorkCalendarBlock) error {
	if dayOfWeek < 0 || dayOfWeek > 6 {
		return ErrInvalidBlock
	}
	if _, err := m.requireCalendarConfig(tenantId, staffId); err != nil {
		return err
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
	bounds, err := m.boundsForDay(nextWeekday(unixNowSeconds(), dayOfWeek))
	if err != nil {
		return err
	}
	if err := validateBlockSet(bounds, blocks); err != nil {
		return err
	}
	if err := m.repo.ReplaceWeekdayBlocks(tenantId, staffId, dayOfWeek, blocks); err != nil {
		return err
	}
	from := tinytime.MidnightUTC(unixNowSeconds())
	return m.recomputeAndPublish(tenantId, staffId, from, forwardHorizon(from))
}

// SaveDateBlocks replaces the dated blocks of ONE date, so a single marked day
// can diverge from the common window it was created with (CU-11).
func (m *Module) SaveDateBlocks(tenantId, staffId string, date int64, blocks []WorkCalendarBlock) error {
	if _, err := m.requireCalendarConfig(tenantId, staffId); err != nil {
		return err
	}
	if err := validateBlocks(blocks); err != nil {
		return err
	}
	for i := range blocks {
		blocks[i].TenantId = tenantId
		blocks[i].StaffId = staffId
		blocks[i].DayOfWeek = int64(tinytime.Weekday(date))
		blocks[i].SpecificDate = date
		blocks[i].IsActive = true
	}
	bounds, err := m.boundsForDay(date)
	if err != nil {
		return err
	}
	if err := validateBlockSet(bounds, blocks); err != nil {
		return err
	}
	if err := m.repo.ReplaceDateBlocks(tenantId, staffId, date, blocks); err != nil {
		return err
	}
	return m.recomputeAndPublish(tenantId, staffId, date, date)
}

// MarkWorkingDays writes ONE dated block per date, all with the same window —
// the bulk gesture behind "mark the days I work over the next 6 months"
// (CU-10/CU-11). Re-marking a date replaces its blocks.
func (m *Module) MarkWorkingDays(tenantId, staffId string, dates []int64, startMin, endMin int) error {
	if startMin < 0 || startMin >= endMin || endMin > 1440 {
		return ErrInvalidBlock
	}
	if len(dates) == 0 {
		return nil
	}
	from, to := dates[0], dates[0]
	for _, d := range dates {
		if d < from {
			from = d
		}
		if d > to {
			to = d
		}
	}
	if _, err := m.requireCalendarConfig(tenantId, staffId); err != nil {
		return err
	}
	for _, d := range dates {
		bounds, err := m.boundsForDay(d)
		if err != nil {
			return err
		}
		if err := checkBlockAgainstBounds(bounds, WorkCalendarBlock{StartMin: int64(startMin), EndMin: int64(endMin)}); err != nil {
			return err
		}
	}
	for _, d := range dates {
		if err := m.repo.ReplaceDateBlocks(tenantId, staffId, d, []WorkCalendarBlock{{
			TenantId: tenantId, StaffId: staffId, DayOfWeek: int64(tinytime.Weekday(d)),
			SpecificDate: d, StartMin: int64(startMin), EndMin: int64(endMin), IsActive: true,
		}}); err != nil {
			return err
		}
	}
	return m.recomputeAndPublish(tenantId, staffId, from, to)
}

// UnmarkWorkingDays deletes every dated block on those dates, returning the
// days to whatever the weekly template says — or to unworked (CU-15).
func (m *Module) UnmarkWorkingDays(tenantId, staffId string, dates []int64) error {
	if len(dates) == 0 {
		return nil
	}
	from, to := dates[0], dates[0]
	for _, d := range dates {
		if d < from {
			from = d
		}
		if d > to {
			to = d
		}
	}
	if _, err := m.requireCalendarConfig(tenantId, staffId); err != nil {
		return err
	}
	for _, d := range dates {
		if err := m.repo.DeleteDateBlocks(tenantId, staffId, d); err != nil {
			return err
		}
	}
	return m.recomputeAndPublish(tenantId, staffId, from, to)
}

func (m *Module) ListBlocks(tenantId, staffId string) ([]WorkCalendarBlock, error) {
	return m.repo.ListBlocks(tenantId, staffId)
}

func (m *Module) ListExceptions(tenantId, staffId string, from, to int64) ([]WorkCalendarException, error) {
	return m.repo.ListExceptions(tenantId, staffId, from, to)
}

func (m *Module) AddException(exc WorkCalendarException) error {
	if err := m.repo.InsertException(exc); err != nil {
		return err
	}
	from := tinytime.MidnightUTC(exc.SpecificDate)
	return m.recomputeAndPublish(exc.TenantId, exc.StaffId, from, from)
}

func (m *Module) RemoveException(tenantId, exceptionId string) error {
	exc, err := m.repo.GetException(tenantId, exceptionId)
	if err != nil {
		return err
	}
	if err := m.repo.DeleteException(tenantId, exceptionId); err != nil {
		return err
	}
	from := tinytime.MidnightUTC(exc.SpecificDate)
	return m.recomputeAndPublish(tenantId, exc.StaffId, from, from)
}

// LocalIntToUnixUTC interpreta localInt como minutos desde la medianoche en la fecha dada (medianoche UTC) en la zona horaria (tz) especificada.
func LocalIntToUnixUTC(date int64, localInt int, tz string) int64 {
	return tinytime.LocalMinutesToUnixUTC(date, localInt, tz)
}

// dayWindow es una ventana UTC reservable de un día.
type dayWindow struct {
	StartUtc int64
	EndUtc   int64
}

// blocksForDate — los bloques DATADOS de la fecha exacta. Un bloque datado ABRE
// su día aunque el template semanal no lo cubra — es la inversión de la vieja
// compuerta de días de ListAvailability (CU-10).
func blocksForDate(blocks []WorkCalendarBlock, date int64) []WorkCalendarBlock {
	var out []WorkCalendarBlock
	for _, b := range blocks {
		if b.SpecificDate == date {
			out = append(out, b)
		}
	}
	return out
}

// blocksForWeekday — los bloques WEEKLY del día de la semana. Sin map: scan
// lineal sobre un conjunto pequeño (regla "cero map" de AGENTS.md).
func blocksForWeekday(blocks []WorkCalendarBlock, dow int) []WorkCalendarBlock {
	var out []WorkCalendarBlock
	for _, b := range blocks {
		if b.SpecificDate == 0 && int(b.DayOfWeek) == dow {
			out = append(out, b)
		}
	}
	return out
}

func dayExceptionsFor(exceptions []WorkCalendarException, d int64) []WorkCalendarException {
	var out []WorkCalendarException
	for _, e := range exceptions {
		if e.SpecificDate == d {
			out = append(out, e)
		}
	}
	return out
}

// subtractWindow recorta [s, e) de las ventanas — la operación con la que una
// excepción BLOCKED deja agujeros en el día. Divide en dos la ventana solapada.
func subtractWindow(windows []dayWindow, s, e int64) []dayWindow {
	var out []dayWindow
	for _, w := range windows {
		if e <= w.StartUtc || s >= w.EndUtc {
			out = append(out, w)
			continue
		}
		if s > w.StartUtc {
			out = append(out, dayWindow{StartUtc: w.StartUtc, EndUtc: s})
		}
		if e < w.EndUtc {
			out = append(out, dayWindow{StartUtc: e, EndUtc: w.EndUtc})
		}
	}
	return out
}

// availableRanges devuelve las ventanas UTC utilizables de un día: bloques
// (datados primero, luego semanales) recortadas a bounds, tras aplicar
// HOLIDAY / SPECIAL_HOURS / BLOCKED. Es la ÚNICA regla "qué rango de minutos de
// este día es reservable" — ListAvailability y la detección de conflictos la
// comparten (anti-drifting, §8.1). bounds debe venir resuelta (nunca el zero
// value): el caller usa boundsForDay.
func (m *Module) availableRanges(d int64, blocks []WorkCalendarBlock, exceptions []WorkCalendarException, cfg WorkCalendarConfig, bounds tinytime.DayBounds) []dayWindow {
	if !bounds.Open {
		return nil
	}
	dayExcs := dayExceptionsFor(exceptions, d)
	var specialHours *WorkCalendarException
	blockedExcs := []WorkCalendarException{}
	for i := range dayExcs {
		e := &dayExcs[i]
		switch e.ExceptionType {
		case ExcHoliday:
			return nil
		case ExcSpecialHours:
			if specialHours == nil {
				specialHours = e
			}
		case ExcBlocked:
			blockedExcs = append(blockedExcs, *e)
		}
	}

	// SPECIAL_HOURS acota el día a una única ventana (comportamiento
	// preexistente preservado): sigue útil para NARROW un día que un bloque
	// semanal/datado cubre. Abrir un día ya no necesita esta excepción.
	if specialHours != nil {
		start, end, ok := bounds.Clamp(int(specialHours.StartTime), int(specialHours.EndTime))
		if !ok {
			return nil
		}
		return []dayWindow{{
			StartUtc: LocalIntToUnixUTC(d, start, cfg.Timezone),
			EndUtc:   LocalIntToUnixUTC(d, end, cfg.Timezone),
		}}
	}

	dayBlocks := blocksForDate(blocks, d)
	if len(dayBlocks) == 0 {
		dayBlocks = blocksForWeekday(blocks, tinytime.Weekday(d))
	}
	if len(dayBlocks) == 0 {
		return nil
	}

	var out []dayWindow
	for _, b := range dayBlocks {
		if !b.IsActive {
			continue
		}
		start, end, ok := bounds.Clamp(int(b.StartMin), int(b.EndMin))
		if !ok {
			continue
		}
		out = append(out, dayWindow{
			StartUtc: LocalIntToUnixUTC(d, start, cfg.Timezone),
			EndUtc:   LocalIntToUnixUTC(d, end, cfg.Timezone),
		})
	}

	// Restar excepciones BLOCKED del día.
	for _, be := range blockedExcs {
		out = subtractWindow(out,
			LocalIntToUnixUTC(d, int(be.StartTime), cfg.Timezone),
			LocalIntToUnixUTC(d, int(be.EndTime), cfg.Timezone),
		)
	}
	return out
}

// fitsWindow reporta si un slot [startUtc, startUtc+span) cabe dentro de alguna
// de las ventanas (half-open: end == window end cuenta como dentro).
func fitsWindow(windows []dayWindow, startUtc, span int64) bool {
	end := startUtc + span
	for _, w := range windows {
		if w.StartUtc <= startUtc && end <= w.EndUtc {
			return true
		}
	}
	return false
}

func (m *Module) ListAvailability(tenantId, staffId, configId string, from, to int64) ([]TimeSlot, error) {
	cfg, err := m.requireCalendarConfig(tenantId, staffId)
	if err != nil {
		return nil, err
	}
	if !cfg.IsActive {
		return []TimeSlot{}, nil
	}

	blocks, err := m.repo.ListBlocks(tenantId, staffId)
	if err != nil {
		return nil, err
	}

	exceptions, err := m.repo.ListExceptions(tenantId, staffId, from, to)
	if err != nil {
		return nil, err
	}

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

	// Para cada día D en [from, to] (timestamps de medianoche UTC): bloques
	// datados PRIMERO (abren su día aunque el template semanal no lo cubra),
	// luego semanales; recortados a bounds; y generar slots dentro de cada
	// ventana, saltando los huecos entre bloques.
	for d := from; d <= to; d += 86400 {
		bounds, err := m.boundsForDay(d)
		if err != nil {
			return nil, err
		}
		if !bounds.Open {
			continue
		}
		ranges := m.availableRanges(d, blocks, exceptions, cfg, bounds)
		for _, win := range ranges {
			curr := win.StartUtc
			for {
				end := curr + int64(durationMin*60)
				endWithBuffer := end + int64(bufferMin*60)
				if endWithBuffer > win.EndUtc {
					break
				}

				// Verificar reservas existentes
				hasOverlap := false
				var resEnd int64
				for _, res := range activeReservations {
					rStart := res.ReservationTime
					rEnd := rStart + int64(res.DurationMinSnapshot*60)
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

// ///////////////////////////////////////////////////////////////////////////
// Stage 5 — conflicts (§8)
// ///////////////////////////////////////////////////////////////////////////

// ConflictHorizonDays es el forward horizon del módulo: cuando no hay rango que
// acote (un cambio semanal, o to == 0), los conflictos se evalúan hasta aquí.
const ConflictHorizonDays = 370

// forwardHorizon convierte un "from" (medianoche UTC) al borde superior del
// horizonte del módulo.
func forwardHorizon(from int64) int64 {
	return from + int64(ConflictHorizonDays*86400)
}

// Motivos de ConflictingReservation.Reason — el vocabulario NEUTRO del módulo.
// Concretamente NO arrastra la razón del establecimiento (por qué un día está
// cerrado): time.DayBounds lo omite deliberadamente y este módulo lo respeta.
const (
	ConflictReasonOutsideBlocks        = "OUTSIDE_BLOCKS"
	ConflictReasonDayClosed            = "DAY_CLOSED"
	ConflictReasonOutsideBusinessHours = "OUTSIDE_BUSINESS_HOURS"
)

// conflictedCandidate — las reservas que un recompute evalúa. Las terminales
// (CANCELLED/COMPLETED/NO_SHOW/EXPIRED/RESCHEDULED) no cambian de estado nunca.
func conflictedCandidate(r Reservation) bool {
	return r.Status == StatusPending || r.Status == StatusConfirmed || r.Status == StatusConflicted
}

// checkReservation aplica la ÚNICA regla "¿este instante aún se puede
// reservar?" a una reserva existente, usando la misma availableRanges que
// ListAvailability (anti-drifting, §8.1). Devuelve (fits, reason):
//
//   - !bounds.Open                                   → DAY_CLOSED
//   - no cabe en los bloques del profesional (sin clamp) → OUTSIDE_BLOCKS
//   - cabe en bloques pero no en las horas del establecimiento → OUTSIDE_BUSINESS_HOURS
//   - cabe en todo                                            → (true, "")
func (m *Module) checkReservation(r Reservation, blocks []WorkCalendarBlock, exceptions []WorkCalendarException, cfg WorkCalendarConfig, bounds tinytime.DayBounds) (bool, string) {
	d := r.ReservationDate
	if !bounds.Open {
		return false, ConflictReasonDayClosed
	}
	span := int64(r.DurationMinSnapshot * 60)
	raw := m.availableRanges(d, blocks, exceptions, cfg, tinytime.Unbounded())
	if !fitsWindow(raw, r.ReservationTime, span) {
		return false, ConflictReasonOutsideBlocks
	}
	clamped := m.availableRanges(d, blocks, exceptions, cfg, bounds)
	if !fitsWindow(clamped, r.ReservationTime, span) {
		return false, ConflictReasonOutsideBusinessHours
	}
	return true, ""
}

// conflictEntry es una reserva que ENTRÓ en conflicto en un recompute (para el
// evento por-reserva de las ediciones del profesional).
type conflictEntry struct {
	reservation Reservation
}

// staffChangeCount agrega por staff los cambios de un recompute. Slice, no map.
type staffChangeCount struct {
	staffId string
	entered int
	cleared int
}

type recomputeResult struct {
	changes      []conflictEntry
	staffCounts  []staffChangeCount
	totalChanged int
}

// staffSchedule es la foto de la agenda de UN staff que un recompute necesita.
type staffSchedule struct {
	staffId    string
	cfg        WorkCalendarConfig
	blocks     []WorkCalendarBlock
	exceptions []WorkCalendarException
}

func (m *Module) loadSchedule(tenantId, staffId string, from, to int64) (staffSchedule, error) {
	cfg, err := m.repo.GetCalendarConfig(tenantId, staffId)
	if err != nil {
		return staffSchedule{}, err
	}
	blocks, err := m.repo.ListBlocks(tenantId, staffId)
	if err != nil {
		return staffSchedule{}, err
	}
	exceptions, err := m.repo.ListExceptions(tenantId, staffId, from, to)
	if err != nil {
		return staffSchedule{}, err
	}
	return staffSchedule{staffId: staffId, cfg: cfg, blocks: blocks, exceptions: exceptions}, nil
}

// recomputeConflictsFor re-evalúa todas las reservas futuras de [from, to]
// contra la agenda ACTUAL y los bounds, marcando o limpiando CONFLICTED según
// la respuesta. onlyStaff != "" restringe a un profesional (el caso de las
// ediciones de su propia agenda); "" evalúa a todo el tenant (el caso del
// establecimiento). Idempotente: llamarlo dos veces sin cambios intermedios no
// cambia nada y no produce eventos (CU-19/CU-29).
func (m *Module) recomputeConflictsFor(tenantId string, from, to int64, onlyStaff string) (recomputeResult, error) {
	if to == 0 {
		to = forwardHorizon(from)
	}
	rows, err := m.repo.ListReservationsByTenantRange(tenantId, from, to)
	if err != nil {
		return recomputeResult{}, err
	}

	type staffReservs struct {
		staffId string
		rsv     []Reservation
	}
	var groups []staffReservs
	for _, r := range rows {
		if !conflictedCandidate(r) {
			continue
		}
		if onlyStaff != "" && r.StaffIdsnapshot != onlyStaff {
			continue
		}
		found := false
		for i := range groups {
			if groups[i].staffId == r.StaffIdsnapshot {
				groups[i].rsv = append(groups[i].rsv, r)
				found = true
				break
			}
		}
		if !found {
			groups = append(groups, staffReservs{staffId: r.StaffIdsnapshot, rsv: []Reservation{r}})
		}
	}

	var result recomputeResult
	for _, g := range groups {
		sched, err := m.loadSchedule(tenantId, g.staffId, from, to)
		if err != nil {
			return recomputeResult{}, err
		}
		var sc staffChangeCount
		sc.staffId = g.staffId
		for _, r := range g.rsv {
			bounds, err := m.boundsForDay(r.ReservationDate)
			if err != nil {
				return recomputeResult{}, err
			}
			fits, _ := m.checkReservation(r, sched.blocks, sched.exceptions, sched.cfg, bounds)
			if fits {
				if r.Status == StatusConflicted {
					if err := m.clearConflict(tenantId, r); err != nil {
						return recomputeResult{}, err
					}
					sc.cleared++
					result.totalChanged++
				}
				continue
			}
			if r.Status != StatusConflicted {
				if err := m.markConflict(tenantId, r); err != nil {
					return recomputeResult{}, err
				}
				sc.entered++
				result.totalChanged++
				result.changes = append(result.changes, conflictEntry{reservation: r})
			}
		}
		if sc.entered > 0 || sc.cleared > 0 {
			result.staffCounts = append(result.staffCounts, sc)
		}
	}
	return result, nil
}

func (m *Module) markConflict(tenantId string, r Reservation) error {
	now := tinytime.Now()
	return m.db.Tx(func(tx *orm.DB) error {
		// StatusBeforeConflict recuerda de dónde venía: sin él, "des-conflictear"
		// tendría que adivinar entre PENDING y CONFIRMED (§8.5).
		return m.repo.UpdateReservationConflictTx(tx, r.Id, tenantId, StatusConflicted, r.Status, "system", now, r.Revision)
	})
}

func (m *Module) clearConflict(tenantId string, r Reservation) error {
	prev := r.StatusBeforeConflict
	if prev == "" || prev == StatusConflicted {
		prev = StatusPending
	}
	now := tinytime.Now()
	return m.db.Tx(func(tx *orm.DB) error {
		return m.repo.UpdateReservationConflictTx(tx, r.Id, tenantId, prev, "", "system", now, r.Revision)
	})
}

func (m *Module) publishScheduleChanged(tenantId, staffId string, from, to int64, conflictCount int) {
	if m.pub == nil {
		return
	}
	m.pub.Publish(events.Event{Topic: EventScheduleChanged, Payload: &ScheduleChangedPayload{
		TenantId: tenantId, StaffId: staffId, FromDate: from, ToDate: to, ConflictCount: conflictCount,
	}})
}

// recomputeAndPublish es el camino de las EDICIONES DEL PROFESIONAL (§8.2):
// recomputa el rango afectado de UN staff y publica.
//
//   - EventScheduleChanged una vez por staff, con el ConflictCount (solo si
//     alguien entró en conflicto — CU-19).
//   - EventReservationConflicted UNA vez por reserva que entró en conflicto
//     (§8.6b): el count de una edición es pequeño y el notificador del paciente
//     necesita el registro individual.
func (m *Module) recomputeAndPublish(tenantId, staffId string, from, to int64) error {
	result, err := m.recomputeConflictsFor(tenantId, from, to, staffId)
	if err != nil {
		return err
	}
	for _, sc := range result.staffCounts {
		if sc.entered == 0 {
			continue // CU-19: sin afectados ⇒ silencio
		}
		m.publishScheduleChanged(tenantId, sc.staffId, from, to, sc.entered)
	}
	if m.pub != nil {
		for _, c := range result.changes {
			m.pub.Publish(events.Event{Topic: EventReservationConflicted, Payload: &c.reservation})
		}
	}
	return nil
}

// ListConflictingReservations returns future reservations that the CURRENT
// schedule no longer covers. from is normally "now". Son las que una edición de
// agenda o del establecimiento dejó huérfanas — el worklist de CU-17.
// Reported, never cancelled: cancelling on the patient's behalf is a decision
// for a person.
func (m *Module) ListConflictingReservations(tenantId, staffId string, from int64) ([]ConflictingReservation, error) {
	to := forwardHorizon(from)
	rows, err := m.repo.ListReservationsByStaff(tenantId, staffId, from, to)
	if err != nil {
		return nil, err
	}
	sched, err := m.loadSchedule(tenantId, staffId, from, to)
	if err != nil {
		return nil, err
	}
	var out []ConflictingReservation
	for _, r := range rows {
		if !conflictedCandidate(r) {
			continue
		}
		bounds, err := m.boundsForDay(r.ReservationDate)
		if err != nil {
			return nil, err
		}
		fits, reason := m.checkReservation(r, sched.blocks, sched.exceptions, sched.cfg, bounds)
		if !fits {
			out = append(out, ConflictingReservation{
				ReservationId: r.Id,
				StartsAt:      r.ReservationTime,
				ClientId:      r.ClientId,
				Reason:        reason,
			})
		}
	}
	return out, nil
}

// RecomputeConflicts re-evaluates every future reservation in [from, to]
// against the CURRENT schedule and bounds, marking or clearing CONFLICTED as
// the answer dictates. Returns how many reservations changed state.
//
// It is idempotent: calling it twice with no intervening change is a no-op and
// publishes nothing (CU-19/CU-29 depend on that).
//
// tenantId scopes it; from/to are midnight UTC seconds. to == 0 means "the
// module's full forward horizon", which is what a weekly-hours change needs
// since it has no bounded date range.
//
// El disparador queda INVERTIDO (§8.4): no hay Subscriber aquí — la aplicación,
// que legítimamente conoce ambos módulos, se suscribe al calendario del
// establecimiento y llama a esto.
func (m *Module) RecomputeConflicts(tenantId string, from, to int64) (int, error) {
	result, err := m.recomputeConflictsFor(tenantId, from, to, "")
	if err != nil {
		return 0, err
	}
	for _, sc := range result.staffCounts {
		if sc.entered == 0 {
			continue // CU-19
		}
		// Un feriado puede conflictar cientos de reservas en decenas de
		// profesionales: UN summary por staff, nunca un evento por reserva (§8.6).
		m.publishScheduleChanged(tenantId, sc.staffId, from, to, sc.entered)
	}
	return result.totalChanged, nil
}
