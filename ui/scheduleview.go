package ui

import (
	tintime "webtyp.com/time"

	"webtyp.com/components/scheduleeditor"
	"webtyp.com/layout/rightpanel"
	"webtyp.com/router"
	"webtyp.com/widget"

	. "webtyp.com/dom"
	. "webtyp.com/html"

	ab "github.com/veltylabs/appointment_booking"
	businesscalendar "github.com/veltylabs/business_calendar"
)

// NameScheduleView es la identidad de widget de esta pestaña.
const NameScheduleView = widget.Name("scheduleview")

const PartHeader = widget.Part("header")

var clsRoot = NameScheduleView.Root()
var clsHeader = NameScheduleView.Class(PartHeader)

func (s *ScheduleView) WidgetName() widget.Name { return NameScheduleView }
func (s *ScheduleView) WidgetKind() widget.Kind { return widget.Region }

const timezoneClinic = "America/Santiago"

// NewScheduleView construye la pestaña "Horario" de la pantalla Personal.
func NewScheduleView(caller router.Caller, tenantID string) Component {
	v := &ScheduleView{caller: caller, tenantID: tenantID}
	v.picker = newStaffPicker(caller, func(string) { v.reload() })
	return v
}

type ScheduleView struct {
	Element
	caller   router.Caller
	tenantID string
	picker   *staffPicker
	editor   *SignalNodes
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

	s.caller.Call(ab.ModelName+"."+ab.OpUpsertCalendarConfig,
		&ab.UpsertCalendarConfigArgs{TenantId: s.tenantID, StaffId: staffID, Timezone: timezoneClinic, IsActive: true},
		nil, func(error) {
			s.fetchAndBuild(staffID)
		})
}

func (s *ScheduleView) fetchAndBuild(staffID string) {
	var blocks ab.WorkCalendarBlockList
	var exceptions ab.WorkCalendarExceptionList
	var holidays businesscalendar.HolidayList
	var closures businesscalendar.ClosureList
	pending := 4

	done := func() {
		pending--
		if pending == 0 {
			s.editor.Set([]*Element{s.buildEditor(staffID, blocks, exceptions, holidays, closures)})
		}
	}

	s.caller.Call(ab.ModelName+"."+ab.OpListBlocks, &ab.ListBlocksArgs{TenantId: s.tenantID, StaffId: staffID}, &blocks,
		func(error) { done() })
	s.caller.Call(ab.ModelName+"."+ab.OpListExceptions,
		&ab.ListExceptionsArgs{TenantId: s.tenantID, StaffId: staffID, From: 0, To: 0}, &exceptions,
		func(error) { done() })
	s.caller.Call(businesscalendar.ModelName+"."+businesscalendar.OpListHolidays, nil, &holidays, func(error) { done() })
	s.caller.Call(businesscalendar.ModelName+"."+businesscalendar.OpListClosures, nil, &closures, func(error) { done() })
}

