package tests

import (
	"testing"

	tinytime "webtyp.com/time"
	ab "github.com/veltylabs/appointment_booking"
)

// openBounds establecimiento típicamente abierto 08:00–20:00.
func openBounds() *MockBoundsReader {
	return &MockBoundsReader{Default: OpenDayBounds}
}

func TestMarkedDayGeneratesSlotsWithoutWeeklyTemplate(t *testing.T) {
	// CU-10 — la regresión del §1.1: una excepción/entrada datada no podía ABRIR
	// un día porque el loop saltaba antes de consultarla. Un día marcado sin
	// template semanal debe producir huecos.
	s, repo := newTestModuleService(t, SetupDependencies())
	cfgID := seedEmployeeConfig(t, repo, "t1", "s1", 60)
	ensureCalendarConfig(t, s, "t1", "s1")

	d := Date(2027, 6, 7, 0, 0, 0, 0)
	if err := s.MarkWorkingDays("t1", "s1", []int64{d}, 540, 1020); err != nil {
		t.Fatalf("MarkWorkingDays: %v", err)
	}

	slots, err := s.ListAvailability("t1", "s1", cfgID, d, d)
	if err != nil {
		t.Fatalf("ListAvailability: %v", err)
	}
	if len(slots) == 0 {
		t.Fatal("expected slots on the marked day with no weekly template")
	}
	for _, slot := range slots {
		slotIn(t, slot, d, 540, 1020)
	}
}

func TestTwoBlocksSameDayLeaveTheGapUnbookable(t *testing.T) {
	// CU-09 — la comida es el GAP entre dos bloques; nunca un slot la cruza.
	s, repo := newTestModuleService(t, SetupDependencies())
	cfgID := seedEmployeeConfig(t, repo, "t1", "s1", 60)
	ensureCalendarConfig(t, s, "t1", "s1")

	d := Date(2027, 6, 7, 0, 0, 0, 0) // Monday
	seedWeekdayBlock(t, s, "t1", "s1", 1, []ab.WorkCalendarBlock{
		{StartMin: 540, EndMin: 720, IsActive: true},  // 09:00–12:00
		{StartMin: 780, EndMin: 1020, IsActive: true}, // 13:00–17:00
	})

	slots, err := s.ListAvailability("t1", "s1", cfgID, d, d)
	if err != nil {
		t.Fatalf("ListAvailability: %v", err)
	}
	if len(slots) == 0 {
		t.Fatal("expected slots in the two blocks")
	}
	for _, slot := range slots {
		startLocal := int((slot.StartUtc - d) / 60)
		endLocal := int((slot.EndUtc - d) / 60)
		inMorning := startLocal >= 540 && endLocal <= 720
		inAfternoon := startLocal >= 780 && endLocal <= 1020
		if !inMorning && !inAfternoon {
			t.Fatalf("slot %v not inside either block (09:00–12:00 / 13:00–17:00)", slot)
		}
		if startLocal < 780 && endLocal > 720 {
			t.Fatalf("slot %v crosses the lunch gap 12:00–13:00", slot)
		}
	}
}

func TestSingleBlockAppliedToSeveralWeekdays(t *testing.T) {
	// CU-08 — el mismo bloque se aplica a varios weekdays.
	s, repo := newTestModuleService(t, SetupDependencies())
	cfgID := seedEmployeeConfig(t, repo, "t1", "s1", 60)
	ensureCalendarConfig(t, s, "t1", "s1")

	for _, dow := range []int{1, 3} { // Monday, Wednesday
		seedWeekdayBlock(t, s, "t1", "s1", dow, defaultBlocks())
	}

	monday := Date(2027, 6, 7, 0, 0, 0, 0)
	wednesday := Date(2027, 6, 9, 0, 0, 0, 0)
	for _, day := range []int64{monday, wednesday} {
		slots, err := s.ListAvailability("t1", "s1", cfgID, day, day)
		if err != nil {
			t.Fatalf("ListAvailability(%d): %v", day, err)
		}
		if len(slots) == 0 {
			t.Fatalf("expected slots on day %d", day)
		}
		for _, slot := range slots {
			slotIn(t, slot, day, 540, 1020)
		}
	}
}

