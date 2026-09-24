package ui

import (
	"webtyp.com/layout/crudview"
	"webtyp.com/layout/rightpanel"
	"webtyp.com/router"
	"webtyp.com/unixid"
	"webtyp.com/widget"

	. "webtyp.com/dom"
	. "webtyp.com/html"

	ab "github.com/veltylabs/appointment_booking"
)

// NameServiceConfigView es la identidad de widget de esta pestaña.
const NameServiceConfigView = widget.Name("serviceconfigview")

const PartServiceHeader = widget.Part("header")

var (
	clsServiceRoot   = NameServiceConfigView.Root()
	clsServiceHeader = NameServiceConfigView.Class(PartServiceHeader)
)

func (s *ServiceConfigView) WidgetName() widget.Name { return NameServiceConfigView }
func (s *ServiceConfigView) WidgetKind() widget.Kind { return widget.Region }

// NewServiceConfigView construye la pestaña "Servicios" de la pantalla Personal.
func NewServiceConfigView(caller router.Caller, tenantID string) Component {
	v := &ServiceConfigView{caller: caller, tenantID: tenantID}
	v.picker = newStaffPicker(caller, func(string) { v.rebuildPanel() })
	return v
}

type ServiceConfigView struct {
	Element
	caller   router.Caller
	tenantID string
	picker   *staffPicker
	panel    *SignalNodes
}

func (s *ServiceConfigView) Init(_ Ctx) {
	if s.panel == nil {
		s.panel = NewNodes()
	}
	s.picker.load(s.rebuildPanel)
}

func (s *ServiceConfigView) rebuildPanel() {
	staffID := s.picker.sel.Get()
	if staffID == "" {
		s.panel.Set([]*Element{Div().Text("No hay profesionales registrados todavía.")})
		return
	}

	ids, err := unixid.NewUnixID()
	if err != nil {
		s.panel.Set([]*Element{Div().Text("Error interno: " + err.Error())})
		return
	}

	cv, err := crudview.New(crudview.Config{
		ParentID:  ID + ".services." + staffID,
		Presenter: ab.NewEmployeeServiceConfigView(s.caller, s.tenantID, staffID),
		IDs:       ids,
	})
	if err != nil {
		s.panel.Set([]*Element{Div().Text("Error interno: " + err.Error())})
		return
	}
	s.panel.Set([]*Element{Div().Child(cv)})
}

func (s *ServiceConfigView) Render() *Element {
	panel := &rightpanel.RightPanel{
		Title:        "Servicios",
		HeadControls: Div().Set(clsServiceHeader.AsAttr()).Child(s.picker.render("service-config-staff")),
		Article:      Div().BindChildren(s.panel),
	}
	return Div().Set(clsServiceRoot.AsAttr()).Child(panel.Render())
}
