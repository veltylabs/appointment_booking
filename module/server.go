//go:build !browser

package module

import (
	"webtyp.com/appkit"
	"webtyp.com/model"

	ab "github.com/veltylabs/appointment_booking"
)

var server = appkit.ModuleServer{Build: build}
var browser appkit.ModuleBrowser

func build(d appkit.Deps) (model.ModuleNaming, error) {
	staff, err := appkit.Find[ab.StaffReader](d)
	if err != nil {
		return nil, err
	}
	catalog, err := appkit.Find[ab.CatalogReader](d)
	if err != nil {
		return nil, err
	}
	patients, err := appkit.Find[ab.PatientDirectory](d)
	if err != nil {
		return nil, err
	}

	hours, _ := appkit.Find[ab.OpeningHours](d)
	payments, _ := appkit.Find[ab.PaymentGateway](d)

	return ab.New(d.DB, ab.Deps{
		IDs:       d.IDs,
		Publisher: d.Events,
		TenantID:  d.TenantID,
		Zone:      d.Zone,
		Staff:     staff,
		Catalog:   catalog,
		Patients:  patients,
		Hours:     hours,
		Payments:  payments,
	})
}
