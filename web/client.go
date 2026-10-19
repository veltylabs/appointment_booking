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
	paymentmanager "github.com/veltylabs/payment_manager"
	"github.com/veltylabs/payment_manager/sandbox"
	paymentmanagerui "github.com/veltylabs/payment_manager/ui"
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

// refundReason es el motivo de la devolución automática de un pago que llegó
// cuando la reserva ya no se podía pagar (su retención venció).
const refundReason = "reservation expired before payment"

// chargeViaBooking resuelve el ciclo de construcción: payment_manager pregunta
// el monto a appointment_booking, que se construye después con payment_manager
// como su PaymentReader.
type chargeViaBooking struct{ booking **ab.Module }

func (c chargeViaBooking) Charge(tenantId, referenceId string) (int64, string, bool, error) {
	return (*c.booking).Charge(tenantId, referenceId)
}

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
		Publisher: broker, TenantID: demoTenantID, ValidateRUT: trustedip.ValidateRUT,
	})
	if err != nil {
		panic(err)
	}
	bc, err := businesscalendar.New(db, businesscalendar.Deps{IDs: ids, Publisher: broker})
	if err != nil {
		panic(err)
	}
	// payment_manager y appointment_booking se necesitan entre sí: pm pregunta
	// el monto a abMod (Charge) y abMod verifica los pagos con pm (PaidAmount).
	var abMod *ab.Module
	pm, err := paymentmanager.New(db, paymentmanager.Deps{
		Publisher: broker,
		Gateway:   sandbox.New("/"),
		ReturnURL: "/",
		Charges:   []paymentmanager.ChargeSource{{Module: ab.ModelName, Reader: chargeViaBooking{&abMod}}},
	})
	if err != nil {
		panic(err)
	}
	abMod, err = ab.New(db, ab.Deps{
		Staff: sm, Catalog: ic, Directory: pd, Bounds: bc, IDs: ids, Publisher: broker, Payments: pm,
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
	caller := loopback.WithTenant(demoTenantID, abMod, sm, dm, ic, pd, bc, pm)

	// 4. Eventos: un cierre del establecimiento recalcula conflictos; un pago
	// confirma su reserva y, si la reserva ya no se puede pagar, se devuelve.
	broker.Subscribe(businesscalendar.EventCalendarChanged, func(ev events.Event) {
		if p, ok := ev.Payload.(*businesscalendar.CalendarChangedPayload); ok && p.Closed {
			_, _ = abMod.RecomputeConflicts(demoTenantID, p.FromDate, p.ToDate)
		}
	})
	broker.Subscribe(paymentmanager.EventPaymentPaid, func(ev events.Event) {
		p, ok := ev.Payload.(*paymentmanager.Payment)
		if !ok || p.ReferenceModule != ab.ModelName {
			return
		}
		err := abMod.ConfirmPaidReservation(p.TenantId, p.ReferenceId, p.Id)
		if err != nil && err.Error() == ab.ErrReservationNotPayable.Error() {
			_, _ = pm.RefundPayment(paymentmanager.RefundPaymentArgs{
				TenantId: p.TenantId, Id: ids.NewID(), PaymentId: p.Id,
				Amount: p.Amount - p.RefundedAmount, Reason: refundReason, Method: p.Method,
			})
		}
	})

	// 5. Construir vistas del navegador
	bookingView, err := ui.Browser(caller, ids, demoTenantID)
	if err != nil {
		panic(err)
	}
	onlineView, err := ui.OnlineBrowser(caller, ids, demoTenantID)
	if err != nil {
		panic(err)
	}
	paymentsView, err := paymentmanagerui.Browser(caller, ids, demoTenantID)
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
		Modules:   []platformd.UIModule{bookingView, onlineView, paymentsView, personalView},
		DefaultID: ui.ID,
	}
	Append("body", p)
	select {}
}
