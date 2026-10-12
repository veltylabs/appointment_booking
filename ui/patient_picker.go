package ui

import (
	"webtyp.com/components/modaldialog"
	. "webtyp.com/dom"
	. "webtyp.com/html"
	"webtyp.com/router"
	"webtyp.com/unixid"

	patientdirectory "github.com/veltylabs/patient_directory"
)

type patientPicker struct {
	caller          router.Caller
	tenantID        string
	rutInput        *SignalString
	selectedID      *SignalString
	selectedName    *SignalString
	infoCard        *SignalNodes
	modal           *modaldialog.ModalDialog
	onSelect        func(id string, name string)

	// Campos para el modal de nuevo paciente
	newRut   *SignalString
	newName  *SignalString
	newPhone *SignalString
	newEmail *SignalString
}

func newPatientPicker(caller router.Caller, tenantID string, onSelect func(id, name string)) *patientPicker {
	p := &patientPicker{
		caller:       caller,
		tenantID:     tenantID,
		rutInput:     NewString(""),
		selectedID:   NewString(""),
		selectedName: NewString(""),
		infoCard:     NewNodes(),
		onSelect:     onSelect,
		newRut:       NewString(""),
		newName:      NewString(""),
		newPhone:     NewString(""),
		newEmail:     NewString(""),
	}
	p.modal = &modaldialog.ModalDialog{
		Title:   "Registrar Nuevo Paciente",
		Content: p.renderModalContent(),
	}
	p.modal.Init(noopCtx{})
	return p
}

func (p *patientPicker) renderModalContent() *Element {
	return Div().Class("patient-modal-form").
		Child(Div().Class("patient-form-field").
			Child(Label().Text("RUT *")).
			Child(NewElement("input").Attr("type", "text").Attr("name", "new-patient-rut").
				Bind(p.newRut).OnChange(func(e Event) { p.newRut.Set(e.TargetValue()) }))).
		Child(Div().Class("patient-form-field").
			Child(Label().Text("Nombre Completo *")).
			Child(NewElement("input").Attr("type", "text").Attr("name", "new-patient-name").
				Bind(p.newName).OnChange(func(e Event) { p.newName.Set(e.TargetValue()) }))).
		Child(Div().Class("patient-form-field").
			Child(Label().Text("Teléfono")).
			Child(NewElement("input").Attr("type", "tel").Attr("name", "new-patient-phone").
				Bind(p.newPhone).OnChange(func(e Event) { p.newPhone.Set(e.TargetValue()) }))).
		Child(Div().Class("patient-form-field").
			Child(Label().Text("Correo Electrónico")).
			Child(NewElement("input").Attr("type", "email").Attr("name", "new-patient-email").
				Bind(p.newEmail).OnChange(func(e Event) { p.newEmail.Set(e.TargetValue()) }))).
		Child(Div().Class("patient-modal-actions").
			Child(Button().Attr("type", "button").Text("Cancelar").
				OnClick(func(Event) { p.modal.Close() })).
			Child(Button().Attr("type", "button").Class("btn-primary").Text("Guardar y Seleccionar").
				OnClick(func(Event) { p.saveNewPatient() })))
}

func (p *patientPicker) searchRUT(rut string) {
	if rut == "" {
		p.infoCard.Set(nil)
		p.selectedID.Set("")
		p.selectedName.Set("")
		return
	}

	var patient patientdirectory.Patient
	p.caller.Call(patientdirectory.ModelName+"."+patientdirectory.OpFindPatientByRut,
		&patientdirectory.FindPatientByRutArgs{TenantId: p.tenantID, Rut: rut},
		&patient,
		func(err error) {
			if err != nil || patient.Id == "" {
				// Paciente no encontrado: ofrecer botón para registrar
				card := Div().Class("patient-not-found").
					Child(Span().Text("Paciente no encontrado para el RUT: "+rut+" ")).
					Child(Button().Attr("type", "button").Class("btn-secondary").Text("+ Registrar Paciente").
						OnClick(func(Event) {
							p.newRut.Set(rut)
							p.modal.Open()
						}))
				p.infoCard.Set([]*Element{card})
				p.selectedID.Set("")
				p.selectedName.Set("")
				return
			}

			// Paciente encontrado
			p.selectedID.Set(patient.Id)
			p.selectedName.Set(patient.Name)
			if p.onSelect != nil {
				p.onSelect(patient.Id, patient.Name)
			}

			card := Div().Class("patient-card-found").
				Child(Div().Class("patient-card-title").Text("✓ "+patient.Name)).
				Child(Div().Class("patient-card-details").
					Child(Span().Text("RUT: "+patient.Rut)).
					Child(Span().Text(" | Tel: "+patient.Phone)).
					Child(Span().Text(" | Email: "+patient.Email)))
			p.infoCard.Set([]*Element{card})
		})
}

func (p *patientPicker) saveNewPatient() {
	rut := p.newRut.Get()
	name := p.newName.Get()
	if rut == "" || name == "" {
		return
	}

	gen, _ := unixid.NewUnixID()
	newID := gen.NewID()

	newPatient := patientdirectory.Patient{
		Id:       newID,
		TenantId: p.tenantID,
		Rut:      rut,
		Name:     name,
		Phone:    p.newPhone.Get(),
		Email:    p.newEmail.Get(),
		IsActive: true,
	}

	p.caller.Call(patientdirectory.ModelName+"."+patientdirectory.OpUpsertPatient,
		&newPatient, nil,
		func(err error) {
			if err != nil {
				return
			}
			p.modal.Close()
			p.rutInput.Set(rut)
			p.selectedID.Set(newID)
			p.selectedName.Set(name)
			if p.onSelect != nil {
				p.onSelect(newID, name)
			}

			card := Div().Class("patient-card-found").
				Child(Div().Class("patient-card-title").Text("✓ "+name+" (Nuevo)")).
				Child(Div().Class("patient-card-details").
					Child(Span().Text("RUT: "+rut)).
					Child(Span().Text(" | Tel: "+newPatient.Phone)))
			p.infoCard.Set([]*Element{card})
		})
}

func (p *patientPicker) Render() *Element {
	return Div().Class("patient-picker").
		Child(Div().Class("patient-search-row").
			Child(Label().Text("Buscar Paciente por RUT")).
			Child(NewElement("input").Attr("type", "text").
				Attr("placeholder", "Ej: 12.345.678-5").
				Attr("name", "patient-rut-search").
				Bind(p.rutInput).
				OnChange(func(e Event) {
					val := e.TargetValue()
					p.rutInput.Set(val)
					p.searchRUT(val)
				})).
			Child(Button().Attr("type", "button").Text("Buscar").
				OnClick(func(Event) { p.searchRUT(p.rutInput.Get()) }))).
		Child(Div().BindChildren(p.infoCard)).
		Child(p.modal)
}

type noopCtx struct{}

func (noopCtx) OnCleanup(func()) {}
