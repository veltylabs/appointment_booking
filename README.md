# appointment-booking
<img src="docs/img/badges.svg">

Manages schedulable service configuration, staff work calendars (blocks), and client reservations.

## Main entities

- `employee_service_config`: configuration of which services each professional handles (duration, price override).
- `reservation`: the scheduled appointment (date/time, client, professional, service) with an FSM-enforced `status`.
- `workcalendar_block`: one row per block of working time — WEEKLY (`specific_date == 0`, applies to `day_of_week`) or DATED (`specific_date > 0`, applies to that date only and opens it). Several blocks per day = the lunch break is the gap between them.
- `workcalendar_exception`: one-off exceptions (personal holidays, special hours, blocked intervals).
- `workcalendar_config`: the IANA timezone of a staff member's calendar (inherited by blocks and exceptions).

## Documentation

- [Architecture](docs/ARCHITECTURE.md)
- Most recently executed local plan: [docs/LAST_PLAN_EXECUTED.md](docs/LAST_PLAN_EXECUTED.md)
- [Database Diagram](docs/diagrams/database.md)
- [FSM Diagram](docs/diagrams/fsm.md)
- [Sequence Diagrams](docs/diagrams/sequence.md)

## Design / decoupling notes

No physical FKs to other modules:
- `reservation.client_id` references a client (Directory/Clinical) by ID.
- `reservation.creator_user_id` references an IAM user.
- `employee_service_config.service_id` references an item from the Catalog module.
- `staff_id` fields reference the Staff module.

Availability rules (blocks + exceptions + the establishment's daily window, crossed as
`webtyp.com/time` `DayBounds`) are enforced at the service layer, not via cross-module FKs.
`appointment_booking/go.mod` keeps **zero veltylabs dependencies** — the establishment bound is a
port declared here (`BoundsReader`), satisfied structurally by `veltylabs/business_calendar`.
Reservation status is enforced by an in-code FSM — no `reservation_status` table.

## Development Rules (SKILL)

### Core Constraints & Rules

- **FSM-only status changes:** `Reservation.Status` MUST only change via `FSM.Transition(current, event)` — except entering/leaving `CONFLICTED`, which a recomputation writes directly and restores via `StatusBeforeConflict`. Valid events: `CONFIRM`, `CANCEL`, `COMPLETE`, `NO_SHOW_EVENT`, `EXPIRE`, `RESCHEDULE`, `CONFLICT`, `RESOLVE`.
- **RESCHEDULED ≠ CANCELLED:** When a reservation is replaced by a new one, the original is marked `RESCHEDULED` (not `CANCELLED`) for audit trail integrity. These are different terminal states. `CONFLICTED` is **not** terminal.
- **Timezone is in WorkCalendarConfig:** `WorkCalendarBlock` and `WorkCalendarException` have NO timezone field. Always load `WorkCalendarConfig` first to get the IANA timezone. Block hours are local minutes from midnight (e.g., `540 = 09:00`), converted to UTC at query time via `LocalIntToUnixUTC`.
- **Snapshotting:** At reservation creation, price, currency, duration, staffID, and serviceID are snapshotted. Never mutate snapshot fields after creation. `StaffIdsnapshot`/`ServiceIdsnapshot` use the historical column names `staff_idsnapshot`/`service_idsnapshot` — never rename.
- **A schedule change reports its conflicts:** every professional edit (`save_day_blocks`, `save_date_blocks`, `mark_working_days`, `unmark_working_days`, exceptions) recomputes future reservations in the affected range and marks/clears `CONFLICTED`. Establishment-wide changes go through `RecomputeConflicts`. When nobody is affected, **nothing is published** (CU-19).
- **No RBAC here:** This module trusts `actorID` as an already-authorized string. Authorization is enforced by the gateway before the service. This module only stores actorID as an audit field.
- **Event publishing is fire-and-forget** via the injected `events.Publisher` (`Deps.Publisher`). `nil` publisher is safe.
- **No cross-module imports.** External dependencies are accessed only via injected interfaces: `StaffReader`, `CatalogReader`, `DirectoryReader`, `BoundsReader`.

### Injected Interfaces (constructor parameters)

The service holds `*orm.DB` directly — no intermediate store interfaces. Only cross-module dependencies are injected:

```go
type Deps struct {
    Staff     StaffReader     // provided by staff module
    Catalog   CatalogReader   // provided by catalog module
    Directory DirectoryReader // provided by directory module
    IDs       model.IDGenerator // required — never constructed inside the module
    Publisher events.Publisher  // nil = events disabled
    Bounds    BoundsReader      // nil = unbounded (a freelancer without an establishment)
}

func New(db *orm.DB, deps Deps) (*Module, error)
```

### Domain Events Published

| Event constant | When |
|---|---|
| `appointment.reservation.created` | After `CreateReservation` commits |
| `appointment.reservation.rescheduled` | For the original reservation during reschedule |
| `appointment.reservation.confirmed` | After CONFIRM transition |
| `appointment.reservation.cancelled` | After CANCEL transition |
| `appointment.reservation.completed` | After COMPLETE transition |
| `appointment.reservation.no_show` | After NO_SHOW transition |
| `appointment.reservation.expired` | After EXPIRE transition |
| `appointment.schedule.changed` | Once per staff whose schedule change put ≥1 reservation in conflict (payload `ScheduleChangedPayload{TenantId, StaffId, FromDate, ToDate, ConflictCount}`) |
| `appointment.reservation.conflicted` | Per conflicted reservation, only when the change came from the professional's own edit |

