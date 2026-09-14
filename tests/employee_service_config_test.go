package tests

import (
	"testing"

	ab "github.com/veltylabs/appointment_booking"
	"webtyp.com/input"
	"webtyp.com/model"
	"webtyp.com/orm"
	"webtyp.com/router"
	"webtyp.com/router/mock"
	"webtyp.com/storage/mem"
)

func TestCreateEmployeeServiceConfig(t *testing.T) {
	db := orm.New(mem.New())
	m, err := ab.New(db, SetupDependencies())
	if err != nil {
		t.Fatalf("ab.New: %v", err)
	}

	cfg := ab.EmployeeServiceConfig{
		TenantId:        "t1",
		StaffId:         "s1",
		ServiceId:       "srv1",
		DurationMin:     30,
		BufferMin:       10,
		PriceOverride:   5000,
		PaymentRequired: true,
		IsActive:        true,
	}

	created, err := m.CreateEmployeeServiceConfig(cfg)
	if err != nil {
		t.Fatalf("CreateEmployeeServiceConfig: %v", err)
	}
	if created.Id == "" {
		t.Fatalf("expected non-empty ID assigned to created config")
	}

	read, err := m.GetEmployeeServiceConfig(created.Id)
	if err != nil {
		t.Fatalf("GetEmployeeServiceConfig: %v", err)
	}
	if read.Id != created.Id || read.TenantId != "t1" || read.StaffId != "s1" || read.ServiceId != "srv1" {
		t.Fatalf("mismatch in read config: %+v", read)
	}
	if read.DurationMin != 30 || read.BufferMin != 10 || read.PriceOverride != 5000 || !read.PaymentRequired || !read.IsActive {
		t.Fatalf("mismatch in read config values: %+v", read)
	}

	fractionalCfg := ab.EmployeeServiceConfig{
		TenantId:        "t1",
		StaffId:         "s1",
		ServiceId:       "srv2",
		DurationMin:     30,
		BufferMin:       10,
		PriceOverride:   12990.50,
		PaymentRequired: true,
		IsActive:        true,
	}

	createdFrac, err := m.CreateEmployeeServiceConfig(fractionalCfg)
	if err != nil {
		t.Fatalf("CreateEmployeeServiceConfig fractional: %v", err)
	}

	readFrac, err := m.GetEmployeeServiceConfig(createdFrac.Id)
	if err != nil {
		t.Fatalf("GetEmployeeServiceConfig fractional: %v", err)
	}
	if readFrac.PriceOverride != 12990.50 {
		t.Fatalf("expected PriceOverride to survive fractional round-trip as 12990.50, got %f", readFrac.PriceOverride)
	}
}

func TestListEmployeeServiceConfigByStaff_ScopedToTenantAndStaff(t *testing.T) {
	db := orm.New(mem.New())
	m, err := ab.New(db, SetupDependencies())
	if err != nil {
		t.Fatalf("ab.New: %v", err)
	}

	_, err = m.CreateEmployeeServiceConfig(ab.EmployeeServiceConfig{TenantId: "t1", StaffId: "s1", ServiceId: "srv1", IsActive: true})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	_, err = m.CreateEmployeeServiceConfig(ab.EmployeeServiceConfig{TenantId: "t1", StaffId: "s1", ServiceId: "srv2", IsActive: false})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	_, err = m.CreateEmployeeServiceConfig(ab.EmployeeServiceConfig{TenantId: "t1", StaffId: "s2", ServiceId: "srv1", IsActive: true})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	_, err = m.CreateEmployeeServiceConfig(ab.EmployeeServiceConfig{TenantId: "t2", StaffId: "s1", ServiceId: "srv1", IsActive: true})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	listT1S1, err := m.ListEmployeeServiceConfigByStaff("t1", "s1")
	if err != nil {
		t.Fatalf("ListEmployeeServiceConfigByStaff(t1, s1): %v", err)
	}
	if len(listT1S1) != 2 {
		t.Fatalf("expected 2 configs for t1/s1, got %d", len(listT1S1))
	}

	listT1S2, err := m.ListEmployeeServiceConfigByStaff("t1", "s2")
	if err != nil {
		t.Fatalf("ListEmployeeServiceConfigByStaff(t1, s2): %v", err)
	}
	if len(listT1S2) != 1 {
		t.Fatalf("expected 1 config for t1/s2, got %d", len(listT1S2))
	}

	listT2S1, err := m.ListEmployeeServiceConfigByStaff("t2", "s1")
	if err != nil {
		t.Fatalf("ListEmployeeServiceConfigByStaff(t2, s1): %v", err)
	}
	if len(listT2S1) != 1 {
		t.Fatalf("expected 1 config for t2/s1, got %d", len(listT2S1))
	}

	listT1S3, err := m.ListEmployeeServiceConfigByStaff("t1", "s3")
	if err != nil {
		t.Fatalf("ListEmployeeServiceConfigByStaff(t1, s3): %v", err)
	}
	if len(listT1S3) != 0 {
		t.Fatalf("expected 0 configs for t1/s3, got %d", len(listT1S3))
	}
}

