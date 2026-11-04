package module

import "webtyp.com/appkit"

// Module conecta appointment_booking en App.Modules de una aplicación.
// Ejemplo: Modules: []appkit.Module{appointmentbooking.Module}
var Module = appkit.Module{Name: "appointment_booking", Server: server, Browser: browser}

