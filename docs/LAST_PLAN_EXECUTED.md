---
PLAN: "feat!: blocks replace the single window, exceptions can open a day, schedule changes report conflicts"
EXECUTOR: jules
REVIEWER: none
---

> This plan is dispatched via the CodeJob workflow. See skill: agents-workflow.
>
> **Depends on a GATE**: `webtyp.com/time` must publish `time.DayBounds` first
> ([time/docs/PLAN.md](https://github.com/webtyp/time/blob/main/docs/PLAN.md)).
>
> **This module stays independent of `business_calendar`.** It declares its own
> port and imports no sibling — see §6. `appointment_booking/go.mod` has zero
> veltylabs dependencies today and **must still have zero when this lands**.
> Orchestrator:
> [webtyp/docs/AGENDA_DOMAIN_MASTER_PLAN.md](https://github.com/webtyp/webtyp/blob/main/docs/AGENDA_DOMAIN_MASTER_PLAN.md).
>
> Read `AGENTS.md` in this repo root **before writing any code**.

# Plan — bloques, días abribles, y conflictos que se reportan

## 1. The three defects this fixes

### 1.1 An exception cannot OPEN a day — verified

`ListAvailability` (`service.go:295-302`) walks days like this:

```go
weekly, hasWeekly := weeklyForDay(activeWeeklies, dow)
if !hasWeekly {
    continue // skips the day
}
// … exceptions are read only AFTER this point
```

The day is skipped **before** exceptions are consulted. Consequence: a
`SPECIAL_HOURS` exception on a date whose weekly template is inactive is **dead
data** — it never produces slots. Today an exception can only *narrow* or
*block*, never **open**.

This is what makes an irregular professional impossible to express. The owner's
model — *"mark the days I will work over the next 6–12 months"* — needs exactly
this: a dated entry that opens a day the weekly template does not cover.

### 1.2 One window and one break per day

`work_calendar_weekly` is one row per `(staff, day_of_week)` with
`work_start`/`work_finish` plus `break_start`/`break_finish`. "Morning 09:00–13:00
and afternoon 15:00–19:00" cannot be expressed.

**A break is the gap between two blocks.** Once a day holds several blocks, the
break fields are redundant and are deleted — this change removes two columns and
two UI controls rather than adding.

### 1.3 A schedule change silently orphans reservations

`UpsertWeeklyCalendar` (`service.go:164-181`) writes, publishes
`appointment.schedule.changed` with `ScheduleChangedPayload{TenantId, StaffId}`,
and returns. **Nothing computes which existing reservations now fall outside the
schedule.** The reschedule machinery exists (`StatusRescheduled`,
`RescheduledFromId`, `EventReservationRescheduled`) but only a manual reschedule
triggers it. Patients keep appointments the professional no longer works.

The payload is also too thin: a consumer receiving `{TenantId, StaffId}` cannot
know what to recompute.

## 2. Decisions this plan implements

From the master plan §1. Note two things it deliberately does **not** do:

- **No `effective_from`/`effective_to`.** Validity was considered and **cut**:
  planning a future change is served by marking the concrete days, and the real
  risk of editing a template (invalidating future reservations) is covered by
  the conflict detection in §6. Losing template history is accepted.
- **No RRULE.** "Last business day of the month" and similar are served by
  marking concrete days over a horizon, not by a recurrence grammar.

## 3. Design gate

Breaking change to public API. Per skill **api-design**:

### 3.1 Prior art

- **Cal.com** — `Availability` rows carry either `days: [1,3,5]` **or** a
  concrete `date`, both with `startTime`/`endTime`. Several rows per day are
  normal; there is no break field, because a break is the gap. This plan's shape
  is the same idea.
- **Calendly** — "Weekly hours" + "Date overrides". An override **replaces** the
  day, and can open a day the weekly rule leaves closed. That is precisely the
  §1.1 fix.
- **Epic Cadence** — provider templates hold named blocks; department operating
  hours bound them. That bounding relationship is what `BoundsReader` (§6.1)
  expresses — and, as in Epic, the department calendar is a separate system the
  scheduler merely reads.

None of the three has a `break_start`/`break_finish` pair. That field only
exists when a day is modelled as a single window, which is the shape being left
behind.

### 3.2 The novice-name test

- `WorkCalendarBlock` — *"a block of time this professional works."*
- `SaveDayBlocks(staffId, dayOfWeek, blocks)` — *"these are the blocks for that
  weekday"* — a whole-day replace, so partial edits cannot desynchronise.
- `MarkWorkingDays(staffId, dates, block)` — *"mark these dates as worked, with
  this block."*
- `ListConflictingReservations(staffId, from)` — *"which booked appointments no
  longer fit?"*

### 3.3 Complexity ledger

| Row | Δ |
|---|---|
| Concepts the developer must learn | **−1** — "break" stops being a concept; a block gap is one |
| Fields on the weekly record | **−4** (`work_start`, `work_finish`, `break_start`, `break_finish`) **+2** (`start_min`, `end_min`) |
| Ways to express a lunch break | **1** (was 1, still 1 — the gap replaces the field) |
| Ways to express two sessions a day | **0 → 1** |
| Ways to express an irregular professional | **0 → 1** |
| Ways to do the same thing | **−1** — `work_calendar_weekly` is deleted, not kept beside blocks |
| Exported surface | **+1 record, +4 ops, −1 record** |

### 3.4 Where it belongs

The professional's agenda is this module's concern; the establishment's calendar
is the institutional calendar's. This plan declares the **behavioural port here**
(`BoundsReader`, §6.1) and lets the value cross as `time.DayBounds`, a neutral
type in a leaf both sides already import. The generic module therefore imports
no specific sibling, and `appointment_booking/go.mod` keeps **zero veltylabs
dependencies**.

### 3.5 What it deletes

`WorkCalendarWeekly` and its table, `break_start`/`break_finish`,
`OpUpsertWeeklyCalendar`, `OpListWeeklyCalendar`, and the `weeklyForDay` gate in
`ListAvailability`. Nothing is deprecated; the single consumer (`webtyp/app-demo`)
migrates in its own plan.

## 4. Stage 1 — `WorkCalendarBlock` replaces `WorkCalendarWeekly`

**File: `model.go`.** Delete `WorkCalendarWeeklyModel`. Add:

```go
// WorkCalendarBlockModel: one row per block of working time. A day may hold
// several — "morning 09:00–13:00, afternoon 15:00–19:00" is two rows, and the
// lunch break is the GAP between them. There is deliberately no break field:
// a break that is a column can only ever describe one interruption, and the
// gap describes any number.
//
// specific_date == 0 → the block is WEEKLY and applies to day_of_week.
// specific_date  > 0 → the block is DATED and applies to that date only,
//                      and day_of_week carries no meaning.
//
// A dated block OPENS its day whether or not a weekly block covers that
// weekday. That is what lets an irregular professional mark the days they
// work with no weekly template at all.
var WorkCalendarBlockModel = model.Definition{
	Name: "work_calendar_block",
	Fields: model.Fields{
		{Name: "id", Type: model.Text(), DB: &model.FieldDB{PK: true}},
		{Name: "tenant_id", Type: model.Text(), NotNull: true},
		{Name: "staff_id", Type: model.Text(), NotNull: true},
		{Name: "day_of_week", Type: model.Int(), NotNull: true},   // 0..6, meaningful when specific_date == 0
		{Name: "specific_date", Type: model.Int(), NotNull: true}, // 0 = weekly; else midnight UTC seconds
		{Name: "start_min", Type: model.Int(), NotNull: true},     // minutes from midnight
		{Name: "end_min", Type: model.Int(), NotNull: true},
		{Name: "is_active", Type: model.Bool(), NotNull: true},
	},
}
```

One table for both shapes, because both answer the same question — *when does
this professional work* — and `ListAvailability` must merge them anyway. A
second table would force every reader to union two queries and keep two
precedence rules in step.

New domain errors:

```go
var (
	ErrBlockOutsideBusinessHours = fmt.Err("appointment_booking: block falls outside the establishment's opening hours")
	ErrBlockOnClosedDay          = fmt.Err("appointment_booking: the establishment is closed on that date")
	ErrBlocksOverlap             = fmt.Err("appointment_booking: two blocks of the same day overlap")
	ErrInvalidBlock              = fmt.Err("appointment_booking: start_min must be < end_min and both within 0..1439")
)
```

Regenerate `model_orm.go` with `ormc`. **Never hand-edit it.**

## 5. Stage 2 — the availability fix (CU-10, the load-bearing change)

**File: `service.go`, `ListAvailability`.** Restructure the per-day loop so
dated blocks are consulted **before** the weekly gate:

```go
for d := from; d <= to; d += 86400 {
	// 1. Establishment first — a closed building closes everyone (CU-04/06).
	bounds, err := m.bounds.GetDayBounds(d)
	if err != nil {
		return nil, err
	}
	if !bounds.Open {
		continue
	}

	// 2. Dated blocks for this exact date. If any exist they define the day
	//    OUTRIGHT, whether or not a weekly block covers this weekday. This is
	//    the inversion: the old code skipped the day here when the weekly
	//    template had no active row, which made a dated entry dead data.
	dayBlocks := blocksForDate(allBlocks, d)
	if len(dayBlocks) == 0 {
		dayBlocks = blocksForWeekday(allBlocks, tinytime.Weekday(d))
	}
	if len(dayBlocks) == 0 {
		continue // genuinely not worked
	}

	// 3. HOLIDAY / BLOCKED exceptions still subtract, as before.
	// 4. Clamp every block to [bounds.OpenMin, bounds.CloseMin].
	// 5. Generate slots per block, skipping the gaps between blocks.
}
```

Both helpers scan a slice linearly — **no `map`**, per `AGENTS.md`.

Read the existing slot-generation loop (`service.go:346`+) before rewriting it:
buffer handling, the `blockedExcs` subtraction and the timezone conversion via
`LocalIntToUnixUTC` must all survive unchanged. Only the day-gating and the
"one window → several blocks" iteration change.

`SPECIAL_HOURS` exceptions become redundant for *opening* a day (a dated block
does that now), but the type stays for narrowing a day that a weekly block
covers. Do **not** delete it.

## 6. Stage 3 — the establishment bound (CU-02, CU-07)

### 6.1 The port is declared HERE — this module imports no sibling

This module is a **generic** booking domain. Importing
`github.com/veltylabs/business_calendar` would make the general depend on the
specific: `appointment_booking/go.mod` has **zero veltylabs dependencies
today**, and that import would be the first, pinning the two siblings to each
other's versions forever and forcing every appointment-booking app to adopt one
particular model of "the establishment" — including apps that have none.

So the **behavioural interface lives here** (the Go idiom: the consumer declares
what it needs), and the **value that crosses** is `time.DayBounds`, a neutral
type in a leaf both sides already import (`tinytime "webtyp.com/time"`,
`service.go:9`). Neither module imports the other; `business_calendar` satisfies
this interface structurally, with no adapter anywhere.

```go
// BoundsReader answers "which minutes of this date are usable at all".
//
// Declared here, not imported: a booking module must not depend on any
// particular institutional calendar. Anything that can answer the question
// satisfies it — veltylabs/business_calendar does, and so would a static
// config or a different organisation's calendar.
//
// date is midnight UTC in seconds.
type BoundsReader interface {
	GetDayBounds(date int64) (tinytime.DayBounds, error)
}

type Deps struct {
	IDs        model.IDGenerator
	Publisher  events.Publisher
	Subscriber events.Subscriber
	// Bounds constrains every block to the institution's usable window.
	// OPTIONAL: nil means unbounded, which is correct for an app with no
	// institution above the professional (a freelancer's booking page). An
	// app that HAS one and forgets to wire it is caught by its own
	// acceptance test, not by a constructor error here — refusing to build
	// would make the freelancer case impossible to express.
	Bounds BoundsReader
}
```

When `Bounds` is nil, resolve through `tinytime.Unbounded()` **once**, at the
top of the day loop, rather than branching on nil at every use. One `nil` check
in the whole module; the rest of the code always has bounds.

**Anti-footgun:** do **not** widen `BoundsReader` to carry *why* a day is
closed. `business_calendar` distinguishes holiday from closure for its own UI;
this module only needs "no slots that day", and its `ConflictingReservation`
reason stays the neutral `"DAY_CLOSED"`. Pulling the reason across would drag
one domain's vocabulary into a generic module — and it is what `time.DayBounds`
deliberately omits.

### 6.2 What the bound is used for

Two uses:

1. **`SaveDayBlocks` rejects out-of-bounds writes** with
   `ErrBlockOutsideBusinessHours` / `ErrBlockOnClosedDay`. The editor already
   only offers legal hours (CU-02), but the ops are exposed through `router` and
   the UI cannot be the only defence (CU-07).
2. **`ListAvailability` clamps** every block to the day's bounds, so narrowing
   the establishment's hours immediately narrows what is offered, with no
   rewrite of anybody's blocks.

New read op so the editor can bound its own controls:

```go
const OpGetDayBounds = "get_day_bounds" // proxies Deps.Bounds (BoundsReader)
```

The editor calls this module, not the calendar module directly — the frontend
already has a caller for this module, and one hop keeps the demo's wiring to a
single loopback registry.

## 7. Stage 4 — write API

**File: `service.go`.** Replace `UpsertWeeklyCalendar` with:

```go
// SaveDayBlocks replaces EVERY weekly block of that weekday with the ones
// given. A whole-day replace, not a per-row upsert: partial edits are what let
// the stored set drift out of step with what the editor is showing.
func (m *Module) SaveDayBlocks(tenantId, staffId string, dayOfWeek int, blocks []WorkCalendarBlock) error

// MarkWorkingDays writes ONE dated block per date, all with the same window —
// the bulk gesture behind "mark the days I work over the next 6 months"
// (CU-10/CU-11). Re-marking a date replaces its blocks.
func (m *Module) MarkWorkingDays(tenantId, staffId string, dates []int64, startMin, endMin int) error

// UnmarkWorkingDays deletes every dated block on those dates, returning the
// days to whatever the weekly template says — or to unworked (CU-15).
func (m *Module) UnmarkWorkingDays(tenantId, staffId string, dates []int64) error

// SaveDateBlocks replaces the dated blocks of ONE date, so a single marked day
// can diverge from the common window it was created with (CU-11).
func (m *Module) SaveDateBlocks(tenantId, staffId string, date int64, blocks []WorkCalendarBlock) error
```

Every one validates (`ErrInvalidBlock`, `ErrBlocksOverlap`, the two bound
errors), then publishes the enriched event of §8.

Ops, replacing `OpUpsertWeeklyCalendar` / `OpListWeeklyCalendar`:

```go
const (
	OpSaveDayBlocks      = "save_day_blocks"
	OpSaveDateBlocks     = "save_date_blocks"
	OpMarkWorkingDays    = "mark_working_days"
	OpUnmarkWorkingDays  = "unmark_working_days"
	OpListBlocks         = "list_blocks"
)
```

`Requires("calendar", model.Create|model.Update)` for the writes that both
create and update; `model.Delete` for `OpUnmarkWorkingDays`; `model.Read` for
`OpListBlocks` and `OpGetDayBounds`.

## 8. Stage 5 — conflicts (CU-16…CU-20)

### 8.1 Detection

```go
// ConflictingReservation is a booked appointment that no longer fits the
// professional's schedule. Reported, never cancelled: cancelling on the
// patient's behalf is a decision for a person.
type ConflictingReservation struct {
	ReservationId string
	StartsAt      int64
	ClientId      string
	Reason        string // "OUTSIDE_BLOCKS" | "DAY_CLOSED" | "OUTSIDE_BUSINESS_HOURS"
}

// ListConflictingReservations returns future reservations that the CURRENT
// schedule no longer covers. from is normally "now".
func (m *Module) ListConflictingReservations(tenantId, staffId string, from int64) ([]ConflictingReservation, error)
```

Op: `OpListConflictingReservations = "list_conflicting_reservations"`,
`Requires("reservation", model.Read)`.

Reuse the availability computation — do **not** re-implement the "does this
instant fit" rule twice. Extract the per-day window resolution from
`ListAvailability` into an unexported helper both call. Two copies of that rule
would drift, and the stale one is what someone would trust.

### 8.2 Marking

Add `StatusConflicted = "CONFLICTED"` to `fsm.go`, with transitions from
`StatusPending` and `StatusConfirmed`, and back to either on resolution. Read
`fsm.go` before editing: transitions are a declared table and the terminal set
(`StatusCancelled`, `StatusCompleted`, `StatusNoShow`, `StatusExpired`,
`StatusRescheduled`) must stay terminal. `CONFLICTED` is **not** terminal.

Every write in §7 recomputes conflicts for the affected staff and marks them.
**CU-19: when the set is empty, nothing is marked and nothing is published** —
no false positives.

### 8.3 The enriched event (CU-20)

```go
type ScheduleChangedPayload struct {
	TenantId string
	StaffId  string
	// FromDate/ToDate bound what changed, so a consumer recomputes a range
	// instead of everything. 0/0 means "unknown, recompute all".
	FromDate int64
	ToDate   int64
	// ConflictCount is how many reservations the change put in conflict.
	// Zero means nobody needs to be told (CU-19).
	ConflictCount int
}
```

Add `EventReservationConflicted = "appointment.reservation.conflicted"`,
published once per conflicted reservation so a notifier can reach the patient
(CU-18) without polling.

**This module does not send notifications.** It publishes events and exposes the
list. Transport (SSE, email) belongs to whoever wires it — `webtyp/sse` already
satisfies `events.Publisher`.

### 8.4 Reacting to the establishment (CU-25…CU-29)

The professional is not the only one who can invalidate a booked appointment.
The **administrator** can too, from `business_calendar`, and it hits **every**
professional at once: a day becomes a holiday, a closure lands on it, or the
opening hours are narrowed under appointments already taken.

**This module does NOT subscribe.** Subscribing to
`business.calendar.changed` would require importing `business_calendar` for the
topic constant — the exact dependency §6.1 removes. Duplicating the topic string
locally would be worse: two copies of a magic string that drift, and the stale
one fails silently.

So the **trigger is inverted**: this module exposes the recomputation, and the
**application** — which legitimately knows both modules — subscribes and calls
it. That is what a composition root is for.

```go
// RecomputeConflicts re-evaluates every future reservation in [from, to]
// against the CURRENT schedule and bounds, marking or clearing CONFLICTED as
// the answer dictates. Returns how many reservations changed state.
//
// It is idempotent: calling it twice with no intervening change is a no-op and
// publishes nothing (CU-19/CU-29 depend on that).
//
// tenantId scopes it; from/to are midnight UTC seconds. to == 0 means "the
// module's full forward horizon", which is what a weekly-hours change needs
// since it has no bounded date range.
func (m *Module) RecomputeConflicts(tenantId string, from, to int64) (int, error)
```

Op: `OpRecomputeConflicts = "recompute_conflicts"`,
`Requires("reservation", model.Update)`.

The application wires it in one place (`app-demo`'s plan §3):

```go
broker.Subscribe(businesscalendar.EventCalendarChanged, func(ev events.Event) {
    var p businesscalendar.CalendarChangedPayload
    // decode; opening time can never invalidate a booking (CU-29's cheap half)
    if !p.Closed {
        return
    }
    _, _ = book.RecomputeConflicts(config.TenantID, p.FromDate, p.ToDate)
})
```

`Deps` therefore gains **nothing** for this. No `Subscriber` field, no second
required dependency:

```go
type Deps struct {
	IDs       model.IDGenerator
	Publisher events.Publisher
	Bounds    BoundsReader // optional — see §6.1
}
```

This is strictly better than the subscription: `RecomputeConflicts` is also
callable from a CLI, a migration or a test without staging an event, and the
"what triggers it" decision stays with the app that owns both modules.

### 8.5 Conflict state is RECOMPUTED, never toggled (CU-28)

The subtle rule, and the one most likely to be got wrong: a reservation can be
conflicted for **more than one reason at once** — a holiday *and* being outside
the professional's blocks.

So removing the holiday must not flip it back to `CONFIRMED`. It must
**recompute**: run the same "does this instant fit" resolution used by
`ListAvailability` and `ListConflictingReservations`, and let the answer decide.

```
recompute(reservation) → fits    ? CONFLICTED → previous status
                       → not fit ? mark/keep CONFLICTED with the current reason
```

To restore the previous status, `Reservation` needs to remember it. Add
`StatusBeforeConflict` alongside the existing `RescheduledFromId`, written when
a reservation enters `CONFLICTED` and cleared when it leaves. Without it,
"un-conflicting" would have to guess between `PENDING` and `CONFIRMED`, and
guessing is what turns a confirmed appointment into a pending one behind the
patient's back.

**Never** implement this as a boolean toggle keyed on the event that fired. Two
independent causes, one flag, and the second cause silently disappears.

### 8.6 One summary event, not one per reservation

A holiday can conflict hundreds of reservations across dozens of professionals.
Publishing `EventReservationConflicted` once per reservation would flood the
broker for a single administrative click.

- Publish **`EventScheduleChanged` once per affected staff**, carrying that
  professional's `ConflictCount` and the date range.
- Publish **`EventReservationConflicted` per reservation only** when the change
  came from the professional's own edit (§8.2), where the count is small and a
  patient-facing notifier needs the individual record.

For the establishment-wide case the consumer reads
`ListConflictingReservations`, which is exactly the administrator worklist
CU-17 already needs.

## 9. Stage 6 — tests (`tests/`)

Consumer-shaped, real stack, `storage/mem` plus a fake `BoundsReader` at the
edge (a struct returning canned `tinytime.DayBounds`). **No veltylabs import
appears in these tests** — that is part of what they prove.

| Test | CU |
|---|---|
| `TestMarkedDayGeneratesSlotsWithoutWeeklyTemplate` | **CU-10** — the §1.1 regression |
| `TestTwoBlocksSameDayLeaveTheGapUnbookable` | CU-09 |
| `TestSingleBlockAppliedToSeveralWeekdays` | CU-08 |
| `TestMarkedDaysShareTheCommonWindow` | CU-11 |
| `TestOneMarkedDayCanDivergeFromTheCommonWindow` | CU-11 |
| `TestWeeklyTemplateAndMarkedDaysCoexist` | CU-12 |
| `TestBlockedExceptionWinsOverBlockAndMarkedDay` | CU-13 |
| `TestUnmarkingRestoresTheWeeklyTemplate` | CU-15 |
| `TestBlockOutsideBusinessHoursIsRejected` | CU-07 |
| `TestClosedEstablishmentDayYieldsNoSlots` | CU-04/06 |
| `TestNarrowingBusinessHoursClampsOfferedSlots` | CU-02 |
| `TestReducingScheduleMarksAffectedReservationsAndPublishes` | CU-16 |
| `TestReducingScheduleWithNoAffectedReservationsIsSilent` | **CU-19** |
| `TestScheduleChangedPayloadCarriesRangeAndCount` | CU-20 |
| `TestConflictedIsNotATerminalStatus` | CU-17 |
| `TestDayBecomingHolidayConflictsItsReservations` | **CU-25** |
| `TestLocalClosureConflictsWithItsOwnReason` | CU-26 |
| `TestNarrowingBusinessHoursConflictsLateReservations` | CU-27 |
| `TestRemovingHolidayRestoresThePreviousStatus` | CU-28 |
| `TestRemovingHolidayKeepsConflictWhenAlsoOutsideBlocks` | **CU-28** — the two-cause case |
| `TestHolidayOnDayWithoutReservationsIsSilent` | CU-29 |
| `TestOpeningTimeNeverConflictsAnything` | CU-29 |
| `TestNewFailsWithoutSubscriber` | — (§8.4: required, not optional) |

`TestRemovingHolidayKeepsConflictWhenAlsoOutsideBlocks` is the one that catches
a toggle implementation: set up a reservation that is **both** on a holiday
**and** outside the professional's blocks, remove the holiday, and assert it is
still `CONFLICTED`. A boolean flip passes every other test in this table and
fails this one.

## 10. Constraints — read before writing code

- **`AGENTS.md` in this repo root is the contract.** Read it first.
- **No stdlib**: `webtyp.com/fmt`, never `errors`/`strconv`/`strings`.
- **No `map`.** The existing code says so explicitly at `service.go:249` and
  `:263`; keep it that way in every helper added here.
- **`model_orm.go` is generated by `ormc` — never hand-edit.**
- **Do NOT import `github.com/veltylabs/business_calendar`.** The port is
  declared here (§6.1) and the value crosses as `tinytime.DayBounds`. This
  module's go.mod must keep zero veltylabs dependencies.
- **Timezone handling is already correct** — `LocalIntToUnixUTC(d, min, tz)`
  converts local minutes to UTC. Do not "simplify" it; the demo already fixed a
  timezone bug there once.
- **`ErrNotFound` mapping**: only `orm.ErrNotFound` maps to a domain
  not-found sentinel; anything else surfaces as the internal error it is. That
  rule is already stated in `work_schedule/module.go:36` and holds here.
- **No `TODO`, no deprecated path.** `WorkCalendarWeekly` is **deleted**, not
  kept beside blocks. Before closing:
  `grep -rn "TODO\|FIXME\|Deprecated" --include='*.go' .`
- `gotest`, never `go test`.

## 11. Acceptance criteria

| # | Check | Expected |
|---|-------|----------|
| 1 | `gotest ./...` | green, every test in §9 present |
| 2 | `grep -rn "WorkCalendarWeekly\|break_start\|break_finish\|BreakStart\|BreakFinish" --include='*.go' .` | **empty** |
| 3 | `grep -rn "weeklyForDay" --include='*.go' .` | **empty** — replaced by the two block helpers |
| 4 | `grep -n "if !hasWeekly" service.go` | **empty** — the gate is gone |
| 5 | `grep -rn "map\[" --include='*.go' .` | **empty** |
| 6 | `grep -rn "veltylabs" go.mod` | **only the module line** — zero veltylabs deps |
| 7 | `grep -rn "type.*Reader interface" --include='*.go' .` | **empty** — no local copy of the port |
| 8 | `grep -n "ConflictCount\|FromDate" model.go service.go` | present in `ScheduleChangedPayload` |
| 9 | `grep -n "func (m \*Module) RecomputeConflicts" service.go` | present — §8.4 |
| 10 | `grep -n "StatusBeforeConflict" model.go` | present — §8.5 needs it to restore |
| 11 | `GOOS=js GOARCH=wasm go build ./...` | compiles |
| 12 | `grep -rn "TODO\|FIXME\|Deprecated" --include='*.go' .` | no hit introduced here |

## 12. Stages

| # | Stage | Files | Done when |
|---|-------|-------|-----------|
| 1 | Blocks replace the weekly window | `model.go`, `model_orm.go` | `WorkCalendarWeekly` gone, break fields gone |
| 2 | Availability fix | `service.go` | a dated block opens its day; CU-10 green |
| 3 | Establishment bound | `module.go`, `ops.go` | `Deps.Calendar` required; writes rejected out of bounds |
| 4 | Write API | `service.go`, `ops.go` | four methods, five ops |
| 5 | Conflicts | `service.go`, `fsm.go`, `model.go` | detection, `CONFLICTED`, `StatusBeforeConflict`, enriched payload |
| 5b | React to the establishment | `module.go`, `service.go` | `Deps.Subscriber` required; `onCalendarChanged` recomputes; CU-25…29 green |
| 6 | Tests | `tests/` | every CU in §9 green |

## 13. What this plan does NOT do

- **No `effective_from`/`effective_to`.** Cut by decision; see §2.
- **No RRULE.** Marked days serve those cases.
- **No notification transport.** This module publishes events and lists
  conflicts; delivery is wired elsewhere.
- **No UI.** `components/scheduleeditor` and `app-demo` have their own plans.
