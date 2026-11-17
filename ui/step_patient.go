package ui

import (
	"webtyp.com/lang"
	"webtyp.com/view"

	. "webtyp.com/dom"
	. "webtyp.com/html"
)

// guest son los datos que escribe quien reserva online: la ficha del paciente la resuelve el servidor al crear la
// reserva online.
type guest struct{ document, name, phone, email string }

// stepPatientForm es el paso 4. Mesón: el PatientStep que pasó la app (buscar o registrar al paciente). Online: el
// paciente escribe sus datos, que viajan con la reserva online. La reserva se crea en el resumen (paso 5).
type stepPatientForm struct {
	Element
	f      *BookingFlow
	errMsg *SignalString

	documentNumber, name, phone, email *SignalString
}

func newStepPatientForm(f *BookingFlow) *stepPatientForm {
	return &stepPatientForm{
		f: f, errMsg: NewString(""),
		documentNumber: NewString(""), name: NewString(""), phone: NewString(""), email: NewString(""),
	}
}

func (s *stepPatientForm) Render() *Element {
	if s.f.online {
		return stepBox().Child(
			errorLine(s.errMsg),
			textField(textRut, "doc", "text", s.documentNumber),
			textField(textFullName, "name", "text", s.name),
			textField(textEmail, "email", "email", s.email),
			textField(textPhone, "phone", "tel", s.phone),
			Div().Set(clsFlowActions.AsAttr()).Child(actionButton(textContinue, nil, s.continueOnline)),
		)
	}
	if s.f.steps.Patient != nil {
		box := stepBox().Child(s.f.steps.Patient.Render(s.choose))
		if s.f.steps.OpenForm != nil {
			formHost := Div()
			box.Child(
				Div().Set(clsFlowActions.AsAttr()).Child(
					actionButton("+ Nuevo cliente", nil, func() {
						comp := s.f.steps.OpenForm("client_directory", func(res view.RecordResult) {
							s.choose(res.ID, res.Label)
						}, func() {
							formHost.Text("")
						})
						if comp != nil {
							formHost.Child(comp)
						}
					}),
				),
				formHost,
			)
		}
		return box
	}
	return stepBox().Child(s.f.steps.OpenForm("client_directory", func(res view.RecordResult) {
		s.choose(res.ID, res.Label)
	}, nil))
}

func (s *stepPatientForm) show() {
	s.errMsg.Set("")
	if !s.f.online && s.f.steps.Patient != nil {
		s.f.steps.Patient.Reset()
	}
}

// choose fija el paciente y avanza al resumen.
func (s *stepPatientForm) choose(id, name string) {
	s.f.choice.patientID, s.f.choice.patientName = id, name
	s.f.next(name)
}

// continueOnline valida el formulario y avanza: la ficha se resuelve al crear la reserva online (N1, N2).
func (s *stepPatientForm) continueOnline() {
	s.errMsg.Set("")
	g := guest{document: s.documentNumber.Get(), name: s.name.Get(), phone: s.phone.Get(), email: s.email.Get()}
	if g.document == "" || g.name == "" {
		s.errMsg.Set(lang.Translate(textDocumentAndNameRequired).String())
		return
	}
	s.f.choice.guest = g
	s.f.choice.patientName = g.name
	s.f.next(g.name)
}
