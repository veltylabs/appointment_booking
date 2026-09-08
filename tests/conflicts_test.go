package tests

import (
	"testing"

	"webtyp.com/events"
	eventmock "webtyp.com/events/mock"
	tinytime "webtyp.com/time"
	ab "github.com/veltylabs/appointment_booking"
)

// newConflictsHarness monta el módulo con un broker que captura payloads y un
// fake BoundsReader (por defecto 08:00–20:00 abierto todos los días).
func newConflictsHarness(t *testing.T) (ab.SchedulingService, *ab.Repository, *MockBoundsReader, *eventmock.Broker) {
	t.Helper()
	bounds := openBounds()
	broker := &eventmock.Broker{}
	deps := SetupDependencies()
	deps.Publisher = broker
	deps.Bounds = bounds
	s, repo, _ := newTestModule(t, deps)
	return s, repo, bounds, broker
}

// schedCapture es el receptor de un suscriptor a EventScheduleChanged.
type schedCapture struct {
	count int
	last  *ab.ScheduleChangedPayload
}

func subscribeScheduleChanged(broker *eventmock.Broker) *schedCapture {
	c := &schedCapture{}
	broker.Subscribe(ab.EventScheduleChanged, func(e events.Event) {
		c.count++
		c.last = e.Payload.(*ab.ScheduleChangedPayload)
	})
	return c
}

func subscribeReservationConflicted(broker *eventmock.Broker) *int {
	n := 0
	broker.Subscribe(ab.EventReservationConflicted, func(e events.Event) {
		n++
	})
	return &n
}

func TestReducingScheduleMarksAffectedReservationsAndPublishes(t *testing.T) {
	// CU-16 — una reducción de agenda marca las reservas afectadas y lo publica.
	s, repo, _, broker := newConflictsHarness(t)

	d := Date(2027, 6, 7, 0, 0, 0, 0)
	cfgID := setupReservable(t, s, repo, "t1", "s1", d+13*3600) // dow blind, block 09–17
	createBooked(t, s, "t1", "s1", cfgID, d+13*3600)            // 13:00–14:00

	schedCap := subscribeScheduleChanged(broker)
	conflictCount := subscribeReservationConflicted(broker)

	dow := int(tinytime.Weekday(d))
	if err := s.SaveDayBlocks("t1", "s1", dow, []ab.WorkCalendarBlock{
		{StartMin: 540, EndMin: 780, IsActive: true}, // 09:00–13:00
	}); err != nil {
		t.Fatalf("SaveDayBlocks: %v", err)
	}

	got, err := s.GetReservation("t1", resFirst(t, s))
	if err != nil {
		t.Fatalf("GetReservation: %v", err)
	}
	if got.Status != ab.StatusConflicted {
		t.Fatalf("expected CONFLICTED, got %s", got.Status)
	}
	if got.StatusBeforeConflict != ab.StatusConfirmed {
		t.Fatalf("expected StatusBeforeConflict CONFIRMED, got %s", got.StatusBeforeConflict)
	}
	if schedCap.count != 1 || schedCap.last == nil {
		t.Fatalf("expected exactly 1 schedule.changed, got %d (%+v)", schedCap.count, schedCap.last)
	}
	if schedCap.last.StaffId != "s1" || schedCap.last.ConflictCount != 1 {
		t.Fatalf("unexpected payload: %+v", schedCap.last)
	}
	if *conflictCount != 1 {
		t.Fatalf("expected 1 reservation.conflicted event, got %d", *conflictCount)
	}
}

func resFirst(t *testing.T, s ab.SchedulingService) string {
	t.Helper()
	rows, err := s.ListReservationsByStaff("t1", "s1", 0, Date(2030, 1, 1, 0, 0, 0, 0))
	if err != nil || len(rows) == 0 {
		t.Fatalf("ListReservationsByStaff: %v, %d", err, len(rows))
	}
	return rows[0].Id
}

