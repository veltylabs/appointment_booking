# appointment-booking
<img src="docs/img/badges.svg">

Manages schedulable service configuration, staff work calendars, and client reservations.

## Main entities

- `employee_service_config`: configuration of which services each professional handles (duration, price override).
- `reservation`: the scheduled appointment (date/time, client, professional, service).
- `workcalendar_weekly`: weekly work schedules per staff member.
- `workcalendar_exception`: one-off exceptions (personal holidays, special hours, blocked intervals).

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

Availability rules (calendar + exceptions) are enforced at the application layer, not via cross-module FKs.
Reservation status is enforced by an in-code FSM — no `reservation_status` table.

## Development Rules (SKILL)

### Core Constraints & Rules

- **FSM-only status changes:** `Reservation.Status` MUST only change via `FSM.Transition(current, event)`. Never set status directly. Valid events: `CONFIRM`, `CANCEL`, `COMPLETE`, `NO_SHOW_EVENT`, `EXPIRE`, `RESCHEDULE`.
- **RESCHEDULED ≠ CANCELLED:** When a reservation is replaced by a new one, the original is marked `RESCHEDULED` (not `CANCELLED`) for audit trail integrity. These are different terminal states.
- **Timezone is in WorkCalendarConfig:** `WorkCalendarWeekly` and `WorkCalendarException` have NO timezone field. Always load `WorkCalendarConfig` first to get the IANA timezone. Working hours are local integers (e.g., `900 = 09:00`), converted to UTC at query time via `LocalIntToUnixUTC`.
- **Snapshotting:** At reservation creation, price, currency, duration, staffID, and serviceID are snapshotted. Never mutate snapshot fields after creation.
- **No RBAC here:** This module trusts `actorID` as an already-authorized string. Authorization is enforced by the MCP gateway before reaching the service. This module only stores actorID as an audit field.
- **EventPublisher is fire-and-forget:** After each successful state mutation, publish a domain event via the injected `EventPublisher`. Publish errors are logged and never fail the operation. `nil` publisher is safe.
- **MCP is the only external entry point.** The service is never called directly except by the MCP handler layer.
- **No cross-module imports.** External dependencies are accessed only via injected interfaces: `StaffReader`, `CatalogReader`, `DirectoryReader`.
- **`tinywasm` packages only** for WASM compatibility: use `tinywasm/fmt`, `tinywasm/time`, `tinywasm/json` — never standard library equivalents.

### Injected Interfaces (constructor parameters)

The service holds `*orm.DB` directly — no intermediate store interfaces. Only cross-module dependencies are injected:

```go
type Deps struct {
    Staff     StaffReader     // provided by staff module
    Catalog   CatalogReader   // provided by catalog module
    Directory DirectoryReader // provided by directory module
    Publisher EventPublisher  // nil = events disabled
}

// Constructor: db is *orm.DB passed directly.
func New(db *orm.DB, deps Deps) SchedulingService
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

### Key Error Sentinels

| Error | When |
|---|---|
| `ErrSlotTaken` | Slot not available or concurrent booking race |
| `ErrConflict` | Optimistic concurrency mismatch on `UpdateReservationStatus` |
| `ErrCalendarConfigNotFound` | `UpsertWeeklyCalendar` called before `UpsertCalendarConfig` |
| `ErrInvalidTransition` | FSM rejects the event for the current status |

### Composition Root (how to wire this module)

```go
scheduling := appointmentbooking.New(db, appointmentbooking.Deps{
    Staff:     staffmodule.New(db),     // implements StaffReader
    Catalog:   catalogmodule.New(db),   // implements CatalogReader
    Directory: directorymodule.New(db), // implements DirectoryReader
    Publisher: eventBus,                // nil = events disabled
})
providers := []mcp.ToolProvider{
    appointmentbooking.NewReservationProvider(scheduling),
    appointmentbooking.NewCalendarProvider(scheduling),
}
// mcp.NewServer(mcp.Config{...}, providers)
```

### Available MCP Tools (13 total)

`list_availability`, `create_reservation`, `get_reservation`, `list_reservations_by_staff`, `list_reservations_by_client`, `change_reservation_status`, `upsert_calendar_config`, `upsert_weekly_calendar`, `add_calendar_exception`, `remove_calendar_exception`, `expire_pending_reservations`, `list_weekly_calendar`, `list_exceptions`

> `expire_pending_reservations` is the **only trigger for the EXPIRE FSM event**. It must be called by an external scheduler — the module has no internal background process.
>
> `list_weekly_calendar` and `list_exceptions` are the raw reads a schedule editor needs; the caller-side face is `NewScheduleClient` (see below).

## ScheduleClient — the schedule-editor face

`NewScheduleClient(caller router.Caller, tenantId, staffId string) *ScheduleClient` is a caller-side
typed client over the calendar ops, intended to be adapted by an app to a `scheduleeditor` UI
component. Importing only `router` + this module's types, it stays renderer-agnostic:

```go
cl := appointmentbooking.NewScheduleClient(caller, "t1", "s1")
cl.Weekly(func(rows []appointmentbooking.WorkCalendarWeekly, err error) { /* … */ })
cl.Exceptions(from, to, func(rows []appointmentbooking.WorkCalendarException, err error) { /* … */ })
cl.SaveWeeklyRow(row, func(err error) { /* … */ })
cl.AddException(exc, func(err error) { /* … */ })
cl.RemoveException(exceptionID, func(err error) { /* … */ })
```

Every successful calendar mutation also publishes the `appointment.schedule.changed` event
(`ScheduleChangedPayload{TenantId, StaffId}`) so consumers can recompute availability.

## Service interface

```go
type SchedulingService interface {
    // Calendar management
    UpsertCalendarConfig(ctx context.Context, cfg WorkCalendarConfig) error
    UpsertWeeklyCalendar(ctx context.Context, cal WorkCalendarWeekly) error
    AddException(ctx context.Context, exc WorkCalendarException) error
    RemoveException(ctx context.Context, tenantID, exceptionID string) error

    // Availability
    ListAvailability(ctx context.Context, tenantID, staffID, configID string, from, to int64) ([]TimeSlot, error)

    // Reservations
    CreateReservation(ctx context.Context, cmd CreateReservationCmd) (Reservation, error)
    GetReservation(ctx context.Context, tenantID, id string) (Reservation, error)
    ListReservationsByStaff(ctx context.Context, tenantID, staffID string, from, to int64) ([]Reservation, error)
    ListReservationsByClient(ctx context.Context, tenantID, clientID string) ([]Reservation, error)
    ChangeReservationStatus(ctx context.Context, cmd ChangeStatusCmd) error
}
```

This interface depends on injected readers:
- `DirectoryReader` — validates client existence
- `StaffReader` — validates staff existence
- `CatalogReader` — validates service existence
