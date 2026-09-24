package tests

import (
	"testing"

	"webtyp.com/events/mock"
	"webtyp.com/orm"
	"webtyp.com/storage/mem"

	"webtyp.com/auth/trusted_ip"

	ab "github.com/veltylabs/appointment_booking"
	"github.com/veltylabs/appointment_booking/seed"
	businesscalendar "github.com/veltylabs/business_calendar"
	calendarseed "github.com/veltylabs/business_calendar/seed"
	devicemanager "github.com/veltylabs/device_manager"
	deviceseed "github.com/veltylabs/device_manager/seed"
	itemcatalog "github.com/veltylabs/item_catalog"
	catalogseed "github.com/veltylabs/item_catalog/seed"
	patientdirectory "github.com/veltylabs/patient_directory"
	patientseed "github.com/veltylabs/patient_directory/seed"
	staffmanager "github.com/veltylabs/staff_manager"
	staffseed "github.com/veltylabs/staff_manager/seed"
)

func TestSeed_LoadPopulatesServicesAndReservations(t *testing.T) {
	db := orm.New(mem.New())
	ids := &fakeIDs{}
	broker := &mock.Broker{}
	tenantID := "demo"

	dm, err := devicemanager.New(db, devicemanager.Deps{IDs: ids, Publisher: broker, TenantID: tenantID})
	if err != nil {
		t.Fatalf("devicemanager.New: %v", err)
	}
	sm, err := staffmanager.New(db, staffmanager.Deps{
		IDs:         ids,
		Publisher:   broker,
		TenantID:    tenantID,
		ValidateRUT: trustedip.ValidateRUT,
		Devices:     devicemanager.IPLocator{Devices: dm, TenantID: tenantID},
	})
	if err != nil {
		t.Fatalf("staffmanager.New: %v", err)
	}
	ic, err := itemcatalog.New(db, itemcatalog.Deps{IDs: ids, Publisher: broker})
	if err != nil {
		t.Fatalf("itemcatalog.New: %v", err)
	}
	pd, err := patientdirectory.New(db, patientdirectory.Deps{
		IDs:         ids,
		Publisher:   broker,
		TenantID:    tenantID,
		ValidateRUT: trustedip.ValidateRUT,
	})
	if err != nil {
		t.Fatalf("patientdirectory.New: %v", err)
	}
	bc, err := businesscalendar.New(db, businesscalendar.Deps{IDs: ids, Publisher: broker})
	if err != nil {
		t.Fatalf("businesscalendar.New: %v", err)
	}

	abMod, err := ab.New(db, ab.Deps{
		Staff: sm, Catalog: ic, Directory: pd, Bounds: bc, IDs: ids, Publisher: broker,
	})
	if err != nil {
		t.Fatalf("ab.New: %v", err)
	}

	if _, err := deviceseed.Load(dm, tenantID); err != nil {
		t.Fatalf("deviceseed.Load: %v", err)
	}
	staffData, err := staffseed.Load(sm, tenantID)
	if err != nil {
		t.Fatalf("staffseed.Load: %v", err)
	}
	catalogData, err := catalogseed.Load(ic, tenantID)
	if err != nil {
		t.Fatalf("catalogseed.Load: %v", err)
	}
	patientData, err := patientseed.Load(pd, tenantID)
	if err != nil {
		t.Fatalf("patientseed.Load: %v", err)
	}
	if _, err := calendarseed.Load(bc); err != nil {
		t.Fatalf("calendarseed.Load: %v", err)
	}

	data, err := seed.Load(abMod, tenantID, seed.Upstream{
		Staff:    staffData,
		Catalog:  catalogData,
		Patients: patientData,
	})
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
	resDay := data.Reservations[0].ReservationDate
	slots, err := abMod.ListAvailability(tenantID, firstConfig.StaffId, firstConfig.Id, resDay, resDay)
	if err != nil {
		t.Fatalf("ListAvailability: %v", err)
	}
	if len(slots) == 0 {
		t.Errorf("ListAvailability returned 0 slots for day %d", resDay)
	}
}
