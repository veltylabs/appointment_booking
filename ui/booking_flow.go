package ui

import (
	"webtyp.com/components/stepaccordion"
	"webtyp.com/fmt"
	"webtyp.com/lang"
	"webtyp.com/model"
	"webtyp.com/router"
	tinytime "webtyp.com/time"
	"webtyp.com/widget"

	. "webtyp.com/dom"
	. "webtyp.com/html"

	ab "github.com/veltylabs/appointment_booking"
	paymentmanager "github.com/veltylabs/payment_manager"
)

// NameBookingFlow es la identidad de widget de la reserva en 6 pasos (mesón y
// online comparten el flujo; docs/diagrams/booking_flow.md).
const NameBookingFlow = widget.Name("bookingflow")

const (
	PartFlowStep    = widget.Part("step")    // contenido de un paso: columna con aire
	PartFlowError   = widget.Part("error")   // error de una llamada, siempre visible
	PartFlowNotice  = widget.Part("notice")  // aviso (hora tomada, pago confirmándose)
	PartFlowField   = widget.Part("field")   // etiqueta + control de un formulario
	PartFlowCheck   = widget.Part("check")   // casilla + su texto, en fila
	PartFlowInput   = widget.Part("input")   // un <input> o <textarea>
	PartFlowActions = widget.Part("actions") // fila de botones de un paso
	PartFlowAction  = widget.Part("action")  // botón principal de un paso
	PartFlowFinal   = widget.Part("final")   // panel final
	PartFlowTitle   = widget.Part("title")   // título del panel final
)

var (
	clsFlowRoot    = NameBookingFlow.Root()
	clsFlowStep    = NameBookingFlow.Class(PartFlowStep)
	clsFlowError   = NameBookingFlow.Class(PartFlowError)
	clsFlowNotice  = NameBookingFlow.Class(PartFlowNotice)
	clsFlowField   = NameBookingFlow.Class(PartFlowField)
	clsFlowCheck   = NameBookingFlow.Class(PartFlowCheck)
	clsFlowInput   = NameBookingFlow.Class(PartFlowInput)
	clsFlowActions = NameBookingFlow.Class(PartFlowActions)
	clsFlowAction  = NameBookingFlow.Class(PartFlowAction)
	clsFlowFinal   = NameBookingFlow.Class(PartFlowFinal)
	clsFlowTitle   = NameBookingFlow.Class(PartFlowTitle)
)

// Claves de los pasos del acordeón.
const (
	stepService = "service"
	stepStaff   = "staff"
	stepSlot    = "slot"
	stepPatient = "patient"
	stepSummary = "summary"
	stepPayment = "payment"
)

// Textos de la pantalla (claves en inglés; su español en lang.json).
const (
	textSpecialtyAndService lang.Text = "Specialty and service"
	textProfessional        lang.Text = "Professional"
	textDateAndTime         lang.Text = "Date and time"
	textSummary             lang.Text = "Summary"
	textPatient             lang.Text = "Patient"
	textPaymentConfirmation lang.Text = "Payment and confirmation"
	textAnyProfessional     lang.Text = "Any available professional"
	textNoServicesToBook    lang.Text = "No services to book"
	textNoTimesAvailable    lang.Text = "No times available"
	textOverbooking         lang.Text = "Overbooking"
	textNotes               lang.Text = "Notes"
	textSearch              lang.Text = "Search"
	textRegisterPatient     lang.Text = "Register patient"
	textContinue            lang.Text = "Continue"
	textConfirmBooking      lang.Text = "Confirm booking"
	textBookingConfirmed    lang.Text = "Booking confirmed"
	textNewBooking          lang.Text = "New booking"
	textRetry               lang.Text = "Retry"
	textTimeJustTaken       lang.Text = "That time was just taken"
	textPendingPaymentHolds lang.Text = "You already have bookings awaiting payment"
	textPaymentRegistered   lang.Text = "Payment registered; confirming…"
	textPaymentIncomplete   lang.Text = "The payment was not completed; the time will be released"
	textConfirmPaymentFail  lang.Text = "We could not confirm the payment; please call the clinic"
	textStartAgain          lang.Text = "Start again"
	textFullName            lang.Text = "Full name"
	textEmail               lang.Text = "Email"
	textPhone               lang.Text = "Phone number"
	textRut                 lang.Text = "RUT"
)

// calendarMonths es cuántos meses ofrece el calendario del paso 3.
const calendarMonths = 3

// secondsPerDay convierte días a segundos (fechas en segundos UTC).
const secondsPerDay = 86400

