//go:build !wasm

package appointment_booking

import (
	appointmentbooking "github.com/veltylabs/appointment_booking"
	"webtyp.com/events"
	"webtyp.com/model"
	"webtyp.com/orm"
	"webtyp.com/router"

	businesscalendarwrapper "github.com/veltylabs/mjosefa-cms/modules/business_calendar"
	itemcatalogwrapper "github.com/veltylabs/mjosefa-cms/modules/item_catalog"
	patientdirectorywrapper "github.com/veltylabs/mjosefa-cms/modules/patient_directory"
	staffmanagerwrapper "github.com/veltylabs/mjosefa-cms/modules/staff_manager"
)

// NewBackend is the SINGLE construction path for the booking stack: it builds
// the four sibling readers this module needs (staff, catalog, directory,
// calendar bounds) and the appointment_booking module over them. Each sibling
// wrapper's own exported NewBackend is called here too — same idiom as
// staff_manager/server.go composing device_manager — so every instance is
// stateless over the shared *orm.DB and cheap to build twice.
//
// The four readers (StaffReader, CatalogReader, DirectoryReader, BoundsReader)
// are satisfied by these modules STRUCTURALLY — no adapter, no import in
// either direction: see each Deps.<Reader> in appointment_booking/service.go.
func NewBackend(db *orm.DB, ids model.IDGenerator, pub events.Publisher, tenantID string) (*appointmentbooking.Module, error) {
	staffMod, _, err := staffmanagerwrapper.NewBackend(db, ids, pub, tenantID)
	if err != nil {
		return nil, err
	}
	catalogMod, err := itemcatalogwrapper.NewBackend(db, ids, pub)
	if err != nil {
		return nil, err
	}
	directoryMod, err := patientdirectorywrapper.NewBackend(db, ids, pub, tenantID)
	if err != nil {
		return nil, err
	}
	boundsMod, err := businesscalendarwrapper.NewBackend(db, ids, pub)
	if err != nil {
		return nil, err
	}

	return appointmentbooking.New(db, appointmentbooking.Deps{
		Staff:     staffMod,
		Catalog:   catalogMod,
		Directory: directoryMod,
		Bounds:    boundsMod,
		IDs:       ids,
		Publisher: pub,
	})
}

// Server builds this module's operations for the registry in
// modules/server.go. db == nil is the safe default when DATABASE_URL is
// unset — this module then contributes nothing, without error.
func Server(db *orm.DB, ids model.IDGenerator, pub events.Publisher, tenantID string) (router.OperationModule, error) {
	if db == nil {
		return nil, nil
	}
	m, err := NewBackend(db, ids, pub, tenantID)
	if err != nil {
		return nil, err
	}
	return m, nil
}
