package appointmentbooking

import (
	"webtyp.com/ddl"
	"webtyp.com/fmt"
	"webtyp.com/model"
	"webtyp.com/orm"
)

// Errores sentinela a nivel de paquete
var (
	ErrNotFound = fmt.Err("record", "not", "found")
	ErrConflict = fmt.Err("optimistic", "concurrency", "conflict")
)

// Repository provee operaciones CRUD para todas las tablas de appointment-booking.
type Repository struct {
	db  *orm.DB
	ids model.IDGenerator
}

// NewRepository crea un nuevo Repository y migra sus 5 tablas propias cuando el backend
// soporta DDL (no-op contra storage/mem, usado por las pruebas propias de este módulo).
func NewRepository(db *orm.DB, ids model.IDGenerator) (*Repository, error) {
	tables := []model.Model{
		&EmployeeServiceConfig{},
		&WorkCalendarConfig{},
		&WorkCalendarBlock{},
		&WorkCalendarException{},
		&Reservation{},
	}
	if ddlCompiler, ok := db.RawConn().(ddl.Compiler); ok {
		for _, t := range tables {
			if err := ddl.New(db.RawConn(), ddlCompiler).CreateTable(t); err != nil {
				return nil, err
			}
		}
	}
	return &Repository{db: db, ids: ids}, nil
}

// ----------------------------------------------------------------------------
// Reservation
// ----------------------------------------------------------------------------

func (r *Repository) InsertReservation(res *Reservation) error {
	if res.Id == "" {
		res.Id = r.ids.NewID()
	}
	res.Revision = 0
	return r.db.Create(res)
}

func (r *Repository) GetReservation(id string) (Reservation, error) {
	m := &Reservation{}
	qb := r.db.Query(m).Where(Reservation_.Id).Eq(id)
	got, err := ReadOneReservation(qb, m)
	if err == orm.ErrNotFound {
		return Reservation{}, ErrNotFound
	}
	if err != nil {
		return Reservation{}, err
	}
	return *got, nil
}

func (r *Repository) GetReservationTx(tx *orm.DB, tenantId, id string) (Reservation, error) {
	m := &Reservation{}
	qb := tx.Query(m).Where(Reservation_.Id).Eq(id).Where(Reservation_.TenantId).Eq(tenantId)
	got, err := ReadOneReservation(qb, m)
	if err == orm.ErrNotFound {
		return Reservation{}, ErrNotFound
	}
	if err != nil {
		return Reservation{}, err
	}
	return *got, nil
}

func (r *Repository) ListReservationsByStaff(tenantId, staffId string, from, to int64) ([]Reservation, error) {
	proxy := &Reservation{}
	qb := r.db.Query(proxy).
		Where(Reservation_.TenantId).Eq(tenantId).
		Where(Reservation_.StaffIdsnapshot).Eq(staffId).
		Where(Reservation_.ReservationDate).Gte(from).
		Where(Reservation_.ReservationDate).Lte(to)
	rows, err := ReadAllReservation(qb)
	if err != nil {
		return nil, err
	}
	out := make([]Reservation, len(rows))
	for i, row := range rows {
		out[i] = *row
	}
	return out, nil
}

// ListReservationsByTenantRange lista las reservas de todo un tenant en un rango
// de fechas — el alcance del recompute del establecimiento (un feriado golpea a
// todos los profesionales a la vez).
func (r *Repository) ListReservationsByTenantRange(tenantId string, from, to int64) ([]Reservation, error) {
	proxy := &Reservation{}
	qb := r.db.Query(proxy).
		Where(Reservation_.TenantId).Eq(tenantId).
		Where(Reservation_.ReservationDate).Gte(from).
		Where(Reservation_.ReservationDate).Lte(to)
	rows, err := ReadAllReservation(qb)
	if err != nil {
		if err == orm.ErrNotFound {
			return nil, nil
		}
		return nil, err
	}
	out := make([]Reservation, len(rows))
	for i, row := range rows {
		out[i] = *row
	}
	return out, nil
}

func (r *Repository) ListReservationsByClient(tenantId, clientId string) ([]Reservation, error) {
	proxy := &Reservation{}
	qb := r.db.Query(proxy).
		Where(Reservation_.TenantId).Eq(tenantId).
		Where(Reservation_.ClientId).Eq(clientId)
	rows, err := ReadAllReservation(qb)
	if err != nil {
		return nil, err
	}
	out := make([]Reservation, len(rows))
	for i, row := range rows {
		out[i] = *row
	}
	return out, nil
}

func (r *Repository) UpdateReservationStatus(id, status, updatedBy string, updatedAt int64, expectedRevision int64) error {
	return r.db.Tx(func(tx *orm.DB) error {
		return r.UpdateReservationStatusTx(tx, id, status, updatedBy, updatedAt, expectedRevision)
	})
}

