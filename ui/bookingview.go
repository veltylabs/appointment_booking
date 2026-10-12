package ui

import (
	. "webtyp.com/dom"
	"webtyp.com/fmt"
	. "webtyp.com/html"
	"webtyp.com/layout/rightpanel"
	"webtyp.com/model"
	"webtyp.com/router"
	"webtyp.com/view"
	"webtyp.com/widget"
)

// NameBookingView es la identidad de widget de esta pantalla.
const NameBookingView = widget.Name("bookingview")

const PartBookingHeader = widget.Part("header")

var (
	clsBookingRoot   = NameBookingView.Root()
	clsBookingHeader = NameBookingView.Class(PartBookingHeader)
)

func (v *BookingView) WidgetName() widget.Name { return NameBookingView }
func (v *BookingView) WidgetKind() widget.Kind { return widget.Region }

const (
	bookingWindowPastDays   = 30
	bookingWindowFutureDays = 365
)

// NewBookingView construye la pantalla "Reserva Hora" con el flujo progresivo en 6 pasos.
func NewBookingView(caller router.Caller, tenantID string) Component {
	v := &BookingView{
		caller:   caller,
		tenantID: tenantID,
		mode:     NewString(string(ModeCounter)),
		panel:    NewNodes(),
	}
	v.accordion = NewBookingAccordion(caller, tenantID, ModeCounter, nil)
	return v
}

type BookingView struct {
	Element
	caller    router.Caller
	tenantID  string
	mode      *SignalString
	accordion *BookingAccordion
	panel     *SignalNodes
}

func (v *BookingView) Init(_ Ctx) {
	if v.accordion == nil {
		v.accordion = NewBookingAccordion(v.caller, v.tenantID, BookingMode(v.mode.Get()), nil)
	}
	v.panel.Set([]*Element{v.accordion.Render()})
}

func (v *BookingView) switchMode(m BookingMode) {
	v.mode.Set(string(m))
	v.accordion = NewBookingAccordion(v.caller, v.tenantID, m, nil)
	v.panel.Set([]*Element{v.accordion.Render()})
}

func (v *BookingView) Render() *Element {
	controls := Div().Set(clsBookingHeader.AsAttr()).
		Child(Button().Attr("type", "button").Class("btn-mode-counter").
			Text("Recepcionista (Mostrador)").
			OnClick(func(Event) { v.switchMode(ModeCounter) })).
		Child(Button().Attr("type", "button").Class("btn-mode-online").
			Text("Paciente Online (Fricción Cero)").
			OnClick(func(Event) { v.switchMode(ModeOnline) }))

	panel := &rightpanel.RightPanel{
		Title:        NavLabel,
		HeadControls: controls,
		Article:      Div().BindChildren(v.panel),
	}
	return Div().Set(clsBookingRoot.AsAttr()).Child(panel.Render())
}

type byDay struct {
	view.Presenter
}

func (p byDay) Filter(term string) []view.Item {
	if term == "" {
		return nil
	}
	var items []view.Item
	for _, it := range p.Presenter.Items() {
		if it.ID != "" && it.LeadMain != "" {
			items = append(items, it)
		}
	}
	return items
}

func (p byDay) Save(recs []model.Model, done func(error)) {
	if s, ok := p.Presenter.(view.Saver); ok {
		s.Save(recs, done)
		return
	}
	done(fmt.Err("byDay: underlying presenter cannot save"))
}
