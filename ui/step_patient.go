package ui

import (
	"webtyp.com/components/contentcard"
	"webtyp.com/components/modaldialog"
	"webtyp.com/lang"
	"webtyp.com/model"

	. "webtyp.com/dom"
	. "webtyp.com/html"

	patientdirectory "github.com/veltylabs/patient_directory"
)

// stepPatientForm es el paso 4. Mesón (diagrama §5): RUT → ficha o registro en
// un modal, y Continuar la elige. Online (§6): el paciente escribe sus datos y
// se vincula su ficha. La reserva se crea en el resumen (paso 5).
type stepPatientForm struct {
	Element
	f      *BookingFlow
	errMsg *SignalString
	busy   *SignalBool

	rut, name, phone, email *SignalString

	result   *SignalNodes // mesón: la ficha encontrada o el botón de registro
	modal    *modaldialog.ModalDialog
	modalErr *SignalString
}

func newStepPatientForm(f *BookingFlow) *stepPatientForm {
	s := &stepPatientForm{
		f: f, errMsg: NewString(""), busy: NewBool(false),
		rut: NewString(""), name: NewString(""), phone: NewString(""), email: NewString(""),
		result: NewNodes(), modalErr: NewString(""),
	}
	s.modal = &modaldialog.ModalDialog{Title: textRegisterPatient, Content: Div().Child(
		errorLine(s.modalErr),
		textField(textRut, "register-rut", "text", s.rut),
		textField(textFullName, "register-name", "text", s.name),
		textField(textPhone, "register-phone", "tel", s.phone),
		textField(textEmail, "register-email", "email", s.email),
		Div().Set(clsFlowActions.AsAttr()).Child(actionButton(textRegisterPatient, s.busy, s.register)),
	)}
	s.modal.Init(nil)
	return s
}

func (s *stepPatientForm) Render() *Element {
	if s.f.online {
		return stepBox().Child(
			errorLine(s.errMsg),
			textField(textRut, "rut", "text", s.rut),
			textField(textFullName, "name", "text", s.name),
			textField(textEmail, "email", "email", s.email),
			textField(textPhone, "phone", "tel", s.phone),
			Div().Set(clsFlowActions.AsAttr()).Child(actionButton(textContinue, s.busy, s.continueOnline)),
		)
	}
	return stepBox().Child(
		errorLine(s.errMsg),
		Label().Set(clsFlowField.AsAttr()).Text(lang.Translate(textRut).String()).Child(
			Div().Set(clsFlowActions.AsAttr()).Child(
				Input("text").Set(clsFlowInput.AsAttr()).Attr("name", "rut").Bind(s.rut),
				actionButton(textSearch, s.busy, s.search),
			),
		),
		stepBox().BindChildren(s.result),
		s.modal,
	)
}

func (s *stepPatientForm) show() {
	s.errMsg.Set("")
	s.result.Set(nil)
}

// search busca la ficha por RUT (M1).
func (s *stepPatientForm) search() {
	s.errMsg.Set("")
	s.result.Set(nil)
	s.busy.Set(true)
	p := &patientdirectory.Patient{}
	s.f.caller.Call(patientdirectory.ModelName+"."+patientdirectory.OpFindPatientByRut,
		&patientdirectory.FindPatientByRutArgs{TenantId: s.f.tenantID, Rut: s.rut.Get()}, p,
		func(err error) {
			s.busy.Set(false)
			switch {
			case err == nil: // M1-encontrado
				s.showCard(*p)
			case err.Error() == patientdirectory.ErrNotFound.Error(): // M1-404
				s.result.Set([]*Element{actionButton(textRegisterPatient, nil, s.openRegister)})
			default: // M1-otro
				s.errMsg.Set(err.Error())
			}
		})
}

func (s *stepPatientForm) openRegister() {
	s.modalErr.Set("")
	s.modal.Open()
}

// register crea la ficha (M2): si falla, el modal sigue abierto con el error.
func (s *stepPatientForm) register() {
	s.modalErr.Set("")
	s.busy.Set(true)
	in := &patientdirectory.Patient{
		Id: s.f.ids.NewID(), TenantId: s.f.tenantID,
		Rut: s.rut.Get(), Name: s.name.Get(), Phone: s.phone.Get(), Email: s.email.Get(), IsActive: true,
	}
	out := &patientdirectory.Patient{}
	s.f.caller.Call(patientdirectory.ModelName+"."+patientdirectory.OpUpsertPatient, in, out, func(err error) {
		s.busy.Set(false)
		if err != nil { // M2-sí
			s.modalErr.Set(err.Error())
			return
		}
		s.modal.Close()
		s.showCard(*out) // M2-no
	})
}

// showCard muestra la ficha encontrada o registrada y el botón que la elige.
func (s *stepPatientForm) showCard(p patientdirectory.Patient) {
	card := &contentcard.ContentCard{Header: Strong().Text(p.Name), Body: P().Text(p.Phone)}
	choose := func() { s.choose(p.Id, p.Name) }
	s.result.Set([]*Element{card.Render(), Div().Set(clsFlowActions.AsAttr()).Child(actionButton(textContinue, nil, choose))})
}

// choose fija el paciente y avanza al resumen.
func (s *stepPatientForm) choose(id, name string) {
	s.f.choice.patientID, s.f.choice.patientName = id, name
	s.f.next(name)
}

// continueOnline valida el formulario y vincula la ficha del paciente (N1, N2).
func (s *stepPatientForm) continueOnline() {
	s.errMsg.Set("")
	link := &patientdirectory.LinkPatientArgs{
		TenantId: s.f.tenantID, Id: s.f.ids.NewID(),
		Rut: s.rut.Get(), Name: s.name.Get(), Phone: s.phone.Get(), Email: s.email.Get(),
	}
	if err := link.Validate(model.ActionCreate); err != nil { // N1-no
		s.errMsg.Set(err.Error())
		return
	}
	s.busy.Set(true)
	linked := &patientdirectory.LinkPatientResult{}
	s.f.caller.Call(patientdirectory.ModelName+"."+patientdirectory.OpLinkPatient, link, linked, func(err error) {
		s.busy.Set(false)
		if err != nil { // N2-sí
			s.errMsg.Set(err.Error())
			return
		}
		s.choose(linked.Id, link.Name)
	})
}
