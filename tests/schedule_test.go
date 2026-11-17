package tests

import (
	"testing"

	ab "github.com/veltylabs/appointment_booking"
	"webtyp.com/crud"
	"webtyp.com/model"
)

func (e *env) status(id string) string { e.t.Helper(); return e.reservation(id).Status }

func TestBlockDelete_ConflictsTheReservation(t *testing.T) {
	e := newEnv(t)
	e.schedule(0)
	if _, err := e.book("r1", 540); err != nil {
		t.Fatal(err)
	}
	e.must(e.do(op("work_calendar_block", model.Delete), &crud.DeleteArgs{IDs: []string{"b1"}}, nil))
	if got := e.status("r1"); got != ab.StatusConflicted {
		t.Fatalf("status = %s, want CONFLICTED", got)
	}
}

func TestBlockCreate_ReleasesTheConflict(t *testing.T) {
	e := newEnv(t)
	e.schedule(0)
	if _, err := e.book("r1", 540); err != nil {
		t.Fatal(err)
	}
	e.must(e.do(op("work_calendar_block", model.Delete), &crud.DeleteArgs{IDs: []string{"b1"}}, nil))
	e.must(e.create(&ab.WorkCalendarBlock{Id: "b2", StaffId: "s1", DayOfWeek: 1, StartMin: 480, EndMin: 1080}, &ab.WorkCalendarBlock{}))
	if got := e.status("r1"); got != ab.StatusPending {
		t.Fatalf("status = %s, want it restored to PENDING", got)
	}
}

func TestExceptionCreate_ConflictsAndDeleteReleases(t *testing.T) {
	e := newEnv(t)
	e.schedule(0)
	if _, err := e.book("r1", 540); err != nil {
		t.Fatal(err)
	}
	ex := &ab.WorkCalendarException{Id: "x1", StaffId: "s1", SpecificDate: e.monday, ExceptionType: ab.ExcHoliday}
	e.must(e.create(ex, &ab.WorkCalendarException{}))
	if got := e.status("r1"); got != ab.StatusConflicted {
		t.Fatalf("after the exception: %s, want CONFLICTED", got)
	}
	e.must(e.do(op("work_calendar_exception", model.Delete), &crud.DeleteArgs{IDs: []string{"x1"}}, nil))
	if got := e.status("r1"); got != ab.StatusPending {
		t.Fatalf("after removing it: %s, want PENDING", got)
	}
}

func TestConflictRecompute_CountsChanges(t *testing.T) {
	e := newEnv(t)
	e.schedule(0)
	if _, err := e.book("r1", 540); err != nil {
		t.Fatal(err)
	}
	// The block disappears behind the rules' back (a direct write), so only the explicit order notices.
	if err := e.mod.StoreWorkCalendarBlock().Delete(&ab.WorkCalendarBlock{}, "b1"); err != nil {
		t.Fatal(err)
	}
	var order ab.ConflictRecompute
	e.must(e.create(&ab.ConflictRecompute{Id: "c1"}, &order))
	if order.Recomputed != 1 {
		t.Fatalf("recomputed = %d, want 1", order.Recomputed)
	}
	if got := e.status("r1"); got != ab.StatusConflicted {
		t.Fatalf("status = %s", got)
	}
}
