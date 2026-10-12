package tests

import (
	"testing"

	"webtyp.com/fmt"
	"webtyp.com/model"
	"webtyp.com/router/mock"
	tinytime "webtyp.com/time"
	ab "github.com/veltylabs/appointment_booking"
)

func TestCreateOverbooking_MissingId(t *testing.T) {
	deps := SetupDependencies()
	m, repo, _ := newTestModule(t, deps)
	slot := Date(2025, 2, 3, 10, 0, 0, 0)
	cfgID := setupReservable(t, m, repo, "t1", "s1", slot)

	_, err := m.CreateOverbooking(ab.CreateOverbookingCmd{
		Id:                      "",
		TenantId:                "t1",
		ClientId:                "c1",
		CreatorUserId:           "u1",
		EmployeeServiceConfigId: cfgID,
		SlotStartUtc:            slot,
		Origin:                  ab.OriginCounter,
	})
	if err == nil || err.Error() != ab.ErrMissingArgs.Error() {
		t.Fatalf("expected ErrMissingArgs, got %v", err)
	}
}

// A reservation made offline must carry the id its device minted: without one, a resend would
// create a second reservation instead of answering the first.
func TestCreateReservation_MissingId(t *testing.T) {
	deps := SetupDependencies()
	m, repo, _ := newTestModule(t, deps)
	slot := Date(2025, 2, 3, 10, 0, 0, 0)
	cfgID := setupReservable(t, m, repo, "t1", "s1", slot)

	_, err := m.CreateReservation(ab.CreateReservationCmd{
		Id:                      "",
		TenantId:                "t1",
		ClientId:                "c1",
		CreatorUserId:           "u1",
		EmployeeServiceConfigId: cfgID,
		SlotStartUtc:            slot,
		Origin:                  ab.OriginCounter,
	})
	if err == nil || err.Error() != ab.ErrMissingArgs.Error() {
		t.Fatalf("expected ErrMissingArgs, got %v", err)
	}
}

func TestCreateOverbooking_Idempotency(t *testing.T) {
	deps := SetupDependencies()
	m, repo, _ := newTestModule(t, deps)
	slot := Date(2025, 2, 3, 10, 0, 0, 0)
	cfgID := setupReservable(t, m, repo, "t1", "s1", slot)

	if err := m.UpsertCalendarConfig(ab.WorkCalendarConfig{
		TenantId:          "t1",
		StaffId:           "s1",
		Timezone:          "UTC",
		IsActive:          true,
		MaxOverbookPerDay: 1,
	}); err != nil {
		t.Fatalf("UpsertCalendarConfig: %v", err)
	}

	id := nextTestID()
	cmd := ab.CreateOverbookingCmd{
		Id:                      id,
		TenantId:                "t1",
		ClientId:                "c1",
		CreatorUserId:           "u1",
		EmployeeServiceConfigId: cfgID,
		SlotStartUtc:            slot,
		Origin:                  ab.OriginCounter,
	}

	res1, err := m.CreateOverbooking(cmd)
	if err != nil {
		t.Fatalf("CreateOverbooking 1: %v", err)
	}
	if !res1.Overbooked {
		t.Fatalf("expected Overbooked true, got false")
	}

	mockPub := deps.Publisher.(*MockEventPublisher)
	eventsCount := len(mockPub.PublishedEvents)

	res2, err := m.CreateOverbooking(cmd)
	if err != nil {
		t.Fatalf("CreateOverbooking 2 (idempotent): %v", err)
	}
	if res2.Id != res1.Id {
		t.Fatalf("expected same id, got %s vs %s", res1.Id, res2.Id)
	}

	if len(mockPub.PublishedEvents) != eventsCount {
		t.Fatalf("expected no new events on retry, had %d now %d", eventsCount, len(mockPub.PublishedEvents))
	}

	day := Date(2025, 2, 3, 0, 0, 0, 0)
	rows, err := m.ListReservationsByStaff("t1", "s1", day, day)
	if err != nil {
		t.Fatalf("ListReservationsByStaff: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row in DB, got %d", len(rows))
	}
}

