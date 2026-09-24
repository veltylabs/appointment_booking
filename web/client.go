//go:build wasm

package main

import (
	. "webtyp.com/dom"
	"webtyp.com/events"
	"webtyp.com/events/mock"
	"webtyp.com/layout/platformd"
	"webtyp.com/orm"
	"webtyp.com/router/loopback"
	"webtyp.com/storage/mem"
	"webtyp.com/unixid"

	"webtyp.com/auth/trusted_ip"

	ab "github.com/veltylabs/appointment_booking"
	"github.com/veltylabs/appointment_booking/seed"
	"github.com/veltylabs/appointment_booking/ui"
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

// demoTenantID es el único tenant de esta demo interactiva en navegador.
const demoTenantID = "demo"

// demoUser es la identidad fija que muestra el shell de la demo (sin login).
type demoUser struct{}

func (demoUser) UserName() string    { return "Demo" }
func (demoUser) UserAvatar() string  { return "" }
func (demoUser) UserRoles() []string { return []string{"Administrador"} }

func main() {
	ids, err := unixid.NewUnixID()
	if err != nil {
		panic(err)
	}
	db := orm.New(mem.New())
	broker := &mock.Broker{}

	// 1. Construir los módulos en orden de dependencias
	dm, err := devicemanager.New(db, devicemanager.Deps{
		IDs: ids, Publisher: broker, TenantID: demoTenantID,
	})
	if err != nil {
		panic(err)
	}
	sm, err := staffmanager.New(db, staffmanager.Deps{
		IDs:         ids,
		Publisher:   broker,
		TenantID:    demoTenantID,
		ValidateRUT: trustedip.ValidateRUT,
		Devices:     devicemanager.IPLocator{Devices: dm, TenantID: demoTenantID},
	})
	if err != nil {
		panic(err)
	}
	ic, err := itemcatalog.New(db, itemcatalog.Deps{IDs: ids, Publisher: broker})
	if err != nil {
		panic(err)
	}
	pd, err := patientdirectory.New(db, patientdirectory.Deps{
		IDs: ids, Publisher: broker, TenantID: demoTenantID, ValidateRUT: trustedip.ValidateRUT,
	})
	if err != nil {
		panic(err)
	}
	bc, err := businesscalendar.New(db, businesscalendar.Deps{IDs: ids, Publisher: broker})
	if err != nil {
		panic(err)
	}
	abMod, err := ab.New(db, ab.Deps{
		Staff: sm, Catalog: ic, Directory: pd, Bounds: bc, IDs: ids, Publisher: broker,
	})
	if err != nil {
		panic(err)
	}

	// 2. Cargar semillas, upstream primero
	if _, err := deviceseed.Load(dm, demoTenantID); err != nil {
		panic(err)
	}
	staffData, err := staffseed.Load(sm, demoTenantID)
	if err != nil {
		panic(err)
	}
	catalogData, err := catalogseed.Load(ic, demoTenantID)
	if err != nil {
		panic(err)
	}
	patientData, err := patientseed.Load(pd, demoTenantID)
	if err != nil {
		panic(err)
	}
	if _, err := calendarseed.Load(bc); err != nil {
		panic(err)
	}
	if _, err := seed.Load(abMod, demoTenantID, seed.Upstream{
		Staff:    staffData,
		Catalog:  catalogData,
		Patients: patientData,
	}); err != nil {
		panic(err)
	}

	// 3. Montar módulos en router loopback
	caller := loopback.WithTenant(demoTenantID, abMod, sm, dm, ic, pd, bc)

	// 4. Suscribir broker a cambios de calendario para recomputar conflictos
	broker.Subscribe(businesscalendar.EventCalendarChanged, func(ev events.Event) {
		if p, ok := ev.Payload.(*businesscalendar.CalendarChangedPayload); ok && p.Closed {
			_, _ = abMod.RecomputeConflicts(demoTenantID, p.FromDate, p.ToDate)
		}
	})

	// 5. Construir vistas del navegador
	bookingView, err := ui.Browser(caller, ids, demoTenantID)
	if err != nil {
		panic(err)
	}
	personalView, err := ui.PersonalBrowser(caller, ids, demoTenantID)
	if err != nil {
		panic(err)
	}

	p := &platformd.Platform{
		AppName:   ui.NavLabel + " — demo",
		User:      demoUser{},
		Modules:   []platformd.UIModule{bookingView, personalView},
		DefaultID: ui.ID,
	}
	Append("body", p)
	select {}
}
