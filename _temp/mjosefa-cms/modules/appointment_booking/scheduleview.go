package appointment_booking

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

// NameScheduleView es la identidad de widget de esta pestaña. rightpanel
// pone el chasis (título, panel, scroll); esta hoja solo ajusta la fila del
// selector de profesional — mismo reparto que el demo de agenda.
const NameScheduleView = widget.Name("scheduleview")

const PartHeader = widget.Part("header")

var clsRoot = NameScheduleView.Root()
var clsHeader = NameScheduleView.Class(PartHeader)

func (s *ScheduleView) WidgetName() widget.Name { return NameScheduleView }
func (s *ScheduleView) WidgetKind() widget.Kind { return widget.Region }

// timezoneClinic es la zona horaria de esta clínica — decisión de despliegue
// de ESTA app, no un valor por defecto de la librería (appointment_booking no
// asume ninguna zona; WorkCalendarConfig.Timezone es por profesional). Un
// valor fijo porque mjosefa-cms es una clínica en Chile — la misma decisión
// implícita que el resto de este repo ya asume (RUT, feriados chilenos).
const timezoneClinic = "America/Santiago"

// NewScheduleView construye la pestaña "Horario" de la pantalla Personal: un
// selector de profesional y el scheduleeditor sobre appointment_booking real.
// Vive aquí (agnóstico wasm/backend, sin contraparte SSR) porque es puro
// componente de cliente, igual que el resto de los tabs de esa pantalla.
//
// Feriados y cierres se leen de business_calendar (list_holidays,
// list_closures) — nunca hardcodeados, a diferencia del demo del que este
// wiring parte (ver docs/PLAN_LOCAL.md, criterio de aceptación §7).
func NewScheduleView(caller router.Caller, tenantID string) Component {
	v := &ScheduleView{caller: caller, tenantID: tenantID}
	v.picker = newStaffPicker(caller, func(string) { v.reload() })
	return v
}

type ScheduleView struct {
	Element  // value embed
	caller   router.Caller
	tenantID string
	picker   *staffPicker
	// editor es el subárbol del scheduleeditor, reconstruido al cambiar de
	// profesional o tras cada escritura — igual patrón que el resto de las
	// pestañas de esta pantalla y que el demo de agenda.
	editor *SignalNodes
}

func (s *ScheduleView) Init(_ Ctx) {
	if s.editor == nil {
		s.editor = NewNodes()
	}
	s.picker.load(s.reload)
}

// reload trae los cuatro insumos del editor en paralelo (bloques, excepciones,
// feriados, cierres) y, cuando los cuatro respondieron, arma el
// scheduleeditor con los datos frescos y lo monta como único hijo de editor.
// Se recarga SIEMPRE tras una escritura — es lo que hace que lo que se ve sea
// siempre lo que está persistido (mismo motivo que la reserva y la agenda
// demo ya documentan).
//
// upsert_calendar_config corre primero e incondicionalmente: SaveDayBlocks /
// SaveDateBlocks fallan con ErrCalendarConfigNotFound si el profesional nunca
// tuvo un config, y UpsertCalendarConfig es un upsert real por
// (tenant,staff) — llamarlo de nuevo con los mismos valores no tiene efecto.
func (s *ScheduleView) reload() {
	staffID := s.picker.sel.Get()
	if staffID == "" {
		s.editor.Set([]*Element{Div().Text("No hay profesionales registrados todavía.")})
		return
	}

	s.caller.Call(ab.ModelName+"."+ab.OpUpsertCalendarConfig,
		&ab.UpsertCalendarConfigArgs{TenantId: s.tenantID, StaffId: staffID, Timezone: timezoneClinic, IsActive: true},
		nil, func(error) {
			// Los errores de configuración se ven al fallar la primera
			// escritura real (SaveDayBlocks etc.), que sí los reporta.
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

// buildEditor traduce las cuatro respuestas al vocabulario de scheduleeditor
// y arma el componente con sus callbacks traducidos a escrituras.
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

// Render arma el chasis con rightpanel: título, panel y la región con scroll
// son suyos — el selector de profesional va en HeadControls, el editor en
// Article. Mismo reparto que el demo de agenda; antes esta vista no tenía
// Render() propio en absoluto y no mostraba nada.
func (s *ScheduleView) Render() *Element {
	panel := &rightpanel.RightPanel{
		Title:        "Horario",
		HeadControls: Div().Set(clsHeader.AsAttr()).Child(s.picker.render("schedule-staff")),
		Article:      Div().BindChildren(s.editor),
	}
	return Div().Set(clsRoot.AsAttr()).Child(panel.Render())
}

// saveDayBlocks agrupa las filas del patrón por día de semana y guarda las
// siete llamadas en paralelo — mismo enfoque que el demo de agenda, adaptado
// al ScheduleClient real de appointment_booking.
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

// saveDateBlock reescribe los bloques de UNA fecha marcada con el nuevo
// horario que el usuario editó. Simplificación consciente: un MarkedDay lleva
// un único rango; una fecha con varios bloques (mañana+tarde) queda
// representada como el único bloque que este editor puede producir. Es la
// misma limitación que scheduleeditor.MarkedDay declara en su propio tipo.
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

// blocksToPattern agrupa los bloques SEMANALES (SpecificDate == 0) por rango
// horario — misma lógica que el demo de agenda.
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

// blocksToMarked agrupa los bloques FECHADOS (SpecificDate > 0) por fecha. Una
// fecha con más de un bloque colapsa al rango más amplio (el mínimo inicio, el
// máximo fin) — MarkedDay solo puede representar un rango por fecha; ver el
// comentario de saveDateBlock.
func blocksToMarked(blocks ab.WorkCalendarBlockList) []scheduleeditor.MarkedDay {
	byDate := map[int64]*scheduleeditor.MarkedDay{}
	order := []int64{}
	for _, b := range blocks {
		if !b.IsActive || b.SpecificDate == 0 {
			continue
		}
		md, ok := byDate[b.SpecificDate]
		if !ok {
			md = &scheduleeditor.MarkedDay{Date: unixToDay(b.SpecificDate), StartMin: int(b.StartMin), EndMin: int(b.EndMin)}
			byDate[b.SpecificDate] = md
			order = append(order, b.SpecificDate)
			continue
		}
		if int(b.StartMin) < md.StartMin {
			md.StartMin = int(b.StartMin)
		}
		if int(b.EndMin) > md.EndMin {
			md.EndMin = int(b.EndMin)
		}
	}
	out := make([]scheduleeditor.MarkedDay, 0, len(order))
	for _, d := range order {
		out = append(out, *byDate[d])
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

// dayToUnix convierte "YYYY-MM-DD" a segundos de medianoche UTC — la
// codificación que work_calendar_block.specific_date y las demás fechas del
// dominio usan. 0 en caso de fallo.
func dayToUnix(day string) int64 {
	nano, err := tintime.ParseDate(day)
	if err != nil {
		return 0
	}
	return nano / 1000000000
}

// unixToDay convierte segundos desde epoch a "YYYY-MM-DD" por
// FormatISO8601, NUNCA por FormatDate: FormatISO8601 está documentada como
// UTC; FormatDate aplica el offset de zona horaria y desplaza la fecha un día
// completo en cualquier huso negativo (Santiago es UTC−3). Ver
// docs/PLAN_LOCAL.md, el plan de appointment_booking (D8) documenta esta
// trampa en detalle.
func unixToDay(seconds int64) string {
	iso := tintime.FormatISO8601(seconds * 1000000000)
	if len(iso) < 10 {
		return iso
	}
	return iso[:10]
}
