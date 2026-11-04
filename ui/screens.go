package ui

import (
	ab "github.com/veltylabs/appointment_booking"
	"webtyp.com/lang"
	"webtyp.com/model"
	"webtyp.com/router"
	"webtyp.com/time"
	"webtyp.com/view"
)

const (
	tabSchedule = "schedule"
	tabServices = "services"

	labelSchedule = "Horario"
	labelServices = "Servicios"

	descBooking        lang.Text = "Reserva de horas médicas y de atención para mesón y recepción."
	descOnline         lang.Text = "Flujo de reserva de horas online para pacientes."
	descPersonal       lang.Text = "Gestión de personal: horario y configuración de servicios."
	descSchedule       lang.Text = "Configuración del horario semanal y excepciones de atención."
	descServicesConfig lang.Text = "Configuración de servicios, duración y precios por profesional."
)

// Screens declara las pantallas que este módulo ofrece al menú de la app.
func Screens(caller router.Caller, zone time.Zone, ids model.IDGenerator, tenantID string) []view.Screen {
	scheduleScreen := view.Screen{
		ID:         PersonalID + "." + tabSchedule,
		Label:      lang.Text(labelSchedule),
		Icon:       PersonalID,
		Permission: model.ResourceOf(&ab.WorkCalendarConfigModel),
		Component:  NewScheduleView(caller, zone, tenantID),
		Help:       descSchedule,
	}

	servicesScreen := view.Screen{
		ID:         PersonalID + "." + tabServices,
		Label:      lang.Text(labelServices),
		Icon:       PersonalID,
		Permission: model.ResourceOf(&ab.EmployeeServiceConfigModel),
		Component:  NewServiceConfigView(caller, tenantID),
		Help:       descServicesConfig,
	}

	personalGroup := view.Screen{
		ID:       PersonalID,
		Label:    lang.Text(PersonalLabel),
		Icon:     PersonalID,
		Help:     descPersonal,
		Children: []view.Screen{scheduleScreen, servicesScreen},
	}

	bookingScreen := view.Screen{
		ID:         ID,
		Label:      lang.Text(NavLabel),
		Icon:       ID,
		Permission: model.ResourceOf(&ab.ReservationModel),
		Component:  NewBookingView(caller, zone, ids, tenantID),
		Help:       descBooking,
	}

	onlineScreen := view.Screen{
		ID:         OnlineID,
		Label:      lang.Text(OnlineLabel),
		Icon:       ID,
		Permission: ab.ResourceReservationOnline,
		Component:  newBookingFlow(caller, zone, ids, tenantID, true),
		Help:       descOnline,
	}

	return []view.Screen{bookingScreen, personalGroup, onlineScreen}
}
