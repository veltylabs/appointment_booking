package ui

import (
	"webtyp.com/fmt"
	"webtyp.com/layout/platformd"
	"webtyp.com/model"
	"webtyp.com/router"
	"webtyp.com/svg"

	. "webtyp.com/dom"

	paymentmanager "github.com/veltylabs/payment_manager"
	paymentmanagerui "github.com/veltylabs/payment_manager/ui"
)

// OnlineID es la pantalla de reserva online (paciente invitado); también es el
// hash al que vuelve la pasarela de pago.
const OnlineID = "appointment_booking_online"

// OnlineLabel es su texto en el menú.
const OnlineLabel = "Online booking"

// OnlineBrowser construye la reserva online: el mismo flujo de 6 pasos, con el
// paso 5 para el paciente y el pago en la pasarela.
func OnlineBrowser(caller router.Caller, ids model.IDGenerator, tenantID string) (platformd.UIModule, error) {
	return platformd.NewUIModule(OnlineID, OnlineLabel, svg.Icon(ID), newBookingFlow(caller, ids, tenantID, true)), nil
}

// showReturn muestra el resultado de un pago online al volver de la pasarela
// (diagrama §8, R2).
func (f *BookingFlow) showReturn(paymentID, status string) {
	summary, _ := LocalStorageGet(pendingSummaryKey)
	_ = LocalStorageDel(pendingSummaryKey)
	switch status {
	case paymentmanager.StatusPaid:
		var lines []string
		if summary != "" {
			lines = fmt.Split(summary, lineSeparator)
		}
		f.show(f.finalPanel(textBookingConfirmed, lines, textNewBooking))
	case paymentmanager.StatusCancelled, paymentmanager.StatusExpired, paymentmanager.StatusFailed:
		f.show(f.finalPanel(textPaymentIncomplete, nil, textStartAgain))
	default:
		f.show(f.finalPanel(textConfirmPaymentFail, []string{paymentID}, textStartAgain))
	}
}

// takeReturn es la lectura de la vuelta de la pasarela (R1).
var takeReturn = paymentmanagerui.TakeReturn