// BookingFlow es la reserva en 6 pasos. Mesón (NewBookingView) y online
// (OnlineBrowser) son el mismo flujo; online cambia el paso 5 (el paciente se
// registra solo), el 6 (paga en la pasarela) y no ofrece sobrecupo.
type BookingFlow struct {
	Element // value embed — NEVER pointer

	caller   router.Caller
	ids      model.IDGenerator
	tenantID string
	online   bool

	content *SignalNodes // el acordeón o el panel final
	acc     *stepaccordion.StepAccordion
	round   int // cuenta las reservas de esta pantalla: da a cada contenido su key

	service *stepServicePicker
	staff   *stepStaffPicker
	slot    *stepSlotPicker
	summary *stepSummaryCard
	patient *stepPatientForm
	payment *stepPaymentPanel

	choice booking
}

// booking es lo elegido en cada paso. Reabrir un paso borra lo de los
// siguientes (clearAfter).
type booking struct {
	specialty   string
	serviceName string
	offers      []ab.BookableService // las configuraciones que ofrecen el servicio elegido
	staffID     string               // "" = cualquier profesional (hasta elegir hora)
	anyStaff    bool                 // el paso 2 eligió "Cualquier profesional disponible"
	slot        ab.BookableSlot
	overbook    bool
	notes       string
	patientID   string
	patientName string
	reservation ab.Reservation
	paymentID   string
}

func (f *BookingFlow) WidgetName() widget.Name { return NameBookingFlow }
func (f *BookingFlow) WidgetKind() widget.Kind { return widget.Form }

func newBookingFlow(caller router.Caller, ids model.IDGenerator, tenantID string, online bool) *BookingFlow {
	return &BookingFlow{caller: caller, ids: ids, tenantID: tenantID, online: online}
}

// Init arma el acordeón y carga el paso 1. En la pantalla online, si la página
// vuelve de la pasarela, muestra el resultado del pago (diagrama §8). Llamarla
// de nuevo no hace nada.
func (f *BookingFlow) Init(_ Ctx) {
	if f.content != nil {
		return
	}
	f.content = NewNodes()
	if f.online {
		if paymentID, status, ok := takeReturn(); ok {
			f.showReturn(paymentID, status)
			return
		}
	}
	f.restart()
}

func (f *BookingFlow) Render() *Element {
	f.Init(nil)
	return Div().Set(clsFlowRoot.AsAttr()).BindChildren(f.content)
}

// restart empieza una reserva nueva desde el paso 1.
func (f *BookingFlow) restart() {
	f.choice = booking{}
	f.service = newStepServicePicker(f)
	f.staff = newStepStaffPicker(f)
	f.slot = newStepSlotPicker(f)
	f.summary = newStepSummaryCard(f)
	f.patient = newStepPatientForm(f)
	f.payment = newStepPaymentPanel(f)
	f.acc = &stepaccordion.StepAccordion{
		Steps: []stepaccordion.Step{
			{Key: stepService, Title: textSpecialtyAndService, Body: f.service},
			{Key: stepStaff, Title: textProfessional, Body: f.staff},
			{Key: stepSlot, Title: textDateAndTime, Body: f.slot},
			{Key: stepPatient, Title: textPatient, Body: f.patient},
			{Key: stepSummary, Title: textSummary, Body: f.summary},
			{Key: stepPayment, Title: textPaymentConfirmation, Body: f.payment},
		},
		OnReopen: func(key string) {
			f.clearAfter(key)
			f.enter(key)
		},
	}
	f.show(f.acc.Render())
	f.enter(stepService)
}

// show reemplaza el contenido de la pantalla. Cada contenido lleva una key
// nueva: dom solo retira los hijos que puede identificar, y el acordeón de la
// reserva anterior tiene que irse.
func (f *BookingFlow) show(el *Element) {
	f.round++
	f.content.Set([]*Element{el.Key(contentKey + fmt.Convert(f.round).String())})
}

// contentKey es el prefijo de la key del contenido de la pantalla.
const contentKey = "flow-"

// next termina el paso abierto con su resumen y entra al siguiente.
func (f *BookingFlow) next(summary string) {
	f.acc.Advance(summary)
	f.enter(f.acc.Open())
}

// enter prepara el paso que se acaba de abrir.
func (f *BookingFlow) enter(key string) {
	switch key {
	case stepService:
		f.service.load()
	case stepStaff:
		f.staff.show()
	case stepSlot:
		f.slot.load()
	case stepPatient:
		f.patient.show()
	case stepSummary:
		f.summary.show()
	case stepPayment:
		f.payment.show()
	}
}

// clearAfter borra lo elegido desde el paso key en adelante (reabrir "Editar").
func (f *BookingFlow) clearAfter(key string) {
	c := &f.choice
	switch key {
	case stepService:
		c.specialty, c.serviceName, c.offers = "", "", nil
		fallthrough
	case stepStaff:
		c.staffID, c.anyStaff = "", false
		fallthrough
	case stepSlot:
		if c.anyStaff { // la hora había fijado al profesional: vuelve a ser "cualquiera"
			c.staffID = ""
		}
		c.slot, c.overbook = ab.BookableSlot{}, false
		fallthrough
	case stepPatient:
		c.patientID, c.patientName = "", ""
		fallthrough
	case stepSummary:
		c.notes, c.reservation = "", ab.Reservation{}
		fallthrough
	case stepPayment:
		c.paymentID = ""
	}
}

