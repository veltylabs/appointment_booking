package ui

import (
	"webtyp.com/components/segmentedcontrol"
	"webtyp.com/fmt"
	"webtyp.com/lang"
	"webtyp.com/model"
	"webtyp.com/router"
	"webtyp.com/time"
	"webtyp.com/widget"

	. "webtyp.com/dom"
	. "webtyp.com/html"

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
	if !s.f.online && !s.f.offer().PaymentRequired { // Y2-no
		s.body.Set([]*Element{actionButton(textConfirmBooking, s.busy, s.confirm)})
		return
	}

	step := &bookingPaymentStep{
		caller:   s.f.caller,
		ids:      s.f.ids,
		tenantID: s.f.tenantID,
		resID:    res.Id,
		onPaid:   s.paid,
	}
	if s.f.online { // Y1-online
		step.channel = "online"
		step.returnTo = OnlineID
		o := s.f.offer()
		lines := append(s.f.summaryLines(), priceLabel(o.Price, o.Currency))
		_ = LocalStorageSet(pendingSummaryKey, fmt.JoinSlice(lines, lineSeparator))
	} else { // Y2-sí
		step.channel = "counter"
	}
	step.init()
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

// paid corre cuando el gateway registró el pago de mesón: la reserva la
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
				time.AfterFunc(confirmRetryMillis, func() { s.checkConfirmed(attempt + 1) })
			}
		})
}

// lineSeparator une las líneas del resumen guardado.
const lineSeparator = "\n"

type bookingPaymentStep struct {
	Element
	caller   router.Caller
	ids      model.IDGenerator
	tenantID string
	channel  string
	resID    string
	returnTo string
	onPaid   func(paymentID string)

	terminal bool
	amount   *SignalString
	errMsg   *SignalString
	body     *SignalNodes
	busy     *SignalBool
	method   string
}

func (s *bookingPaymentStep) init() {
	s.amount = NewString("Cargando…")
	s.errMsg = NewString("")
	s.body = NewNodes()
	s.busy = NewBool(false)
	s.method = "cash"
	s.loadCharge()
}

func (s *bookingPaymentStep) loadCharge() {
	out := &ab.BookingChargeResult{}
	s.caller.Call(ab.ModelName+"."+ab.OpGetBookingCharge,
		&ab.BookingChargeArgs{TenantId: s.tenantID, ReservationId: s.resID}, out,
		func(err error) {
			if err != nil {
				s.amount.Set("")
				s.errMsg.Set(err.Error())
				return
			}
			if !out.Payable {
				s.amount.Set("Sin cobro pendiente")
				return
			}
			s.terminal = out.Terminal
			s.amount.Set(FormatAmount(out.Amount, out.Currency))
			if s.channel == "online" {
				s.body.Set([]*Element{s.payButton("Pagar online", s.startOnline)})
				return
			}
			s.body.Set([]*Element{s.methodPicker(), s.payButton("Registrar pago", s.registerCounter)})
		})
}

func (s *bookingPaymentStep) methodPicker() *Element {
	picker := &segmentedcontrol.SegmentedControl{
		Options: []segmentedcontrol.Option{
			{Value: "cash", Label: "Efectivo"},
			{Value: "debit", Label: "Débito"},
			{Value: "credit", Label: "Crédito"},
			{Value: "transfer", Label: "Transferencia"},
		},
		Selected: s.method,
		OnChange: func(v string) { s.method = v },
	}
	return picker.Render()
}

func (s *bookingPaymentStep) payButton(label string, run func()) *Element {
	btn := Button().
		Attr("type", "button").
		BindState(widget.Busy, s.busy).
		Text(label).
		OnClick(func(Event) {
			if s.busy.Get() {
				return
			}
			s.busy.Set(true)
			s.errMsg.Set("")
			run()
		})
	return Div().Child(btn)
}

func (s *bookingPaymentStep) registerCounter() {
	if s.terminal && (s.method == "debit" || s.method == "credit") {
		s.startTerminal()
		return
	}
	out := &ab.BookingPaymentResult{}
	args := &ab.BookingPaymentArgs{
		TenantId:      s.tenantID,
		Id:            s.ids.NewID(),
		ReservationId: s.resID,
		Method:        s.method,
	}
	s.caller.Call(ab.ModelName+"."+ab.OpCreateBookingCounterPayment, args, out, func(err error) {
		s.busy.Set(false)
		if err != nil {
			s.errMsg.Set(err.Error())
			return
		}
		if s.onPaid != nil {
			s.onPaid(out.PaymentId)
		}
	})
}

func (s *bookingPaymentStep) startTerminal() {
	out := &ab.BookingPaymentResult{}
	args := &ab.BookingPaymentArgs{
		TenantId:      s.tenantID,
		Id:            s.ids.NewID(),
		ReservationId: s.resID,
		Method:        s.method,
	}
	s.caller.Call(ab.ModelName+"."+ab.OpStartBookingTerminalPayment, args, out, func(err error) {
		if err != nil {
			s.busy.Set(false)
			s.errMsg.Set(err.Error())
			return
		}
		s.pollTerminal(out.PaymentId)
	})
}

func (s *bookingPaymentStep) pollTerminal(paymentID string) {
	out := &ab.BookingPaymentResult{}
	args := &ab.BookingPaymentArgs{
		TenantId: s.tenantID,
		Id:       paymentID,
	}
	s.caller.Call(ab.ModelName+"."+ab.OpRefreshBookingTerminalPayment, args, out, func(err error) {
		if err != nil {
			s.busy.Set(false)
			s.errMsg.Set(err.Error())
			return
		}
		if out.Status == "paid" {
			s.busy.Set(false)
			if s.onPaid != nil {
				s.onPaid(paymentID)
			}
			return
		}
		if out.Status == "pending" {
			time.AfterFunc(3000, func() { s.pollTerminal(paymentID) })
			return
		}
		s.busy.Set(false)
		s.errMsg.Set("Pago terminal: " + out.Status)
	})
}

func (s *bookingPaymentStep) startOnline() {
	out := &ab.BookingPaymentResult{}
	args := &ab.BookingPaymentArgs{
		TenantId:      s.tenantID,
		Id:            s.ids.NewID(),
		ReservationId: s.resID,
		ReturnTo:      s.returnTo,
	}
	s.caller.Call(ab.ModelName+"."+ab.OpStartBookingOnlinePayment, args, out, func(err error) {
		s.busy.Set(false)
		if err != nil {
			s.errMsg.Set(err.Error())
			return
		}
		if out.TokenField != "" && out.Token != "" {
			NavigatePost(out.RedirectUrl, []fmt.KeyValue{{Key: out.TokenField, Value: out.Token}})
			return
		}
		if out.RedirectUrl != "" {
			Navigate(out.RedirectUrl)
		}
	})
}

func (s *bookingPaymentStep) Render() *Element {
	return Div().Child(
		P().BindText(s.amount),
		P().Attr("role", "alert").BindText(s.errMsg),
		Div().BindChildren(s.body),
	)
}
