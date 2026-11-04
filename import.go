package appointmentbooking

import (
	"webtyp.com/fmt"
	"webtyp.com/model"
	"webtyp.com/orm"
	tinytime "webtyp.com/time"
)

// ImportReservation stores a booking from a previous system with its own id, date and status: it creates it, or
// updates it when that id already exists, so importing twice leaves the same row. It checks what CreateReservation
// checks about the data — id, tenant, a known status and origin, the client, the professional and the service exist,
// the configuration exists (active or not: a historical one may be inactive) — but none of the live rules: no free
// slot, no PENDING start, no hold, no event, and no transaction of its own (the importer already runs inside one).
//
// Example: r, err := booking.ImportReservation(Reservation{Id: "1650000000000000000", TenantId: t, ClientId: p,
//	EmployeeServiceConfigId: cfg, ReservationTime: int64(start), Status: StatusCompleted, Origin: OriginCounter})
func (m *Module) ImportReservation(r Reservation) (Reservation, error) {
	if r.Id == "" || r.TenantId == "" || r.ClientId == "" || r.EmployeeServiceConfigId == "" || r.ReservationTime <= 0 {
		return Reservation{}, ErrMissingArgs
	}

	if r.Status != StatusConfirmed && r.Status != StatusCancelled && r.Status != StatusCompleted && r.Status != StatusNoShow {
		return Reservation{}, ErrImportStatus
	}

	if r.Origin != OriginCounter && r.Origin != OriginOnline {
		return Reservation{}, ErrMissingArgs
	}

	cfg, err := m.repo.GetEmployeeServiceConfig(r.EmployeeServiceConfigId)
	if err != nil {
		return Reservation{}, err
	}
	if cfg.TenantId != r.TenantId {
		return Reservation{}, ErrNotFound
	}

	clientExists, err := m.directory.ClientExists(r.TenantId, r.ClientId)
	if err != nil {
		return Reservation{}, err
	}
	if !clientExists {
		return Reservation{}, ErrClientNotFound
	}

	staffExists, err := m.staff.StaffExists(r.TenantId, cfg.StaffId)
	if err != nil {
		return Reservation{}, err
	}
	if !staffExists {
		return Reservation{}, fmt.Err("staff", "not", "found")
	}

	serviceExists, err := m.catalog.ServiceExists(r.TenantId, cfg.ServiceId)
	if err != nil {
		return Reservation{}, err
	}
	if !serviceExists {
		return Reservation{}, fmt.Err("service", "not", "found")
	}

	r.StaffIdsnapshot = cfg.StaffId
	r.ServiceIdsnapshot = cfg.ServiceId
	if r.DurationMinSnapshot == 0 {
		r.DurationMinSnapshot = cfg.DurationMin
	}

	r.ReservationDate = tinytime.MidnightUTC(tinytime.Unix(r.ReservationTime))

	// La fecha y hora locales que trae el sistema anterior son la verdad (ver docs/TIEMPO.md): se conservan.
	// Solo si vienen vacías se calculan, con el desfase fijo de webtyp/time (puede errar en 1 hora por el
	// horario de verano, por eso no se usan cuando el dato original existe).
	if r.LocalStringDate == "" {
		r.LocalStringDate = tinytime.FormatDate(tinytime.Unix(r.ReservationTime))
	}
	if r.LocalStringTime == "" {
		r.LocalStringTime = tinytime.FormatTime(tinytime.Unix(r.ReservationTime))
	}

	r.UpdatedAt = tinytime.NowNano()
	r.UpdatedBy = r.CreatorUserId
	r.Revision = 1

	existing, err := m.repo.GetReservation(r.Id)
	if err == nil {
		if existing.TenantId != r.TenantId {
			return Reservation{}, ErrIdTaken
		}
		if err := m.db.Update(&r, orm.Eq(Reservation_.Id, r.Id), orm.Eq(Reservation_.TenantId, r.TenantId)); err != nil {
			return Reservation{}, err
		}
		return r, nil
	} else if model.KindOf(err) != model.ErrorNotFound {
		return Reservation{}, err
	}

	if err := m.db.Create(&r); err != nil {
		return Reservation{}, err
	}

	return r, nil
}
