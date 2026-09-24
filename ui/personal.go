package ui

import (
	"webtyp.com/components/decktabs"
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

// PersonalBrowser construye la pantalla compuesta "Personal" en tres pestañas:
// Datos y dispositivos (staff_manager), Horario y Servicios (ambos de appointment_booking).
func PersonalBrowser(caller router.Caller, ids model.IDGenerator, tenantID string) (platformd.UIModule, error) {
	staffPanel, err := staffui.StaffPanel(caller, ids, PersonalID+".staff")
	if err != nil {
		return nil, err
	}

	tabs := &decktabs.DeckTabs{
		Label: PersonalLabel,
		Items: []decktabs.Item{
			{ID: tabStaff, Label: labelStaff, Panel: staffPanel},
			{ID: tabSchedule, Label: labelSchedule, Panel: NewScheduleView(caller, tenantID)},
			{ID: tabServices, Label: labelServices, Panel: NewServiceConfigView(caller, tenantID)},
		},
	}
	return platformd.NewUIModule(PersonalID, PersonalLabel, svg.Icon(PersonalID), tabs), nil
}
