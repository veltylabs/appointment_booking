package ui

import (
	"webtyp.com/components/stepaccordion"
	"webtyp.com/router"
	. "webtyp.com/dom"
	. "webtyp.com/html"
	ab "github.com/veltylabs/appointment_booking"
)

type bookingFlow struct {
	caller          router.Caller
	tenantID        string
	isOnline        bool

	specialty       string
	service         *ab.BookableService

	staffId         string // "" = Any

	day             int64
	slot            *ab.BookableSlot
	overbook        bool

	notes           string

	patientId       string

	reservationId   string
	paymentRequired bool
	price           float64
	currency        string

	accordion       *stepaccordion.StepAccordion
}

func newBookingFlow(caller router.Caller, tenantID string, isOnline bool) Component {
	f := &bookingFlow{
		caller:   caller,
		tenantID: tenantID,
		isOnline: isOnline,
	}

	f.accordion = &stepaccordion.StepAccordion{
		Steps: []stepaccordion.Step{
			{Key: "service", Title: textSpecialtyAndService, Body: Div()},
			{Key: "staff", Title: textProfessional, Body: Div()},
			{Key: "slot", Title: textDateAndTime, Body: Div()},
			{Key: "summary", Title: textSummary, Body: Div()},
			{Key: "patient", Title: textPatient, Body: Div()},
			{Key: "payment", Title: textPaymentConfirmation, Body: Div()},
		},
		OnReopen: func(key string) {
			switch key {
			case "service":
				f.specialty = ""
				f.service = nil
				fallthrough
			case "staff":
				f.staffId = ""
				fallthrough
			case "slot":
				f.day = 0
				f.slot = nil
				f.overbook = false
				fallthrough
			case "summary":
				f.notes = ""
				fallthrough
			case "patient":
				f.patientId = ""
			}
		},
	}

	return f.accordion
}
