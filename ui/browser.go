package ui

import (
	"webtyp.com/layout/platformd"
	"webtyp.com/model"
	"webtyp.com/router"
	"webtyp.com/svg"
)

// Browser construye la pantalla Reserva Hora (mesón).
func Browser(caller router.Caller, ids model.IDGenerator, tenantID string) (platformd.UIModule, error) {
	return platformd.NewUIModule(ID, NavLabel, svg.Icon(ID), NewBookingView(caller, ids, tenantID)), nil
}
