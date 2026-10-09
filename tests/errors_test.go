package tests

import (
	"testing"
	ab "github.com/veltylabs/appointment_booking"
)

func TestErrorStrings(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"ErrCalendarConfigNotFound", ab.ErrCalendarConfigNotFound, "calendar config not found"},
		{"ErrSlotTaken", ab.ErrSlotTaken, "slot taken"},
		{"ErrMissingArgs", ab.ErrMissingArgs, "missing args"},
		{"ErrInvalidBlock", ab.ErrInvalidBlock, "appointment_booking: start_min must be < end_min and both within 0..1439"},
		{"ErrBlocksOverlap", ab.ErrBlocksOverlap, "appointment_booking: two blocks of the same day overlap"},
		{"ErrBlockOutsideBusinessHours", ab.ErrBlockOutsideBusinessHours, "appointment_booking: block falls outside the establishment's opening hours"},
		{"ErrBlockOnClosedDay", ab.ErrBlockOnClosedDay, "appointment_booking: the establishment is closed on that date"},
		{"ErrNoServiceConfig", ab.ErrNoServiceConfig, "appointment_booking: FormConfig.ServiceConfigId is required to book — the professional has no service configured"},
		{"ErrIncompleteSlot", ab.ErrIncompleteSlot, "appointment_booking: a booking needs both a day and an hour"},
		{"ErrNotFound", ab.ErrNotFound, "record not found"},
		{"ErrConflict", ab.ErrConflict, "optimistic concurrency conflict"},
		{"ErrInvalidTransition", ab.ErrInvalidTransition, "invalid transition"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.err.Error() != tc.want {
				t.Errorf("expected %q, got %q", tc.want, tc.err.Error())
			}
		})
	}
}
