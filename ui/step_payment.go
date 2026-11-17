package ui

import (
	"webtyp.com/crud"
	"webtyp.com/fmt"
	"webtyp.com/lang"
	"webtyp.com/model"
	"webtyp.com/time"

	. "webtyp.com/dom"

	ab "github.com/veltylabs/appointment_booking"
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
// PaymentGateway y espera la confirmación; online paga en la pasarela.
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
	switch {
	case s.f.online: // Y1-online: la reserva online ya trae el cobro que empezó el servidor
		o := s.f.offer()
		lines := append(s.f.summaryLines(), priceLabel(o.Price, o.Currency))
		_ = LocalStorageSet(pendingSummaryKey, fmt.JoinSlice(lines, lineSeparator))
		s.body.Set([]*Element{s.onlinePayButton()})
	case !s.f.offer().PaymentRequired: // Y2-no
		s.body.Set([]*Element{actionButton(textConfirmBooking, s.busy, s.confirm)})
	case s.f.steps.Payment == nil: // Y2-sí, pero la app no cobra
		s.errMsg.Set(ab.ErrPaymentsUnavailable.Error())
	default: // Y2-sí: el cobro es el PaymentStep de la app
		s.body.Set([]*Element{s.f.steps.Payment.Render(res.Id, s.paid)})
	}
}

// onlinePayButton lleva a la pasarela que el servidor dejó lista en la reserva online: con token, por POST.
func (s *stepPaymentPanel) onlinePayButton() *Element {
	return actionButton(textPayOnline, s.busy, func() {
		o := s.f.choice.online
		switch {
		case o.PayField != "" && o.PayToken != "":
			NavigatePost(o.PaymentUrl, []fmt.KeyValue{{Key: o.PayField, Value: o.PayToken}})
		case o.PaymentUrl != "":
			Navigate(o.PaymentUrl)
		default:
			s.errMsg.Set(ab.ErrPaymentsUnavailable.Error())
		}
	})
}

// confirm confirma una reserva de mesón sin pago (Y3).
func (s *stepPaymentPanel) confirm() {
	s.errMsg.Set("")
	s.busy.Set(true)
	res := s.f.choice.reservation
	rec := &ab.Reservation{Id: res.Id, Status: ab.StatusConfirmed}
	s.f.caller.Call(operation(rec, model.Update),
		&crud.UpdateArgs{IDs: []string{res.Id}, Fields: []string{ab.Reservation_.Status}, Record: rec}, nil, func(err error) {
			s.busy.Set(false)
			if err != nil { // Y3-sí
				s.errMsg.Set(err.Error())
				return
			}
			s.f.finish()
		})
}

// paid corre cuando el gateway registró el pago de mesón: la reserva la
// confirma la raíz de composición al recibir payment.paid (Y4).
func (s *stepPaymentPanel) paid(paymentID string) {
	s.f.choice.paymentID = paymentID
	s.checkConfirmed(1)
}

func (s *stepPaymentPanel) checkConfirmed(attempt int) {
	list := ab.ReservationList{}
	s.f.caller.Call(operation(&ab.Reservation{}, model.Read), &crud.ReadArgs{ID: s.f.choice.reservation.Id}, &list,
		func(err error) {
			if err == nil && len(list) == 0 {
				err = ab.ErrNotFound
			}
			if err != nil {
				s.errMsg.Set(err.Error())
				return
			}
			if list[0].Status == ab.StatusConfirmed { // Y4-sí
				s.f.finish()
				return
			}
			s.notice.Set(lang.Translate(textPaymentRegistered).String()) // Y4-no
			if attempt < confirmAttempts {
				time.AfterFunc(confirmRetryMillis, func() { s.checkConfirmed(attempt + 1) })
			}
		})
}

// lineSeparator une las líneas del resumen guardado.
const lineSeparator = "\n"
