package ui

import (
	"webtyp.com/components/calendarslider"
	. "webtyp.com/dom"
	"webtyp.com/fmt"
	. "webtyp.com/html"
	"webtyp.com/router"
	"webtyp.com/time"
	"webtyp.com/unixid"

	ab "github.com/veltylabs/appointment_booking"
	itemcatalog "github.com/veltylabs/item_catalog"
	patientdirectory "github.com/veltylabs/patient_directory"
	staffmanager "github.com/veltylabs/staff_manager"
)

// PaymentHandler es la interfaz inyectable para el Paso 6 (Pasarela de Pago).
type PaymentHandler interface {
	RequiresPayment(cfg ab.EmployeeServiceConfig) bool
	RenderPayment(amount float64, currency string, onDone func(paymentID string, err error)) *Element
}

// DefaultPaymentSimulator es una implementación por defecto de pasarela para demostración y testing.
type DefaultPaymentSimulator struct{}

func (s DefaultPaymentSimulator) RequiresPayment(cfg ab.EmployeeServiceConfig) bool {
	return cfg.PaymentRequired
}

func (s DefaultPaymentSimulator) RenderPayment(amount float64, currency string, onDone func(string, error)) *Element {
	curr := currency
	if curr == "" {
		curr = "CLP"
	}
	return Div().Class("payment-step-box").
		Child(H3().Text("Pago Requerido")).
		Child(P().Text(fmt.Sprintf("Monto total a cancelar: $%.0f %s", amount, curr))).
		Child(P().Class("payment-note").Text("Simulador de pasarela de pago (Transbank / WebPay / Tarjeta)")).
		Child(Button().Attr("type", "button").Class("btn-primary").Text("Pagar Ahora (Simular Pago Exitoso)").
			OnClick(func(Event) {
				onDone("pay_sim_"+fmt.Sprint(time.Now()/1000000), nil)
			}))
}

// BookingMode define el canal de atención
type BookingMode string

const (
	ModeOnline  BookingMode = "online"
	ModeCounter BookingMode = "counter"
)

// BookingAccordion es el componente central de reserva en 6 pasos progresivos.
type BookingAccordion struct {
	caller   router.Caller
	tenantID string
	mode     BookingMode
	payment  PaymentHandler

	// Paso activo: "1", "2", "3", "4", "5", "6", "done"
	step *SignalString

	// Paso 1: Especialidad & Servicio
	specialties     []string
	selectedArea    *SignalString
	catalogItems    []itemcatalog.CatalogItem
	selectedConfig  *SignalString
	selectedCfgData ab.EmployeeServiceConfig

	// Paso 2: Profesional
	staffMembers  []staffmanager.StaffMember
	selectedStaff *SignalString
	staffName     *SignalString

	// Paso 3: Fecha & Hora
	selectedDay  *SignalString
	selectedHour *SignalString
	freeSlots    *SignalNodes
	calSlider    *calendarslider.CalendarSlider

	// Paso 4: Notas
	notes *SignalString

	// Paso 5: Paciente
	patientPicker *patientPicker
	patientID     *SignalString
	patientName   *SignalString
	patientRUT    *SignalString
	patientPhone  *SignalString
	patientEmail  *SignalString

	// Paso 6: Pago
	paymentID *SignalString

	// Resultado final
	confirmedReservation *SignalNodes

	// Contenedor reactivo
	rootNode *SignalNodes
}

