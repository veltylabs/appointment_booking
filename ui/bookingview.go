package ui

import (
	"webtyp.com/model"
	"webtyp.com/router"
	"webtyp.com/time"
)

// NewBookingView construye la pantalla "Reserva Hora" de mesón: la reserva en
// 6 pasos que hace el personal (docs/diagrams/booking_flow.md).
func NewBookingView(caller router.Caller, zone time.Zone, ids model.IDGenerator, tenantID string) *BookingFlow {
	return newBookingFlow(caller, zone, ids, tenantID, false)
}
