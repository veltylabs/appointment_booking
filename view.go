package appointmentbooking

import (
	"webtyp.com/router"
	tinytime "webtyp.com/time"
	"webtyp.com/view"
)

const (
	titleReservations          = "Reservas"
	titleBooking               = "Reserva"
	titleEmployeeServiceConfig = "Services"
)

// Item implements view.Itemizer.
func (c *EmployeeServiceConfig) Item() view.Item {
	return view.Item{ID: c.Id, Label: c.ServiceId, Description: c.StaffId}
}

// NewEmployeeServiceConfigView builds a Presenter scoped to one professional —
// same shape as NewView(caller, tenantId, staffId) above: there is no
// "list every service config in the tenant" op, on purpose, mirroring why
// NewView itself is staff-scoped.
func NewEmployeeServiceConfigView(caller router.Caller, tenantId, staffId string) view.Presenter {
	return view.New(
		employeeServiceConfigLister{caller: caller, tenantId: tenantId, staffId: staffId},
		&EmployeeServiceConfig{},
		view.WithTitle(titleEmployeeServiceConfig),
	)
}

// Item implementa view.Itemizer — el ÚNICO código específico de view que carga este registro. El
// Presenter indexa las filas por ID a partir de esto durante Reload; no hay lookup manual byId/WithFill.
func (r *Reservation) Item() view.Item {
	return view.Item{
		ID:          r.Id,
		Label:       r.LocalStringDate + " " + r.LocalStringTime,
		Description: r.Status,
	}
}

// Item implementa view.Itemizer. LeadMain es la hora porque la lista de reservas
// se lee hacia abajo en una columna de horas; un widget de lista que encabeza con una hora
// (webtyp/components targethour) se vincula a este campo.
func (r *ReservationForm) Item() view.Item {
	return view.Item{
		ID:          r.Id,
		LeadMain:    r.Hour,
		Label:       r.ClientId,
		Description: r.Status,
	}
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
	return view.New(
		reservationLister{caller: caller, tenantId: tenantId, staffId: staffId},
		&Reservation{},
		view.WithTitle(titleReservations),
	)
}

// NewFormView construye un presenter que TANTO LISTA las reservas de un profesional
// COMO CREA nuevas — la superficie que necesita una pantalla de reservas. NewView, arriba,
// sigue siendo la superficie solo de lista para un consumidor de solo lectura.
func NewFormView(caller router.Caller, cfg FormConfig) view.Presenter {
	return view.New(
		&reservationFormStore{caller: caller, cfg: cfg},
		&ReservationForm{},
		view.WithTitle(titleBooking),
	)
}

// FreeSlots devuelve los huecos reservables de un día como cadenas "HH:MM" en
// cfg.Timezone, listos para que un widget de lista los renderice como filas vacías.
//
// day es "YYYY-MM-DD". Devuelve nil (sin error) cuando el alcance está incompleto —
// sin staff o sin configuración de servicio significa que no hay nada que calcular, no un
// fallo.
func FreeSlots(caller router.Caller, cfg FormConfig, day string) ([]string, error) {
	if cfg.StaffId == "" || cfg.ServiceConfigId == "" {
		return nil, nil
	}
	daySec := dayToUnix(day)
	if daySec == 0 {
		return nil, nil
	}
	out := &TimeSlotList{}
	ch := make(chan error, 1)
	caller.Call(
		OpListAvailability,
		&ListAvailabilityArgs{
			TenantId: cfg.TenantId,
			StaffId:  cfg.StaffId,
			ConfigId: cfg.ServiceConfigId,
			From:     daySec,
			To:       daySec,
		},
		out,
		func(err error) { ch <- err },
	)
	if err := <-ch; err != nil {
		return nil, err
	}
	slots := make([]string, 0, out.Len())
	for i := 0; i < out.Len(); i++ {
		ts := out.At(i).(*TimeSlot)
		tStr := tinytime.FormatTime(ts.StartUtc * 1000000000)
		if len(tStr) >= 5 {
			tStr = tStr[:5]
		}
		slots = append(slots, tStr)
	}
	return slots, nil
}