### Key Error Sentinels

| Error | When |
|---|---|
| `ErrSlotTaken` | Slot not available or concurrent booking race |
| `ErrConflict` | Optimistic concurrency mismatch on reservation updates |
| `ErrCalendarConfigNotFound` | Saving blocks before `upsert_calendar_config` |
| `ErrInvalidBlock` | `start_min` not `< end_min`, or outside 0..1439 |
| `ErrBlocksOverlap` | Two blocks of the same day overlap |
| `ErrBlockOutsideBusinessHours` | A block falls outside the establishment's opening window |
| `ErrBlockOnClosedDay` | The establishment is closed on that date |
| `ErrInvalidTransition` | FSM rejects the event for the current status |

### Composition Root (how to wire this module)

```go
scheduling, _ := appointmentbooking.New(db, appointmentbooking.Deps{
    Staff:     staffmodule.New(db),        // implements StaffReader
    Catalog:   catalogmodule.New(db),      // implements CatalogReader
    Directory: directorymodule.New(db),    // implements DirectoryReader
    IDs:       idGen,                      // model.IDGenerator
    Publisher: eventBus,                   // nil = events disabled
    Bounds:    businesscalendarModule,     // implements BoundsReader; nil = unbounded
})
scheduling.MountOps(opRegistry)            // transport harvests the ops
reservationsView := scheduling.NewView(caller, tenantId, staffId)
```

Establishment-wide recomputation is wired by the application (it knows both modules):

```go
broker.Subscribe(businesscalendar.EventCalendarChanged, func(ev events.Event) {
    var p businesscalendar.CalendarChangedPayload
    // decode; opening earlier can never invalidate a booking
    if !p.Closed {
        return
    }
    _, _ = book.RecomputeConflicts(config.TenantID, p.FromDate, p.ToDate)
})
```

### Available Ops (19 total)

`create_reservation`, `get_reservation`, `list_reservations_by_staff`, `list_reservations_by_client`,
`change_reservation_status`, `expire_pending_reservations`, `list_conflicting_reservations`,
`recompute_conflicts`, `upsert_calendar_config`, `save_day_blocks`, `save_date_blocks`,
`mark_working_days`, `unmark_working_days`, `list_blocks`, `get_day_bounds`, `add_calendar_exception`,
`remove_calendar_exception`, `list_availability`, `list_exceptions`

> `expire_pending_reservations` is the **only trigger for the EXPIRE FSM event**; it must be called by an
> external scheduler — the module has no internal background process.
>
> `list_blocks` and `list_exceptions` are the raw reads a schedule editor needs; the caller-side face
> is `NewScheduleClient` (see below).

## ScheduleClient — the schedule-editor face

`NewScheduleClient(caller router.Caller, tenantId, staffId string) *ScheduleClient` is a caller-side
typed client over the calendar ops, intended to be adapted by an app to a `scheduleeditor` UI
component. Importing only `router` + this module's types, it stays renderer-agnostic:

```go
cl := appointmentbooking.NewScheduleClient(caller, "t1", "s1")
cl.Blocks(func(rows []appointmentbooking.WorkCalendarBlock, err error) { /* … */ })
cl.SaveDayBlocks(dayOfWeek, blocks, func(err error) { /* … */ })
cl.Exceptions(from, to, func(rows []appointmentbooking.WorkCalendarException, err error) { /* … */ })
cl.AddException(exc, func(err error) { /* … */ })
cl.RemoveException(exceptionID, func(err error) { /* … */ })
```

A schedule mutation publishes `appointment.schedule.changed` (with the affected range and conflict
count) only when it actually put reservations in conflict.

## Service interface

```go
type SchedulingService interface {
    // Calendar management
    UpsertCalendarConfig(cfg WorkCalendarConfig) error
    SaveDayBlocks(tenantId, staffId string, dayOfWeek int, blocks []WorkCalendarBlock) error
    SaveDateBlocks(tenantId, staffId string, date int64, blocks []WorkCalendarBlock) error
    MarkWorkingDays(tenantId, staffId string, dates []int64, startMin, endMin int) error
    UnmarkWorkingDays(tenantId, staffId string, dates []int64) error
    ListBlocks(tenantId, staffId string) ([]WorkCalendarBlock, error)
    AddException(exc WorkCalendarException) error
    RemoveException(tenantId, exceptionId string) error
    ListExceptions(tenantId, staffId string, from, to int64) ([]WorkCalendarException, error)
    GetDayBounds(date int64) (tinytime.DayBounds, error)

    // Availability
    ListAvailability(tenantId, staffId, configId string, from, to int64) ([]TimeSlot, error)

    // Reservations
    CreateReservation(cmd CreateReservationCmd) (Reservation, error)
    GetReservation(tenantId, id string) (Reservation, error)
    ListReservationsByStaff(tenantId, staffId string, from, to int64) ([]Reservation, error)
    ListReservationsByClient(tenantId, clientId string) ([]Reservation, error)
    ChangeReservationStatus(cmd ChangeStatusCmd) error
    ExpirePendingReservations(tenantId string, before int64) (int, error)

    // Conflicts
    ListConflictingReservations(tenantId, staffId string, from int64) ([]ConflictingReservation, error)
    RecomputeConflicts(tenantId string, from, to int64) (int, error)
}
```

This interface depends on injected readers:
- `DirectoryReader` — validates client existence
- `StaffReader` — validates staff existence
- `CatalogReader` — validates service existence
- `BoundsReader` — answers "which minutes of a date are usable at all" (optional)