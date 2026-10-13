package ui

import (
	"webtyp.com/model"
	"webtyp.com/router"
)

// NewBookingView construye la pantalla "Reserva Hora" de mesón: la reserva en
// 6 pasos que hace el personal (docs/diagrams/booking_flow.md).
func NewBookingView(caller router.Caller, ids model.IDGenerator, tenantID string) *BookingFlow {
	return newBookingFlow(caller, ids, tenantID, false)
}
