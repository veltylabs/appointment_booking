package appointmentbooking

import (
	"webtyp.com/router"
	tinytime "webtyp.com/time"
	"webtyp.com/view"
)

const (
	titleReservations = "Reservas"
	titleBooking      = "Reserva"
)

// Item implementa view.Itemizer — el ÚNICO código específico de view que carga este registro. El
// Presenter indexa las filas por ID a partir de esto durante Reload; no hay lookup manual byId/WithFill.
func (r *Reservation) Item() view.Item {
	return view.Item{
		ID:          r.Id,
		Label:       r.LocalStringDate + " " + r.LocalStringTime,
		Description: r.Status,
	}
}

// Item implements view.Itemizer. LeadMain is the hour because the booking list
// is read down a column of times; a list widget that leads with a time
// (webtyp/components targethour) binds to it.
func (r *ReservationForm) Item() view.Item {
	return view.Item{
		ID:          r.Id,
		LeadMain:    r.Hour,
		Label:       r.ClientId,
		Description: r.Status,
	}
}

// FormConfig is the scope a booking form works inside, plus the one thing this
// module cannot know: how to name a client.
type FormConfig struct {
	TenantId string // required
	StaffId  string // required — the professional whose agenda is being booked
	// ServiceConfigId is the employee_service_config the reservation points at.
	// It is what fixes duration and price; required to save or to list slots.
	ServiceConfigId string
	// Timezone is the IANA zone the day/hour pair is read in, e.g.
	// "America/Santiago". Required.
	//
	// KNOWN LIMITATION: the authoritative value lives in this module's own
	// work_calendar_config.timezone, and there is no read op for it — a caller
	// therefore supplies it. Adding get_calendar_config is deliberately out of
	// this plan's scope; until it exists, an establishment whose professionals
	// span two zones cannot be served by one FormConfig.
	Timezone string
	// From and To bound the listing, as midnight-UTC seconds.
	From, To int64
	// ActorId is recorded as creator_user_id. Optional.
	ActorId string
	// LabelFor turns a client id into the name shown in the list. Optional: nil
	// falls back to the raw id.
	//
	// It is a function, not a directory dependency, on purpose — this module
	// resolves a client through the injected DirectoryReader on the SERVER, and
	// must not acquire a second, caller-side dependency on any particular
	// directory implementation.
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

// NewFormView builds a presenter that both LISTS a professional's reservations
// and CREATES new ones — the surface a booking screen needs. NewView, above,
// remains the list-only surface for a read-only consumer.
func NewFormView(caller router.Caller, cfg FormConfig) view.Presenter {
	return view.New(
		&reservationFormStore{caller: caller, cfg: cfg},
		&ReservationForm{},
		view.WithTitle(titleBooking),
	)
}

// FreeSlots returns the bookable slots of one day as "HH:MM" strings in
// cfg.Timezone, ready for a list widget to render as empty rows.
//
// day is "YYYY-MM-DD". Returns nil (no error) when the scope is incomplete —
// no staff or no service config means there is nothing to compute, not a
// failure.
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
