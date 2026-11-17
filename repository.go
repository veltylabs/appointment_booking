package appointmentbooking

import (
	"webtyp.com/crud"
	"webtyp.com/fmt"
	"webtyp.com/model"
	"webtyp.com/storage"
	"webtyp.com/time"
)

// Errores sentinela a nivel de paquete
var ErrPaymentsUnavailable = fmt.Err("payments are not configured")

const (
	ErrCannotRead            model.Invalid  = "appointment_booking: cannot read the request"
	ErrNotFound              model.NotFound = "record not found"
	ErrConflict              model.Conflict = "optimistic concurrency conflict"
	ErrTooManyHolds          model.Conflict = "too many online reservations awaiting payment"
	ErrPaymentNotVerified    model.Invalid  = "payment could not be verified for this reservation"
	ErrAlreadyPaid           model.Invalid  = "reservation already has a payment"
	ErrReservationNotPayable model.Invalid  = "reservation can no longer be paid"
	ErrUnsupportedCurrency   model.Invalid  = "unsupported currency"
)

// Repository lee y escribe las tablas del módulo a través de un crud.Tx: el Store del módulo (una operación suelta)
// o la transacción de una regla (varias escrituras que se confirman juntas). El Store ya acota todo al tenant del
// módulo, así que ninguna consulta repite tenant_id.
type Repository struct {
	tx  crud.Tx
	ids model.IDGenerator
}

// repoOn devuelve el Repository que trabaja dentro de tx.
func (m *Module) repoOn(tx crud.Tx) *Repository { return &Repository{tx: tx, ids: m.ids} }

func isNotFound(err error) bool { return model.KindOf(err) == model.ErrorNotFound }

// ----------------------------------------------------------------------------
// Reservation
// ----------------------------------------------------------------------------

func (r *Repository) reservations(conds ...storage.Condition) ([]Reservation, error) {
	var out []Reservation
	err := r.tx.ReadAll(func() model.Model { return &Reservation{} }, crud.Query{Conditions: conds}, func(row model.Model) {
		out = append(out, *row.(*Reservation))
	})
	return out, err
}

func (r *Repository) GetReservation(id string) (Reservation, error) {
	var res Reservation
	if err := r.tx.ReadOne(&res, id); err != nil {
		if isNotFound(err) {
			return Reservation{}, ErrNotFound
		}
		return Reservation{}, err
	}
	return res, nil
}

func (r *Repository) ListReservationsByStaff(staffId string, fromUtc, toUtc time.Unix) ([]Reservation, error) {
	return r.reservations(
		storage.Eq(Reservation_.StaffIdsnapshot, staffId),
		storage.Gte(Reservation_.ReservationTime, int64(fromUtc)),
		storage.Lt(Reservation_.ReservationTime, int64(toUtc)))
}

// ListReservationsByTenantRange lista las reservas de todo el tenant en un rango de fechas — el alcance del
// recompute del establecimiento (un feriado golpea a todos los profesionales a la vez).
func (r *Repository) ListReservationsByTenantRange(fromUtc, toUtc time.Unix) ([]Reservation, error) {
	return r.reservations(
		storage.Gte(Reservation_.ReservationTime, int64(fromUtc)),
		storage.Lt(Reservation_.ReservationTime, int64(toUtc)))
}

func (r *Repository) ListReservationsByClient(clientId string) ([]Reservation, error) {
	return r.reservations(storage.Eq(Reservation_.ClientId, clientId))
}

func (r *Repository) ListReservationsByStatus(status string) ([]Reservation, error) {
	return r.reservations(storage.Eq(Reservation_.Status, status))
}

// UpdateReservationStatus cambia el estado con optimismo: si la revisión guardada no es la esperada, ErrConflict.
func (r *Repository) UpdateReservationStatus(id, status, updatedBy string, updatedAt time.Nano, expectedRevision int64) error {
	got, err := r.GetReservation(id)
	if err != nil {
		return err
	}
	if got.Revision != expectedRevision {
		return ErrConflict
	}
	got.Status, got.UpdatedBy, got.UpdatedAt = status, updatedBy, updatedAt
	got.Revision++
	return r.tx.UpdateFields(&got, id, []string{Reservation_.Status, Reservation_.UpdatedBy, Reservation_.UpdatedAt, Reservation_.Revision})
}

// UpdateReservationConflict escribe el estado CONFLICTED (o su restauración) con optimismo. statusBefore queda
// grabado al entrar en conflicto y "" al salir (fuente única de restauración, §8.5).
func (r *Repository) UpdateReservationConflict(id, status, statusBefore, updatedBy string, updatedAt time.Nano, expectedRevision int64) error {
	got, err := r.GetReservation(id)
	if err != nil {
		return err
	}
	if got.Revision != expectedRevision {
		return ErrConflict
	}
	got.Status, got.StatusBeforeConflict, got.UpdatedBy, got.UpdatedAt = status, statusBefore, updatedBy, updatedAt
	got.Revision++
	return r.tx.UpdateFields(&got, id, []string{Reservation_.Status, Reservation_.StatusBeforeConflict, Reservation_.UpdatedBy, Reservation_.UpdatedAt, Reservation_.Revision})
}

// ----------------------------------------------------------------------------
// WorkCalendarException
// ----------------------------------------------------------------------------