func TestCreateOverbooking_SameIdDifferentClient(t *testing.T) {
	deps := SetupDependencies()
	m, repo, _ := newTestModule(t, deps)
	slot := Date(2025, 2, 3, 10, 0, 0, 0)
	cfgID := setupReservable(t, m, repo, "t1", "s1", slot)

	if err := m.UpsertCalendarConfig(ab.WorkCalendarConfig{
		TenantId:          "t1",
		StaffId:           "s1",
		Timezone:          "UTC",
		IsActive:          true,
		MaxOverbookPerDay: 2,
	}); err != nil {
		t.Fatalf("UpsertCalendarConfig: %v", err)
	}

	id := nextTestID()
	_, err := m.CreateOverbooking(ab.CreateOverbookingCmd{
		Id:                      id,
		TenantId:                "t1",
		ClientId:                "c1",
		CreatorUserId:           "u1",
		EmployeeServiceConfigId: cfgID,
		SlotStartUtc:            slot,
		Origin:                  ab.OriginCounter,
	})
	if err != nil {
		t.Fatalf("CreateOverbooking: %v", err)
	}

	_, err = m.CreateOverbooking(ab.CreateOverbookingCmd{
		Id:                      id,
		TenantId:                "t1",
		ClientId:                "c2",
		CreatorUserId:           "u1",
		EmployeeServiceConfigId: cfgID,
		SlotStartUtc:            slot,
		Origin:                  ab.OriginCounter,
	})
	if err == nil || err.Error() != ab.ErrIdTaken.Error() {
		t.Fatalf("expected ErrIdTaken, got %v", err)
	}

	orig, err := m.GetReservation("t1", id)
	if err != nil {
		t.Fatalf("GetReservation: %v", err)
	}
	if orig.ClientId != "c1" {
		t.Fatalf("expected ClientId c1, got %s", orig.ClientId)
	}
}

func TestCreateReservation_DifferentIdsSameSlot(t *testing.T) {
	deps := SetupDependencies()
	m, repo, _ := newTestModule(t, deps)
	slot := Date(2025, 2, 3, 10, 0, 0, 0)
	cfgID := setupReservable(t, m, repo, "t1", "s1", slot)

	_, err := m.CreateReservation(ab.CreateReservationCmd{
		Id:                      nextTestID(),
		TenantId:                "t1",
		ClientId:                "c1",
		CreatorUserId:           "u1",
		EmployeeServiceConfigId: cfgID,
		SlotStartUtc:            slot,
		Origin:                  ab.OriginCounter,
	})
	if err != nil {
		t.Fatalf("CreateReservation 1: %v", err)
	}

	_, err = m.CreateReservation(ab.CreateReservationCmd{
		Id:                      nextTestID(),
		TenantId:                "t1",
		ClientId:                "c2",
		CreatorUserId:           "u2",
		EmployeeServiceConfigId: cfgID,
		SlotStartUtc:            slot,
		Origin:                  ab.OriginCounter,
	})
	if err == nil || err.Error() != ab.ErrSlotTaken.Error() {
		t.Fatalf("expected ErrSlotTaken, got %v", err)
	}
}

func TestCreateOverbooking_MaxOverbookZero(t *testing.T) {
	deps := SetupDependencies()
	m, repo, _ := newTestModule(t, deps)
	slot := Date(2025, 2, 3, 10, 0, 0, 0)
	cfgID := setupReservable(t, m, repo, "t1", "s1", slot)

	if err := m.UpsertCalendarConfig(ab.WorkCalendarConfig{
		TenantId:          "t1",
		StaffId:           "s1",
		Timezone:          "UTC",
		IsActive:          true,
		MaxOverbookPerDay: 0,
	}); err != nil {
		t.Fatalf("UpsertCalendarConfig: %v", err)
	}

	_, err := m.CreateOverbooking(ab.CreateOverbookingCmd{
		Id:                      nextTestID(),
		TenantId:                "t1",
		ClientId:                "c1",
		CreatorUserId:           "u1",
		EmployeeServiceConfigId: cfgID,
		SlotStartUtc:            slot,
		Origin:                  ab.OriginCounter,
	})
	if err == nil || err.Error() != ab.ErrOverbookNotAllowed.Error() {
		t.Fatalf("expected ErrOverbookNotAllowed, got %v", err)
	}
}

