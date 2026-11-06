package appointmentbooking

import (
	"webtyp.com/model"
	"webtyp.com/orm"
	"webtyp.com/time"
)

// ImportReservation stores a booking from a previous system with its own id, date and status: it creates it, or
// updates it when that id already exists, so importing twice leaves the same row. It checks what CreateReservation
// checks about the data — id, tenant, a known status and origin, the client, the professional and the service exist,
// the configuration exists (active or not: a historical one may be inactive) — but none of the live rules: no free
// slot, no PENDING start, no hold, no event, and no transaction of its own (the importer already runs inside one).
//
// Example: r, err := booking.ImportReservation(Reservation{Id: "1650000000000000000", TenantId: t, ClientId: p,
//
//	EmployeeServiceConfigId: cfg, ReservationTime: int64(start), Status: StatusCompleted, Origin: OriginCounter})
func (m *Module) ImportReservation(r Reservation) (Reservation, error) {
	if r.Id == "" || r.TenantId == "" || r.ClientId == "" || r.EmployeeServiceConfigId == "" || r.ReservationTime <= 0 {
		return Reservation{}, ErrMissingArgs
	}

	switch r.Status {
	case StatusConfirmed, StatusCancelled, StatusCompleted, StatusNoShow:
	default:
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

	_, _, _, _, patientExists, err := m.patients.GetPatient(r.TenantId, r.ClientId)
	if err != nil {
		return Reservation{}, err
	}
	if !patientExists {
		return Reservation{}, ErrClientNotFound
	}

	staffExists, err := m.staff.StaffExists(r.TenantId, cfg.StaffId)
	if err != nil {
		return Reservation{}, err
	}
	if !staffExists {
		return Reservation{}, ErrStaffNotFound
	}

	// La reserva es historia: basta con que el servicio exista, activo o no (ServiceExists exige activo, y el
	// catálogo deja inactivo un servicio sin precio).
	if _, _, _, _, err := m.catalog.ServiceCard(r.TenantId, cfg.ServiceId); err != nil {
		if model.KindOf(err) == model.ErrorNotFound {
			return Reservation{}, ErrServiceNotFound
		}
		return Reservation{}, err
	}

	r.StaffIdsnapshot = cfg.StaffId
	r.ServiceIdsnapshot = cfg.ServiceId
	if r.DurationMinSnapshot == 0 {
		r.DurationMinSnapshot = cfg.DurationMin
	}

	r.UpdatedAt = time.NowNano()
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

// ImportDayBlocks stores the weekly blocks of one weekday from a previous system as they were: it replaces that
// day's blocks (importing twice leaves the same rows) and checks what SaveDayBlocks checks about the data — the
// calendar config exists, each block is a valid [start, end) inside the day — but not the establishment's opening
// hours, a rule of today: a block outside them is kept, so the importer reports it and the front desk adjusts it
// later (availability offers only the opening hours anyway). No event and no recompute: nothing is booked yet.
//
// Example: err := booking.ImportDayBlocks(t, staffID, 6, []WorkCalendarBlock{{StartMin: 840, EndMin: 1080}})
func (m *Module) ImportDayBlocks(tenantId, staffId string, dayOfWeek int, blocks []WorkCalendarBlock) error {
	if dayOfWeek < 0 || dayOfWeek > 6 {
		return ErrInvalidBlock
	}
	if _, err := m.requireCalendarConfig(tenantId, staffId); err != nil {
		return err
	}
	if err := validateBlocks(blocks); err != nil {
		return err
	}
	for i := range blocks {
		blocks[i].TenantId = tenantId
		blocks[i].StaffId = staffId
		blocks[i].DayOfWeek = int64(dayOfWeek)
		blocks[i].SpecificDate = 0
		blocks[i].IsActive = true
	}
	return m.repo.ReplaceWeekdayBlocks(tenantId, staffId, dayOfWeek, blocks)
}