func NewBookingAccordion(caller router.Caller, tenantID string, mode BookingMode, payment PaymentHandler) *BookingAccordion {
	if payment == nil {
		payment = DefaultPaymentSimulator{}
	}

	b := &BookingAccordion{
		caller:               caller,
		tenantID:             tenantID,
		mode:                 mode,
		payment:              payment,
		step:                 NewString("1"),
		selectedArea:         NewString(""),
		selectedConfig:       NewString(""),
		selectedStaff:        NewString(""),
		staffName:            NewString(""),
		selectedDay:          NewString(""),
		selectedHour:         NewString(""),
		freeSlots:            NewNodes(),
		notes:                NewString(""),
		patientID:            NewString(""),
		patientName:          NewString(""),
		patientRUT:           NewString(""),
		patientPhone:         NewString(""),
		patientEmail:         NewString(""),
		paymentID:            NewString(""),
		confirmedReservation: NewNodes(),
		rootNode:             NewNodes(),
	}

	b.patientPicker = newPatientPicker(caller, tenantID, func(id, name string) {
		b.patientID.Set(id)
		b.patientName.Set(name)
	})

	b.calSlider = &calendarslider.CalendarSlider{NumMonths: 3}
	b.calSlider.OnFilterChange(func(day string) {
		b.selectedDay.Set(day)
		b.loadSlotsForDay(day)
	})

	b.loadCatalogAndStaff()
	return b
}

func (b *BookingAccordion) loadCatalogAndStaff() {
	var staffList staffmanager.StaffMemberList
	var itemsList itemcatalog.CatalogItemList
	pending := 2

	done := func() {
		pending--
		if pending != 0 {
			return
		}

		b.staffMembers = make([]staffmanager.StaffMember, 0, len(staffList))
		for _, s := range staffList {
			b.staffMembers = append(b.staffMembers, *s)
		}

		b.catalogItems = make([]itemcatalog.CatalogItem, 0, len(itemsList))
		for _, it := range itemsList {
			b.catalogItems = append(b.catalogItems, *it)
		}

		// Extraer especialidades únicas
		areasMap := make(map[string]bool)
		for _, sm := range b.staffMembers {
			if sm.Specialty != "" {
				areasMap[sm.Specialty] = true
			}
		}
		b.specialties = make([]string, 0, len(areasMap))
		for a := range areasMap {
			b.specialties = append(b.specialties, a)
		}

		b.renderAll()
	}

	b.caller.Call(staffmanager.ModelName+"."+staffmanager.OpListStaff,
		&staffmanager.ListStaffArgs{}, &staffList,
		func(error) { done() })
	b.caller.Call(itemcatalog.ModelName+"."+itemcatalog.OpListItems,
		&itemcatalog.ListItemsArgs{TenantId: b.tenantID, ActiveOnly: true}, &itemsList,
		func(error) { done() })
}

func (b *BookingAccordion) loadSlotsForDay(day string) {
	if day == "" || b.selectedStaff.Get() == "" || b.selectedConfig.Get() == "" {
		b.freeSlots.Set(nil)
		return
	}

	now := time.Now() / 1000000000
	cfg := ab.FormConfig{
		TenantId:        b.tenantID,
		StaffId:         b.selectedStaff.Get(),
		ServiceConfigId: b.selectedConfig.Get(),
		Timezone:        timezoneClinic,
		From:            now - bookingWindowPastDays*86400,
		To:              now + bookingWindowFutureDays*86400,
	}

	b.freeSlots.Set([]*Element{Div().Class("loading-slots").Text("Consultando horarios disponibles...")})

	ab.FreeSlots(b.caller, cfg, day, func(slots []string, err error) {
		if err != nil || len(slots) == 0 {
			b.freeSlots.Set([]*Element{
				Div().Class("no-slots-msg").Text("No hay horas disponibles para este día. Por favor elija otra fecha."),
			})
			return
		}

		chipsRow := Div().Class("slots-chips-row")
		for _, s := range slots {
			slotTime := s
			chip := Button().Attr("type", "button").Class("slot-chip").Text(slotTime).
				OnClick(func(Event) {
					b.selectedHour.Set(slotTime)
					b.step.Set("4") // Avanzar al Resumen
					b.renderAll()
				})
			chipsRow.Child(chip)
		}

		container := Div().Class("slots-grid").
			Child(H4().Text("Seleccione un horario disponible:")).
			Child(chipsRow)
		b.freeSlots.Set([]*Element{container})
	})
}

