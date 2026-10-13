package ui

import (
	"webtyp.com/fmt"
	"webtyp.com/lang"
	tinytime "webtyp.com/time"

	. "webtyp.com/dom"

	ab "github.com/veltylabs/appointment_booking"
	paymentmanager "github.com/veltylabs/payment_manager"
	paymentmanagerui "github.com/veltylabs/payment_manager/ui"
)

// confirmAttempts es cuántas veces se relee la reserva después de un pago de
// mesón esperando que la raíz de composición la confirme (Y4).
const confirmAttempts = 3

// confirmRetryMillis es la espera entre esas lecturas.
const confirmRetryMillis = 1000

// pendingSummaryKey guarda el resumen de una reserva online mientras el
// paciente paga en la pasarela: la página se va y vuelve (diagrama §8).
const pendingSummaryKey = OnlineID + ".pending"

// stepPaymentPanel es el paso 6 (diagrama §7): mesón confirma, o cobra con
// payment_manager y espera la confirmación; online paga en la pasarela.
type stepPaymentPanel struct {
	Element
	f      *BookingFlow
	errMsg *SignalString
	notice *SignalString
	busy   *SignalBool
	body   *SignalNodes
}

func newStepPaymentPanel(f *BookingFlow) *stepPaymentPanel {
	return &stepPaymentPanel{f: f, errMsg: NewString(""), notice: NewString(""), busy: NewBool(false), body: NewNodes()}
}

func (s *stepPaymentPanel) Render() *Element {
	return stepBox().Child(noticeLine(s.notice), errorLine(s.errMsg), stepBox().BindChildren(s.body))
}

func (s *stepPaymentPanel) show() {
	s.errMsg.Set("")
	s.notice.Set("")
	res := s.f.choice.reservation
	step := &paymentmanagerui.PaymentStep{
		Caller: s.f.caller, IDs: s.f.ids, TenantID: s.f.tenantID,
		ReferenceModule: ab.ModelName, ReferenceID: res.Id,
	}
	switch {
	case s.f.online: // Y1-online
		step.Channel = paymentmanager.ChannelOnline
		step.ReturnTo = OnlineID
		o := s.f.offer()
		lines := append(s.f.summaryLines(), priceLabel(o.Price, o.Currency))
		_ = LocalStorageSet(pendingSummaryKey, fmt.JoinSlice(lines, lineSeparator))
	case s.f.offer().PaymentRequired: // Y2-sí
		step.Channel = paymentmanager.ChannelCounter
		step.OnPaid = s.paid
	default: // Y2-no
		s.body.Set([]*Element{actionButton(textConfirmBooking, s.busy, s.confirm)})
		return
	}
	s.body.Set([]*Element{step.Render()})
}

// confirm confirma una reserva de mesón sin pago (Y3).
func (s *stepPaymentPanel) confirm() {
	s.errMsg.Set("")
	s.busy.Set(true)
	res := s.f.choice.reservation
	s.f.caller.Call(ab.ModelName+"."+ab.OpChangeReservationStatus, &ab.ChangeReservationStatusArgs{
		TenantId: s.f.tenantID, Id: res.Id, Event: ab.EventConfirm, Revision: res.Revision,
	}, nil, func(err error) {
		s.busy.Set(false)
		if err != nil { // Y3-sí
			s.errMsg.Set(err.Error())
			return
		}
		s.f.finish()
	})
}

// paid corre cuando payment_manager registró el pago de mesón: la reserva la
// confirma la raíz de composición al recibir payment.paid (Y4).
func (s *stepPaymentPanel) paid(paymentID string) {
	s.f.choice.paymentID = paymentID
	s.checkConfirmed(1)
}

func (s *stepPaymentPanel) checkConfirmed(attempt int) {
	res := &ab.Reservation{}
	s.f.caller.Call(ab.ModelName+"."+ab.OpGetReservation, &ab.GetReservationArgs{TenantId: s.f.tenantID, Id: s.f.choice.reservation.Id}, res,
		func(err error) {
			if err != nil {
				s.errMsg.Set(err.Error())
				return
			}
			if res.Status == ab.StatusConfirmed { // Y4-sí
				s.f.finish()
				return
			}
			s.notice.Set(lang.Translate(textPaymentRegistered).String()) // Y4-no
			if attempt < confirmAttempts {
				tinytime.AfterFunc(confirmRetryMillis, func() { s.checkConfirmed(attempt + 1) })
			}
		})
}

// lineSeparator une las líneas del resumen guardado.
const lineSeparator = "\n"
