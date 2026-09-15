package appointmentbooking

import (
	"webtyp.com/fmt"
	"webtyp.com/model"
	"webtyp.com/router"
	tinytime "webtyp.com/time"
	"webtyp.com/view"
)

// reservationLister adapts router.Caller + the staff-scoped list op to
// view.Lister. view.NewCallerLister is unusable here — it sends nil args,
// and this list is scoped to (tenantId, staffId).
//
// The result arrives asynchronously through done, which is always non-nil:
// List never blocks waiting for the transport (see view.Lister's contract).
type reservationLister struct {
	caller   router.Caller
	tenantId string
	staffId  string
}

func (l reservationLister) List(done func([]model.Model, error)) {
	out := &ReservationList{}
	l.caller.Call(
		OpListReservationsByStaff,
		&ListReservationsByStaffArgs{TenantId: l.tenantId, StaffId: l.staffId},
		out,
		func(err error) {
			if err != nil {
				done(nil, err)
				return
			}
			rows := make([]model.Model, 0, out.Len())
			for i := 0; i < out.Len(); i++ {
				rows = append(rows, out.At(i).(*Reservation))
			}
			done(rows, nil)
		},
	)
}

var _ view.Lister = reservationLister{}

type reservationFormStore struct {
	caller router.Caller
	cfg    FormConfig
}

func (s *reservationFormStore) List(done func([]model.Model, error)) {
	if s.cfg.StaffId == "" {
		done(nil, nil)
		return
	}
	out := &ReservationList{}
	s.caller.Call(
		OpListReservationsByStaff,
		&ListReservationsByStaffArgs{
			TenantId: s.cfg.TenantId,
			StaffId:  s.cfg.StaffId,
			From:     s.cfg.From,
			To:       s.cfg.To,
		},
		out,
		func(err error) {
			if err != nil {
				done(nil, err)
				return
			}
			rows := make([]model.Model, 0, out.Len())
			for i := 0; i < out.Len(); i++ {
				r := out.At(i).(*Reservation)
				clientId := r.ClientId
				if s.cfg.LabelFor != nil {
					if lbl := s.cfg.LabelFor(r.ClientId); lbl != "" {
						clientId = lbl
					}
				}
				rows = append(rows, &ReservationForm{
					Id:       r.Id,
					ClientId: clientId,
					Day:      r.LocalStringDate,
					Hour:     r.LocalStringTime,
					Notes:    r.Notes,
					Status:   r.Status,
				})
			}
			done(rows, nil)
		},
	)
}

// Save crea cada reserva en orden, una llamada por registro, encadenada
// mediante el callback del transporte — nunca bloquea (ver view.Saver). La
// validación por registro corre justo antes de su llamada, igual que el
// comportamiento original secuencial: un fallo en el registro i aborta el
// resto y viaja por done.
func (s *reservationFormStore) Save(recs []model.Model, done func(error)) {
	if done == nil {
		done = func(error) {}
	}
	if len(recs) == 0 {
		done(fmt.Err("appointment_booking: save: empty records"))
		return
	}
	if s.cfg.ServiceConfigId == "" {
		done(ErrNoServiceConfig)
		return
	}
	var create func(i int)
	create = func(i int) {
		if i == len(recs) {
			done(nil)
			return
		}
		r, ok := recs[i].(*ReservationForm)
		if !ok {
			done(fmt.Err("appointment_booking: save: expected *ReservationForm"))
			return
		}
		d := dayToUnix(r.Day)
		m := minutesOfDay(r.Hour)
		if d == 0 || m < 0 {
			done(ErrIncompleteSlot)
			return
		}
		slot := LocalIntToUnixUTC(d, m, s.cfg.Timezone)
		if slot == 0 {
			done(ErrIncompleteSlot)
			return
		}
		s.caller.Call(
			OpCreateReservation,
			&CreateReservationArgs{
				TenantId:                s.cfg.TenantId,
				ClientId:                r.ClientId,
				CreatorUserId:           s.cfg.ActorId,
				EmployeeServiceConfigId: s.cfg.ServiceConfigId,
				SlotStartUtc:            slot,
				Notes:                   r.Notes,
			},
			&Reservation{},
			func(err error) {
				if err != nil {
					done(err)
					return
				}
				create(i + 1)
			},
		)
	}
	create(0)
}

var (
	_ view.Lister = (*reservationFormStore)(nil)
	_ view.Saver  = (*reservationFormStore)(nil)
)

type employeeServiceConfigLister struct {
	caller   router.Caller
	tenantId string
	staffId  string
}

func (l employeeServiceConfigLister) List(done func([]model.Model, error)) {
	out := &EmployeeServiceConfigList{}
	l.caller.Call(
		OpListEmployeeServiceConfigsByStaff,
		&ListEmployeeServiceConfigsByStaffArgs{TenantId: l.tenantId, StaffId: l.staffId},
		out,
		func(err error) {
			if err != nil {
				done(nil, err)
				return
			}
			rows := make([]model.Model, 0, out.Len())
			for i := 0; i < out.Len(); i++ {
				rows = append(rows, out.At(i).(*EmployeeServiceConfig))
			}
			done(rows, nil)
		},
	)
}

// Save crea o actualiza cada configuración en orden, encadenada mediante el
// callback del transporte — nunca bloquea (ver view.Saver).
func (l employeeServiceConfigLister) Save(recs []model.Model, done func(error)) {
	if done == nil {
		done = func(error) {}
	}
	var saveNext func(i int)
	saveNext = func(i int) {
		if i == len(recs) {
			done(nil)
			return
		}
		cfg, ok := recs[i].(*EmployeeServiceConfig)
		if !ok {
			done(fmt.Err("appointment_booking: save: expected *EmployeeServiceConfig"))
			return
		}
		cfg.TenantId = l.tenantId
		cfg.StaffId = l.staffId
		op := OpCreateEmployeeServiceConfig
		if cfg.Id != "" {
			op = OpUpdateEmployeeServiceConfig
		}
		l.caller.Call(op, cfg, nil, func(err error) {
			if err != nil {
				done(err)
				return
			}
			saveNext(i + 1)
		})
	}
	saveNext(0)
}

var (
	_ view.Lister = employeeServiceConfigLister{}
	_ view.Saver  = employeeServiceConfigLister{}
)

// dayToUnix convierte "YYYY-MM-DD" a segundos de medianoche UTC — la codificación
// que usan work_calendar_block.specific_date y los argumentos de op From/To. 0 en caso de fallo.
func dayToUnix(day string) int64 {
	nano, err := tinytime.ParseDate(day)
	if err != nil {
		return 0
	}
	return nano / 1000000000
}

// minutesOfDay convierte "HH:MM" a minutos desde la medianoche. -1 en caso de fallo.
func minutesOfDay(hour string) int {
	min, err := tinytime.ParseTime(hour)
	if err != nil {
		return -1
	}
	return int(min)
}