func (b *BookingAccordion) confirmBooking() {
	staffID := b.selectedStaff.Get()
	cfgID := b.selectedConfig.Get()
	dayStr := b.selectedDay.Get()
	hourStr := b.selectedHour.Get()
	pID := b.patientID.Get()

	if pID == "" && b.mode == ModeOnline {
		// Paciente guest: crear paciente en patient_directory antes de confirmar
		rut := b.patientRUT.Get()
		name := b.patientName.Get()
		if rut == "" || name == "" {
			return
		}

		gen, _ := unixid.NewUnixID()
		pID = gen.NewID()
		b.patientID.Set(pID)

		newPatient := patientdirectory.Patient{
			Id:       pID,
			TenantId: b.tenantID,
			Rut:      rut,
			Name:     name,
			Phone:    b.patientPhone.Get(),
			Email:    b.patientEmail.Get(),
			IsActive: true,
		}

		b.caller.Call(patientdirectory.ModelName+"."+patientdirectory.OpUpsertPatient,
			&newPatient, nil, func(err error) {
				if err != nil {
					return
				}
				b.proceedReservationCreation(staffID, cfgID, dayStr, hourStr, pID)
			})
		return
	}

	b.proceedReservationCreation(staffID, cfgID, dayStr, hourStr, pID)
}

func (b *BookingAccordion) proceedReservationCreation(staffID, cfgID, dayStr, hourStr, pID string) {
	daySec := time.DateKeyToUnix(dayStr)
	var hourMin int64
	if len(hourStr) >= 5 {
		h, _ := fmt.Convert(hourStr[:2]).Int64()
		m, _ := fmt.Convert(hourStr[3:5]).Int64()
		hourMin = h*60 + m
	}
	slotStartUtc := daySec + hourMin*60

	gen, _ := unixid.NewUnixID()
	resID := gen.NewID()

	origin := ab.OriginCounter
	if b.mode == ModeOnline {
		origin = ab.OriginOnline
	}

	createArgs := ab.CreateReservationArgs{
		Id:                      resID,
		TenantId:                b.tenantID,
		ClientId:                pID,
		CreatorUserId:           "user-booking",
		EmployeeServiceConfigId: cfgID,
		SlotStartUtc:            slotStartUtc,
		Origin:                  origin,
		Notes:                   b.notes.Get(),
	}

	b.caller.Call(ab.ModelName+"."+ab.OpCreateReservation,
		&createArgs, nil,
		func(err error) {
			if err != nil {
				return
			}

			// Si el paso de pago generó un paymentID o requiere confirmación con pago
			payID := b.paymentID.Get()
			if payID != "" {
				b.caller.Call(ab.ModelName+"."+ab.OpChangeReservationStatus,
					&ab.ChangeReservationStatusArgs{
						TenantId:  b.tenantID,
						Id:        resID,
						Event:     ab.EventConfirm,
						PaymentId: payID,
						Revision:  0,
					}, nil, func(error) {
						b.showSuccessVoucher(resID, dayStr, hourStr)
					})
				return
			}

			// Si es Counter o no requería pago
			b.showSuccessVoucher(resID, dayStr, hourStr)
		})
}

func (b *BookingAccordion) showSuccessVoucher(resID, dayStr, hourStr string) {
	b.step.Set("done")

	voucher := Div().Class("booking-success-voucher").
		Child(H2().Class("success-title").Text("✓ ¡Reserva Confirmada Exitosamente!")).
		Child(Div().Class("voucher-card").
			Child(P().Text("Código de Cita: " + resID)).
			Child(P().Text("Profesional: " + b.staffName.Get())).
			Child(P().Text("Fecha: " + dayStr + " a las " + hourStr + " hrs")).
			Child(P().Text("Paciente: " + b.patientName.Get()))).
		Child(Button().Attr("type", "button").Class("btn-primary").Text("Nueva Reserva").
			OnClick(func(Event) {
				b.resetFlow()
			}))

	b.confirmedReservation.Set([]*Element{voucher})
	b.renderAll()
}