func TestUpdateEmployeeServiceConfig_PersistsChanges(t *testing.T) {
	db := orm.New(mem.New())
	m, err := ab.New(db, SetupDependencies())
	if err != nil {
		t.Fatalf("ab.New: %v", err)
	}

	cfg, err := m.CreateEmployeeServiceConfig(ab.EmployeeServiceConfig{
		TenantId:      "t1",
		StaffId:       "s1",
		ServiceId:     "srv1",
		DurationMin:   30,
		PriceOverride: 1000,
		IsActive:      true,
	})
	if err != nil {
		t.Fatalf("CreateEmployeeServiceConfig: %v", err)
	}

	cfg.DurationMin = 45
	cfg.PriceOverride = 1500
	cfg.IsActive = false

	if err := m.UpdateEmployeeServiceConfig(cfg); err != nil {
		t.Fatalf("UpdateEmployeeServiceConfig: %v", err)
	}

	read, err := m.GetEmployeeServiceConfig(cfg.Id)
	if err != nil {
		t.Fatalf("GetEmployeeServiceConfig: %v", err)
	}
	if read.DurationMin != 45 || read.PriceOverride != 1500 || read.IsActive {
		t.Fatalf("updated values did not persist: %+v", read)
	}
}

func TestOpsMountedWithPolicy(t *testing.T) {
	db := orm.New(mem.New())
	m, err := ab.New(db, SetupDependencies())
	if err != nil {
		t.Fatalf("ab.New: %v", err)
	}

	reg := &mock.Router{}
	m.MountOperations(reg)

	routes := reg.Routes()
	findRoute := func(opName string) *router.RouteInfo {
		path := "/" + opName
		for i := range routes {
			if routes[i].Method == "OP" && routes[i].Path == path {
				return &routes[i]
			}
		}
		return nil
	}

	expectedOps := []struct {
		name string
		res  model.Resource
		act  model.Action
	}{
		{name: ab.OpCreateEmployeeServiceConfig, res: "employee_service_config", act: model.Create},
		{name: ab.OpGetEmployeeServiceConfig, res: "employee_service_config", act: model.Read},
		{name: ab.OpListEmployeeServiceConfigsByStaff, res: "employee_service_config", act: model.Read},
		{name: ab.OpUpdateEmployeeServiceConfig, res: "employee_service_config", act: model.Update},
	}

	for _, expected := range expectedOps {
		route := findRoute(expected.name)
		if route == nil {
			t.Fatalf("op %s not mounted on router", expected.name)
		}
		if route.Resource != expected.res {
			t.Errorf("op %s resource mismatch: got %q, expected %q", expected.name, route.Resource, expected.res)
		}
		if route.Action != expected.act {
			t.Errorf("op %s action mismatch: got %d, expected %d", expected.name, route.Action, expected.act)
		}
	}
}

func TestNoDeleteOp(t *testing.T) {
	db := orm.New(mem.New())
	m, err := ab.New(db, SetupDependencies())
	if err != nil {
		t.Fatalf("ab.New: %v", err)
	}

	reg := &mock.Router{}
	m.MountOperations(reg)

	deletePath := "/delete_employee_service_config"
	for _, r := range reg.Routes() {
		if r.Path == deletePath {
			t.Fatalf("op delete_employee_service_config should NOT exist")
		}
	}
}

func TestEmployeeServiceConfigForm_HasWidgetsOnEveryEditableField(t *testing.T) {
	def := ab.EmployeeServiceConfigModel
	if def.Name != "employee_service_config" {
		t.Fatalf("unexpected model name: %s", def.Name)
	}

	for _, f := range def.Fields {
		if f.Name == "id" || f.Name == "tenant_id" {
			if _, ok := f.Type.(input.Input); ok {
				t.Errorf("field %s should NOT carry an input.Input widget", f.Name)
			}
		} else {
			if _, ok := f.Type.(input.Input); !ok {
				t.Errorf("field %s missing input.Input widget, got type %T", f.Name, f.Type)
			}
		}
	}
}
