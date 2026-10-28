package tests

import (
	"testing"

	"github.com/veltylabs/appointment_booking/ui"
	"webtyp.com/router/mock"
)

func TestPersonalGroup(t *testing.T) {
	caller := &mock.Caller{}
	ids := &fakeIDs{}
	tenantID := "tenant-test"

	screens := ui.Screens(caller, ids, tenantID)
	if len(screens) != 3 {
		t.Fatalf("Expected 3 screens, got %d", len(screens))
	}
	for _, s := range screens {
		if err := s.Validate(); err != nil {
			t.Fatalf("screen %s validation error: %v", s.ID, err)
		}
	}

	personalGroup := screens[1]
	if personalGroup.ID != ui.PersonalID {
		t.Fatalf("expected personal group, got %s", personalGroup.ID)
	}
	if len(personalGroup.Children) != 3 {
		t.Fatalf("Expected 3 children, got %d", len(personalGroup.Children))
	}
	expectedIDs := []string{
		"personal.staff",
		"personal.schedule",
		"personal.services",
	}
	for i, child := range personalGroup.Children {
		if child.ID != expectedIDs[i] {
			t.Errorf("Child %d expected ID %s, got %s", i, expectedIDs[i], child.ID)
		}
	}
}
