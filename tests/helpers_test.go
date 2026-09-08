package tests

import (
	"testing"

	"webtyp.com/orm"
	"webtyp.com/storage/mem"
	tinytime "webtyp.com/time"
	ab "github.com/veltylabs/appointment_booking"
)

// newTestModule monta el stack real del módulo sobre storage/mem + los deps
// dados (que incluyen el Broker/BoundsReader del test).
func newTestModule(t *testing.T, deps ab.Deps) (*ab.Module, *ab.Repository, *orm.DB) {
	t.Helper()
	db := orm.New(mem.New())
	repo, err := ab.NewRepository(db, deps.IDs)
	if err != nil {
		t.Fatalf("ab.NewRepository: %v", err)
	}
	m, err := ab.New(db, deps)
	if err != nil {
		t.Fatalf("ab.New: %v", err)
	}
	return m, repo, db
}

// newTestModuleService es el mismo stack pero expuesto como SchedulingService.
func newTestModuleService(t *testing.T, deps ab.Deps) (ab.SchedulingService, *ab.Repository) {
	t.Helper()
	m, repo, _ := newTestModule(t, deps)
	return m, repo
}

// seedEmployeeConfig inserta un EmployeeServiceConfig activo y devuelve su id.
func seedEmployeeConfig(t *testing.T, repo *ab.Repository, tenant, staff string, durationMin int64) string {
	t.Helper()
	cfg := ab.EmployeeServiceConfig{
		TenantId: tenant, StaffId: staff, ServiceId: "srv1",
		DurationMin: durationMin, IsActive: true,
	}
	if err := repo.InsertEmployeeServiceConfig(cfg); err != nil {
		t.Fatalf("InsertEmployeeServiceConfig: %v", err)
	}
	cfgs, err := repo.ListEmployeeServiceConfigByStaff(tenant, staff)
	if err != nil || len(cfgs) == 0 {
		t.Fatalf("ListEmployeeServiceConfigByStaff: %v, %d", err, len(cfgs))
	}
	return cfgs[0].Id
}

// ensureCalendarConfig activa el calendario de (tenant, staff) en UTC.
func ensureCalendarConfig(t *testing.T, s ab.SchedulingService, tenant, staff string) {
	t.Helper()
	if err := s.UpsertCalendarConfig(ab.WorkCalendarConfig{
		TenantId: tenant, StaffId: staff, Timezone: "UTC", IsActive: true,
	}); err != nil {
		t.Fatalf("UpsertCalendarConfig: %v", err)
	}
}

// seedWeekdayBlock pisa la plantilla semanal de un weekday.
func seedWeekdayBlock(t *testing.T, s ab.SchedulingService, tenant, staff string, dow int, blocks []ab.WorkCalendarBlock) {
	t.Helper()
	if err := s.SaveDayBlocks(tenant, staff, dow, blocks); err != nil {
		t.Fatalf("SaveDayBlocks (dow %d): %v", dow, err)
	}
}

// defaultBlocks es la jornada típica 09:00–17:00.
func defaultBlocks() []ab.WorkCalendarBlock {
	return []ab.WorkCalendarBlock{{StartMin: 540, EndMin: 1020, IsActive: true}}
}

// setupReservable prepara staff + config de servicio + bloque semanal 09–17 de
// modo que un slot en slotStart es reservable, y devuelve el cfgID.
func setupReservable(t *testing.T, s ab.SchedulingService, repo *ab.Repository, tenant, staff string, slotStart int64) string {
	t.Helper()
	cfgID := seedEmployeeConfig(t, repo, tenant, staff, 60)
	ensureCalendarConfig(t, s, tenant, staff)
	seedWeekdayBlock(t, s, tenant, staff, int(tinytime.Weekday(slotStart)), defaultBlocks())
	return cfgID
}

// createBooked crea una reserva en slotStart y la confirma (revision 0 → 1).
func createBooked(t *testing.T, s ab.SchedulingService, tenant, staff, cfgID string, slotStart int64) ab.Reservation {
	t.Helper()
	res, err := s.CreateReservation(ab.CreateReservationCmd{
		TenantId:                tenant,
		ClientId:                "c1",
		CreatorUserId:           "u1",
		EmployeeServiceConfigId: cfgID,
		SlotStartUtc:            slotStart,
	})
	if err != nil {
		t.Fatalf("CreateReservation: %v", err)
	}
	if err := s.ChangeReservationStatus(ab.ChangeStatusCmd{
		TenantId: tenant, Id: res.Id, Event: ab.EventConfirm, ActorId: "u1", Revision: 0,
	}); err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	got, err := s.GetReservation(tenant, res.Id)
	if err != nil {
		t.Fatalf("GetReservation: %v", err)
	}
	return got
}

// slotIn asserts a slot is entirely inside [startMin, endMin) local minutes of date dayStart.
func slotIn(t *testing.T, slot ab.TimeSlot, dayStart int64, startMin, endMin int) {
	t.Helper()
	startLocal := int((slot.StartUtc - dayStart) / 60)
	endLocal := int((slot.EndUtc - dayStart) / 60)
	if startLocal < startMin || endLocal > endMin {
		t.Fatalf("slot %v outside window [%d, %d) local minutes", slot, startMin, endMin)
	}
}

// slotsOnDay filtra los slots de un día.
func slotsOnDay(slots []ab.TimeSlot, dayStart int64) []ab.TimeSlot {
	var out []ab.TimeSlot
	for _, s := range slots {
		if s.StartUtc >= dayStart && s.StartUtc < dayStart+86400 {
			out = append(out, s)
		}
	}
	return out
}