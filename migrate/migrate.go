package migrate

import (
	"webtyp.com/ddl"
	"webtyp.com/fmt"
	"webtyp.com/model"

	appointmentbooking "github.com/veltylabs/appointment_booking"
)

// errNoCompiler es un dato y no texto a traducir: por eso es variable (langc traduce las constantes de fmt.Err).
var errNoCompiler = "appointment_booking/migrate: the connection does not compile SQL (ddl.Compiler); pass one"

// Migrate reconcilia el esquema de base de datos que posee appointment_booking:
// EmployeeServiceConfig, WorkCalendarConfig, WorkCalendarBlock,
// WorkCalendarException, Reservation y las cuatro tablas de órdenes (sobrecupo, reserva online,
// vencimiento y recálculo de conflictos).
//
// Se ejecuta deliberadamente como un paso de despliegue (deploy-time) y NO es
// llamado por NewRepository ni por ningún constructor. Vive en su propio
// subpaquete para evitar que webtyp.com/ddl ingrese al grafo de compilación WASM
// de aplicaciones consumidoras que importan el paquete raíz (por ejemplo para
// NewView o NewScheduleClient).
//
// conn es un ddl.Execer, no un *orm.DB, por lo que un transporte de despliegue
// satisface la interfaz. RawConn() de *orm.DB también la satisface para pruebas:
//
//	conn, _ := postgres.Open(dsn)
//	compiler, _ := conn.(ddl.Compiler)
//	err := migrate.Migrate(conn, compiler)
func Migrate(conn ddl.Execer, ddlCompiler ddl.Compiler) error {
	if ddlCompiler == nil {
		if compiler, ok := conn.(ddl.Compiler); ok {
			ddlCompiler = compiler
		}
	}
	if ddlCompiler == nil {
		return fmt.Err(errNoCompiler)
	}
	d := ddl.New(conn, ddlCompiler)
	tables := []model.Model{
		&appointmentbooking.EmployeeServiceConfig{},
		&appointmentbooking.WorkCalendarConfig{},
		&appointmentbooking.WorkCalendarBlock{},
		&appointmentbooking.WorkCalendarException{},
		&appointmentbooking.Reservation{},
		&appointmentbooking.ReservationOverbooking{},
		&appointmentbooking.OnlineReservation{},
		&appointmentbooking.ReservationExpiry{},
		&appointmentbooking.ConflictRecompute{},
	}
	return d.Sync(tables...)
}