func TestCreateOverbooking_MaxOverbookLimit(t *testing.T) {
	deps := SetupDependencies()
	m, repo, _ := newTestModule(t, deps)
	slotA := Date(2025, 2, 3, 10, 0, 0, 0)
	slotB := Date(2025, 2, 3, 11, 0, 0, 0)
	slotC := Date(2025, 2, 4, 10, 0, 0, 0)

	cfgID := setupReservable(t, m, repo, "t1", "s1", slotA)
	seedWeekdayBlock(t, m, "t1", "s1", int(tinytime.Weekday(slotC)), defaultBlocks())

	if err := m.UpsertCalendarConfig(ab.WorkCalendarConfig{
		TenantId:          "t1",
		StaffId:           "s1",
		Timezone:          "UTC",
		IsActive:          true,
		MaxOverbookPerDay: 1,
	}); err != nil {
		t.Fatalf("UpsertCalendarConfig: %v", err)
	}

	_, err := m.CreateReservation(ab.CreateReservationCmd{
		Id:                      nextTestID(),
		TenantId:                "t1",
		ClientId:                "c0",
		CreatorUserId:           "u1",
		EmployeeServiceConfigId: cfgID,
		SlotStartUtc:            slotA,
		Origin:                  ab.OriginCounter,
	})
	if err != nil {
		t.Fatalf("CreateReservation regular: %v", err)
	}

	resOver1, err := m.CreateOverbooking(ab.CreateOverbookingCmd{
		Id:                      nextTestID(),
		TenantId:                "t1",
		ClientId:                "c1",
		CreatorUserId:           "u1",
		EmployeeServiceConfigId: cfgID,
		SlotStartUtc:            slotA,
		Origin:                  ab.OriginCounter,
	})
	if err != nil {
		t.Fatalf("CreateOverbooking 1 on slotA: %v", err)
	}
	if !resOver1.Overbooked {
		t.Fatalf("expected Overbooked true")
	}

	_, err = m.CreateOverbooking(ab.CreateOverbookingCmd{
		Id:                      nextTestID(),
		TenantId:                "t1",
		ClientId:                "c2",
		CreatorUserId:           "u1",
		EmployeeServiceConfigId: cfgID,
		SlotStartUtc:            slotB,
		Origin:                  ab.OriginCounter,
	})
	if err == nil || err.Error() != ab.ErrOverbookNotAllowed.Error() {
		t.Fatalf("expected ErrOverbookNotAllowed, got %v", err)
	}

	resOver2, err := m.CreateOverbooking(ab.CreateOverbookingCmd{
		Id:                      nextTestID(),
		TenantId:                "t1",
		ClientId:                "c3",
		CreatorUserId:           "u1",
		EmployeeServiceConfigId: cfgID,
		SlotStartUtc:            slotC,
		Origin:                  ab.OriginCounter,
	})
	if err != nil {
		t.Fatalf("CreateOverbooking next day slotC: %v", err)
	}
	if !resOver2.Overbooked {
		t.Fatalf("expected Overbooked true for next day")
	}
}

