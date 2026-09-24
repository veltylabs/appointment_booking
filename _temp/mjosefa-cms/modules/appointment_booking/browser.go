package appointment_booking

import (
	"webtyp.com/layout/platformd"
	"webtyp.com/model"
	"webtyp.com/router"
	"webtyp.com/svg"
)

// Browser builds the Reserva Hora screen. tenantID IS used (unlike most
// modules' Browser) — NewBookingView scopes its calendar/staff pickers to
// this installation. ids is unused: NewBookingView never mints an id itself.
//
// Never fails: NewBookingView returns a bare Component, no error — there is
// nothing this function can propagate.
func Browser(caller router.Caller, ids model.IDGenerator, tenantID string) (platformd.UIModule, error) {
	return platformd.NewUIModule(ID, NavLabel, svg.Icon(ID), NewBookingView(caller, tenantID)), nil
}