// offer es la configuración del profesional elegido (o fijado al elegir hora).
func (f *BookingFlow) offer() ab.BookableService {
	for _, o := range f.choice.offers {
		if o.StaffId == f.choice.staffID {
			return o
		}
	}
	return ab.BookableService{}
}

// offer0 es una configuración cualquiera del servicio elegido (todas comparten
// servicio, nombre y duración).
func (f *BookingFlow) offer0() ab.BookableService {
	if len(f.choice.offers) == 0 {
		return ab.BookableService{}
	}
	return f.choice.offers[0]
}

// finish muestra el panel final de una reserva hecha en esta pantalla (§9).
func (f *BookingFlow) finish() {
	f.show(f.finalPanel(textBookingConfirmed, f.summaryLines(), textNewBooking))
}

// summaryLines son los datos de la reserva para el panel final: profesional,
// servicio, día, hora, paciente y, si se pagó, el monto.
func (f *BookingFlow) summaryLines() []string {
	o := f.offer()
	lines := []string{o.StaffName, o.ServiceName, slotLabel(f.choice.slot.StartUtc), f.choice.patientName}
	if f.choice.paymentID != "" {
		lines = append(lines, priceLabel(o.Price, o.Currency))
	}
	return lines
}

// finalPanel es el cierre del flujo: un título, sus líneas y el botón que
// empieza de nuevo.
func (f *BookingFlow) finalPanel(title lang.Text, lines []string, again lang.Text) *Element {
	panel := Div().Set(clsFlowFinal.AsAttr()).Child(H3().Set(clsFlowTitle.AsAttr()).Text(lang.Translate(title).String()))
	for _, l := range lines {
		if l != "" {
			panel.Child(P().Text(l))
		}
	}
	return panel.Child(Div().Set(clsFlowActions.AsAttr()).Child(actionButton(again, nil, f.restart)))
}

// slotLabel es "2026-10-13 · 10:30": día y hora de un inicio en segundos UTC.
func slotLabel(startUtc tinytime.Unix) string {
	if startUtc == 0 {
		return ""
	}
	return slotDay(startUtc) + " · " + slotTime(startUtc)
}

// hourMinuteLen es el largo de "HH:MM" dentro de "HH:MM:SS".
const hourMinuteLen = 5

func slotDay(startUtc tinytime.Unix) string { return tinytime.FormatDate(startUtc) }

// slotTime es la hora "10:30" (sin segundos) de un inicio en segundos UTC.
func slotTime(startUtc tinytime.Unix) string {
	t := tinytime.FormatTime(startUtc)
	if len(t) > hourMinuteLen {
		return t[:hourMinuteLen]
	}
	return t
}

// priceLabel muestra un precio con el formato de payment_manager.
func priceLabel(price float64, currency string) string {
	amount, err := ab.MinorUnits(price, currency)
	if err != nil {
		return fmt.Convert(price).String() + " " + currency
	}
	return paymentmanager.FormatAmount(amount, currency)
}

// stepBox es el contenedor del contenido de un paso.
func stepBox() *Element { return Div().Set(clsFlowStep.AsAttr()) }

// errorLine es el mensaje de error de un paso: vacío no ocupa lugar.
func errorLine(msg *SignalString) *Element {
	return P().Set(clsFlowError.AsAttr()).Attr("role", "alert").BindText(msg)
}

// noticeLine es un aviso de un paso.
func noticeLine(msg *SignalString) *Element {
	return P().Set(clsFlowNotice.AsAttr()).Attr("role", "status").BindText(msg)
}

// actionButton es el botón principal de un paso; con busy no se puede pulsar
// dos veces mientras una llamada está en curso.
func actionButton(label lang.Text, busy *SignalBool, run func()) *Element {
	btn := Button().Set(clsFlowAction.AsAttr()).
		Attr("type", "button").
		Text(lang.Translate(label).String())
	if busy != nil {
		btn.BindState(widget.Busy, busy)
	}
	return btn.OnClick(func(Event) {
		if busy != nil && busy.Get() {
			return
		}
		run()
	})
}

// textField es una etiqueta con su <input> enlazado a value.
func textField(label lang.Text, name, kind string, value *SignalString) *Element {
	return Label().Set(clsFlowField.AsAttr()).
		Text(lang.Translate(label).String()).
		Child(Input(kind).Set(clsFlowInput.AsAttr()).Attr("name", name).Bind(value))
}
