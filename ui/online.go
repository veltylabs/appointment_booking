package ui

import (
	"webtyp.com/dom"
	"webtyp.com/fmt"

	. "webtyp.com/dom"
)

// OnlineID es la pantalla de reserva online (paciente invitado); también es el
// hash al que vuelve la pasarela de pago.
const OnlineID = "appointment_booking_online"

// OnlineLabel es su texto en el menú.
const OnlineLabel = "Online booking"

// Constantes de estado de pasarela
const (
	StatusPaid      = "paid"
	StatusCancelled = "cancelled"
	StatusExpired   = "expired"
	StatusFailed    = "failed"
)

// showReturn muestra el resultado de un pago online al volver de la pasarela
// (diagrama §8, R2).
func (f *BookingFlow) showReturn(paymentID, status string) {
	summary, _ := LocalStorageGet(pendingSummaryKey)
	_ = LocalStorageDel(pendingSummaryKey)
	switch status {
	case StatusPaid:
		var lines []string
		if summary != "" {
			lines = fmt.Split(summary, lineSeparator)
		}
		f.show(f.finalPanel(textBookingConfirmed, lines, textNewBooking))
	case StatusCancelled, StatusExpired, StatusFailed:
		f.show(f.finalPanel(textPaymentIncomplete, nil, textStartAgain))
	default:
		f.show(f.finalPanel(textConfirmPaymentFail, []string{paymentID}, textStartAgain))
	}
}

// TakeReturn lee y borra de la URL el resultado que dejó la ruta de retorno de
// la pasarela (?payment=<id>&status=<estado>). ok es false si la página no
// viene de un pago.
func TakeReturn() (paymentID, status string, ok bool) {
	status, hasStatus := dom.GetQuery("status")
	if !hasStatus {
		return "", "", false
	}
	paymentID, _ = dom.GetQuery("payment")
	dom.ClearQuery()
	return paymentID, status, true
}

// takeReturn es la lectura de la vuelta de la pasarela (R1).
var takeReturn = TakeReturn
