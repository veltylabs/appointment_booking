package ui

// ID es la identidad de este módulo: prefijo de recurso RBAC en el servidor,
// ruta de navegación en el cliente. Compartido por svg.go y browser.go.
const ID = "appointment_booking"

// NavLabel es el texto visible en el menú de navegación para la pantalla
// diaria de reservas para recepción, distinta de "Personal" (administración):
// Horario y Servicios viven dentro de Personal.
//
// Se llama NavLabel (no Label) porque los archivos de este paquete
// dot-importan webtyp.com/html para sus elementos, y html.Label() ya usa
// ese nombre en el alcance de este paquete.
const NavLabel = "Reserva Hora"

// PersonalID es la identidad de la pantalla compuesta Personal.
const PersonalID = "personal"

// PersonalLabel es el texto visible de la pantalla Personal.
const PersonalLabel = "Personal"