func (s *ScheduleView) buildEditor(
	staffID string,
	blocks ab.WorkCalendarBlockList,
	exceptions ab.WorkCalendarExceptionList,
	holidays businesscalendar.HolidayList,
	closures businesscalendar.ClosureList,
) *Element {
	editor := &scheduleeditor.ScheduleEditor{
		Pattern:  blocksToPattern(blocks),
		Marked:   blocksToMarked(blocks),
		Holidays: datesOf(holidays),
		Closures: closureDatesOf(closures),
		Exceptions: func() []scheduleeditor.Exception {
			out := make([]scheduleeditor.Exception, 0, len(exceptions))
			for _, x := range exceptions {
				out = append(out, scheduleeditor.Exception{
					ID:       x.Id,
					Date:     unixToDay(x.SpecificDate),
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
			s.caller.Call(ab.ModelName+"."+ab.OpAddCalendarException, &ab.AddCalendarExceptionArgs{
				TenantId: s.tenantID, StaffId: staffID, SpecificDate: dayToUnix(x.Date),
				ExceptionType: x.Type, StartTime: int64(x.StartMin), EndTime: int64(x.EndMin), Notes: x.Notes,
			}, nil, func(error) { s.reload() })
		},
		OnExceptionRemove: func(id string) {
			s.caller.Call(ab.ModelName+"."+ab.OpRemoveCalendarException,
				&ab.RemoveCalendarExceptionArgs{TenantId: s.tenantID, ExceptionId: id}, nil,
				func(error) { s.reload() })
		},
	}

	return Div().Child(editor)
}

func (s *ScheduleView) Render() *Element {
	panel := &rightpanel.RightPanel{
		Title:        "Horario",
		HeadControls: Div().Set(clsHeader.AsAttr()).Child(s.picker.render("schedule-staff")),
		Article:      Div().BindChildren(s.editor),
	}
	return Div().Set(clsRoot.AsAttr()).Child(panel.Render())
}

func (s *ScheduleView) saveDayBlocks(staffID string, rows []scheduleeditor.PatternRow) {
	pending := 7
	for dow := 0; dow <= 6; dow++ {
		var dayBlocks []ab.WorkCalendarBlock
		for _, r := range rows {
			for _, d := range r.Days {
				if d == dow {
					dayBlocks = append(dayBlocks, ab.WorkCalendarBlock{
						TenantId: s.tenantID, StaffId: staffID, DayOfWeek: int64(dow),
						StartMin: int64(r.StartMin), EndMin: int64(r.EndMin), IsActive: true,
					})
				}
			}
		}
		s.caller.Call(ab.ModelName+"."+ab.OpSaveDayBlocks,
			&ab.SaveDayBlocksArgs{TenantId: s.tenantID, StaffId: staffID, DayOfWeek: int64(dow), Blocks: dayBlocks},
			nil, func(error) {
				pending--
				if pending == 0 {
					s.reload()
				}
			})
	}
}

func (s *ScheduleView) markDays(staffID string, dates []string, startMin, endMin int) {
	unixDates := make([]int, 0, len(dates))
	for _, d := range dates {
		unixDates = append(unixDates, int(dayToUnix(d)))
	}
	s.caller.Call(ab.ModelName+"."+ab.OpMarkWorkingDays,
		&ab.MarkWorkingDaysArgs{TenantId: s.tenantID, StaffId: staffID, Dates: unixDates, StartMin: int64(startMin), EndMin: int64(endMin)},
		nil, func(error) { s.reload() })
}

func (s *ScheduleView) unmarkDays(staffID string, dates []string) {
	unixDates := make([]int, 0, len(dates))
	for _, d := range dates {
		unixDates = append(unixDates, int(dayToUnix(d)))
	}
	s.caller.Call(ab.ModelName+"."+ab.OpUnmarkWorkingDays,
		&ab.UnmarkWorkingDaysArgs{TenantId: s.tenantID, StaffId: staffID, Dates: unixDates},
		nil, func(error) { s.reload() })
}

func (s *ScheduleView) saveDateBlock(staffID string, day scheduleeditor.MarkedDay) {
	specificDate := dayToUnix(day.Date)
	s.caller.Call(ab.ModelName+"."+ab.OpSaveDateBlocks, &ab.SaveDateBlocksArgs{
		TenantId: s.tenantID, StaffId: staffID, SpecificDate: specificDate,
		Blocks: []ab.WorkCalendarBlock{{
			TenantId: s.tenantID, StaffId: staffID, SpecificDate: specificDate,
			StartMin: int64(day.StartMin), EndMin: int64(day.EndMin), IsActive: true,
		}},
	}, nil, func(error) { s.reload() })
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
	date int64
	day  scheduleeditor.MarkedDay
}

func blocksToMarked(blocks ab.WorkCalendarBlockList) []scheduleeditor.MarkedDay {
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
				day:  scheduleeditor.MarkedDay{Date: unixToDay(b.SpecificDate), StartMin: int(b.StartMin), EndMin: int(b.EndMin)},
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

func datesOf(holidays businesscalendar.HolidayList) []string {
	out := make([]string, 0, len(holidays))
	for _, h := range holidays {
		out = append(out, unixToDay(h.SpecificDate))
	}
	return out
}

func closureDatesOf(closures businesscalendar.ClosureList) []string {
	out := make([]string, 0, len(closures))
	for _, c := range closures {
		out = append(out, unixToDay(c.SpecificDate))
	}
	return out
}

func dayToUnix(day string) int64 {
	nano, err := tintime.ParseDate(day)
	if err != nil {
		return 0
	}
	return nano / 1000000000
}

func unixToDay(seconds int64) string {
	iso := tintime.FormatISO8601(seconds * 1000000000)
	if len(iso) < 10 {
		return iso
	}
	return iso[:10]
}
