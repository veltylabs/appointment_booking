package ui

import (
	"webtyp.com/fmt"
	"webtyp.com/model"
	"webtyp.com/time"

	"webtyp.com/components/scheduleeditor"
	"webtyp.com/crud"
	"webtyp.com/layout/rightpanel"
	"webtyp.com/router"
	"webtyp.com/widget"

	. "webtyp.com/dom"
	. "webtyp.com/html"

	ab "github.com/veltylabs/appointment_booking"
)

// NameScheduleView es la identidad de widget de esta pestaña.
const NameScheduleView = widget.Name("scheduleview")

const PartHeader = widget.Part("header")

var clsRoot = NameScheduleView.Root()
var clsHeader = NameScheduleView.Class(PartHeader)

func (s *ScheduleView) WidgetName() widget.Name { return NameScheduleView }
func (s *ScheduleView) WidgetKind() widget.Kind { return widget.Region }

// NewScheduleView construye la pestaña "Horario" de la pantalla Personal.
func NewScheduleView(caller router.Caller, zone time.Zone, ids model.IDGenerator) Component {
	v := &ScheduleView{
		caller:      caller,
		zone:        zone,
		ids:         ids,
		overbookVal: NewString("0"),
	}
	v.picker = newStaffPicker(caller, func(string) { v.reload() })
	return v
}

type ScheduleView struct {
	Element
	caller      router.Caller
	zone        time.Zone
	ids         model.IDGenerator
	calendar    ab.WorkCalendarConfig
	blocks      ab.WorkCalendarBlockList
	picker      *staffPicker
	editor      *SignalNodes
	overbookVal *SignalString
}

func (s *ScheduleView) Init(_ Ctx) {
	if s.editor == nil {
		s.editor = NewNodes()
	}
	s.picker.load(s.reload)
}

func (s *ScheduleView) reload() {
	staffID := s.picker.sel.Get()
	if staffID == "" {
		s.editor.Set([]*Element{Div().Text("No hay profesionales registrados todavía.")})
		return
	}

	var configs ab.WorkCalendarConfigList
	readFiltered(s.caller, &ab.WorkCalendarConfig{}, &ab.WorkCalendarConfig{StaffId: staffID}, &configs, func(err error) {
		if err == nil && len(configs) > 0 {
			s.calendar = *configs[0]
			s.overbookVal.Set(fmt.Sprint(s.calendar.MaxOverbookPerDay))
			s.fetchAndBuild(staffID)
			return
		}
		// Un profesional sin calendario recibe el suyo, abierto y sin sobrecupos.
		s.overbookVal.Set("0")
		s.calendar = ab.WorkCalendarConfig{Id: s.ids.NewID(), StaffId: staffID, IsActive: true}
		createOne(s.caller, &s.calendar, nil, func(error) { s.fetchAndBuild(staffID) })
	})
}

func (s *ScheduleView) fetchAndBuild(staffID string) {
	var blocks ab.WorkCalendarBlockList
	var exceptions ab.WorkCalendarExceptionList
	var closures ab.ClosureOptionList
	pending := 3

	done := func() {
		pending--
		if pending == 0 {
			s.blocks = blocks
			holidays, closed := splitClosures(closures)
			s.editor.Set([]*Element{s.buildEditor(staffID, blocks, exceptions, holidays, closed)})
		}
	}

	filter := &ab.WorkCalendarBlock{StaffId: staffID}
	readFiltered(s.caller, &ab.WorkCalendarBlock{}, filter, &blocks, func(error) { done() })
	readFiltered(s.caller, &ab.WorkCalendarException{}, &ab.WorkCalendarException{StaffId: staffID}, &exceptions, func(error) { done() })
	readFiltered(s.caller, &ab.ClosureOption{}, &ab.ClosureOption{}, &closures, func(error) { done() })
}

