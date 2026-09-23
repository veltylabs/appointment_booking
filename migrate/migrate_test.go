package migrate_test

import (
	"testing"

	"webtyp.com/ddl"
	"webtyp.com/model"

	appointmentbooking "github.com/veltylabs/appointment_booking"
	"github.com/veltylabs/appointment_booking/migrate"
)

type dummyExecer struct{ calls []string }

func (d *dummyExecer) Exec(query string, args ...any) error {
	d.calls = append(d.calls, query)
	return nil
}

type dummyCompiler struct{}

func (d *dummyCompiler) CompileDDL(stmt ddl.Stmt, m model.Model) (string, []any, error) {
	return stmt.Table, nil, nil
}

func TestMigrate_CreatesFiveTables(t *testing.T) {
	execer := &dummyExecer{}
	compiler := &dummyCompiler{}

	err := migrate.Migrate(execer, compiler)
	if err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}

	var seen []string
	for _, call := range execer.calls {
		if len(seen) == 0 || seen[len(seen)-1] != call {
			seen = append(seen, call)
		}
	}

	if len(seen) != 5 {
		t.Fatalf("expected 5 table migrations, got %d (%v)", len(seen), seen)
	}
}

func TestMigrate_TableOrder(t *testing.T) {
	execer := &dummyExecer{}
	compiler := &dummyCompiler{}

	err := migrate.Migrate(execer, compiler)
	if err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}

	expected := []string{
		appointmentbooking.EmployeeServiceConfigModel.Name,
		appointmentbooking.WorkCalendarConfigModel.Name,
		appointmentbooking.WorkCalendarBlockModel.Name,
		appointmentbooking.WorkCalendarExceptionModel.Name,
		appointmentbooking.ReservationModel.Name,
	}

	var seen []string
	for _, call := range execer.calls {
		if len(seen) == 0 || seen[len(seen)-1] != call {
			seen = append(seen, call)
		}
	}

	if len(seen) != len(expected) {
		t.Fatalf("expected %d table migrations, got %d", len(expected), len(seen))
	}

	for i, want := range expected {
		if seen[i] != want {
			t.Errorf("table at index %d: expected %q, got %q", i, want, seen[i])
		}
	}
}
