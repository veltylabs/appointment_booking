package appointment_booking

// ID is this module's identity: RBAC resource prefix on the server, nav
// route on the client. Shared by svg.go and browser.go.
const ID = "appointment_booking"

// NavLabel is the nav item's display text — the daily-use booking screen for
// reception, distinct from "Personal" (administration): Horario and
// Servicios (this module's other two surfaces) live inside Personal instead
// — see modules/personal.
//
// Named NavLabel, not the plain Label every other module uses, because this
// package's own files (bookingview.go, scheduleview.go, ...) dot-import
// webtyp.com/html for its element builders, and html.Label() (the <label>
// tag builder) already claims that name in this package's scope.
const NavLabel = "Reserva Hora"
