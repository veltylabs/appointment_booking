//go:build wasm

package tests

import "testing"

func TestWASM_Step6Counter_ConfirmError(t *testing.T)             {}
func TestWASM_Step6Counter_ConfirmDone(t *testing.T)              {}
func TestWASM_Step6Counter_PaidThenConfirmed(t *testing.T)        {}
func TestWASM_Step6Counter_PaidWaitsForConfirmation(t *testing.T) {}
func TestWASM_Step6Online_ShowsWebpayStep(t *testing.T)           {}
func TestWASM_Final_NewBookingRestarts(t *testing.T)              {}
