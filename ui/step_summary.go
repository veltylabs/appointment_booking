package ui

import (
	"webtyp.com/components/contentcard"
	"webtyp.com/fmt"
	"webtyp.com/lang"
	"webtyp.com/model"

	. "webtyp.com/dom"
	. "webtyp.com/html"

	ab "github.com/veltylabs/appointment_booking"
)

// stepSummaryCard es el paso 5, "revise sus respuestas": todo lo elegido
// (profesional, servicio, precio, día, hora y paciente) y las notas. Continuar
// crea la reserva: de mesón (o sobrecupo) u online retenida (M3, M4, N3).
type stepSummaryCard struct {
	Element
	f      *BookingFlow
	errMsg *SignalString
	busy   *SignalBool
	card   *SignalNodes
	notes  *SignalString
}

func newStepSummaryCard(f *BookingFlow) *stepSummaryCard {
	return &stepSummaryCard{f: f, errMsg: NewString(""), busy: NewBool(false), card: NewNodes(), notes: NewString("")}
}

func (s *stepSummaryCard) Render() *Element {
	return stepBox().Child(
		errorLine(s.errMsg),
		Div().BindChildren(s.card),
		Label().Set(clsFlowField.AsAttr()).
			Text(lang.Translate(textNotes).String()).
			Child(NewElement("textarea").Set(clsFlowInput.AsAttr()).Attr("name", "notes").Bind(s.notes)),
		Div().Set(clsFlowActions.AsAttr()).Child(actionButton(textContinue, s.busy, s.confirm)),
	)
}

func (s *stepSummaryCard) show() {
	s.errMsg.Set("")
	o := s.f.offer()
	s.notes.Set(s.f.choice.notes)
	body := Div().Child(
		P().Text(o.ServiceName+" · "+fmt.Convert(o.DurationMin).String()+" min"),
		P().Text(slotLabel(s.f.choice.slot.StartUtc)),
		P().Text(s.f.choice.patientName),
		Strong().Text(priceLabel(o.Price, o.Currency)),
	)
	card := &contentcard.ContentCard{Header: Strong().Text(o.StaffName), Body: body}
	s.card.Set([]*Element{card.Render()})
}

// confirm guarda las notas y crea la reserva. Hora tomada: aviso y vuelve al
// paso 3.
func (s *stepSummaryCard) confirm() {
	c := &s.f.choice
	c.notes = fmt.Convert(s.notes.Get()).TrimSpace().String()
	s.errMsg.Set("")
	id := s.f.ids.NewID()
	cfg := s.f.offer().ConfigId
	var op string
	var args model.Encodable
	switch {
	case s.f.online:
		op = ab.OpCreateOnlineReservation
		args = &ab.CreateOnlineReservationArgs{TenantId: s.f.tenantID, Id: id, ClientId: c.patientID,
			EmployeeServiceConfigId: cfg, SlotStartUtc: c.slot.StartUtc, Notes: c.notes}
	case c.overbook: // M3-sí
		op = ab.OpCreateOverbooking
		args = &ab.CreateOverbookingArgs{TenantId: s.f.tenantID, Id: id, ClientId: c.patientID,
			EmployeeServiceConfigId: cfg, SlotStartUtc: c.slot.StartUtc, Notes: c.notes, Origin: ab.OriginCounter}
	default: // M3-no
		op = ab.OpCreateReservation
		args = &ab.CreateReservationArgs{TenantId: s.f.tenantID, Id: id, ClientId: c.patientID,
			EmployeeServiceConfigId: cfg, SlotStartUtc: c.slot.StartUtc, Notes: c.notes, Origin: ab.OriginCounter}
	}
	s.busy.Set(true)
	res := &ab.Reservation{}
	s.f.caller.Call(ab.ModelName+"."+op, args, res, func(err error) {
		s.busy.Set(false)
		switch {
		case err == nil:
			c.reservation = *res
			o := s.f.offer()
			s.f.next(priceLabel(o.Price, o.Currency))
		case err.Error() == ab.ErrSlotTaken.Error(): // M4 / N3 hora tomada
			s.f.acc.Reopen(stepSlot)
			s.f.slot.notice.Set(lang.Translate(textTimeJustTaken).String())
		case err.Error() == ab.ErrTooManyHolds.Error(): // N3 demasiadas retenciones
			s.errMsg.Set(lang.Translate(textPendingPaymentHolds).String())
		default:
			s.errMsg.Set(err.Error())
		}
	})
}
