package appointment_booking

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

// NameServiceConfigView es la identidad de widget de esta pestaña —
// rightpanel pone el chasis, esta hoja solo ajusta la fila del selector.
const NameServiceConfigView = widget.Name("serviceconfigview")

const PartServiceHeader = widget.Part("header")

var (
	clsServiceRoot   = NameServiceConfigView.Root()
	clsServiceHeader = NameServiceConfigView.Class(PartServiceHeader)
)

func (s *ServiceConfigView) WidgetName() widget.Name { return NameServiceConfigView }
func (s *ServiceConfigView) WidgetKind() widget.Kind { return widget.Region }

// NewServiceConfigView construye la pestaña "Servicios" de la pantalla
// Personal: selector de profesional + crudview sobre
// appointment_booking.NewEmployeeServiceConfigView (D12), acotado a ese
// profesional.
//
// Mismo patrón self-contained que NewScheduleView (Horario) — no comparten
// selección de profesional todavía; ambas pestañas mantienen su propio
// picker. Unificarlas en un único selector compartido a nivel de la
// pantalla Personal queda anotado como pulido pendiente, no un defecto
// funcional (ver docs/PLAN_LOCAL.md Etapa 7).
//
// El formulario generado por crudview muestra un campo staff_id editable
// (EmployeeServiceConfigModel lo declara input.Text() porque el propio
// campo es genuinamente editable en otros contextos de la librería) pero
// employeeServiceConfigLister.Save sobreescribe ese valor con el
// profesional scopeado en cada guardado — lo que el usuario escriba ahí no
// tiene efecto. Confuso, no incorrecto: el valor persistido siempre es el
// correcto.
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

// rebuildPanel reconstruye el crudview completo — el Presenter de
// appointment_booking está atado a (tenantId, staffId) desde su
// construcción, así que cambiar de profesional exige un Presenter nuevo, no
// una recarga del existente. Mismo enfoque que el editor de horario.
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

// Render arma el chasis con rightpanel — título, panel y scroll son suyos.
// El selector de profesional va en HeadControls, el crudview en Article.
func (s *ServiceConfigView) Render() *Element {
	panel := &rightpanel.RightPanel{
		Title:        "Servicios",
		HeadControls: Div().Set(clsServiceHeader.AsAttr()).Child(s.picker.render("service-config-staff")),
		Article:      Div().BindChildren(s.panel),
	}
	return Div().Set(clsServiceRoot.AsAttr()).Child(panel.Render())
}
