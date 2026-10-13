package ui

import (
	"webtyp.com/layout/platformd"
	"webtyp.com/router"
	"webtyp.com/model"
	"webtyp.com/svg"
)

const OnlineID = "appointment_booking_online"
const OnlineLabel = "Online booking"

func OnlineBrowser(caller router.Caller, ids model.IDGenerator, tenantID string) (platformd.UIModule, error) {
	return platformd.NewUIModule(OnlineID, OnlineLabel, svg.Icon(OnlineID), newBookingFlow(caller, tenantID, true)), nil
}
