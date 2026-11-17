package ui

import (
	ab "github.com/veltylabs/appointment_booking"
	"webtyp.com/lang"
	"webtyp.com/model"
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

// Screens declara las pantallas que este módulo ofrece al menú de la app. El tenant lo pone el servidor en cada
// operación: las pantallas no lo envían. Los pasos «paciente» y «cobro» de la reserva de mesón son de otros módulos:
// la app los arma y los pasa en steps. El paciente es obligatorio; sin Payment (una app sin cobros), una reserva cuyo servicio
// exige pago avisa que los pagos no están configurados.
func Screens(d view.BrowserDeps, steps Steps) []view.Screen {
	if steps.Patient == nil {
		panic("appointment_booking/ui: Screens: Steps.Patient is required — the app builds it from patient_directory")
	}
	caller, zone, ids := d.Caller, d.Zone, d.IDs
	scheduleScreen := view.Screen{
		ID:         PersonalID + "." + tabSchedule,
		Label:      lang.Text(labelSchedule),
		Icon:       PersonalID,
		Permission: model.ResourceOf(&ab.WorkCalendarConfigModel),
		Component:  NewScheduleView(caller, zone, ids),
		Help:       descSchedule,
	}

	servicesScreen := view.Screen{
		ID:         PersonalID + "." + tabServices,
		Label:      lang.Text(labelServices),
		Icon:       PersonalID,
		Permission: model.ResourceOf(&ab.EmployeeServiceConfigModel),
		Component:  NewServiceConfigView(caller),
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
		Component:  NewBookingView(caller, zone, ids, steps),
		Help:       descBooking,
	}

	onlineScreen := view.Screen{
		ID:         OnlineID,
		Label:      lang.Text(OnlineLabel),
		Icon:       ID,
		Permission: model.ResourceOf(&ab.OnlineReservationModel),
		Component:  newBookingFlow(caller, zone, ids, Steps{}, true),
		Help:       descOnline,
	}

	return []view.Screen{bookingScreen, personalGroup, onlineScreen}
}
