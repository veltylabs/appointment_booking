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
type reservationLister struct {
	caller   router.Caller
	tenantId string
	staffId  string
}

func (l reservationLister) List() ([]model.Model, error) {
	out := &ReservationList{}
	ch := make(chan error, 1)
	l.caller.Call(
		OpListReservationsByStaff,
		&ListReservationsByStaffArgs{TenantId: l.tenantId, StaffId: l.staffId},
		out,
		func(err error) { ch <- err },
	)
	if err := <-ch; err != nil {
		return nil, err
	}
	rows := make([]model.Model, 0, out.Len())
	for i := 0; i < out.Len(); i++ {
		rows = append(rows, out.At(i).(*Reservation))
	}
	return rows, nil
}

var _ view.Lister = reservationLister{}

type reservationFormStore struct {
	caller router.Caller
	cfg    FormConfig
}

func (s *reservationFormStore) List() ([]model.Model, error) {
	if s.cfg.StaffId == "" {
		return nil, nil
	}
	out := &ReservationList{}
	ch := make(chan error, 1)
	s.caller.Call(
		OpListReservationsByStaff,
		&ListReservationsByStaffArgs{
			TenantId: s.cfg.TenantId,
			StaffId:  s.cfg.StaffId,
			From:     s.cfg.From,
			To:       s.cfg.To,
		},
		out,
		func(err error) { ch <- err },
	)
	if err := <-ch; err != nil {
		return nil, err
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
	return rows, nil
}

func (s *reservationFormStore) Save(recs ...model.Model) error {
	if len(recs) == 0 {
		return fmt.Err("appointment_booking: save: empty records")
	}
	if s.cfg.ServiceConfigId == "" {
		return ErrNoServiceConfig
	}
	for _, rec := range recs {
		r, ok := rec.(*ReservationForm)
		if !ok {
			return fmt.Err("appointment_booking: save: expected *ReservationForm")
		}
		d := dayToUnix(r.Day)
		m := minutesOfDay(r.Hour)
		if d == 0 || m < 0 {
			return ErrIncompleteSlot
		}
		slot := LocalIntToUnixUTC(d, m, s.cfg.Timezone)
		if slot == 0 {
			return ErrIncompleteSlot
		}
		ch := make(chan error, 1)
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
			func(err error) { ch <- err },
		)
		if err := <-ch; err != nil {
			return err
		}
	}
	return nil
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

func (l employeeServiceConfigLister) List() ([]model.Model, error) {
	out := &EmployeeServiceConfigList{}
	ch := make(chan error, 1)
	l.caller.Call(
		OpListEmployeeServiceConfigsByStaff,
		&ListEmployeeServiceConfigsByStaffArgs{TenantId: l.tenantId, StaffId: l.staffId},
		out,
		func(err error) { ch <- err },
	)
	if err := <-ch; err != nil {
		return nil, err
	}
	rows := make([]model.Model, 0, out.Len())
	for i := 0; i < out.Len(); i++ {
		rows = append(rows, out.At(i).(*EmployeeServiceConfig))
	}
	return rows, nil
}

func (l employeeServiceConfigLister) Save(recs ...model.Model) error {
	for _, rec := range recs {
		cfg, ok := rec.(*EmployeeServiceConfig)
		if !ok {
			return fmt.Err("appointment_booking: save: expected *EmployeeServiceConfig")
		}
		cfg.TenantId = l.tenantId
		cfg.StaffId = l.staffId
		op := OpCreateEmployeeServiceConfig
		if cfg.Id != "" {
			op = OpUpdateEmployeeServiceConfig
		}
		ch := make(chan error, 1)
		l.caller.Call(op, cfg, nil, func(err error) { ch <- err })
		if err := <-ch; err != nil {
			return err
		}
	}
	return nil
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
