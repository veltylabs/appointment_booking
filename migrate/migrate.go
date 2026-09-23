package migrate

import (
	"webtyp.com/ddl"
	"webtyp.com/model"

	appointmentbooking "github.com/veltylabs/appointment_booking"
)

// Migrate reconcilia el esquema de base de datos que posee appointment_booking:
// EmployeeServiceConfig, WorkCalendarConfig, WorkCalendarBlock,
// WorkCalendarException y Reservation.
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
	d := ddl.New(conn, ddlCompiler)
	tables := []model.Model{
		&appointmentbooking.EmployeeServiceConfig{},
		&appointmentbooking.WorkCalendarConfig{},
		&appointmentbooking.WorkCalendarBlock{},
		&appointmentbooking.WorkCalendarException{},
		&appointmentbooking.Reservation{},
	}
	return d.Sync(tables...)
}
