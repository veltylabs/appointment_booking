//go:build wasm

package tests

import "testing"

func TestWASM_Step5Counter_Found(t *testing.T)                             {}
func TestWASM_Step5Counter_NotFoundOffersRegister(t *testing.T)            {}
func TestWASM_Step5Counter_LookupError(t *testing.T)                       {}
func TestWASM_Step5Counter_RegisterErrorKeepsModal(t *testing.T)           {}
func TestWASM_Step5Counter_RegisterSelects(t *testing.T)                   {}
func TestWASM_Step5Counter_OverbookingCallsCreateOverbooking(t *testing.T) {}
func TestWASM_Step5Counter_CallsCreateReservation(t *testing.T)            {}
func TestWASM_Step5Counter_TakenReopensStep3(t *testing.T)                 {}
func TestWASM_Step5Counter_CreateErrorVisible(t *testing.T)                {}
func TestWASM_Step5Counter_Advances(t *testing.T)                          {}

func TestWASM_Step5Online_InvalidFields(t *testing.T)     {}
func TestWASM_Step5Online_LinkError(t *testing.T)         {}
func TestWASM_Step5Online_TakenReopensStep3(t *testing.T) {}
func TestWASM_Step5Online_TooManyHolds(t *testing.T)      {}
func TestWASM_Step5Online_CreateError(t *testing.T)       {}
func TestWASM_Step5Online_Advances(t *testing.T)          {}