func (b *BookingAccordion) resetFlow() {
	b.step.Set("1")
	b.selectedConfig.Set("")
	b.selectedStaff.Set("")
	b.staffName.Set("")
	b.selectedDay.Set("")
	b.selectedHour.Set("")
	b.patientID.Set("")
	b.patientName.Set("")
	b.patientRUT.Set("")
	b.paymentID.Set("")
	b.confirmedReservation.Set(nil)
	b.renderAll()
}

func (b *BookingAccordion) renderAll() {
	current := b.step.Get()
	if current == "done" {
		b.rootNode.Set([]*Element{Div().BindChildren(b.confirmedReservation)})
		return
	}

	accordion := Div().Class("booking-accordion-container").
		Child(b.renderStep1(current)).
		Child(b.renderStep2(current)).
		Child(b.renderStep3(current)).
		Child(b.renderStep4(current)).
		Child(b.renderStep5(current)).
		Child(b.renderStep6(current))

	b.rootNode.Set([]*Element{accordion})
}

func (b *BookingAccordion) renderStep1(current string) *Element {
	isOpen := current == "1"
	isDone := current > "1"

	header := Div().Class("accordion-header").
		Child(Span().Class("step-badge").Text("1")).
		Child(H3().Text("Especialidad y Servicio"))

	if isDone {
		header.Child(Span().Class("step-summary").Text(b.selectedArea.Get())).
			Child(Button().Attr("type", "button").Class("btn-edit-step").Text("Editar").
				OnClick(func(Event) { b.step.Set("1"); b.renderAll() }))
		return Div().Class("accordion-step step-completed").Child(header)
	}

	if !isOpen {
		return Div().Class("accordion-step step-disabled").Child(header)
	}

	body := Div().Class("accordion-body")

	// Selector de Especialidad
	areaSelect := NewElement("select").Attr("name", "accordion-area").
		Child(Option("", "Seleccione Especialidad...")).
		OnChange(func(e Event) {
			b.selectedArea.Set(e.TargetValue())
			b.renderAll()
		})
	for _, a := range b.specialties {
		opt := Option(a, a)
		if a == b.selectedArea.Get() {
			opt = SelectedOption(a, a)
		}
		areaSelect.Child(opt)
	}

	body.Child(Div().Class("step-field").
		Child(Label().Text("Especialidad / Área")).
		Child(areaSelect))

	// Tarjetas de servicios
	if b.selectedArea.Get() != "" {
		servicesList := Div().Class("services-grid")
		for _, it := range b.catalogItems {
			itCopy := it
			card := Div().Class("service-card").
				Child(H4().Text(itCopy.Name)).
				Child(P().Text(fmt.Sprintf("$%.0f CLP", itCopy.Price))).
				OnClick(func(Event) {
					b.selectedConfig.Set(itCopy.Id)
					b.selectedCfgData = ab.EmployeeServiceConfig{
						Id:              itCopy.Id,
						ServiceId:       itCopy.Id,
						DurationMin:     30,
						PriceOverride:   itCopy.Price,
						PaymentRequired: false,
					}
					b.step.Set("2")
					b.renderAll()
				})
			servicesList.Child(card)
		}
		body.Child(H4().Text("Seleccione el Servicio:")).Child(servicesList)
	}

	return Div().Class("accordion-step step-active").Child(header).Child(body)
}

