package personal

import (
	"webtyp.com/components/decktabs"
	"webtyp.com/layout/crudview"
	"webtyp.com/layout/platformd"
	"webtyp.com/model"
	"webtyp.com/router"
	"webtyp.com/svg"

	appointmentbooking "github.com/veltylabs/mjosefa-cms/modules/appointment_booking"
	staffmanager "github.com/veltylabs/staff_manager"
)

// Browser composes the screen in three tabs: Datos y dispositivos
// (staff_manager), Horario and Servicios (both over appointment_booking,
// unlocked by D12 — see docs/PLAN_LOCAL.md). Horario and Servicios each keep
// their own professional picker for now; unifying them into one shared
// picker at this screen's level remains a polish item, not a functional
// defect.
//
// Imports BOTH libraries (staff_manager, appointment_booking) directly,
// never their sibling wrappers (modules/staff_manager, modules/
// appointment_booking): appointment_booking's own wrapper already imports
// modules/staff_manager to build its four readers (see its server.go), so if
// this package imported that wrapper AND something upstream imported this
// package from staff_manager, it would close a cycle. Living outside both
// wrappers and imported by neither, personal can depend on both — but only
// by consuming the libraries, not the wrappers, so it never drags
// appointment_booking's composition (the four readers) in here.
func Browser(caller router.Caller, ids model.IDGenerator, tenantID string) (platformd.UIModule, error) {
	staffView, err := crudview.New(crudview.Config{
		ParentID:  ID + ".staff",
		Presenter: staffmanager.NewView(caller),
		IDs:       ids,
	})
	if err != nil {
		return nil, err
	}

	tabs := &decktabs.DeckTabs{
		Label: Label,
		Items: []decktabs.Item{
			{ID: "staff", Label: "Datos y dispositivos", Panel: staffView},
			{ID: "schedule", Label: "Horario", Panel: appointmentbooking.NewScheduleView(caller, tenantID)},
			{ID: "services", Label: "Servicios", Panel: appointmentbooking.NewServiceConfigView(caller, tenantID)},
		},
	}
	return platformd.NewUIModule(ID, Label, svg.Icon(ID), tabs), nil
}
