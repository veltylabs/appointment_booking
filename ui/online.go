package ui

import (
	"webtyp.com/fmt"

	. "webtyp.com/dom"

	paymentmanager "github.com/veltylabs/payment_manager"
	paymentmanagerui "github.com/veltylabs/payment_manager/ui"
)

// OnlineID es la pantalla de reserva online (paciente invitado); también es el
// hash al que vuelve la pasarela de pago.
const OnlineID = "appointment_booking_online"

// OnlineLabel es su texto en el menú.
const OnlineLabel = "Online booking"

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
