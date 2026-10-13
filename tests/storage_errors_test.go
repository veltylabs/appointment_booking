package tests

import (
	"testing"

	"webtyp.com/storage"

	ab "github.com/veltylabs/appointment_booking"
)

// Un error de base en cualquier método nuevo se devuelve tal cual: nunca se
// confunde con "no hay" ni se traga.
func TestStorageErrors_NewMethods(t *testing.T) {
	cases := []struct {
		name string
		run  func(t *testing.T, e *bookEnv) error
	}{
		{"CreateOnlineReservation: reservas del paciente", func(t *testing.T, e *bookEnv) error {
			cfg := e.config(t, "s1", 0, false)
			e.conn.failOn(tblRes, storage.ActionReadAll, 0)
			_, err := e.m.CreateOnlineReservation(ab.CreateReservationCmd{Id: "r1", TenantId: bTenant, ClientId: "c1", EmployeeServiceConfigId: cfg, SlotStartUtc: bookDay + 9*3600})
			return err
		}},
		{"ListServiceAvailability: agenda del profesional", func(t *testing.T, e *bookEnv) error {
			e.config(t, "s1", 0, false)
			e.conn.failOn(ab.WorkCalendarConfigModel.Name, storage.ActionReadOne, 0)
			_, err := e.m.ListServiceAvailability(availArgs("", false))
			return err
		}},
		{"ListServiceAvailability: horas tomadas", func(t *testing.T, e *bookEnv) error {
			e.config(t, "s1", 0, false)
			e.conn.failOn(tblRes, storage.ActionReadAll, 1) // la segunda lectura de reservas
			_, err := e.m.ListServiceAvailability(availArgs("s1", true))
			return err
		}},
		{"ConfirmPaidReservation: relectura en la transacción", func(t *testing.T, e *bookEnv) error {
			e.reserve(t, "r1", e.config(t, "s1", 0, false), "c1", 9, ab.OriginCounter)
			e.conn.failOn(tblRes, storage.ActionReadOne, 1)
			return e.m.ConfirmPaidReservation(bTenant, "r1", "p1")
		}},
		{"ExpirePendingReservations: lista", func(t *testing.T, e *bookEnv) error {
			e.conn.failOn(tblRes, storage.ActionReadAll, 0)
			_, err := e.m.ExpirePendingReservations(bTenant, bookNow())
			return err
		}},
		{"ExpirePendingReservations: cambio de estado", func(t *testing.T, e *bookEnv) error {
			e.reserve(t, "r1", e.config(t, "s1", 0, false), "c1", 9, ab.OriginCounter)
			e.conn.failOn(tblRes, storage.ActionUpdate, 0)
			_, err := e.m.ExpirePendingReservations(bTenant, bookDay+10*3600)
			return err
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := c.run(t, newBookEnv(t)); err != errStorage {
				t.Fatalf("err = %v, want %v", err, errStorage)
			}
		})
	}
}
