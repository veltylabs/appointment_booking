package ui

import (
	"webtyp.com/router"
	"webtyp.com/dom"
)

func NewBookingView(caller router.Caller, tenantID string) dom.Component {
	return newBookingFlow(caller, tenantID, false)
}
