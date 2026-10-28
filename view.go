package appointmentbooking

import (
	"webtyp.com/router"
	tinytime "webtyp.com/time"
	"webtyp.com/view"
)

const (
	titleReservations          = "Reservas"
	titleBooking               = "Reserva Hora"
	titleEmployeeServiceConfig = "Servicios"
)

// NewEmployeeServiceConfigView builds a Presenter scoped to one professional —
// same shape as NewView(caller, tenantId, staffId) above: there is no
// "list every service config in the tenant" op, on purpose, mirroring why
// NewView itself is staff-scoped.
func NewEmployeeServiceConfigView(caller router.Caller, tenantId, staffId string) view.Presenter {
	recs := view.NewRecords()
	return view.New(
		&employeeServiceConfigLister{records: recs, caller: recs.Caller(caller), tenantId: tenantId, staffId: staffId},
		&EmployeeServiceConfig{},
		view.WithTitle(titleEmployeeServiceConfig),
	)
}

// FormConfig es el alcance en el que trabaja un formulario de reserva, más lo único que
// este módulo no puede saber: cómo nombrar a un cliente.
type FormConfig struct {
	TenantId string // requerido
	StaffId  string // requerido — el profesional cuya agenda se está reservando
	// ServiceConfigId es el employee_service_config al que apunta la reserva.
	// Es lo que fija la duración y el precio; requerido para guardar o listar huecos.
	ServiceConfigId string
	// Timezone es la zona IANA en la que se lee el par día/hora, p. ej.
	// "America/Santiago". Requerido.
	//
	// LIMITACIÓN CONOCIDA: el valor autorizado vive en work_calendar_config.timezone
	// de este módulo, y no existe operación de lectura para él — por lo tanto, el
	// llamador lo proporciona. Agregar get_calendar_config está fuera del alcance
	// de este plan.
	Timezone string
	// From y To delimitan el listado, como segundos a medianoche UTC.
	From, To int64
	// ActorId se graba como creator_user_id. Opcional.
	ActorId string
	// LabelFor convierte un id de cliente en el nombre que se muestra en la lista. Opcional: nil
	// recurre al id sin formato.
	//
	// Es una función, no una dependencia de directorio, a propósito — este módulo
	// resuelve un cliente a través del DirectoryReader inyectado en el SERVIDOR, y
	// no debe adquirir una segunda dependencia en el lado del cliente sobre ninguna
	// implementación particular de directorio.
	LabelFor func(clientId string) string
}

// NewView construye el Presenter de Reservation — acotado al horario de un solo staff, ya que no
// existe una operación "listar todas las reservas de un tenant" sin acotar (ver
// docs/ARCHITECTURE.md §7). Solo lista: sin capacidad Saver/Deleter (las reservas solo mutan vía las
// transiciones FSM-guardadas de ChangeReservationStatus, y nunca se eliminan físicamente).
func NewView(caller router.Caller, tenantId, staffId string) view.Presenter {
	recs := view.NewRecords()
	return view.New(
		&reservationLister{records: recs, caller: recs.Caller(caller), tenantId: tenantId, staffId: staffId},
		&Reservation{},
		view.WithTitle(titleReservations),
	)
}

// NewFormView construye un presenter que TANTO LISTA las reservas de un profesional
// COMO CREA nuevas — la superficie que necesita una pantalla de reservas. NewView, arriba,
// sigue siendo la superficie solo de lista para un consumidor de solo lectura.
func NewFormView(caller router.Caller, cfg FormConfig) view.Presenter {
	recs := view.NewRecords()
	return view.New(
		&reservationFormStore{records: recs, caller: recs.Caller(caller), cfg: cfg},
		&ReservationForm{},
		view.WithTitle(titleBooking),
	)
}

// FreeSlots devuelve los huecos reservables de un día como cadenas "HH:MM" en
// cfg.Timezone, listos para que un widget de lista los renderice como filas vacías.
//
// day es "YYYY-MM-DD". done recibe (nil, nil) cuando el alcance está incompleto —
// sin staff o sin configuración de servicio significa que no hay nada que calcular, no un
// fallo. done siempre se invoca exactamente una vez, nunca de forma síncrona antes de que
// FreeSlots retorne cuando hay una llamada de red real en curso — igual que
// router.Caller.Call.
func FreeSlots(caller router.Caller, cfg FormConfig, day string, done func([]string, error)) {
	if cfg.StaffId == "" || cfg.ServiceConfigId == "" {
		done(nil, nil)
		return
	}
	daySec := dayToUnix(day)
	if daySec == 0 {
		done(nil, nil)
		return
	}
	out := &TimeSlotList{}
	caller.Call(
		qualifiedOp(OpListAvailability),
		&ListAvailabilityArgs{
			TenantId: cfg.TenantId,
			StaffId:  cfg.StaffId,
			ConfigId: cfg.ServiceConfigId,
			From:     int64(daySec),
			To:       int64(daySec),
		},
		out,
		func(err error) {
			if err != nil {
				done(nil, err)
				return
			}
			slots := make([]string, 0, out.Len())
			for i := 0; i < out.Len(); i++ {
				ts := out.At(i).(*TimeSlot)
				tStr := tinytime.FormatTime(ts.StartUtc)
				if len(tStr) >= 5 {
					tStr = tStr[:5]
				}
				slots = append(slots, tStr)
			}
			done(slots, nil)
		},
	)
}
