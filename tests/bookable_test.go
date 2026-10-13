package tests

import (
	"testing"

	"webtyp.com/storage"
)

func TestBookable_ListsAndOrders(t *testing.T) {
	e := newBookEnv(t)
	e.staff.cards = append(e.staff.cards, staffCard{"s3", "Dr. Ciro", "Ecografía", true})
	e.catalog.items = append(e.catalog.items, serviceItem{"srv2", "Ecografía abdominal", "CLP", 40000, true})
	e.config(t, "s2", 0, true)
	e.config(t, "s1", 20000, false)
	if _, err := e.m.CreateEmployeeServiceConfig(cfgFor("s3", "srv2")); err != nil {
		t.Fatal(err)
	}
	list, err := e.m.ListBookableServices(bTenant, "")
	if err != nil || len(list) != 3 {
		t.Fatalf("got %d, %v", len(list), err)
	}
	if list[0].Specialty != "Ecografía" || list[1].StaffName != "Dr. Beto" || list[2].StaffName != "Dra. Ana" {
		t.Fatalf("order: %+v", list)
	}
	if list[1].Price != 25000 || !list[1].PaymentRequired || list[2].Price != 20000 || list[1].ServiceName != "Consulta" || list[1].Currency != "CLP" {
		t.Fatalf("fields: %+v", list)
	}
	// misma especialidad: ordena por servicio antes que por profesional
	e.catalog.items = append(e.catalog.items, serviceItem{"srv3", "Control", "CLP", 15000, true})
	if _, err := e.m.CreateEmployeeServiceConfig(cfgFor("s1", "srv3")); err != nil {
		t.Fatal(err)
	}
	if again, _ := e.m.ListBookableServices(bTenant, "Medicina General"); len(again) != 3 || again[0].ServiceName != "Consulta" || again[2].ServiceName != "Control" {
		t.Fatalf("by service: %+v", again)
	}
	only, _ := e.m.ListBookableServices(bTenant, "Ecografía")
	if len(only) != 1 || only[0].StaffId != "s3" {
		t.Fatalf("specialty filter: %+v", only)
	}
}

func TestBookable_StorageError(t *testing.T) {
	e := newBookEnv(t)
	e.conn.failOn(tblCfg, storage.ActionReadAll, 0)
	_, err := e.m.ListBookableServices(bTenant, "")
	wantErrIs(t, err, errStorage)
}

func TestBookable_StaffMissingSkipped(t *testing.T) {
	e := newBookEnv(t)
	e.config(t, "s1", 0, false)
	e.staff.cards = e.staff.cards[1:] // s1 deja de existir
	if list, err := e.m.ListBookableServices(bTenant, ""); err != nil || len(list) != 0 {
		t.Fatalf("got %+v, %v", list, err)
	}
}

func TestBookable_StaffErrorPropagates(t *testing.T) {
	e := newBookEnv(t)
	e.config(t, "s1", 0, false)
	e.staff.existsErr = errStorage
	_, err := e.m.ListBookableServices(bTenant, "")
	wantErrIs(t, err, errStorage)
	e.staff.existsErr = nil
	e.staff.cardErr = errStorage
	_, err = e.m.ListBookableServices(bTenant, "")
	wantErrIs(t, err, errStorage)
}

func TestBookable_InactiveStaffOrOtherSpecialtySkipped(t *testing.T) {
	e := newBookEnv(t)
	e.config(t, "s1", 0, false)
	if list, _ := e.m.ListBookableServices(bTenant, "Traumatología"); len(list) != 0 {
		t.Fatalf("other specialty: %+v", list)
	}
	e.staff.cards[0].active = false
	if list, _ := e.m.ListBookableServices(bTenant, ""); len(list) != 0 {
		t.Fatalf("inactive staff: %+v", list)
	}
}

func TestBookable_ServiceMissingSkipped(t *testing.T) {
	e := newBookEnv(t)
	e.config(t, "s1", 0, false)
	e.catalog.items = nil
	if list, err := e.m.ListBookableServices(bTenant, ""); err != nil || len(list) != 0 {
		t.Fatalf("got %+v, %v", list, err)
	}
}

func TestBookable_ServiceErrorPropagates(t *testing.T) {
	e := newBookEnv(t)
	e.config(t, "s1", 0, false)
	e.catalog.existsErr = errStorage
	_, err := e.m.ListBookableServices(bTenant, "")
	wantErrIs(t, err, errStorage)
	e.catalog.existsErr = nil
	e.catalog.cardErr = errStorage
	_, err = e.m.ListBookableServices(bTenant, "")
	wantErrIs(t, err, errStorage)
}

func TestBookable_InactiveServiceSkipped(t *testing.T) {
	e := newBookEnv(t)
	e.config(t, "s1", 0, false)
	e.catalog.items[0].active = false
	if list, _ := e.m.ListBookableServices(bTenant, ""); len(list) != 0 {
		t.Fatalf("got %+v", list)
	}
}