func TestMarkedDaysShareTheCommonWindow(t *testing.T) {
	// CU-11 — MarkWorkingDays marca varias fechas con la MISMA ventana.
	s, repo := newTestModuleService(t, SetupDependencies())
	cfgID := seedEmployeeConfig(t, repo, "t1", "s1", 60)
	ensureCalendarConfig(t, s, "t1", "s1")

	d1 := Date(2027, 6, 7, 0, 0, 0, 0)
	d2 := Date(2027, 6, 8, 0, 0, 0, 0)
	d3 := Date(2027, 6, 9, 0, 0, 0, 0)
	if err := s.MarkWorkingDays("t1", "s1", []int64{d1, d2, d3}, 540, 1020); err != nil {
		t.Fatalf("MarkWorkingDays: %v", err)
	}

	for _, day := range []int64{d1, d2, d3} {
		slots, err := s.ListAvailability("t1", "s1", cfgID, day, day)
		if err != nil {
			t.Fatalf("ListAvailability(%d): %v", day, err)
		}
		if len(slots) == 0 {
			t.Fatalf("expected slots on marked day %d", day)
		}
		for _, slot := range slots {
			slotIn(t, slot, day, 540, 1020)
		}
	}
}

func TestOneMarkedDayCanDivergeFromTheCommonWindow(t *testing.T) {
	// CU-11 — SaveDateBlocks deja que un día se separe de la ventana común.
	s, repo := newTestModuleService(t, SetupDependencies())
	cfgID := seedEmployeeConfig(t, repo, "t1", "s1", 60)
	ensureCalendarConfig(t, s, "t1", "s1")

	d1 := Date(2027, 6, 7, 0, 0, 0, 0)
	d2 := Date(2027, 6, 8, 0, 0, 0, 0)
	if err := s.MarkWorkingDays("t1", "s1", []int64{d1, d2}, 540, 1020); err != nil {
		t.Fatalf("MarkWorkingDays: %v", err)
	}
	if err := s.SaveDateBlocks("t1", "s1", d2, []ab.WorkCalendarBlock{
		{StartMin: 480, EndMin: 720, IsActive: true},
	}); err != nil {
		t.Fatalf("SaveDateBlocks: %v", err)
	}

	for _, day := range []int64{d1, d2} {
		slots, err := s.ListAvailability("t1", "s1", cfgID, day, day)
		if err != nil {
			t.Fatalf("ListAvailability(%d): %v", day, err)
		}
		if len(slots) == 0 {
			t.Fatalf("expected slots on day %d", day)
		}
	}

	// d2 ya no es 09:00–17:00; es 08:00–12:00.
	slots2, _ := s.ListAvailability("t1", "s1", cfgID, d2, d2)
	for _, slot := range slots2 {
		slotIn(t, slot, d2, 480, 720)
	}
}

func TestWeeklyTemplateAndMarkedDaysCoexist(t *testing.T) {
	// CU-12 — un día marcado usa SOLO su bloque datado; un día sin marcar usa
	// el template semanal. Conviven sin pisarse.
	s, repo := newTestModuleService(t, SetupDependencies())
	cfgID := seedEmployeeConfig(t, repo, "t1", "s1", 60)
	ensureCalendarConfig(t, s, "t1", "s1")

	seedWeekdayBlock(t, s, "t1", "s1", 1, defaultBlocks()) // Mondays 09–17

	mondayMarked := Date(2027, 6, 7, 0, 0, 0, 0)
	if err := s.MarkWorkingDays("t1", "s1", []int64{mondayMarked}, 480, 1020); err != nil {
		t.Fatalf("MarkWorkingDays: %v", err)
	}

	mondayUnmarked := Date(2027, 6, 14, 0, 0, 0, 0)
	slotsMarked, err := s.ListAvailability("t1", "s1", cfgID, mondayMarked, mondayMarked)
	if err != nil {
		t.Fatalf("ListAvailability(marked): %v", err)
	}
	slotsUnmarked, err := s.ListAvailability("t1", "s1", cfgID, mondayUnmarked, mondayUnmarked)
	if err != nil {
		t.Fatalf("ListAvailability(unmarked): %v", err)
	}
	if len(slotsMarked) == 0 || len(slotsUnmarked) == 0 {
		t.Fatal("expected slots on both the marked and the unmarked monday")
	}
	for _, slot := range slotsMarked {
		slotIn(t, slot, mondayMarked, 480, 1020) // 08:00 start from the dated block
	}
	for _, slot := range slotsUnmarked {
		slotIn(t, slot, mondayUnmarked, 540, 1020) // 09:00 from the weekly template
	}
}