func TestReducingScheduleWithNoAffectedReservationsIsSilent(t *testing.T) {
	// CU-19 — reducir sin afectar a nadie ⇒ ni marca ni publica.
	s, repo, _, broker := newConflictsHarness(t)

	d := Date(2027, 6, 7, 0, 0, 0, 0)
	cfgID := setupReservable(t, s, repo, "t1", "s1", d+10*3600) // 10:00
	createBooked(t, s, "t1", "s1", cfgID, d+10*3600)            // 10:00–11:00

	schedCap := subscribeScheduleChanged(broker)
	conflictCount := subscribeReservationConflicted(broker)

	// Reducir de 09–17 a 09–13 deja 10:00–11:00 intacto.
	dow := int(tinytime.Weekday(d))
	if err := s.SaveDayBlocks("t1", "s1", dow, []ab.WorkCalendarBlock{
		{StartMin: 540, EndMin: 780, IsActive: true},
	}); err != nil {
		t.Fatalf("SaveDayBlocks: %v", err)
	}

	got, _ := s.GetReservation("t1", resFirst(t, s))
	if got.Status != ab.StatusConfirmed {
		t.Fatalf("expected still CONFIRMED, got %s", got.Status)
	}
	if schedCap.count != 0 || *conflictCount != 0 {
		t.Fatalf("expected silence, got %d schedule.changed and %d conflicted events", schedCap.count, *conflictCount)
	}
}

func TestScheduleChangedPayloadCarriesRangeAndCount(t *testing.T) {
	// CU-20 — el payload lleva el rango afectado y el count.
	s, repo, _, broker := newConflictsHarness(t)

	d := Date(2027, 6, 7, 0, 0, 0, 0)
	cfgID := setupReservable(t, s, repo, "t1", "s1", d+16*3600) // 16:00
	createBooked(t, s, "t1", "s1", cfgID, d+16*3600)            // 16:00–17:00

	schedCap := subscribeScheduleChanged(broker)

	if err := s.SaveDateBlocks("t1", "s1", d, []ab.WorkCalendarBlock{
		{StartMin: 540, EndMin: 780, IsActive: true}, // solo hasta las 13:00
	}); err != nil {
		t.Fatalf("SaveDateBlocks: %v", err)
	}

	if schedCap.last == nil {
		t.Fatal("expected a schedule.changed event")
	}
	if schedCap.last.FromDate != d || schedCap.last.ToDate != d {
		t.Fatalf("expected range [%d, %d], got [%d, %d]", d, d, schedCap.last.FromDate, schedCap.last.ToDate)
	}
	if schedCap.last.ConflictCount != 1 {
		t.Fatalf("expected ConflictCount 1, got %d", schedCap.last.ConflictCount)
	}
}

func TestConflictedIsNotATerminalStatus(t *testing.T) {
	// CU-17 — CONFLICTED no es terminal: se entra desde PENDING/CONFIRMED y se
	// sale con RESOLVE.
	if ab.IsTerminal(ab.StatusConflicted) {
		t.Fatal("CONFLICTED must not be a terminal status")
	}
	next, err := ab.Transition(ab.StatusPending, ab.EventConflict)
	if err != nil || next != ab.StatusConflicted {
		t.Fatalf("Transition(PENDING, CONFLICT) = %s, %v", next, err)
	}
	if _, err := ab.Transition(ab.StatusConflicted, ab.EventResolve); err != nil {
		t.Fatalf("Transition(CONFLICTED, RESOLVE) should be legal, got %v", err)
	}
}

func TestDayBecomingHolidayConflictsItsReservations(t *testing.T) {
	// CU-25 — el establecimiento declara un feriado: TODAS las reservas de ese
	// día caen en conflicto con razón DAY_CLOSED.
	s, repo, bounds, broker := newConflictsHarness(t)

	d := Date(2027, 6, 7, 0, 0, 0, 0)
	cfgID := setupReservable(t, s, repo, "t1", "s1", d+12*3600)
	createBooked(t, s, "t1", "s1", cfgID, d+12*3600)

	pre, err := s.RecomputeConflicts("t1", d, d)
	if err != nil || pre != 0 {
		t.Fatalf("baseline recompute: %d, %v", pre, err)
	}

	bounds.Overrides = []DateBound{{Date: d, Bounds: tinytime.DayBounds{}}} // holiday

	schedCap := subscribeScheduleChanged(broker)
	n, err := s.RecomputeConflicts("t1", d, d)
	if err != nil {
		t.Fatalf("RecomputeConflicts: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 changed reservation, got %d", n)
	}

	got, _ := s.GetReservation("t1", resFirst(t, s))
	if got.Status != ab.StatusConflicted {
		t.Fatalf("expected CONFLICTED, got %s", got.Status)
	}

	conflicts, err := s.ListConflictingReservations("t1", "s1", Date(2027, 6, 1, 0, 0, 0, 0))
	if err != nil || len(conflicts) != 1 {
		t.Fatalf("ListConflictingReservations: %v, %d", err, len(conflicts))
	}
	if conflicts[0].Reason != ab.ConflictReasonDayClosed {
		t.Fatalf("expected reason DAY_CLOSED, got %s", conflicts[0].Reason)
	}
	if schedCap.count != 1 {
		t.Fatalf("expected exactly 1 summary event, got %d", schedCap.count)
	}
}

