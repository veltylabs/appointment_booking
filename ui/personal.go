package ui

import (
	"webtyp.com/layout/platformd"
	"webtyp.com/model"
	"webtyp.com/router"
	"webtyp.com/svg"

	staffui "github.com/veltylabs/staff_manager/ui"
)

const (
	tabStaff    = "staff"
	tabSchedule = "schedule"
	tabServices = "services"

	labelStaff    = "Datos y dispositivos"
	labelSchedule = "Horario"
	labelServices = "Servicios"
)

// PersonalBrowser construye la pantalla compuesta "Personal" en un grupo de tres secciones:
// Datos y dispositivos (staff_manager), Horario y Servicios (ambos de appointment_booking).
func PersonalBrowser(caller router.Caller, ids model.IDGenerator, tenantID string) (platformd.UIModule, error) {
	staffPanel, err := staffui.StaffPanel(caller, ids, PersonalID+".staff")
	if err != nil {
		return nil, err
	}

	staffMod := platformd.NewUIModule(PersonalID+"."+tabStaff, labelStaff, svg.Icon(PersonalID), staffPanel)
	scheduleMod := platformd.NewUIModule(PersonalID+"."+tabSchedule, labelSchedule, svg.Icon(PersonalID), NewScheduleView(caller, tenantID))
	servicesMod := platformd.NewUIModule(PersonalID+"."+tabServices, labelServices, svg.Icon(PersonalID), NewServiceConfigView(caller, tenantID))

	return platformd.NewGroupUIModule(PersonalID, PersonalLabel, svg.Icon(PersonalID), []platformd.UIModule{
		staffMod, scheduleMod, servicesMod,
	}), nil
}
