package ui

import (
	staffmanager "github.com/veltylabs/staff_manager"
	"webtyp.com/lang"
	"webtyp.com/model"
	"webtyp.com/router"
	"webtyp.com/view"
)

const (
	tabStaff    = "staff"
	tabSchedule = "schedule"
	tabServices = "services"

	labelStaff    = "Datos y dispositivos"
	labelSchedule = "Horario"
	labelServices = "Servicios"

	descBooking        lang.Text = "Reserva de horas médicas y de atención para mesón y recepción."
	descOnline         lang.Text = "Flujo de reserva de horas online para pacientes."
	descPersonal       lang.Text = "Gestión de personal: datos, horario y configuración de servicios."
	descStaff          lang.Text = "Datos personales, de contacto y credenciales del profesional."
	descSchedule       lang.Text = "Configuración del horario semanal y excepciones de atención."
	descServicesConfig lang.Text = "Configuración de servicios, duración y precios por profesional."
)

// Screens declara las pantallas que este módulo ofrece al menú de la app.
func Screens(caller router.Caller, ids model.IDGenerator, tenantID string) []view.Screen {
	staffScreen := view.Screen{
		ID:         PersonalID + "." + tabStaff,
		Label:      lang.Text(labelStaff),
		Icon:       PersonalID,
		Permission: "staff_manager",
		Presenter:  staffmanager.NewView(caller, view.WithDescription(descStaff)),
	}

	scheduleScreen := view.Screen{
		ID:         PersonalID + "." + tabSchedule,
		Label:      lang.Text(labelSchedule),
		Icon:       PersonalID,
		Permission: "calendar",
		Component:  NewScheduleView(caller, tenantID),
		Help:       descSchedule,
	}

	servicesScreen := view.Screen{
		ID:         PersonalID + "." + tabServices,
		Label:      lang.Text(labelServices),
		Icon:       PersonalID,
		Permission: "employee_service_config",
		Component:  NewServiceConfigView(caller, tenantID),
		Help:       descServicesConfig,
	}

	personalGroup := view.Screen{
		ID:       PersonalID,
		Label:    lang.Text(PersonalLabel),
		Icon:     PersonalID,
		Help:     descPersonal,
		Children: []view.Screen{staffScreen, scheduleScreen, servicesScreen},
	}

	bookingScreen := view.Screen{
		ID:         ID,
		Label:      lang.Text(NavLabel),
		Icon:       ID,
		Permission: "reservation",
		Component:  NewBookingView(caller, ids, tenantID),
		Help:       descBooking,
	}

	onlineScreen := view.Screen{
		ID:         OnlineID,
		Label:      lang.Text(OnlineLabel),
		Icon:       ID,
		Permission: "reservation_online",
		Component:  newBookingFlow(caller, ids, tenantID, true),
		Help:       descOnline,
	}

	return []view.Screen{bookingScreen, personalGroup, onlineScreen}
}

