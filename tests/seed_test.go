package tests

import (
	"testing"

	"webtyp.com/events/mock"
	"webtyp.com/orm"
	"webtyp.com/storage/mem"
	"webtyp.com/time"

	ab "github.com/veltylabs/appointment_booking"
	"github.com/veltylabs/appointment_booking/seed"
)

func TestSeed_LoadPopulatesServicesAndReservations(t *testing.T) {
	db := orm.New(mem.New())
	ids := &fakeIDs{}
	broker := &mock.Broker{}
	tenantID := "demo"
	zone, _ := time.LoadZone("America/Santiago")

	staffMock := &MockStaffReader{Exists: true}
	catMock := &MockCatalogReader{Exists: true, Name: "Consulta General", Price: 35000, Currency: "CLP", Active: true}
	patMock := &MockDirectoryReader{Exists: true}

	abMod, err := ab.New(db, ab.Deps{
		Staff:     staffMock,
		Catalog:   catMock,
		Patients:  patMock,
		IDs:       ids,
		Publisher: broker,
		Zone:      zone,
		TenantID:  tenantID,
	})
	if err != nil {
		t.Fatalf("ab.New: %v", err)
	}

	opts := seed.LoadOptions{
		Staff: []seed.StaffSeed{
			{ID: "sm-1", Name: "Dr. Uno", Specialty: "Pediatría"},
			{ID: "sm-2", Name: "Dra. Dos", Specialty: "Cardiología"},
			{ID: "sm-3", Name: "Dr. Tres", Specialty: "Medicina General"},
		},
		Services: []seed.ServiceSeed{
			{ID: "svc-1", Specialty: "Pediatría"},
			{ID: "svc-2", Specialty: "Cardiología"},
			{ID: "svc-3", Specialty: "Medicina General"},
		},
		PatientIDs: []string{"pat-1", "pat-2"},
	}

	data, err := seed.Load(abMod, tenantID, opts)
	if err != nil {
		t.Fatalf("seed.Load: %v", err)
	}

	if len(data.ServiceConfigs) != 3 {
		t.Errorf("want 3 service configs, got %d", len(data.ServiceConfigs))
	}
	if len(data.Reservations) != 2 {
		t.Errorf("want 2 confirmed reservations, got %d", len(data.Reservations))
	}

	firstConfig := data.ServiceConfigs[0]
	resUtc := data.Reservations[0].ReservationTime
	resDay := time.MidnightUTC(time.Unix(resUtc))
	slots, err := abMod.ListAvailability(tenantID, firstConfig.StaffId, firstConfig.Id, resDay, resDay+time.Day)
	if err != nil {
		t.Fatalf("ListAvailability: %v", err)
	}
	if len(slots) == 0 {
		t.Errorf("ListAvailability returned 0 slots for day %v", resDay)
	}
}
