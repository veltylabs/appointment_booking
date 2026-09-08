package tests

import (
	ab "github.com/veltylabs/appointment_booking"
	"testing"
)

func TestFSM(t *testing.T) {
	// Valid transitions
	validTests := []struct {
		current  string
		event    string
		expected string
	}{
		{ab.StatusPending, ab.EventConfirm, ab.StatusConfirmed},
		{ab.StatusPending, ab.EventCancel, ab.StatusCancelled},
		{ab.StatusPending, ab.EventExpire, ab.StatusExpired},
		{ab.StatusPending, ab.EventReschedule, ab.StatusRescheduled},

		{ab.StatusConfirmed, ab.EventCancel, ab.StatusCancelled},
		{ab.StatusConfirmed, ab.EventComplete, ab.StatusCompleted},
		{ab.StatusConfirmed, ab.EventNoShow, ab.StatusNoShow},
		{ab.StatusConfirmed, ab.EventReschedule, ab.StatusRescheduled},

		// CONFLICTED se entra desde PENDING o CONFIRMED y no es terminal.
		{ab.StatusPending, ab.EventConflict, ab.StatusConflicted},
		{ab.StatusConfirmed, ab.EventConflict, ab.StatusConflicted},
	}

	for _, tc := range validTests {
		t.Run(tc.current+"_"+tc.event, func(t *testing.T) {
			next, err := ab.Transition(tc.current, tc.event)
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			if next != tc.expected {
				t.Fatalf("expected %s, got %s", tc.expected, next)
			}
		})
	}

	// Invalid transitions
	invalidTests := []struct {
		current string
		event   string
	}{
		{ab.StatusPending, ab.EventComplete},
		{ab.StatusPending, ab.EventNoShow},
		{ab.StatusConfirmed, ab.EventConfirm},
		{ab.StatusConfirmed, ab.EventExpire},
	}

	for _, tc := range invalidTests {
		t.Run("invalid_"+tc.current+"_"+tc.event, func(t *testing.T) {
			_, err := ab.Transition(tc.current, tc.event)
			if err != ab.ErrInvalidTransition {
				t.Fatalf("expected ab.ErrInvalidTransition, got %v", err)
			}
		})
	}

	// Terminal states
	terminalStates := []string{
		ab.StatusCancelled,
		ab.StatusCompleted,
		ab.StatusNoShow,
		ab.StatusExpired,
		ab.StatusRescheduled,
	}

	for _, state := range terminalStates {
		t.Run("terminal_"+state, func(t *testing.T) {
			if !ab.IsTerminal(state) {
				t.Fatalf("expected %s to be terminal", state)
			}

			// Try to apply any event to a terminal state
			_, err := ab.Transition(state, ab.EventConfirm)
			if err != ab.ErrInvalidTransition {
				t.Fatalf("expected ab.ErrInvalidTransition for terminal state %s, got %v", state, err)
			}
		})
	}

	// Non-terminal states
	nonTerminalStates := []string{
		ab.StatusPending,
		ab.StatusConfirmed,
		ab.StatusConflicted,
	}

	for _, state := range nonTerminalStates {
		t.Run("non_terminal_"+state, func(t *testing.T) {
			if ab.IsTerminal(state) {
				t.Fatalf("expected %s to be non-terminal", state)
			}
		})
	}

	// Invalid state entirely
	t.Run("invalid_state", func(t *testing.T) {
		_, err := ab.Transition("UNKNOWN_STATE", ab.EventConfirm)
		if err != ab.ErrInvalidTransition {
			t.Fatalf("expected ab.ErrInvalidTransition for unknown state, got %v", err)
		}
	})
}