func TestCreateOverbooking_SlotOutsideSchedule(t *testing.T) {
	deps := SetupDependencies()
	m, repo, _ := newTestModule(t, deps)
	slot := Date(2025, 2, 3, 10, 0, 0, 0)
	cfgID := setupReservable(t, m, repo, "t1", "s1", slot)

	if err := m.UpsertCalendarConfig(ab.WorkCalendarConfig{
		TenantId:          "t1",
		StaffId:           "s1",
		Timezone:          "UTC",
		IsActive:          true,
		MaxOverbookPerDay: 5,
	}); err != nil {
		t.Fatalf("UpsertCalendarConfig: %v", err)
	}

	outsideSlot := Date(2025, 2, 3, 22, 0, 0, 0)

	_, err := m.CreateOverbooking(ab.CreateOverbookingCmd{
		Id:                      nextTestID(),
		TenantId:                "t1",
		ClientId:                "c1",
		CreatorUserId:           "u1",
		EmployeeServiceConfigId: cfgID,
		SlotStartUtc:            outsideSlot,
		Origin:                  ab.OriginCounter,
	})
	if err == nil || err.Error() != ab.ErrSlotNotInSchedule.Error() {
		t.Fatalf("expected ErrSlotNotInSchedule, got %v", err)
	}
}

func TestCreateOverbooking_CancelledNotCounted(t *testing.T) {
	deps := SetupDependencies()
	m, repo, _ := newTestModule(t, deps)
	slotA := Date(2025, 2, 3, 10, 0, 0, 0)
	slotB := Date(2025, 2, 3, 11, 0, 0, 0)
	cfgID := setupReservable(t, m, repo, "t1", "s1", slotA)

	if err := m.UpsertCalendarConfig(ab.WorkCalendarConfig{
		TenantId:          "t1",
		StaffId:           "s1",
		Timezone:          "UTC",
		IsActive:          true,
		MaxOverbookPerDay: 1,
	}); err != nil {
		t.Fatalf("UpsertCalendarConfig: %v", err)
	}

	overId := nextTestID()
	res, err := m.CreateOverbooking(ab.CreateOverbookingCmd{
		Id:                      overId,
		TenantId:                "t1",
		ClientId:                "c1",
		CreatorUserId:           "u1",
		EmployeeServiceConfigId: cfgID,
		SlotStartUtc:            slotA,
		Origin:                  ab.OriginCounter,
	})
	if err != nil {
		t.Fatalf("CreateOverbooking: %v", err)
	}

	err = m.ChangeReservationStatus(ab.ChangeStatusCmd{
		TenantId: "t1",
		Id:       res.Id,
		Event:    ab.EventCancel,
		ActorId:  "u1",
		Revision: 0,
	})
	if err != nil {
		t.Fatalf("Cancel overbooking: %v", err)
	}

	_, err = m.CreateOverbooking(ab.CreateOverbookingCmd{
		Id:                      nextTestID(),
		TenantId:                "t1",
		ClientId:                "c2",
		CreatorUserId:           "u1",
		EmployeeServiceConfigId: cfgID,
		SlotStartUtc:            slotB,
		Origin:                  ab.OriginCounter,
	})
	if err != nil {
		t.Fatalf("CreateOverbooking after cancel: %v", err)
	}
}

