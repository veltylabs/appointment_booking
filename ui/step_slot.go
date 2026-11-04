package ui

import (
	"webtyp.com/components/calendarslider"
	"webtyp.com/components/chipgrid"
	"webtyp.com/fmt"
	"webtyp.com/lang"
	"webtyp.com/time"

	. "webtyp.com/dom"
	. "webtyp.com/html"

	ab "github.com/veltylabs/appointment_booking"
)

// stepSlotPicker es el paso 3 (diagrama §4): calendario con la ocupación de
// cada día y las horas del día elegido. En mesón, con un profesional fijo, la
// casilla Sobrecupo suma las horas ya tomadas.
type stepSlotPicker struct {
	Element
	f        *BookingFlow
	errMsg   *SignalString
	notice   *SignalString
	cal      *SignalNodes
	hours    *chipgrid.ChipGrid
	extra    *SignalNodes
	day      string
	overbook bool
	slots    []ab.BookableSlot
}

func newStepSlotPicker(f *BookingFlow) *stepSlotPicker {
	s := &stepSlotPicker{f: f, errMsg: NewString(""), notice: NewString(""), cal: NewNodes(), extra: NewNodes()}
	s.hours = &chipgrid.ChipGrid{OnPick: s.pickHour}
	s.hours.Init(nil)
	return s
}

func (s *stepSlotPicker) Render() *Element {
	return stepBox().Child(
		noticeLine(s.notice),
		errorLine(s.errMsg),
		stepBox().BindChildren(s.cal),
		s.hours,
		stepBox().BindChildren(s.extra),
	)
}

func (s *stepSlotPicker) show() {
	s.load()
}

// load pide las horas del rango del calendario. Con un profesional fijo trae
// también las tomadas: cuentan para la ocupación y para el sobrecupo.
func (s *stepSlotPicker) load() {
	s.errMsg.Set("")
	s.day, s.overbook, s.slots = "", false, nil
	s.cal.Set(nil)
	s.hours.SetChips(nil)
	s.extra.Set(nil)
	from := time.MidnightUTC(time.NowUnix())
	args := &ab.ListServiceAvailabilityArgs{
		TenantId:     s.f.tenantID,
		ServiceId:    s.f.offer0().ServiceId,
		StaffId:      s.f.choice.staffID,
		From:         int64(from),
		To:           int64(from) + calendarMonths*31*secondsPerDay,
		IncludeTaken: s.f.choice.staffID != "",
	}
	list := ab.BookableSlotList{}
	s.f.caller.Call(ab.ModelName+"."+ab.OpListServiceAvailability, args, &list, func(err error) {
		if err != nil { // U4-sí
			s.errMsg.Set(err.Error())
			return
		}
		for _, it := range list {
			s.slots = append(s.slots, *it)
		}
		cal := &calendarslider.CalendarSlider{NumMonths: calendarMonths, Occupation: s.dayOccupation(s.slots), OnSelect: s.pickDay}
		cal.Init(nil)
		s.cal.Set([]*Element{cal.Render()})
		if !s.f.online && s.f.choice.staffID != "" { // U7: sobrecupo solo en mesón y con profesional fijo
			s.extra.Set([]*Element{Label().Set(clsFlowCheck.AsAttr()).Child(
				Input("checkbox").Attr("name", "overbooking").OnChange(func(e Event) {
					s.overbook = e.TargetChecked()
					s.showHours()
				}),
				Span().Text(lang.Translate(textOverbooking).String()),
			)})
		}
	})
}

func (s *stepSlotPicker) slotDay(startUtc time.Unix) string {
	return s.f.zone.Date(startUtc)
}

func (s *stepSlotPicker) slotTime(startUtc time.Unix) string {
	return s.f.zone.Clock(startUtc)
}

// dayOccupation agrupa las horas por día y calcula el porcentaje de ocupación
// (0 si no hay horas).
func (s *stepSlotPicker) dayOccupation(slots []ab.BookableSlot) []calendarslider.OccupationDay {
	type dayLoad struct {
		day   string
		free  int
		taken int
	}
	var loads []*dayLoad
	at := func(day string) *dayLoad {
		for _, l := range loads {
			if l.day == day {
				return l
			}
		}
		l := &dayLoad{day: day}
		loads = append(loads, l)
		return l
	}
	for _, sl := range slots {
		l := at(s.slotDay(sl.StartUtc))
		if sl.Taken {
			l.taken++
		} else {
			l.free++
		}
	}
	out := make([]calendarslider.OccupationDay, 0, len(loads))
	for _, l := range loads {
		out = append(out, calendarslider.OccupationDay{Date: l.day, Percent: l.taken * 100 / (l.taken + l.free)})
	}
	return out
}

func (s *stepSlotPicker) pickDay(day string) {
	s.day = day
	s.showHours()
}

// showHours pone en los chips las horas del día elegido (U5); las tomadas,
// solo con Sobrecupo marcado (U7).
func (s *stepSlotPicker) showHours() {
	var chips []chipgrid.Chip
	for _, sl := range s.slots {
		if s.slotDay(sl.StartUtc) != s.day || (sl.Taken && !s.overbook) {
			continue
		}
		chips = append(chips, chipgrid.Chip{Value: fmt.Convert(sl.StartUtc).String(), Label: s.slotTime(sl.StartUtc)})
	}
	s.hours.SetChips(chips)
}

// pickHour fija la hora; con "Cualquiera" fija también el profesional de esa
// hora (U6) y avanza con "2026-10-13 · 10:30".
func (s *stepSlotPicker) pickHour(value string) {
	for _, sl := range s.slots {
		if fmt.Convert(sl.StartUtc).String() != value || s.slotDay(sl.StartUtc) != s.day {
			continue
		}
		c := &s.f.choice
		c.slot = sl
		c.overbook = sl.Taken
		if c.staffID == "" { // U6-sí
			c.staffID = sl.StaffId
		}
		s.notice.Set("")
		s.f.next(s.f.slotLabel(sl.StartUtc))
		return
	}
}