func TestBlockedExceptionWinsOverBlockAndMarkedDay(t *testing.T) {
	// CU-13 — una excepción BLOCKED agujerea un día marcado.
	s, repo := newTestModuleService(t, SetupDependencies())
	cfgID := seedEmployeeConfig(t, repo, "t1", "s1", 60)
	ensureCalendarConfig(t, s, "t1", "s1")

	d := Date(2027, 6, 7, 0, 0, 0, 0)
	if err := s.MarkWorkingDays("t1", "s1", []int64{d}, 540, 1020); err != nil {
		t.Fatalf("MarkWorkingDays: %v", err)
	}
	if err := s.AddException(ab.WorkCalendarException{
		TenantId: "t1", StaffId: "s1", SpecificDate: d,
		ExceptionType: ab.ExcBlocked, StartTime: 720, EndTime: 780,
	}); err != nil {
		t.Fatalf("AddException: %v", err)
	}

	slots, err := s.ListAvailability("t1", "s1", cfgID, d, d)
	if err != nil {
		t.Fatalf("ListAvailability: %v", err)
	}
	if len(slots) == 0 {
		t.Fatal("expected slots outside the blocked window")
	}
	for _, slot := range slots {
		startLocal := int((slot.StartUtc - d) / 60)
		endLocal := int((slot.EndUtc - d) / 60)
		if startLocal < 780 && endLocal > 720 {
			t.Fatalf("slot %v overlaps the blocked window 12:00–13:00", slot)
		}
	}
}

func TestUnmarkingRestoresTheWeeklyTemplate(t *testing.T) {
	// CU-15 — quitar el marcado devuelve el día al template semanal.
	s, repo := newTestModuleService(t, SetupDependencies())
	cfgID := seedEmployeeConfig(t, repo, "t1", "s1", 60)
	ensureCalendarConfig(t, s, "t1", "s1")

	seedWeekdayBlock(t, s, "t1", "s1", 1, defaultBlocks())
	d := Date(2027, 6, 7, 0, 0, 0, 0) // Monday
	if err := s.MarkWorkingDays("t1", "s1", []int64{d}, 480, 1020); err != nil {
		t.Fatalf("MarkWorkingDays: %v", err)
	}
	if err := s.UnmarkWorkingDays("t1", "s1", []int64{d}); err != nil {
		t.Fatalf("UnmarkWorkingDays: %v", err)
	}

	slots, err := s.ListAvailability("t1", "s1", cfgID, d, d)
	if err != nil {
		t.Fatalf("ListAvailability: %v", err)
	}
	if len(slots) == 0 {
		t.Fatal("expected slots restored from the weekly template")
	}
	for _, slot := range slots {
		slotIn(t, slot, d, 540, 1020)
	}
}