func TestCreateOverbooking_RBAC(t *testing.T) {
	deps := SetupDependencies()
	m, repo, _ := newTestModule(t, deps)
	slot := Date(2025, 2, 3, 10, 0, 0, 0)
	cfgID := setupReservable(t, m, repo, "t1", "s1", slot)

	if err := m.UpsertCalendarConfig(ab.WorkCalendarConfig{
		TenantId:          "t1",
		StaffId:           "s1",
		Timezone:          "UTC",
		IsActive:          true,
		MaxOverbookPerDay: 1,
	}); err != nil {
		t.Fatalf("UpsertCalendarConfig: %v", err)
	}

	regDeny := &mock.Router{}
	regDeny.Configure(mock.Config{
		Authorize: func(userID string, r model.Resource, a model.Action) bool {
			if string(r) == "overbooking" {
				return false
			}
			return true
		},
	})
	m.MountOperations(regDeny)

	body := []byte(`{"id":"` + nextTestID() + `","tenant_id":"t1","client_id":"c1","creator_user_id":"u1",` +
		`"employee_service_config_id":"` + cfgID + `","slot_start_utc":` + fmt.Convert(slot).String() + `,"origin":"` + ab.OriginCounter + `"}`)

	ctxDeny := &mock.Context{InBody: body}
	ctxDeny.SetUserID("u1")
	regDeny.Invoke("OP", "/"+ab.OpCreateOverbooking, ctxDeny)
	if ctxDeny.Status != 403 {
		t.Fatalf("expected 403 when RBAC denied, got %d", ctxDeny.Status)
	}

	regAllow := &mock.Router{}
	regAllow.Configure(mock.Config{
		Authorize: func(userID string, r model.Resource, a model.Action) bool { return true },
	})
	m.MountOperations(regAllow)

	bodyAllow := []byte(`{"id":"` + nextTestID() + `","tenant_id":"t1","client_id":"c1","creator_user_id":"u1",` +
		`"employee_service_config_id":"` + cfgID + `","slot_start_utc":` + fmt.Convert(slot).String() + `,"origin":"` + ab.OriginCounter + `"}`)

	ctxAllow := &mock.Context{InBody: bodyAllow}
	ctxAllow.SetUserID("u1")
	regAllow.Invoke("OP", "/"+ab.OpCreateOverbooking, ctxAllow)
	if ctxAllow.Status != 0 && ctxAllow.Status != 200 {
		t.Fatalf("expected 200 when RBAC allowed, got %d, body=%s", ctxAllow.Status, ctxAllow.ResponseBody())
	}
}

func TestCreateOverbooking_AvailabilityRegression(t *testing.T) {
	deps := SetupDependencies()
	m, repo, _ := newTestModule(t, deps)
	slot := Date(2025, 2, 3, 10, 0, 0, 0)
	cfgID := setupReservable(t, m, repo, "t1", "s1", slot)

	if err := m.UpsertCalendarConfig(ab.WorkCalendarConfig{
		TenantId:          "t1",
		StaffId:           "s1",
		Timezone:          "UTC",
		IsActive:          true,
		MaxOverbookPerDay: 2,
	}); err != nil {
		t.Fatalf("UpsertCalendarConfig: %v", err)
	}

	dayStart := Date(2025, 2, 3, 0, 0, 0, 0)
	dayEnd := Date(2025, 2, 3, 23, 59, 59, 0)

	availBefore, err := m.ListAvailability("t1", "s1", cfgID, dayStart, dayEnd)
	if err != nil {
		t.Fatalf("ListAvailability before: %v", err)
	}
	foundSlot := false
	for _, sl := range availBefore {
		if sl.StartUtc == slot {
			foundSlot = true
			break
		}
	}
	if !foundSlot {
		t.Fatalf("expected slot 10:00 to be available initially")
	}

	_, err = m.CreateReservation(ab.CreateReservationCmd{
		Id:                      nextTestID(),
		TenantId:                "t1",
		ClientId:                "c1",
		CreatorUserId:           "u1",
		EmployeeServiceConfigId: cfgID,
		SlotStartUtc:            slot,
		Origin:                  ab.OriginCounter,
	})
	if err != nil {
		t.Fatalf("CreateReservation: %v", err)
	}

	_, err = m.CreateOverbooking(ab.CreateOverbookingCmd{
		Id:                      nextTestID(),
		TenantId:                "t1",
		ClientId:                "c2",
		CreatorUserId:           "u1",
		EmployeeServiceConfigId: cfgID,
		SlotStartUtc:            slot,
		Origin:                  ab.OriginCounter,
	})
	if err != nil {
		t.Fatalf("CreateOverbooking: %v", err)
	}

	availAfter, err := m.ListAvailability("t1", "s1", cfgID, dayStart, dayEnd)
	if err != nil {
		t.Fatalf("ListAvailability after: %v", err)
	}
	for _, sl := range availAfter {
		if sl.StartUtc == slot {
			t.Fatalf("expected slot 10:00 to be unavailable, but found in available slots")
		}
	}
}