func (s *ScheduleView) buildEditor(
	staffID string,
	blocks ab.WorkCalendarBlockList,
	exceptions ab.WorkCalendarExceptionList,
	holidays []string,
	closures []string,
) *Element {
	editor := &scheduleeditor.ScheduleEditor{
		Zone:     s.zone,
		Pattern:  blocksToPattern(blocks),
		Marked:   s.blocksToMarked(blocks),
		Holidays: holidays,
		Closures: closures,
		Exceptions: func() []scheduleeditor.Exception {
			out := make([]scheduleeditor.Exception, 0, len(exceptions))
			for _, x := range exceptions {
				out = append(out, scheduleeditor.Exception{
					ID:       x.Id,
					Date:     s.unixToDay(x.SpecificDate),
					Type:     x.ExceptionType,
					StartMin: int(x.StartTime),
					EndMin:   int(x.EndTime),
					Notes:    x.Notes,
				})
			}
			return out
		}(),

		OnPatternChange: func(rows []scheduleeditor.PatternRow) {
			s.saveDayBlocks(staffID, rows)
		},
		OnDaysMarked: func(dates []string, startMin, endMin int) {
			s.markDays(staffID, dates, startMin, endMin)
		},
		OnDaysUnmarked: func(dates []string) {
			s.unmarkDays(staffID, dates)
		},
		OnMarkedDayEdit: func(day scheduleeditor.MarkedDay) {
			s.saveDateBlock(staffID, day)
		},
		OnExceptionAdd: func(x scheduleeditor.Exception) {
			rec := &ab.WorkCalendarException{
				Id: s.ids.NewID(), StaffId: staffID, SpecificDate: s.dayToUnix(x.Date),
				ExceptionType: x.Type, StartTime: int64(x.StartMin), EndTime: int64(x.EndMin), Notes: x.Notes,
			}
			createOne(s.caller, rec, nil, func(error) { s.reload() })
		},
		OnExceptionRemove: func(id string) {
			deleteOne(s.caller, &ab.WorkCalendarException{}, id, func(error) { s.reload() })
		},
	}

	return Div().Child(editor)
}

func (s *ScheduleView) Render() *Element {
	overbookInput := NewElement("input").
		Attr("type", "number").
		Attr("min", "0").
		Attr("name", "max-overbook-per-day").
		Bind(s.overbookVal).
		OnChange(func(e Event) {
			valStr := e.TargetValue()
			s.overbookVal.Set(valStr)
			val, _ := fmt.Convert(valStr).Int64()
			if val < 0 {
				val = 0
			}
			staffID := s.picker.sel.Get()
			if staffID != "" && s.calendar.Id != "" {
				s.calendar.MaxOverbookPerDay = val
				s.caller.Call(operation(&s.calendar, model.Update),
					&crud.UpdateArgs{IDs: []string{s.calendar.Id}, Fields: []string{ab.WorkCalendarConfig_.MaxOverbookPerDay}, Record: &s.calendar},
					nil, func(error) {})
			}
		})

	controls := Div().Set(clsHeader.AsAttr()).
		Child(s.picker.render("schedule-staff")).
		Child(Label().Text("Máx. sobrecupo/día:")).
		Child(overbookInput)

	panel := &rightpanel.RightPanel{
		Article: Div().Child(controls, Div().BindChildren(s.editor)),
	}
	return Div().Set(clsRoot.AsAttr()).Child(panel.Render())
}

// blockStep crea un bloque; dropBlock borra el que ya existe.
func (s *ScheduleView) blockStep(b ab.WorkCalendarBlock) step {
	b.Id = s.ids.NewID()
	return func(done func(error)) { createOne(s.caller, &b, nil, done) }
}

func (s *ScheduleView) dropBlock(id string) step {
	return func(done func(error)) { deleteOne(s.caller, &ab.WorkCalendarBlock{}, id, done) }
}

// dropWhere borra los bloques guardados que cumplen pick.
func (s *ScheduleView) dropWhere(pick func(ab.WorkCalendarBlock) bool) []step {
	var out []step
	for _, b := range s.blocks {
		if pick(*b) {
			out = append(out, s.dropBlock(b.Id))
		}
	}
	return out
}

// apply corre los pasos y vuelve a leer lo guardado, haya salido bien o no: la pantalla muestra lo que quedó.
func (s *ScheduleView) apply(steps []step) {
	runSteps(steps, func(error) { s.reload() })
}

