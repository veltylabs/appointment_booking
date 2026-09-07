# appointment-booking Architecture

> **Status note:** this document describes the current shape of the module after the reusable-module
> harness adoption (`router.OpModule`, `Deps.IDs model.IDGenerator`, `events.Publisher`,
> `ddl.CreateTable`, `storage/mem` tests) and the schedule-editor addition.
> `AGENTS.md` (this repo's root) is the authority on the whitelist/blacklist this module holds to.

## 1. Domain Scope

The `appointment-booking` module manages the complete lifecycle of a scheduled service appointment. It is responsible for:
- Configuring which services each staff member offers (duration, price, buffer time).
- Defining staff availability via weekly calendars and one-off exceptions (holidays, special hours, blocked intervals).
- Calculating free time slots and creating reservations with atomic conflict prevention.
- Enforcing reservation state transitions via a Finite State Machine (FSM).

## 2. Core Entities

- **`EmployeeServiceConfig`:** Maps a staff member to a service item, defining duration, buffer time, and price override. The source of truth for slot granularity.
- **`Reservation`:** The appointment itself. Stores snapshots of staff, service, price, and currency at creation time for financial auditability — these never change even if the source data is later modified.
- **`WorkCalendarConfig`:** One row per staff member. Single source of truth for the IANA timezone of the staff calendar. Must exist before weekly rows can be inserted.
- **`WorkCalendarWeekly`:** Recurring weekly schedule for a staff member (one row per working day). Defines local working hours and break. Does not carry timezone — inherits it from `WorkCalendarConfig`.
- **`WorkCalendarException`:** One-off overrides for a specific date: `HOLIDAY` (no availability), `SPECIAL_HOURS` (different hours), or `BLOCKED` (interval subtracted from available window).

This module owns and migrates the schema for all five entities above (unlike e.g. `work_schedule`,
which only reads read-only tables owned elsewhere) — see §7.

## 3. Finite State Machine (FSM)

Reservation status transitions are enforced in code — there is no `reservation_status` DB table.

See: [FSM Diagram](diagrams/fsm.md)

Key decisions:
- `RESCHEDULED` is a distinct terminal state (not `CANCELLED`) to preserve audit trail clarity in analytics.
- `EXPIRED` is triggered exclusively by an external scheduler via the `expire_pending_reservations` operation — the module does not run background goroutines.

## 4. Architectural Patterns

1. **Dependency Injection:** the module receives three external readers (`StaffReader`,
   `CatalogReader`, `DirectoryReader`) and one `events.Publisher` via `Deps` at construction (`New(db,
   deps)`). No global state, no direct imports from other modules.

2. **Direct ORM access (no store interfaces):** the module holds `*orm.DB` directly (through an
   internal `*Repository`) and calls ORM functions from `model_orm.go`. There is no intermediate
   `ReservationStore`, `CalendarStore`, or `ConfigStore` **interface** — `Repository` is a plain
   struct, not an abstraction boundary a test could swap for a mock. This keeps the internal boundary
   thin and lets the module's own tests exercise the real `github.com/webtyp/orm` query builder
   against `github.com/webtyp/storage/mem` — the in-memory reference backend — catching real
   constraint and concurrency bugs (optimistic-lock conflicts, uniqueness violations) instead of
   hiding them behind mocks, with no concrete database driver in the module's dependency graph at
   all. Only cross-module interfaces (`StaffReader`, `CatalogReader`, `DirectoryReader`) and the
   injected `events.Publisher` are mockable.

3. **Soft References (no physical FK):** `client_id`, `staff_id`, `service_id`, `creator_user_id`, and `payment_id` reference entities in other modules by ID only. Cross-module existence is validated at the application layer via injected readers, not via DB constraints.

4. **Snapshotting:** Price, currency, duration, staff ID, and service ID are snapshotted at reservation creation. Downstream changes to catalog or staff data do not alter existing reservations.

5. **Local Integer Time + IANA Timezone (Single Source of Truth):** Working hours in `WorkCalendarWeekly` are stored as local integers (e.g., `900 = 09:00`). The IANA timezone is stored exclusively in `WorkCalendarConfig` (one row per staff) — `WorkCalendarWeekly` and `WorkCalendarException` do not carry timezone fields. This prevents per-row timezone inconsistency by construction. The `ListAvailability` algorithm loads `WorkCalendarConfig` first to obtain the timezone, then converts local boundaries to Unix UTC using `github.com/webtyp/time` (imported directly — no local fork). This design ensures recurring schedules remain correct across DST transitions.

6. **Optimistic Concurrency:** `Reservation.revision` is incremented on each status update. `UpdateReservationStatus` enforces `WHERE revision = N` — a mismatch returns `ErrConflict`, preventing silent overwrites.

7. **Atomic Reschedule:** Rescheduling is not a status — it is a transactional operation: create new reservation + mark original as `RESCHEDULED` within a single DB transaction.

## 5. Identity Contract & RBAC

This module does **not** implement authorization or role-based access control. It operates under the following contract:

- `actorID` is a plain string — already authenticated and authorized by the caller.
- The transport adapter or middleware layer is responsible for verifying that the authenticated user has permission to perform the operation **before** the op handler runs (`router.Route.Requires(resource, action)` is where that gate is declared — see §7's Ops table).
- This module stores `actorID` as an audit field (`creator_user_id`, `updated_by`) only.
- **RBAC belongs to a separate IAM module.** Changes to roles or permissions require no changes to this module.

## 6. Event Publishing & Inter-Module Communication

This module communicates outbound via the injected `github.com/webtyp/events` `events.Publisher` —
**not** a self-declared interface. Before the harness migration this module declared its own local
`EventPublisher interface { Publish(ctx *tinyctx.Context, event string, payload any) error }`, which
duplicated the ecosystem's `events.Publisher` contract and additionally depended on
`github.com/webtyp/context` (not on this module's import whitelist — see `AGENTS.md`). After the
migration, `Deps.Publisher` is `events.Publisher` directly:

```go
type Publisher interface { Publish(e Event) } // github.com/webtyp/events
type Event struct { Topic string; Payload model.Encodable }
```

After each successful state mutation, the module publishes a domain event with a typed payload:

| Operation | Event constant |
|---|---|
| `CreateReservation` | `appointment.reservation.created` |
| `ChangeStatus` CONFIRM | `appointment.reservation.confirmed` |
| `ChangeStatus` CANCEL | `appointment.reservation.cancelled` |
| `ChangeStatus` COMPLETE | `appointment.reservation.completed` |
| `ChangeStatus` NO_SHOW | `appointment.reservation.no_show` |
| `ChangeStatus` EXPIRE | `appointment.reservation.expired` |
| Reschedule (original) | `appointment.reservation.rescheduled` |
| `UpsertWeeklyCalendar` / `AddException` / `RemoveException` | `appointment.schedule.changed` |

The `appointment.schedule.changed` event carries a `ScheduleChangedPayload{TenantId, StaffId}` and is
published whenever a staff member's schedule mutates. It implements both `model.Encodable` (required
by `events.Event.Payload`) and `model.Decodable`, so a wire-crossing broker (`webtyp/sse`) can serialize
it; an in-process broker delivers the concrete pointer without encoding. Consumers (e.g. a reservation
screen that recomputes free slots) subscribe and act when the event's `StaffId` matches their scope.

**Rules:**
- Event publishing is **fire-and-forget** — `events.Publisher.Publish` has no error return; a
  broker-side failure is the broker's concern, never the module's.
- Passing `nil` as `Deps.Publisher` safely disables events (useful in tests or CLI tools).
- The concrete broker (in-process, `github.com/webtyp/sse`, a queue adapter) is decided by the
  composition root, never by this module.

## 7. Transport, Identity, View — Composition Root

The module implements `router.OpModule` (`ModelName() string` + `MountOps(reg router.OpRegistry)`)
instead of `mcp.ToolProvider` — it never imports `tinywasm/mcp`. All 13 operations (6 reservation +
7 calendar) are registered by a single `*Module`; the transport adapter that harvests them (`mcp`
today, any future `router.OpRegistry`-satisfying transport tomorrow) is the composition root's
choice, not this module's.

### Ops (via `MountOps`)

| Op | Action | Resource | Description |
|---|---|---|---|
| `create_reservation` | `c` | `reservation` | Creates a new reservation (atomic reschedule if `RescheduledFromId` is set) |
| `get_reservation` | `r` | `reservation` | Gets a reservation by ID |
| `list_reservations_by_staff` | `r` | `reservation` | Lists reservations by staff ID and date range |
| `list_reservations_by_client` | `r` | `reservation` | Lists reservations by client ID |
| `change_reservation_status` | `u` | `reservation` | Changes a reservation status via FSM event |
| `expire_pending_reservations` | `u` | `reservation` | Expires unconfirmed pending reservations (called by an external scheduler) |
| `upsert_calendar_config` | `u` | `calendar` | Sets IANA timezone for a staff member |
| `upsert_weekly_calendar` | `u` | `calendar` | Sets weekly schedule for a staff member |
| `add_calendar_exception` | `c` | `calendar` | Adds a calendar exception for a specific date |
| `remove_calendar_exception` | `d` | `calendar` | Removes a calendar exception |
| `list_availability` | `r` | `calendar` | Lists available time slots for a staff member |
| `list_weekly_calendar` | `r` | `calendar` | Lists the raw weekly rows (for the schedule editor) |
| `list_exceptions` | `r` | `calendar` | Lists calendar exceptions in a date range (for the schedule editor) |

### View

`NewView(caller router.Caller, tenantId, staffId string) view.Presenter` builds a **list/select-only**
`view.Presenter` over `Reservation`, scoped to one staff member's schedule (backed by
`list_reservations_by_staff` — there is no unscoped "list all reservations for a tenant" operation,
so the view needs the staff id at construction time; this is the one deliberate deviation from the
bare `NewView(caller router.Caller)` shape used by simpler modules like `item_catalog`). It exposes no
`Saver`/`Deleter` capability: reservations are never edited as a whole record (they mutate only
through `ChangeReservationStatus`'s FSM-gated transitions) and are never hard-deleted, so
`view.WithSaveOp`/`view.WithDeleteOp` are intentionally omitted — a bare `Presenter` (list + select)
is the correct, complete shape here, not a gap.

**No second `view.Presenter` for calendar configuration, but an editor face exists.** `WorkCalendarConfig`
(one row per staff), `WorkCalendarWeekly` (at most 7 rows per staff), and `WorkCalendarException` (a
handful of one-off rows) are narrow configuration data, not a browsable list a user scrolls through
the way they browse a catalog or a reservation list — there is no natural "list op" a calendar view
would page over, and the `upsert_*`/`add_*`/`remove_*` ops cover every calendar mutation a UI needs
to drive directly. A `view.Presenter` list UI would be manufacturing a list for data that isn't
list-shaped; instead, this module exposes the schedule editor's face as a **caller-side typed client**:

- `list_weekly_calendar` + `list_exceptions` — the raw reads a `scheduleeditor` needs (previously
  internal to `Repository`).
- `NewScheduleClient(caller, tenantId, staffId)` — a caller-side typed client (`Weekly`, `Exceptions`,
  `SaveWeeklyRow`, `AddException`, `RemoveException`) over those ops plus the existing write ops. The
  app (e.g. `app-demo`) adapts it to a UI component's callbacks; this module never imports a renderer.

So a schedule editor screen is a **small, separate addition** — delivered here as `ScheduleClient`,
consumed by whatever UI the composition root chooses.

### Composition Root Example

```go
staffSvc     := staffmodule.New(db, staffmodule.Deps{IDs: idGen})            // implements StaffReader
directorySvc := directorymodule.New(db, directorymodule.Deps{IDs: idGen})   // implements DirectoryReader

// item_catalog implements the CatalogReader interface (ServiceExists) — see
// github.com/veltylabs/item_catalog.
catalogSvc, _ := itemcatalog.New(db, itemcatalog.Deps{
    IDs:       idGen,       // model.IDGenerator
    Publisher: eventBroker, // events.Publisher, nil disables publishing
})

// db is *orm.DB — shared with the rest of the monolith or module-specific.
scheduling, _ := appointmentbooking.New(db, appointmentbooking.Deps{
    Staff:     staffSvc,
    Catalog:   catalogSvc,   // *itemcatalog.Module satisfies CatalogReader
    Directory: directorySvc,
    IDs:       idGen,        // model.IDGenerator — unixid.NewUnixID() (or any generator) injected here, never constructed inside the module
    Publisher: eventBroker,  // events.Publisher, nil disables publishing
})

scheduling.MountOps(opRegistry)               // router.OpRegistry — mcp.HarvestOps(scheduling, ...) today
reservationsView := scheduling.NewView(caller, tenantId, staffId) // router.Caller -> view.Presenter
```

No module imports another directly — `appointment_booking` defines `StaffReader`, `CatalogReader`,
and `DirectoryReader`; the sibling modules satisfy them structurally with no import back into this
package.

## 8. Availability Calculation

Free slots are derived at query time from the intersection of:
- Weekly calendar rules (recurring working hours)
- One-off exceptions (override or subtract from weekly rules)
- Existing non-terminal reservations (block occupied intervals including buffer time)

Exception priority: `HOLIDAY` > `SPECIAL_HOURS` > `BLOCKED`.


Also see: [Composition Root Sequence Diagram](diagrams/sequence.md)

## 9. Related Documents

- [Database Diagram](diagrams/database.md)
- [FSM Diagram](diagrams/fsm.md)
- [Sequence Diagrams](diagrams/sequence.md) — ListAvailability, CreateReservation, ChangeReservationStatus, ExpirePendingReservations
- [Test Coverage Backlog](PLAN_TESTS_BACKUP.md) — independent, longer-lived list of missing test cases (UC-01…UC-20), not part of the harness migration