func TestLocalClosureConflictsWithItsOwnReason(t *testing.T) {
	// CU-26 — un cierre local (día entero cerrado) conflictúa con su propia
	// razón: DAY_CLOSED. La distinción feriado/cierre es del calendario del
	// establecimiento; este módulo solo ve "cerrado".
	s, repo, bounds, _ := newConflictsHarness(t)

	d := Date(2027, 6, 8, 0, 0, 0, 0)
	cfgID := setupReservable(t, s, repo, "t1", "s1", d+12*3600)
	createBooked(t, s, "t1", "s1", cfgID, d+12*3600)

	bounds.Overrides = []DateBound{{Date: d, Bounds: tinytime.DayBounds{}}}
	if _, err := s.RecomputeConflicts("t1", d, d); err != nil {
		t.Fatalf("RecomputeConflicts: %v", err)
	}

	conflicts, err := s.ListConflictingReservations("t1", "s1", Date(2027, 6, 1, 0, 0, 0, 0))
	if err != nil || len(conflicts) != 1 {
		t.Fatalf("ListConflictingReservations: %v, %d", err, len(conflicts))
	}
	if conflicts[0].Reason != ab.ConflictReasonDayClosed {
		t.Fatalf("expected reason DAY_CLOSED, got %s", conflicts[0].Reason)
	}
}

func TestNarrowingBusinessHoursConflictsLateReservations(t *testing.T) {
	// CU-27 — recortar las horas de apertura bajo una reserva ya tomada la
	// marcaría como OUTSIDE_BUSINESS_HOURS.
	s, repo, bounds, _ := newConflictsHarness(t)

	d := Date(2027, 6, 9, 0, 0, 0, 0)
	cfgID := setupReservable(t, s, repo, "t1", "s1", d+16*3600) // 16:00 dentro de 09–17 y 08–20
	createBooked(t, s, "t1", "s1", cfgID, d+16*3600)

	bounds.Overrides = []DateBound{{Date: d, Bounds: tinytime.DayBounds{Open: true, OpenMin: 540, CloseMin: 780}}}
	if _, err := s.RecomputeConflicts("t1", d, d); err != nil {
		t.Fatalf("RecomputeConflicts: %v", err)
	}

	conflicts, err := s.ListConflictingReservations("t1", "s1", Date(2027, 6, 1, 0, 0, 0, 0))
	if err != nil || len(conflicts) != 1 {
		t.Fatalf("ListConflictingReservations: %v, %d", err, len(conflicts))
	}
	if conflicts[0].Reason != ab.ConflictReasonOutsideBusinessHours {
		t.Fatalf("expected reason OUTSIDE_BUSINESS_HOURS, got %s", conflicts[0].Reason)
	}
}

func TestRemovingHolidayRestoresThePreviousStatus(t *testing.T) {
	// CU-28 — quitar el feriado restaura el estado ANTERIOR (CONFIRMED), no un
	// guess hacia PENDING.
	s, repo, bounds, _ := newConflictsHarness(t)

	d := Date(2027, 6, 10, 0, 0, 0, 0)
	cfgID := setupReservable(t, s, repo, "t1", "s1", d+12*3600)
	createBooked(t, s, "t1", "s1", cfgID, d+12*3600) // CONFIRMED

	bounds.Overrides = []DateBound{{Date: d, Bounds: tinytime.DayBounds{}}}
	if _, err := s.RecomputeConflicts("t1", d, d); err != nil {
		t.Fatalf("mark: %v", err)
	}
	got, _ := s.GetReservation("t1", resFirst(t, s))
	if got.Status != ab.StatusConflicted || got.StatusBeforeConflict != ab.StatusConfirmed {
		t.Fatalf("expected CONFLICTED with before=CONFIRMED, got %s/%s", got.Status, got.StatusBeforeConflict)
	}

	bounds.Overrides = nil // el feriado se levanta: el día vuelve a la ventana por defecto
	n, err := s.RecomputeConflicts("t1", d, d)
	if err != nil {
		t.Fatalf("clear: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 restored reservation, got %d", n)
	}
	got, _ = s.GetReservation("t1", resFirst(t, s))
	if got.Status != ab.StatusConfirmed {
		t.Fatalf("expected restored CONFIRMED, got %s", got.Status)
	}
	if got.StatusBeforeConflict != "" {
		t.Fatalf("expected cleared StatusBeforeConflict, got %s", got.StatusBeforeConflict)
	}
}