func (r *Repository) ListExceptions(staffId string, from, to time.Unix) ([]WorkCalendarException, error) {
	var out []WorkCalendarException
	err := r.tx.ReadAll(func() model.Model { return &WorkCalendarException{} }, crud.Query{Conditions: []storage.Condition{
		storage.Eq(WorkCalendarException_.StaffId, staffId),
		storage.Gte(WorkCalendarException_.SpecificDate, int64(from)),
		storage.Lte(WorkCalendarException_.SpecificDate, int64(to)),
	}}, func(row model.Model) { out = append(out, *row.(*WorkCalendarException)) })
	return out, err
}

func (r *Repository) GetException(id string) (WorkCalendarException, error) {
	var e WorkCalendarException
	if err := r.tx.ReadOne(&e, id); err != nil {
		if isNotFound(err) {
			return WorkCalendarException{}, ErrNotFound
		}
		return WorkCalendarException{}, err
	}
	return e, nil
}

// ----------------------------------------------------------------------------
// EmployeeServiceConfig
// ----------------------------------------------------------------------------

func (r *Repository) GetEmployeeServiceConfig(id string) (EmployeeServiceConfig, error) {
	var c EmployeeServiceConfig
	if err := r.tx.ReadOne(&c, id); err != nil {
		if isNotFound(err) {
			return EmployeeServiceConfig{}, ErrNotFound
		}
		return EmployeeServiceConfig{}, err
	}
	return c, nil
}

func (r *Repository) ListEmployeeServiceConfigs(conds ...storage.Condition) ([]EmployeeServiceConfig, error) {
	var out []EmployeeServiceConfig
	err := r.tx.ReadAll(func() model.Model { return &EmployeeServiceConfig{} }, crud.Query{Conditions: conds}, func(row model.Model) {
		out = append(out, *row.(*EmployeeServiceConfig))
	})
	return out, err
}

func (r *Repository) ListEmployeeServiceConfigByStaff(staffId string) ([]EmployeeServiceConfig, error) {
	return r.ListEmployeeServiceConfigs(storage.Eq(EmployeeServiceConfig_.StaffId, staffId))
}

// ----------------------------------------------------------------------------
// WorkCalendarConfig
// ----------------------------------------------------------------------------

func (r *Repository) GetCalendarConfig(staffId string) (WorkCalendarConfig, error) {
	var found WorkCalendarConfig
	var has bool
	err := r.tx.ReadAll(func() model.Model { return &WorkCalendarConfig{} }, crud.Query{Conditions: []storage.Condition{
		storage.Eq(WorkCalendarConfig_.StaffId, staffId),
	}}, func(row model.Model) {
		if !has {
			found, has = *row.(*WorkCalendarConfig), true
		}
	})
	if err != nil {
		return WorkCalendarConfig{}, err
	}
	if !has {
		return WorkCalendarConfig{}, ErrNotFound
	}
	return found, nil
}

// ----------------------------------------------------------------------------
// WorkCalendarBlock
// ----------------------------------------------------------------------------

func (r *Repository) blocks(conds ...storage.Condition) ([]WorkCalendarBlock, error) {
	var out []WorkCalendarBlock
	err := r.tx.ReadAll(func() model.Model { return &WorkCalendarBlock{} }, crud.Query{Conditions: conds}, func(row model.Model) {
		out = append(out, *row.(*WorkCalendarBlock))
	})
	return out, err
}

// ListBlocks devuelve todos los bloques de un staff — semanales y datados mezclados; ListAvailability los separa
// por fecha o día de la semana.
func (r *Repository) ListBlocks(staffId string) ([]WorkCalendarBlock, error) {
	return r.blocks(storage.Eq(WorkCalendarBlock_.StaffId, staffId))
}

func (r *Repository) deleteBlocks(conds ...storage.Condition) error {
	rows, err := r.blocks(conds...)
	if err != nil {
		return err
	}
	for i := range rows {
		if err := r.tx.Delete(&WorkCalendarBlock{}, rows[i].Id); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) insertBlocks(blocks []WorkCalendarBlock) error {
	for i := range blocks {
		blocks[i].Id = r.ids.NewID()
		if err := r.tx.Create(&blocks[i]); err != nil {
			return err
		}
	}
	return nil
}

// ReplaceWeekdayBlocks pisa todos los bloques semanales del weekday dado — un replace de día completo, no un
// upsert por fila: los edits parciales son lo que desincroniza lo guardado de lo que muestra el editor (§7).
func (r *Repository) ReplaceWeekdayBlocks(staffId string, dayOfWeek int, blocks []WorkCalendarBlock) error {
	if err := r.deleteBlocks(
		storage.Eq(WorkCalendarBlock_.StaffId, staffId),
		storage.Eq(WorkCalendarBlock_.DayOfWeek, int64(dayOfWeek)),
		storage.Eq(WorkCalendarBlock_.SpecificDate, int64(0))); err != nil {
		return err
	}
	return r.insertBlocks(blocks)
}

// ReplaceDateBlocks pisa los bloques DATADOS de una fecha (CU-11: un día marcado puede divergir de la ventana
// común con la que se creó).
func (r *Repository) ReplaceDateBlocks(staffId string, date int64, blocks []WorkCalendarBlock) error {
	if err := r.DeleteDateBlocks(staffId, date); err != nil {
		return err
	}
	return r.insertBlocks(blocks)
}

// DeleteDateBlocks borra todos los bloques datados de esa fecha (CU-15: devuelve el día a lo que diga el
// template semanal, o a des-trabajado).
func (r *Repository) DeleteDateBlocks(staffId string, date int64) error {
	return r.deleteBlocks(
		storage.Eq(WorkCalendarBlock_.StaffId, staffId),
		storage.Eq(WorkCalendarBlock_.SpecificDate, date))
}
