package tests

import (
	"sync"
	"testing"
	gotime "time"

	ab "github.com/veltylabs/appointment_booking"
	"webtyp.com/crud"
	"webtyp.com/events/inprocess"
	"webtyp.com/model"
)

// staffSeen collects the professionals WatchStaffAgenda reports, from another goroutine.
type staffSeen struct {
	mu  sync.Mutex
	ids []string
}

func (s *staffSeen) add(id string) { s.mu.Lock(); s.ids = append(s.ids, id); s.mu.Unlock() }

// waitFor waits up to one second until n reports arrived and returns them.
func (s *staffSeen) waitFor(n int) []string {
	for i := 0; i < 100; i++ {
		s.mu.Lock()
		got := append([]string{}, s.ids...)
		s.mu.Unlock()
		if len(got) >= n {
			return got
		}
		gotime.Sleep(10 * gotime.Millisecond)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string{}, s.ids...)
}

// La agenda del doctor se refresca sola: crear una reserva y cambiarle el estado por crud avisan al profesional, aunque
// la actualización solo traiga el estado (el profesional se lee de la reserva guardada).
func TestWatchStaffAgenda_ReportsCreateAndStatusChange(t *testing.T) {
	broker := &inprocess.Broker{}
	e := newEnv(t, func(d *ab.Deps, _ *env) { d.Publisher, d.Subscriber = broker, broker })
	e.caller = crud.LoopbackActingAsWithQueue(broker, "u1", e.queue, e.mod)
	e.schedule(0)
	seen := &staffSeen{}
	e.mod.WatchStaffAgenda(seen.add)

	if _, err := e.book("r1", 540); err != nil {
		t.Fatal(err)
	}
	if got := seen.waitFor(1); len(got) == 0 || got[0] != "s1" {
		t.Fatalf("after create: %v, want s1", got)
	}
	before := len(seen.waitFor(1))
	rec := &ab.Reservation{Id: "r1", Status: ab.StatusConfirmed}
	if err := e.do(op("reservation", model.Update), &crud.UpdateArgs{IDs: []string{"r1"}, Fields: []string{ab.Reservation_.Status}, Record: rec}, nil); err != nil {
		t.Fatal(err)
	}
	got := seen.waitFor(before + 1)
	if len(got) <= before || got[len(got)-1] != "s1" {
		t.Fatalf("after status change: %v, want one more s1", got)
	}
}

// ActiveByStaff trae las reservas de la agenda con su estado y deja fuera las canceladas.
func TestActiveByStaff_StatusesWithoutCancelled(t *testing.T) {
	e := newEnv(t)
	e.schedule(0)
	for _, id := range []string{"r1", "r2"} {
		if _, err := e.book(id, map[string]int{"r1": 540, "r2": 570}[id]); err != nil {
			t.Fatal(err)
		}
	}
	cancel := &ab.Reservation{Id: "r2", Status: ab.StatusCancelled}
	if err := e.do(op("reservation", model.Update), &crud.UpdateArgs{IDs: []string{"r2"}, Fields: []string{ab.Reservation_.Status}, Record: cancel}, nil); err != nil {
		t.Fatal(err)
	}
	ids, _, _, _, statuses, err := e.mod.ActiveByStaff("t1", "s1", e.monday, e.monday+86400)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 1 || ids[0] != "r1" || statuses[0] != ab.StatusPending {
		t.Fatalf("ids=%v statuses=%v, want [r1] [PENDING]", ids, statuses)
	}
}