func TestRemovingHolidayKeepsConflictWhenAlsoOutsideBlocks(t *testing.T) {
	// CU-28 segunda causa — la reserva está en conflicto por DOS causas a la vez:
	// un feriado Y estar fuera de los bloques del profesional. Quitar el feriado
	// NO debe "re-confirmarla": la respuesta sale de recomputar, no de un toggle.
	s, repo, bounds, _ := newConflictsHarness(t)

	d := Date(2027, 6, 11, 0, 0, 0, 0)
	cfgID := setupReservable(t, s, repo, "t1", "s1", d+16*3600) // 16:00 dentro de 09–17
	createBooked(t, s, "t1", "s1", cfgID, d+16*3600)            // CONFIRMED

	// Causa 1: el feriado cae.
	bounds.Overrides = []DateBound{{Date: d, Bounds: tinytime.DayBounds{}}}
	if _, err := s.RecomputeConflicts("t1", d, d); err != nil {
		t.Fatalf("holiday recompute: %v", err)
	}
	got, _ := s.GetReservation("t1", resFirst(t, s))
	if got.Status != ab.StatusConflicted {
		t.Fatalf("expected CONFLICTED under the holiday, got %s", got.Status)
	}

	// Causa 2 va a persistir: el bloque del profesional se reduce a 09–13,
	// así que 16:00 queda fuera aunque el feriado desaparezca.
	bounds.Overrides = nil // el feriado se levanta
	dow := int(tinytime.Weekday(d))
	if err := s.SaveDayBlocks("t1", "s1", dow, []ab.WorkCalendarBlock{
		{StartMin: 540, EndMin: 780, IsActive: true},
	}); err != nil {
		t.Fatalf("SaveDayBlocks: %v", err)
	}

	if _, err := s.RecomputeConflicts("t1", d, d); err != nil {
		t.Fatalf("final recompute: %v", err)
	}
	got, _ = s.GetReservation("t1", resFirst(t, s))
	if got.Status != ab.StatusConflicted {
		t.Fatalf("expected STILL CONFLICTED when also outside blocks, got restored to %s", got.Status)
	}
	if got.StatusBeforeConflict != ab.StatusConfirmed {
		t.Fatalf("expected StatusBeforeConflict to remain CONFIRMED, got %s", got.StatusBeforeConflict)
	}
	conflicts, err := s.ListConflictingReservations("t1", "s1", Date(2027, 6, 1, 0, 0, 0, 0))
	if err != nil || len(conflicts) != 1 {
		t.Fatalf("ListConflictingReservations: %v, %d", err, len(conflicts))
	}
	if conflicts[0].Reason != ab.ConflictReasonOutsideBlocks {
		t.Fatalf("expected reason OUTSIDE_BLOCKS, got %s", conflicts[0].Reason)
	}
}

func TestHolidayOnDayWithoutReservationsIsSilent(t *testing.T) {
	// CU-29 — un feriado en un día sin reservas: nada que marcar, nada que
	// publicar.
	s, repo, bounds, broker := newConflictsHarness(t)

	d := Date(2027, 6, 14, 0, 0, 0, 0)
	setupReservable(t, s, repo, "t1", "s1", d+10*3600) // staff con agenda, sin reservas

	bounds.Overrides = []DateBound{{Date: d, Bounds: tinytime.DayBounds{}}}
	schedCap := subscribeScheduleChanged(broker)

	n, err := s.RecomputeConflicts("t1", d, d)
	if err != nil {
		t.Fatalf("RecomputeConflicts: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected 0 changed, got %d", n)
	}
	if schedCap.count != 0 {
		t.Fatalf("expected no events, got %d", schedCap.count)
	}
}

func TestOpeningTimeNeverConflictsAnything(t *testing.T) {
	// CU-29 — adelantar la hora de apertura no puede invalidar nada.
	s, repo, bounds, broker := newConflictsHarness(t)

	d := Date(2027, 6, 15, 0, 0, 0, 0)
	cfgID := setupReservable(t, s, repo, "t1", "s1", d+12*3600)
	createBooked(t, s, "t1", "s1", cfgID, d+12*3600)

	bounds.Default = tinytime.DayBounds{Open: true, OpenMin: 360, CloseMin: 1380} // abre 1h antes
	schedCap := subscribeScheduleChanged(broker)

	n, err := s.RecomputeConflicts("t1", d, d)
	if err != nil {
		t.Fatalf("RecomputeConflicts: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected 0 changed when opening earlier, got %d", n)
	}
	if schedCap.count != 0 {
		t.Fatalf("expected no events, got %d", schedCap.count)
	}
}