//go:build browser

package module

import (
	"webtyp.com/appkit"
	"webtyp.com/view"

	"github.com/veltylabs/appointment_booking/ui"
)

var server appkit.ModuleServer

var browser = appkit.ModuleBrowser{
	Screens: func(d appkit.BrowserDeps) []view.Screen {
		return ui.Screens(d.Caller, d.Zone, nil, "")
	},
}