func TestBlockOutsideBusinessHoursIsRejected(t *testing.T) {
	// CU-07 — la UI no puede ser la única defensa: la op rechaza un bloque que
	// cae fuera de las horas del establecimiento.
	deps := SetupDependenciesWithBounds(openBounds())
	s, _ := newTestModuleService(t, deps)
	ensureCalendarConfig(t, s, "t1", "s1")

	d := Date(2027, 6, 7, 0, 0, 0, 0)
	err := s.MarkWorkingDays("t1", "s1", []int64{d}, 0, 480)
	if err != ab.ErrBlockOutsideBusinessHours {
		t.Fatalf("expected ErrBlockOutsideBusinessHours, got %v", err)
	}

	// Un día que el establecimiento cierra entero se rechaza igual.
	err = s.MarkWorkingDays("t1", "s1", []int64{d}, 540, 1020) // within hours -> ok
	if err != nil {
		t.Fatalf("MarkWorkingDays within hours should succeed, got %v", err)
	}
	bounds := deps.Bounds.(*MockBoundsReader)
	bounds.Overrides = append(bounds.Overrides, DateBound{Date: d, Bounds: tinytime.DayBounds{}})
	err = s.MarkWorkingDays("t1", "s1", []int64{d}, 540, 1020)
	if err != ab.ErrBlockOnClosedDay {
		t.Fatalf("expected ErrBlockOnClosedDay, got %v", err)
	}
}

func TestClosedEstablishmentDayYieldsNoSlots(t *testing.T) {
	// CU-04/06 — un edificio cerrado cierra a todos, con reservas o sin ellas.
	deps := SetupDependenciesWithBounds(openBounds())
	s, repo := newTestModuleService(t, deps)
	cfgID := seedEmployeeConfig(t, repo, "t1", "s1", 60)
	ensureCalendarConfig(t, s, "t1", "s1")
	seedWeekdayBlock(t, s, "t1", "s1", 1, defaultBlocks())

	d := Date(2027, 6, 7, 0, 0, 0, 0) // Monday
	bounds := deps.Bounds.(*MockBoundsReader)
	bounds.Overrides = []DateBound{{Date: d, Bounds: tinytime.DayBounds{}}} // closed

	slots, err := s.ListAvailability("t1", "s1", cfgID, d, d)
	if err != nil {
		t.Fatalf("ListAvailability: %v", err)
	}
	if len(slots) != 0 {
		t.Fatalf("expected 0 slots on the closed establishment day, got %d", len(slots))
	}
}

func TestNarrowingBusinessHoursClampsOfferedSlots(t *testing.T) {
	// CU-02 — recortar las horas del establecimiento recorta los huecos que se
	// ofrecen sin reescribir los bloques de nadie.
	deps := SetupDependenciesWithBounds(&MockBoundsReader{
		Default: tinytime.DayBounds{Open: true, OpenMin: 540, CloseMin: 780}, // 09:00–13:00
	})
	s, repo := newTestModuleService(t, deps)
	cfgID := seedEmployeeConfig(t, repo, "t1", "s1", 60)
	ensureCalendarConfig(t, s, "t1", "s1")
	seedWeekdayBlock(t, s, "t1", "s1", 1, defaultBlocks())

	d := Date(2027, 6, 7, 0, 0, 0, 0)
	slots, err := s.ListAvailability("t1", "s1", cfgID, d, d)
	if err != nil {
		t.Fatalf("ListAvailability: %v", err)
	}
	if len(slots) == 0 {
		t.Fatal("expected clamped slots")
	}
	for _, slot := range slots {
		slotIn(t, slot, d, 540, 780) // nunca más tarde de las 13:00
	}
}

func TestBlocksGapIsFree(t *testing.T) {
	// CU-09 sanity: con un único bloque no hay agujero interno.
	s, repo := newTestModuleService(t, SetupDependencies())
	cfgID := seedEmployeeConfig(t, repo, "t1", "s1", 60)
	ensureCalendarConfig(t, s, "t1", "s1")
	seedWeekdayBlock(t, s, "t1", "s1", 5, defaultBlocks())

	d := Date(2027, 6, 11, 0, 0, 0, 0) // Friday
	slots, err := s.ListAvailability("t1", "s1", cfgID, d, d)
	if err != nil {
		t.Fatalf("ListAvailability: %v", err)
	}
	if len(slots) < 8 {
		t.Fatalf("expected the full 09:00–17:00 day (8 slots/hour) offered, got %d", len(slots))
	}
	for _, slot := range slots {
		slotIn(t, slot, d, 540, 1020)
	}
}