func (r *Repository) UpdateReservationStatusTx(tx *orm.DB, id, status, updatedBy string, updatedAt int64, expectedRevision int64) error {
	current := &Reservation{}
	qb := tx.Query(current).Where(Reservation_.Id).Eq(id)
	got, err := ReadOneReservation(qb, current)
	if err == orm.ErrNotFound {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if got.Revision != expectedRevision {
		return ErrConflict
	}
	got.Status = status
	got.UpdatedBy = updatedBy
	got.UpdatedAt = updatedAt
	got.Revision++
	return tx.Update(got, orm.Eq(Reservation_.Id, id), orm.Eq(Reservation_.TenantId, got.TenantId))
}

// UpdateReservationConflictTx escribe el estado CONFLICTED (o su restauración)
// con optimismo: WHERE revision = N y tenant scope en ambas ramas. statusBefore
// queda grabado al entrar en conflicto y "" al salir (fuente única de
// restauración, §8.5).
func (r *Repository) UpdateReservationConflictTx(tx *orm.DB, id, tenantId, status, statusBefore, updatedBy string, updatedAt int64, expectedRevision int64) error {
	current := &Reservation{}
	qb := tx.Query(current).Where(Reservation_.Id).Eq(id).Where(Reservation_.TenantId).Eq(tenantId)
	got, err := ReadOneReservation(qb, current)
	if err == orm.ErrNotFound {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if got.Revision != expectedRevision {
		return ErrConflict
	}
	got.Status = status
	got.StatusBeforeConflict = statusBefore
	got.UpdatedBy = updatedBy
	got.UpdatedAt = updatedAt
	got.Revision++
	return tx.Update(got, orm.Eq(Reservation_.Id, id), orm.Eq(Reservation_.TenantId, tenantId))
}

// ----------------------------------------------------------------------------
// WorkCalendarException
// ----------------------------------------------------------------------------

func (r *Repository) InsertException(exc WorkCalendarException) error {
	if exc.Id == "" {
		exc.Id = r.ids.NewID()
	}
	return r.db.Create(&exc)
}

func (r *Repository) ListExceptions(tenantId, staffId string, from, to int64) ([]WorkCalendarException, error) {
	proxy := &WorkCalendarException{}
	qb := r.db.Query(proxy).
		Where(WorkCalendarException_.TenantId).Eq(tenantId).
		Where(WorkCalendarException_.StaffId).Eq(staffId).
		Where(WorkCalendarException_.SpecificDate).Gte(from).
		Where(WorkCalendarException_.SpecificDate).Lte(to)
	rows, err := ReadAllWorkCalendarException(qb)
	if err != nil {
		return nil, err
	}
	out := make([]WorkCalendarException, len(rows))
	for i, row := range rows {
		out[i] = *row
	}
	return out, nil
}

func (r *Repository) DeleteException(tenantId, id string) error {
	return r.db.Delete(&WorkCalendarException{}, orm.Eq(WorkCalendarException_.Id, id), orm.Eq(WorkCalendarException_.TenantId, tenantId))
}

func (r *Repository) GetException(tenantId, id string) (WorkCalendarException, error) {
	m := &WorkCalendarException{}
	qb := r.db.Query(m).
		Where(WorkCalendarException_.Id).Eq(id).
		Where(WorkCalendarException_.TenantId).Eq(tenantId)
	got, err := ReadOneWorkCalendarException(qb, m)
	if err == orm.ErrNotFound {
		return WorkCalendarException{}, ErrNotFound
	}
	if err != nil {
		return WorkCalendarException{}, err
	}
	return *got, nil
}

// ----------------------------------------------------------------------------
// EmployeeServiceConfig
// ----------------------------------------------------------------------------

func (r *Repository) InsertEmployeeServiceConfig(cfg EmployeeServiceConfig) error {
	if cfg.Id == "" {
		cfg.Id = r.ids.NewID()
	}
	return r.db.Create(&cfg)
}

func (r *Repository) GetEmployeeServiceConfig(id string) (EmployeeServiceConfig, error) {
	m := &EmployeeServiceConfig{}
	qb := r.db.Query(m).Where(EmployeeServiceConfig_.Id).Eq(id)
	got, err := ReadOneEmployeeServiceConfig(qb, m)
	if err == orm.ErrNotFound {
		return EmployeeServiceConfig{}, ErrNotFound
	}
	if err != nil {
		return EmployeeServiceConfig{}, err
	}
	return *got, nil
}

func (r *Repository) ListEmployeeServiceConfigByStaff(tenantId, staffId string) ([]EmployeeServiceConfig, error) {
	proxy := &EmployeeServiceConfig{}
	qb := r.db.Query(proxy).
		Where(EmployeeServiceConfig_.TenantId).Eq(tenantId).
		Where(EmployeeServiceConfig_.StaffId).Eq(staffId)
	rows, err := ReadAllEmployeeServiceConfig(qb)
	if err != nil {
		return nil, err
	}
	out := make([]EmployeeServiceConfig, len(rows))
	for i, row := range rows {
		out[i] = *row
	}
	return out, nil
}

func (r *Repository) UpdateEmployeeServiceConfig(cfg EmployeeServiceConfig) error {
	return r.db.Update(&cfg, orm.Eq(EmployeeServiceConfig_.Id, cfg.Id), orm.Eq(EmployeeServiceConfig_.TenantId, cfg.TenantId))
}

// ----------------------------------------------------------------------------
// WorkCalendarConfig
// ----------------------------------------------------------------------------

func (r *Repository) UpsertCalendarConfig(cfg WorkCalendarConfig) error {
	// Intenta encontrar un registro existente para este (tenantId, staffId)
	existing := &WorkCalendarConfig{}
	qb := r.db.Query(existing).
		Where(WorkCalendarConfig_.TenantId).Eq(cfg.TenantId).
		Where(WorkCalendarConfig_.StaffId).Eq(cfg.StaffId)
	got, err := ReadOneWorkCalendarConfig(qb, existing)
	if err != nil && err != orm.ErrNotFound {
		return err
	}
	if err == orm.ErrNotFound {
		// No existe — crear
		cfg.Id = r.ids.NewID()
		return r.db.Create(&cfg)
	}
	// Existe — actualizar en el lugar (preservando el ID original)
	cfg.Id = got.Id
	return r.db.Update(&cfg, orm.Eq(WorkCalendarConfig_.Id, cfg.Id), orm.Eq(WorkCalendarConfig_.TenantId, cfg.TenantId))
}

func (r *Repository) GetCalendarConfig(tenantId, staffId string) (WorkCalendarConfig, error) {
	m := &WorkCalendarConfig{}
	qb := r.db.Query(m).
		Where(WorkCalendarConfig_.TenantId).Eq(tenantId).
		Where(WorkCalendarConfig_.StaffId).Eq(staffId)
	got, err := ReadOneWorkCalendarConfig(qb, m)
	if err == orm.ErrNotFound {
		return WorkCalendarConfig{}, ErrNotFound
	}
	if err != nil {
		return WorkCalendarConfig{}, err
	}
	return *got, nil
}

// ----------------------------------------------------------------------------
// WorkCalendarBlock
// ----------------------------------------------------------------------------

// ListBlocks devuelve todos los bloques de un staff — semanales y datados
// mezclados; ListAvailability los separa por fecha/weekday con las helpers del
// servicio.
func (r *Repository) ListBlocks(tenantId, staffId string) ([]WorkCalendarBlock, error) {
	proxy := &WorkCalendarBlock{}
	qb := r.db.Query(proxy).
		Where(WorkCalendarBlock_.TenantId).Eq(tenantId).
		Where(WorkCalendarBlock_.StaffId).Eq(staffId)
	rows, err := ReadAllWorkCalendarBlock(qb)
	if err != nil {
		return nil, err
	}
	out := make([]WorkCalendarBlock, len(rows))
	for i, row := range rows {
		out[i] = *row
	}
	return out, nil
}

// ReplaceWeekdayBlocks pisa todos los bloques semanales del weekday dado — un
// replace de día completo, no un upsert por fila: los edits parciales son lo
// que desincroniza el conjunto guardado de lo que muestra el editor (§7).
func (r *Repository) ReplaceWeekdayBlocks(tenantId, staffId string, dayOfWeek int, blocks []WorkCalendarBlock) error {
	return r.db.Tx(func(tx *orm.DB) error {
		if err := tx.Delete(&WorkCalendarBlock{},
			orm.Eq(WorkCalendarBlock_.TenantId, tenantId),
			orm.Eq(WorkCalendarBlock_.StaffId, staffId),
			orm.Eq(WorkCalendarBlock_.DayOfWeek, int64(dayOfWeek)),
			orm.Eq(WorkCalendarBlock_.SpecificDate, int64(0)),
		); err != nil {
			return err
		}
		for i := range blocks {
			blocks[i].Id = r.ids.NewID()
			if err := tx.Create(&blocks[i]); err != nil {
				return err
			}
		}
		return nil
	})
}

// ReplaceDateBlocks pisa los bloques DATADOS de una fecha (CU-11: un día
// marcado puede divergir de la ventana común con la que se creó).
func (r *Repository) ReplaceDateBlocks(tenantId, staffId string, date int64, blocks []WorkCalendarBlock) error {
	return r.db.Tx(func(tx *orm.DB) error {
		if err := tx.Delete(&WorkCalendarBlock{},
			orm.Eq(WorkCalendarBlock_.TenantId, tenantId),
			orm.Eq(WorkCalendarBlock_.StaffId, staffId),
			orm.Eq(WorkCalendarBlock_.SpecificDate, date),
		); err != nil {
			return err
		}
		for i := range blocks {
			blocks[i].Id = r.ids.NewID()
			if err := tx.Create(&blocks[i]); err != nil {
				return err
			}
		}
		return nil
	})
}

// DeleteDateBlocks borra todos los bloques datados de esas fechas (CU-15:
// devuelve el día a lo que diga el template semanal, o a des-trabajado).
func (r *Repository) DeleteDateBlocks(tenantId, staffId string, date int64) error {
	return r.db.Delete(&WorkCalendarBlock{},
		orm.Eq(WorkCalendarBlock_.TenantId, tenantId),
		orm.Eq(WorkCalendarBlock_.StaffId, staffId),
		orm.Eq(WorkCalendarBlock_.SpecificDate, date),
	)
}