// saveDayBlocks reemplaza el patrón semanal: borra los bloques semanales y crea los de las filas del editor.
func (s *ScheduleView) saveDayBlocks(staffID string, rows []scheduleeditor.PatternRow) {
	steps := s.dropWhere(func(b ab.WorkCalendarBlock) bool { return b.SpecificDate == 0 })
	for _, r := range rows {
		for _, d := range r.Days {
			steps = append(steps, s.blockStep(ab.WorkCalendarBlock{
				StaffId: staffID, DayOfWeek: int64(d), StartMin: int64(r.StartMin), EndMin: int64(r.EndMin), IsActive: true,
			}))
		}
	}
	s.apply(steps)
}

func (s *ScheduleView) markDays(staffID string, dates []string, startMin, endMin int) {
	var steps []step
	for _, d := range dates {
		day := s.dayToUnix(d)
		steps = append(steps, s.dropWhere(func(b ab.WorkCalendarBlock) bool { return b.SpecificDate == day })...)
		steps = append(steps, s.blockStep(ab.WorkCalendarBlock{
			StaffId: staffID, SpecificDate: day, StartMin: int64(startMin), EndMin: int64(endMin), IsActive: true,
		}))
	}
	s.apply(steps)
}

func (s *ScheduleView) unmarkDays(_ string, dates []string) {
	var steps []step
	for _, d := range dates {
		day := s.dayToUnix(d)
		steps = append(steps, s.dropWhere(func(b ab.WorkCalendarBlock) bool { return b.SpecificDate == day })...)
	}
	s.apply(steps)
}

func (s *ScheduleView) saveDateBlock(staffID string, day scheduleeditor.MarkedDay) {
	s.markDays(staffID, []string{day.Date}, day.StartMin, day.EndMin)
}

func blocksToPattern(blocks ab.WorkCalendarBlockList) []scheduleeditor.PatternRow {
	type rangeKey struct{ start, end int }
	var keys []rangeKey
	daysByKey := make([][]int, 0)

	for _, b := range blocks {
		if !b.IsActive || b.SpecificDate != 0 || b.DayOfWeek < 0 || b.DayOfWeek > 6 {
			continue
		}
		sm, em := int(b.StartMin), int(b.EndMin)
		dow := int(b.DayOfWeek)
		idx := -1
		for i, k := range keys {
			if k.start == sm && k.end == em {
				idx = i
				break
			}
		}
		if idx == -1 {
			keys = append(keys, rangeKey{sm, em})
			daysByKey = append(daysByKey, []int{dow})
		} else {
			daysByKey[idx] = append(daysByKey[idx], dow)
		}
	}

	rows := make([]scheduleeditor.PatternRow, len(keys))
	for i, k := range keys {
		rows[i] = scheduleeditor.PatternRow{StartMin: k.start, EndMin: k.end, Days: daysByKey[i]}
	}
	return rows
}

type markedDateKV struct {
	date time.Unix
	day  scheduleeditor.MarkedDay
}

func (s *ScheduleView) blocksToMarked(blocks ab.WorkCalendarBlockList) []scheduleeditor.MarkedDay {
	var kvs []markedDateKV
	for _, b := range blocks {
		if !b.IsActive || b.SpecificDate == 0 {
			continue
		}
		var found *scheduleeditor.MarkedDay
		for i := range kvs {
			if kvs[i].date == b.SpecificDate {
				found = &kvs[i].day
				break
			}
		}
		if found == nil {
			kvs = append(kvs, markedDateKV{
				date: b.SpecificDate,
				day:  scheduleeditor.MarkedDay{Date: s.unixToDay(b.SpecificDate), StartMin: int(b.StartMin), EndMin: int(b.EndMin)},
			})
		} else {
			if int(b.StartMin) < found.StartMin {
				found.StartMin = int(b.StartMin)
			}
			if int(b.EndMin) > found.EndMin {
				found.EndMin = int(b.EndMin)
			}
		}
	}
	out := make([]scheduleeditor.MarkedDay, len(kvs))
	for i, kv := range kvs {
		out[i] = kv.day
	}
	return out
}

func splitClosures(closures ab.ClosureOptionList) (holidays, closed []string) {
	for _, c := range closures {
		if c.IsHoliday {
			holidays = append(holidays, c.Date)
		} else {
			closed = append(closed, c.Date)
		}
	}
	return
}

func (s *ScheduleView) dayToUnix(day string) time.Unix {
	u, _ := time.ParseDateUTC(day)
	return u
}

func (s *ScheduleView) unixToDay(seconds time.Unix) string {
	return s.zone.Date(seconds)
}
