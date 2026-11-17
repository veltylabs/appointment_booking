package migrate_test

import (
	"testing"

	"webtyp.com/ddl"

	appointmentbooking "github.com/veltylabs/appointment_booking"
	"github.com/veltylabs/appointment_booking/migrate"
)

// Migrate crea todas las tablas del módulo, en el orden en que unas dependen de otras (una que falte aparece
// recién en producción: «relation … does not exist»).
func TestMigrate_CreatesEveryTableInOrder(t *testing.T) {
	tables, err := ddl.Capture([]ddl.Step{{Name: appointmentbooking.ModelName, Run: migrate.Migrate}})
	if err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}
	want := []string{
		appointmentbooking.EmployeeServiceConfigModel.Name,
		appointmentbooking.WorkCalendarConfigModel.Name,
		appointmentbooking.WorkCalendarBlockModel.Name,
		appointmentbooking.WorkCalendarExceptionModel.Name,
		appointmentbooking.ReservationModel.Name,
		appointmentbooking.ReservationOverbookingModel.Name,
		appointmentbooking.OnlineReservationModel.Name,
		appointmentbooking.ReservationExpiryModel.Name,
		appointmentbooking.ConflictRecomputeModel.Name,
	}
	if len(tables) != len(want) {
		t.Fatalf("tables = %d, want %d", len(tables), len(want))
	}
	for i, tb := range tables {
		if got := tb.Model.ModelName(); got != want[i] {
			t.Errorf("table %d = %q, want %q", i, got, want[i])
		}
	}
}

// Sin compilador de SQL no hay cómo crear las tablas: Migrate lo dice, no termina en silencio.
func TestMigrate_NoCompilerIsAnError(t *testing.T) {
	if err := migrate.Migrate(nil, nil); err == nil {
		t.Fatal("Migrate with no compiler must return an error")
	}
}