func (b *BookingAccordion) renderStep2(current string) *Element {
	isOpen := current == "2"
	isDone := current > "2"

	header := Div().Class("accordion-header").
		Child(Span().Class("step-badge").Text("2")).
		Child(H3().Text("Profesional Médico"))

	if isDone {
		header.Child(Span().Class("step-summary").Text(b.staffName.Get())).
			Child(Button().Attr("type", "button").Class("btn-edit-step").Text("Editar").
				OnClick(func(Event) { b.step.Set("2"); b.renderAll() }))
		return Div().Class("accordion-step step-completed").Child(header)
	}

	if !isOpen {
		return Div().Class("accordion-step step-disabled").Child(header)
	}

	body := Div().Class("accordion-body")
	staffGrid := Div().Class("staff-grid")

	for _, sm := range b.staffMembers {
		if b.selectedArea.Get() != "" && sm.Specialty != b.selectedArea.Get() {
			continue
		}
		smCopy := sm
		card := Div().Class("staff-card").
			Child(H4().Text(smCopy.Name)).
			Child(P().Class("staff-specialty").Text(smCopy.Specialty)).
			OnClick(func(Event) {
				b.selectedStaff.Set(smCopy.Id)
				b.staffName.Set(smCopy.Name)
				b.step.Set("3")
				b.renderAll()
			})
		staffGrid.Child(card)
	}

	body.Child(H4().Text("Elija el profesional para su atención:")).Child(staffGrid)
	return Div().Class("accordion-step step-active").Child(header).Child(body)
}

func (b *BookingAccordion) renderStep3(current string) *Element {
	isOpen := current == "3"
	isDone := current > "3"

	header := Div().Class("accordion-header").
		Child(Span().Class("step-badge").Text("3")).
		Child(H3().Text("Fecha y Horario"))

	if isDone {
		header.Child(Span().Class("step-summary").Text(b.selectedDay.Get() + " " + b.selectedHour.Get() + " hrs")).
			Child(Button().Attr("type", "button").Class("btn-edit-step").Text("Editar").
				OnClick(func(Event) { b.step.Set("3"); b.renderAll() }))
		return Div().Class("accordion-step step-completed").Child(header)
	}

	if !isOpen {
		return Div().Class("accordion-step step-disabled").Child(header)
	}

	body := Div().Class("accordion-body").
		Child(Div().Class("calendar-section").Child(b.calSlider)).
		Child(Div().BindChildren(b.freeSlots))

	return Div().Class("accordion-step step-active").Child(header).Child(body)
}

func (b *BookingAccordion) renderStep4(current string) *Element {
	isOpen := current == "4"
	isDone := current > "4"

	header := Div().Class("accordion-header").
		Child(Span().Class("step-badge").Text("4")).
		Child(H3().Text("Resumen de Cita y Motivo"))

	if isDone {
		header.Child(Span().Class("step-summary").Text("Resumen Revisado")).
			Child(Button().Attr("type", "button").Class("btn-edit-step").Text("Editar").
				OnClick(func(Event) { b.step.Set("4"); b.renderAll() }))
		return Div().Class("accordion-step step-completed").Child(header)
	}

	if !isOpen {
		return Div().Class("accordion-step step-disabled").Child(header)
	}

	body := Div().Class("accordion-body").
		Child(Div().Class("summary-card").
			Child(P().Text("Profesional: " + b.staffName.Get())).
			Child(P().Text("Fecha: " + b.selectedDay.Get() + " a las " + b.selectedHour.Get() + " hrs"))).
		Child(Div().Class("step-field").
			Child(Label().Text("Notas o Motivo de Consulta (Opcional)")).
			Child(NewElement("textarea").Attr("name", "booking-notes").
				Bind(b.notes).
				OnChange(func(e Event) { b.notes.Set(e.TargetValue()) }))).
		Child(Button().Attr("type", "button").Class("btn-primary").Text("Continuar a Identificación").
			OnClick(func(Event) {
				b.step.Set("5")
				b.renderAll()
			}))

	return Div().Class("accordion-step step-active").Child(header).Child(body)
}

