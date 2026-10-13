package tests

import (
	"testing"

	"webtyp.com/json"
	"webtyp.com/model"

	ab "github.com/veltylabs/appointment_booking"
)

func TestModels_NewRoundTrip(t *testing.T) {
	cases := []struct {
		name  string
		v     model.Encodable
		fresh model.Decodable
	}{
		{"list_bookable_services_args", &ab.ListBookableServicesArgs{TenantId: bTenant, Specialty: "x"}, &ab.ListBookableServicesArgs{}},
		{"bookable_service", &ab.BookableService{ConfigId: "c", StaffId: "s", StaffName: "n", Specialty: "e", ServiceId: "v", ServiceName: "m", DurationMin: 30, Price: 25000, Currency: "CLP", PaymentRequired: true}, &ab.BookableService{}},
		{"list_service_availability_args", &ab.ListServiceAvailabilityArgs{TenantId: bTenant, ServiceId: "v", StaffId: "s", From: 1, To: 2, IncludeTaken: true}, &ab.ListServiceAvailabilityArgs{}},
		{"bookable_slot", &ab.BookableSlot{StartUtc: 1, EndUtc: 2, StaffId: "s", ConfigId: "c", Taken: true}, &ab.BookableSlot{}},
		{"create_online_reservation_args", &ab.CreateOnlineReservationArgs{TenantId: bTenant, Id: "r", ClientId: "c", EmployeeServiceConfigId: "e", SlotStartUtc: 3, Notes: "n"}, &ab.CreateOnlineReservationArgs{}},
	}
	for _, c := range cases {
		var first, second []byte
		if err := json.Encode(c.v, &first); err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if err := json.Decode(string(first), c.fresh); err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if err := json.Encode(c.fresh.(model.Encodable), &second); err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if string(first) != string(second) {
			t.Errorf("%s:\n%s\n%s", c.name, first, second)
		}
	}
	if err := (&ab.ListServiceAvailabilityArgs{TenantId: bTenant}).Validate(model.ActionRead); err == nil {
		t.Error("service_id is required")
	}
}
