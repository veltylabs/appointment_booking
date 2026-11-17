package appointmentbooking

import (
	"webtyp.com/time"
)

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
func checkBlockAgainstBounds(bounds time.DayLimits, b WorkCalendarBlock) error {
	if !bounds.Open {
		return ErrBlockOnClosedDay
	}
	if _, _, ok := bounds.Clamp(int(b.StartMin), int(b.EndMin)); !ok {
		return ErrBlockOutsideBusinessHours
	}
	return nil
}

func validateBlockSet(bounds time.DayLimits, blocks []WorkCalendarBlock) error {
	for i := range blocks {
		if err := checkBlockAgainstBounds(bounds, blocks[i]); err != nil {
			return err
		}
	}
	return nil
}

// SaveDayBlocks replaces EVERY weekly block of that weekday with the ones
// given. A whole-day replace, not a per-row upsert: partial edits are what let

// dayWindow es una ventana UTC reservable de un día.
type dayWindow struct {
	StartUtc time.Unix
	EndUtc   time.Unix
}

// blocksForDate — los bloques DATADOS de la fecha exacta. Un bloque datado ABRE
// su día aunque el template semanal no lo cubra — es la inversión de la vieja
// compuerta de días de ListAvailability (CU-10).
func blocksForDate(blocks []WorkCalendarBlock, date time.Unix) []WorkCalendarBlock {
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

func dayExceptionsFor(exceptions []WorkCalendarException, d time.Unix) []WorkCalendarException {
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
func subtractWindow(windows []dayWindow, s, e time.Unix) []dayWindow {
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
// value): el caller usa limitsForDay.
func (m *Module) availableRanges(d time.Unix, blocks []WorkCalendarBlock, exceptions []WorkCalendarException, cfg WorkCalendarConfig, bounds time.DayLimits) []dayWindow {
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
			StartUtc: m.at(d, start),
			EndUtc:   m.at(d, end),
		}}
	}

	dayBlocks := blocksForDate(blocks, d)
	if len(dayBlocks) == 0 {
		dayBlocks = blocksForWeekday(blocks, time.WeekdayUTC(d))
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
			StartUtc: m.at(d, start),
			EndUtc:   m.at(d, end),
		})
	}

	// Restar excepciones BLOCKED del día.
	for _, be := range blockedExcs {
		out = subtractWindow(out,
			m.at(d, int(be.StartTime)),
			m.at(d, int(be.EndTime)),
		)
	}
	return out
}

// fitsWindow reporta si un slot [startUtc, startUtc+span) cabe dentro de alguna
// de las ventanas (half-open: end == window end cuenta como dentro).
func fitsWindow(windows []dayWindow, startUtc, span int64) bool {
	st := time.Unix(startUtc)
	end := time.Unix(startUtc + span)
	for _, w := range windows {
		if w.StartUtc <= st && end <= w.EndUtc {
			return true
		}
	}
	return false
}

func (m *Module) scheduleSlots(from, to time.Unix, blocks []WorkCalendarBlock, exceptions []WorkCalendarException, cfg WorkCalendarConfig, empSvcCfg EmployeeServiceConfig, activeReservations []Reservation) ([]TimeSlot, error) {
	durationMin := int(empSvcCfg.DurationMin)
	bufferMin := int(empSvcCfg.BufferMin)

	slots := []TimeSlot{}

	// Para cada día D en [from, to] (timestamps de medianoche UTC): bloques
	// datados PRIMERO (abren su día aunque el template semanal no lo cubra),
	// luego semanales; recortados a bounds; y generar slots dentro de cada
	// ventana, saltando los huecos entre bloques.
	for d := from; d <= to; d += time.Day {
		bounds, err := m.limitsForDay(d)
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
				end := curr + time.Unix(durationMin*60)
				endWithBuffer := end + time.Unix(bufferMin*60)
				if endWithBuffer > win.EndUtc {
					break
				}

				// Verificar reservas existentes
				hasOverlap := false
				var resEnd time.Unix
				for _, res := range activeReservations {
					rStart := time.Unix(res.ReservationTime)
					rEnd := rStart + time.Unix(res.DurationMinSnapshot*60)
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
					curr += time.Unix(durationMin * 60)
				}
			}
		}
	}

	return slots, nil
}


// ConflictHorizonDays es el forward horizon del módulo: cuando no hay rango que
// acote (un cambio semanal, o to == 0), los conflictos se evalúan hasta aquí.
const ConflictHorizonDays = 370

// forwardHorizon convierte un "from" (medianoche UTC) al borde superior del
// horizonte del módulo.
func forwardHorizon(from time.Unix) time.Unix {
	return from + time.Unix(ConflictHorizonDays*time.Day)
}

// Motivos de ConflictingReservation.Reason — el vocabulario NEUTRO del módulo.
// Concretamente NO arrastra la razón del establecimiento (por qué un día está
// cerrado): time.DayLimits lo omite deliberadamente y este módulo lo respeta.
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
func (m *Module) checkReservation(r Reservation, blocks []WorkCalendarBlock, exceptions []WorkCalendarException, cfg WorkCalendarConfig, bounds time.DayLimits) (bool, string) {
	d := m.dayKey(time.Unix(r.ReservationTime))
	if !bounds.Open {
		return false, ConflictReasonDayClosed
	}
	span := int64(r.DurationMinSnapshot * 60)
	raw := m.availableRanges(d, blocks, exceptions, cfg, time.Unlimited())
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

