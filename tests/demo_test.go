//go:build wasm

package tests

import (
	"testing"

	"github.com/veltylabs/appointment_booking/seed"
	"webtyp.com/view"
)

func TestDemo_Valid(t *testing.T) {
	demo, err := seed.Demo()
	if err != nil {
		t.Fatal(err)
	}
	if err := demo.Validate(); err != nil {
		t.Fatal(err)
	}
	if len(demo.Screens(view.BrowserDeps{})) == 0 {
		t.Fatal("the demo shows no screen")
	}
}