func (b *BookingAccordion) renderStep5(current string) *Element {
	isOpen := current == "5"
	isDone := current > "5"

	header := Div().Class("accordion-header").
		Child(Span().Class("step-badge").Text("5")).
		Child(H3().Text("Identificación del Paciente"))

	if isDone {
		header.Child(Span().Class("step-summary").Text(b.patientName.Get())).
			Child(Button().Attr("type", "button").Class("btn-edit-step").Text("Editar").
				OnClick(func(Event) { b.step.Set("5"); b.renderAll() }))
		return Div().Class("accordion-step step-completed").Child(header)
	}

	if !isOpen {
		return Div().Class("accordion-step step-disabled").Child(header)
	}

	body := Div().Class("accordion-body")

	if b.mode == ModeCounter {
		body.Child(b.patientPicker.Render())
		body.Child(Button().Attr("type", "button").Class("btn-primary").Text("Continuar").
			OnClick(func(Event) {
				if b.patientID.Get() != "" {
					b.step.Set("6")
					b.renderAll()
				}
			}))
	} else {
		body.Child(Div().Class("guest-patient-form").
			Child(Div().Class("step-field").
				Child(Label().Text("RUT *")).
				Child(NewElement("input").Attr("type", "text").Attr("name", "guest-rut").
					Attr("placeholder", "Ej: 12.345.678-5").
					Bind(b.patientRUT).OnChange(func(e Event) { b.patientRUT.Set(e.TargetValue()) }))).
			Child(Div().Class("step-field").
				Child(Label().Text("Nombre Completo *")).
				Child(NewElement("input").Attr("type", "text").Attr("name", "guest-name").
					Bind(b.patientName).OnChange(func(e Event) { b.patientName.Set(e.TargetValue()) }))).
			Child(Div().Class("step-field").
				Child(Label().Text("Teléfono")).
				Child(NewElement("input").Attr("type", "tel").Attr("name", "guest-phone").
					Bind(b.patientPhone).OnChange(func(e Event) { b.patientPhone.Set(e.TargetValue()) }))).
			Child(Div().Class("step-field").
				Child(Label().Text("Correo Electrónico")).
				Child(NewElement("input").Attr("type", "email").Attr("name", "guest-email").
					Bind(b.patientEmail).OnChange(func(e Event) { b.patientEmail.Set(e.TargetValue()) }))).
			Child(Button().Attr("type", "button").Class("btn-primary").Text("Continuar").
				OnClick(func(Event) {
					if b.patientRUT.Get() != "" && b.patientName.Get() != "" {
						b.step.Set("6")
						b.renderAll()
					}
				})))
	}

	return Div().Class("accordion-step step-active").Child(header).Child(body)
}

func (b *BookingAccordion) renderStep6(current string) *Element {
	isOpen := current == "6"

	header := Div().Class("accordion-header").
		Child(Span().Class("step-badge").Text("6")).
		Child(H3().Text("Pago y Confirmación"))

	if !isOpen {
		return Div().Class("accordion-step step-disabled").Child(header)
	}

	body := Div().Class("accordion-body")

	if b.payment.RequiresPayment(b.selectedCfgData) {
		payEl := b.payment.RenderPayment(b.selectedCfgData.PriceOverride, "CLP", func(paymentID string, err error) {
			if err != nil {
				return
			}
			b.paymentID.Set(paymentID)
			b.confirmBooking()
		})
		body.Child(payEl)
	} else {
		body.Child(P().Text("Esta atención no requiere pago anticipado. Se confirmará directamente.")).
			Child(Button().Attr("type", "button").Class("btn-primary").Text("Confirmar Reserva").
				OnClick(func(Event) {
					b.confirmBooking()
				}))
	}

	return Div().Class("accordion-step step-active").Child(header).Child(body)
}

func (b *BookingAccordion) Render() *Element {
	return Div().Class("booking-accordion-wrapper").BindChildren(b.rootNode)
}
