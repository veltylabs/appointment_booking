package tests

import (
	"testing"

	"github.com/veltylabs/appointment_booking/ui"
	"webtyp.com/layout/platformd"
	"webtyp.com/router/mock"
)

func TestPersonalGroup(t *testing.T) {
	caller := &mock.Caller{}
	ids := &fakeIDs{}
	tenantID := "tenant-test"

	mod, err := ui.PersonalBrowser(caller, ids, tenantID)
	if err != nil {
		t.Fatalf("PersonalBrowser() error = %v", err)
	}

	group, ok := mod.(platformd.GroupModule)
	if !ok {
		t.Fatalf("Expected mod to be a platformd.GroupModule, got %T", mod)
	}

	children := group.Children()
	if len(children) != 3 {
		t.Fatalf("Expected 3 children, got %d", len(children))
	}

	expectedIDs := []string{
		"personal.staff",
		"personal.schedule",
		"personal.services",
	}

	for i, child := range children {
		if child.ModelName() != expectedIDs[i] {
			t.Errorf("Child %d expected ID %s, got %s", i, expectedIDs[i], child.ModelName())
		}
	}
}
