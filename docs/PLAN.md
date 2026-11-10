---
PLAN: "refactor!: appointment_booking on the crud-by-declaration framework — Allow, Rules and ExternalCalls in definition.go, persistence by crud, only domain rules in rule files"
EXECUTOR: jules
REVIEWER: none
STATUS: running
SESSION: 8573121615622831321
---

> Read AGENTS.md at the repo root before any change.
> **Finish by yourself:** when the delivery check passes, commit, push and open the pull request. Never ask whether to
> proceed or finalize — nobody answers, and a session that waits ends without your work. Commit after every stage.
> Never run `git reset`, `git restore`, `git checkout -- <file>` or `git stash` on your own work.
> Never edit `definition_gen.go`: a build error there means `definition.go`, a rule file or a call file is wrong; fix that and run `definitionc` again.
> This plan is dispatched in the second wave of modules (D53), after `patient_directory` (the reference) was approved and `externalcalls` + `crud` D58 are published.

# Plan: `appointment_booking` on the crud-by-declaration framework

Today 38 operations. D26: the 8 that only forward to `patient_directory` and `payment_manager` are deleted (the app's booking screen uses those modules' tables); the visitor's online flow is one public record, `online_reservation` orchestrated with `ExternalCalls` (D36, D58, D60). Actions with their own permission or that work in batch become records (D18): overbooking, expiring pending reservations, recomputing conflicts. `ReservationForm` is a screen form (not migrated): remove its `DB:` marks; the screen converts it to `Reservation`. The client never sends `creator_user_id` (D9): it is `a.UserID`.

Reference implementation of the same migration (read it first, copy its shape): `patient_directory` — `definition.go` (`Definition`, `Allow`, `Rules`), `rule_patient_create.go`/`rule_patient_update.go`/`rule_patient_search.go` (the rules), `store.go`, `readers.go` (signatures kept, bodies on `m.store`).

## Tools

```bash
go install webtyp.com/devflow/cmd/gotest@latest
go install webtyp.com/lang/cmd/langc@latest
go install webtyp.com/definitionc/cmd/definitionc@v0.1.43
go get webtyp.com/model@v0.2.20 webtyp.com/crud@v0.0.14 webtyp.com/orm@v0.12.30 webtyp.com/storage@v0.2.1 webtyp.com/ddl@v0.0.49 webtyp.com/router@v0.4.21 webtyp.com/definitionc@v0.1.43 webtyp.com/view@v0.6.72 webtyp.com/appkit@v0.2.36 webtyp.com/rpc@v0.2.21 webtyp.com/externalcalls@v0.2.0
```
Remove every `replace` from `go.mod`; `go mod tidy`. Before any change check, with `go doc`, that these exist: `model.ModuleDefinition`, `model.Rules` (`model.AddMyBusinessRule`, `model.HaveNoBusinessRule`), `model.ExternalCallDefinition`, `crud.CurrentUser`, `crud.RuleAnswer`, `crud.ApplyAndPublish`, `crud.ApplyWithoutPublishing`, `crud.Topic`, `crud.Loopback`, and that `definitionc` writes a `definition_gen.go` containing `func (m *Module) MountOperations(`. If one is missing, stop and say so in the PR.
Prerequisite of the whole wave: `patient_directory` was executed, reviewed and approved by the owner; it is the reference and the 17 other modules are dispatched together after it.

## Lessons of the reference module (`patient_directory`, executed locally)

- Needs `crud` ≥ v0.0.14 (a filter has no required field) and `definitionc` ≥ v0.1.43. `Search(a, filter *X, text, m)`: a text of `""` is normal (the rule runs on every read).
- Running `definitionc` rewrites `go.mod` to older versions: run the `go get` list of «Tools» again after it, then `go mod tidy`; raise stale indirect dependencies (`rbac`, `mcp`, `appkit`) to the published ones.
- In tests, events reach the publisher given to `crud.LoopbackActingAs(pub, "u1", mod)`, not `Deps.Publisher`; a caller double takes `router.Operation`; a `ddl.Execer` double returns `(storage.Result, error)`; a rule that rejects answers `return crud.RuleAnswer{}, err` (assert the refusal and that nothing was stored).
- `langc sync .` can warn that other languages are untranslated: that is the repo's `lang.json`, not this plan.

## The rule of this plan (decisions D28–D50, D58, D60 of the architecture master)

The framework persists; the module declares and writes only domain rules.

| The module no longer writes | Who does it now |
|---|---|
| id, `tenant_id`, `created_at`/`updated_at`, the transaction loop, create/update/delete bodies, «not found», batch atomicity, idempotent retry by id, paging by `After`, `tenant_id` in every query, schema checks repeated from the model | `crud` + `orm.TenantStore` / `orm.GlobalStore` |
| an operation per verb (`list_*`, `save_*`, `upsert_*`, `delete_*`), its handler, `ops.go`, the permission resources and the event topics | `Allow` + `Rules` in `definition.go`; `definitionc` writes `MountOperations`; `crud` names the operation (`router.Operation{Module, Entity, Action}`; on the wire `GET\|POST\|PATCH\|DELETE /api/<module>/<entity>`, in MCP `<module>.<entity>.read\|create\|update\|delete`) and publishes `<module>.<entity>.created\|updated\|deleted` with a `*crud.Change` after the commit |
| an action with its own permission (approve, settle, receive stock…) | a **record created** in its own table, or a **read model** with its own name (D18) |
| external system calls, outbox tables, retry loops | `Definition.ExternalCalls` + `crud.ApplyAndPublish(calls...)`; `crud` records the call within `tx` and orchestrates execution (`WaitForReply`, `RetryInBackground`, `WaitForConfirmation`) |

A module writes: `definition.go` (`var Definition = model.ModuleDefinition{…}` and, per table, `Allow`, `Rules`, and `ExternalCalls`), one `rule_<entity>_<verb>.go` for every action declared `model.AddMyBusinessRule` (created once by `definitionc`; the skeleton does not compile until completed), one `call_<entity>_<call>.go` for every external call declared, and `store.go` (`func (m *Module) Store<Entity>() crud.Store`, one line per table). Every allowed action that has no rule below is declared `model.HaveNoBusinessRule`: `crud` alone checks it. Read the reference first: `patient_directory` (`definition.go`, `rule_patient_*.go`, `store.go`).

| Rule | Form (the receiver is the entity) |
|---|---|
| Create | `func (p *X) Create(a crud.CurrentUser, tx crud.Tx, m *Module) (crud.RuleAnswer, error)` |
| Update | `func (next *X) Update(a crud.CurrentUser, tx crud.Tx, old *X, fields []string, m *Module) (crud.RuleAnswer, error)` (`next` carries only the sent `fields`) |
| Delete | `func (p *X) Delete(a crud.CurrentUser, tx crud.Tx, m *Module) (crud.RuleAnswer, error)` (`p` is the stored row) |
| Search (Read of a table) | `func (p *X) Search(a crud.CurrentUser, filter *X, text string, m *Module) ([]storage.Condition, error)` — AND conditions only; it runs on every read, `text` may be `""`, and `filter` is a record of the entity with only the fields the caller sent (D52: the entity is its own filter) |
| Read (model without a table) | `func (p *X) Read(a crud.CurrentUser, filter *X, args crud.ReadArgs, m *Module) (model.ModelSlice, error)` (`filter` carries the fields the caller sent, D52) |
| External Call | `func (p *X) CallName(a crud.CurrentUser, m *Module, key crud.IdempotencyKey) (crud.FieldsToSave, error)` |

A rule ends with `return crud.ApplyAndPublish(calls...), nil` (saves, announces, and enqueues external calls) or `crud.ApplyWithoutPublishing(calls...)` (saves and enqueues without announcing); a rejection is `return crud.RuleAnswer{}, err`. It queries with `tx`, never with `m.db`. To write another row it calls `tx.Create`/`tx.UpdateFields`. The old `Committed` hook does not exist: external calls are declared in `Definition.ExternalCalls` and orchestrated by `crud`.

## Tables, stores and rules

An action with a rule below is declared `model.AddMyBusinessRule` in the table's `Rules`; every other allowed action is `model.HaveNoBusinessRule`. A `Search` below is the rule of `Read`.

| Table (`<X>Model`) | Store | `Allow` | Rules (`rule_<entity>_<verb>.go`) & ExternalCalls |
|---|---|---|---|
| `EmployeeServiceConfig` | `orm.TenantStore` | `model.Create \| model.Read \| model.Update` | checks of `CreateEmployeeServiceConfig` / `UpdateEmployeeServiceConfig` |
| `WorkCalendarConfig` | `orm.TenantStore` | `model.Create \| model.Read \| model.Update` | checks of `UpsertCalendarConfig` |
| `WorkCalendarBlock` | `orm.TenantStore` | `model.Create \| model.Read \| model.Update \| model.Delete` | checks of `SaveDayBlocks` / `SaveDateBlocks` / `MarkWorkingDays` per row (inside working hours from `Deps.Hours`, no overlap within the staff/day). ExternalCall: `RecomputeConflicts` (`RetryInBackground`, `MaxAttempts: 3`) |
| `WorkCalendarException` | `orm.TenantStore` | `model.Create \| model.Read \| model.Delete` | checks of `AddCalendarException`. ExternalCall: `RecomputeConflicts` (`RetryInBackground`, `MaxAttempts: 3`) |
| `Reservation` | `orm.TenantStore` | `model.Create \| model.Read \| model.Update` | `Create`: logic of `CreateReservation` (slot free in this transaction — no double booking), `creator_user_id` = `a.UserID`; `Update`: only `["status"]` with the transitions of `ChangeReservationStatus` |
| `ReservationOverbooking` (**new table** `reservation_overbooking`) | `orm.TenantStore` | `model.Create \| model.Read` | own permission (was `ResourceOverbooking`): `Create` runs the logic of `CreateOverbooking` and writes the `Reservation` with `tx.Create`; stores `reservation_id` |
| `OnlineReservation` (**new table** `online_reservation`) | `orm.TenantStore` | `model.Create \| model.Read` | public (the app lists it in `AppServer.Public` with Create and Read, D23): `Create` holds the slot in transaction, status `pending`; returns `crud.ApplyAndPublish(OnlineReservationProcess{})`. ExternalCall: `Process` (`WaitForReply`, `MaxWaitSeconds: 10`) resolves patient through `Deps.Patients` and starts online payment through `Deps.Payments`, updating `patient_id`, `payment_id`, `payment_url`, `status` |
| `ReservationExpiry` (**new table** `reservation_expiry`) | `orm.TenantStore` | `model.Create \| model.Read` | `Create`: runs `ExpirePendingReservations(tenant, now)` with `tx` and stores `expired` |
| `ConflictRecompute` (**new table** `conflict_recompute`) | `orm.TenantStore` | `model.Create \| model.Read` | `Create`: runs `RecomputeConflicts(tenant, from, to)` with `tx` and stores `recomputed` |

## Read models (D18 — no table, `Allow: model.Read` only, `Rules.Read: model.AddMyBusinessRule`)

| Model | Filter | Answered by |
|---|---|---|
| `BookableService` (existing) — public by the app | `BookableFilter{specialty string}` | `ListBookableServices` |
| `BookableSlot` (existing) — public by the app | `ServiceAvailabilityFilter` (from `ListServiceAvailabilityArgsModel`) | `ListServiceAvailability` |
| `TimeSlot` (existing; no `DB:` marks) | `AvailabilityFilter{staff_id, config_id string; from, to int64}` | `ListAvailability` |
| `DayLimits` (the result of `get_day_limits` today) | `DayLimitsFilter{date int64}` | its handler body (through `Deps.Hours`) |
| `ConflictingReservation` (existing) | `ConflictFilter{staff_id string; from int64}` | `ListConflictingReservations` |
| `StaffOption` (the rows of `list_staff_options`) | none | its handler body (through `Deps.Staff`) |
| `ClosureOption` (the rows of `list_calendar_closures`) | none | its handler body (through `Deps.Hours`) |

A read model is a `model.Definition` **without** `DB:` marks, with `Allow: model.Read` and `Rules: model.Rules{Read: model.AddMyBusinessRule}`, listed in `Definition.Entities`; `definitionc` mounts it and creates its `rule_<model name>_read.go`. Its permission resource is its own `ModelName`: list it in the README permissions table (roles must be granted it).

## Operation mapping (every operation of today has exactly one row)

«After» is `router.Operation{Module, Entity, Action}` written `<entity>.<verb>`; the wire form is `crud`'s (REST `/api/<module>/<entity>` by method, MCP `<module>.<entity>.<verb>`).

| Today | After |
|---|---|
| `list_bookable_services` | read model `BookableService` (public) |
| `list_service_availability` | read model `BookableSlot` (public) |
| `create_online_reservation + resolve_online_patient + start_booking_online_payment` | `online_reservation.create` (public) — one request |
| `create_reservation` | `reservation.create` |
| `create_overbooking` | `reservation_overbooking.create` |
| `get_reservation / list_reservations_by_staff / list_reservations_by_client` | `reservation.read` (with `id`, or filter `{staff_id}` / `{client_id}`) |
| `change_reservation_status` | `reservation.update` with `["status"]` |
| `expire_pending_reservations` | `reservation_expiry.create` |
| `get_calendar_config / upsert_calendar_config` | `work_calendar_config.read`, `work_calendar_config.create`, `work_calendar_config.update` |
| `save_day_blocks / save_date_blocks` | the screen sends `work_calendar_block.delete` for removed blocks and `_c` (batch) for new ones — two requests; a failure between them is shown and retried |
| `mark_working_days` | `work_calendar_block.create` batch: the rows `MarkWorkingDays` writes today, one per date |
| `unmark_working_days` | `work_calendar_block.delete` of those rows |
| `list_blocks` | `work_calendar_block.read` with filter `{staff_id}` |
| `get_day_limits` | read model `DayLimits` |
| `add_calendar_exception / remove_calendar_exception / list_exceptions` | `work_calendar_exception.create`, `work_calendar_exception.delete`, `work_calendar_exception.read` |
| `list_availability` | read model `TimeSlot` |
| `list_conflicting_reservations` | read model `ConflictingReservation` |
| `recompute_conflicts` | `conflict_recompute.create` |
| `create/get/list_by_staff/update employee_service_config` | `employee_service_config.create`, `employee_service_config.read`, `employee_service_config.update` |
| `list_staff_options` | read model `StaffOption` |
| `list_calendar_closures` | read model `ClosureOption` |
| `find_booking_patient, register_booking_patient` | **deleted** — the app's screen uses `patient_directory` (D26) |
| `get_booking_charge, create_booking_counter_payment, start_booking_terminal_payment, refresh_booking_terminal_payment` | **deleted** — the app's screen uses `payment_manager` (D26) |

## Stage 1 — `definition.go`

- Set `Allow` on each table exactly as in «Tables, stores and rules»; a table not listed there gets no `Allow` (internal).
- Mark `ServerOnly: true`: `tenant_id`, `created_at`, `updated_at`, and every field the server fills (`creator_user_id`, `actor_id`, `status` on create, `reservation_id`, `patient_id`, `payment_id`, `payment_url`, `failure`, counts).
- Delete every `*Args` / `*Result` definition that only carried a removed operation (`grep -n "ArgsModel\|ResultModel" model.go` → only the ones this plan keeps).
- New tables (add to `migrate/`): `ReservationOverbookingModel` (`reservation_overbooking`: id, tenant_id, the fields of `CreateOverbookingArgsModel`, `reservation_id`, `actor_id`, `created_at`), `OnlineReservationModel` (`online_reservation`: id, tenant_id, the fields of `CreateReservationArgsModel` a visitor sends incl. the patient's document, name, phone, email, plus `reservation_id`, `patient_id`, `payment_id`, `payment_url`, `status`, `failure`, `created_at`), `ReservationExpiryModel` (`reservation_expiry`: id, tenant_id, `expired` int, `actor_id`, `created_at`), `ConflictRecomputeModel` (`conflict_recompute`: id, tenant_id, `from`, `to`, `recomputed` int, `actor_id`, `created_at`). Server-filled fields `ServerOnly`.
- Declare `ExternalCalls: []model.ExternalCallDefinition{...}` on:
  - `OnlineReservationModel`: `{Name: "Process", Mode: model.WaitForReply, MaxWaitSeconds: 10}`.
  - `WorkCalendarBlockModel`: `{Name: "RecomputeConflicts", Mode: model.RetryInBackground, MaxAttempts: 3}`.
  - `WorkCalendarExceptionModel`: `{Name: "RecomputeConflicts", Mode: model.RetryInBackground, MaxAttempts: 3}`.
- Remove the `DB:` marks of `ReservationFormModel` and of every result/view definition the migration does not create (`TimeSlot`…).
- `ResourceBookingCatalog`, `ResourceReservationOnline`, `ResourceOverbooking` are deleted (each table/read model is its own resource; the app opens the public ones).
- `Deps.Payments` and `Deps.Patients` stay (used by `online_reservation`'s external call); the methods of those ports only the deleted operations used are removed from the port interfaces.
- **Keep every `*Args` model that an exported reader's signature still uses** (`grep -n "^func (m \*Module) [A-Z][A-Za-z]*([^)]*Args" *.go` lists them): mark each `Wire: true`; delete only the `*Args` no remaining signature uses.
- D52 (the entity is its own filter): a filter model (`*Filter`, `*FilterModel`, an `*ArgsModel` that only carried filter values) is not kept: its fields become fields of the read model or table (`OmitEmpty`), and the rule reads them from its receiver; delete the filter type and its `Wire`/`NewFilter` plumbing.
- Add `var Definition = model.ModuleDefinition{Name: "appointment_booking", Entities: []model.Definition{…every table and read model of this plan…}}` and, on every table and read model, `Rules` as «Tables, stores and rules» says (`AddMyBusinessRule` where a rule is named, `HaveNoBusinessRule` for the other allowed actions; a read model: `Read: model.AddMyBusinessRule`). Delete the hand topics (`Topic*`) and `Err*`/resources no code uses any more.

## Stage 2 — `module.go`

`Deps` keeps its fields; add `IDs model.IDGenerator` (required, `depErr("IDs")` when nil) if it is missing.
`Module` gains `store *orm.Store`, built once in `New`: `m.store = orm.TenantStore(db, deps.TenantID, deps.IDs)`.
Delete the hand-written persistence (create/update/upsert/patch/delete/list/get bodies that only move rows) once its rules moved to the rule files: the row-moving bodies of every create/upsert/save/mark/unmark/add/remove/change/expire/recompute function (their checks move to the rules); every handler of the 8 deleted forwarding operations and their `*Args`/`*Result` definitions.

## Stage 3 — generate, then write the rules & external calls

1. Delete `ops.go`, `crud.go`, `queries.go`, `model_crud.go`, `model_orm.go` and `definition_crud.go` (those that exist). Run `definitionc`: it writes one `definition_gen.go` (the types, schemas, external call types `OnlineReservationProcess`, `WorkCalendarBlockRecomputeConflicts`, and on `Module`, `ModelName()`, `Definition()`, `MountOperations()`) and the skeletons:
   - `rule_<entity>_<verb>.go` for every action declared `AddMyBusinessRule`.
   - `call_<entity>_<call>.go` for every call in `ExternalCalls`.
2. Add `store.go`: for each table of «Tables, stores and rules» that has a Store, `// Store<Entity> is the store of <entity>.` `func (m *Module) Store<Entity>() crud.Store { return m.store }`.
3. Fill each rule skeleton:
   - `OnlineReservation.Create`: holds slot, verifies no collision with `tx`, returns `crud.ApplyAndPublish(OnlineReservationProcess{})`.
   - `WorkCalendarBlock.Create/Delete`: checks block hours/overlap, returns `crud.ApplyAndPublish(WorkCalendarBlockRecomputeConflicts{})`.
   - Rejection: `return crud.RuleAnswer{}, err`.
4. Fill each `call_<entity>_<call>.go` skeleton:
   - `OnlineReservation.Process(a crud.CurrentUser, m *Module, key crud.IdempotencyKey) (crud.FieldsToSave, error)`:
     1) calls `m.deps.Patients` to find or create the patient.
     2) if `m.deps.Payments != nil`: starts payment via `m.deps.Payments.StartOnlinePayment(...)`.
     On success: `return crud.FieldsToSave{"patient_id": patientID, "payment_id": paymentID, "payment_url": paymentURL, "status": "done"}, nil`. If `Payments` is nil: `status = "done"`.
     On failure: `return crud.FieldsToSave{"status": "failed", "failure": err.Error()}, err`.
   - `WorkCalendarBlock.RecomputeConflicts(a crud.CurrentUser, m *Module, key crud.IdempotencyKey) (crud.FieldsToSave, error)`: runs `m.RecomputeConflicts(...)` for affected range.
Delete every `Op*` constant, `res*`/`Resource*` variable, every `op*` handler, every `view.MountSave`/`view.MountDelete`/`reg.Operation` call and the `webtyp.com/router`, `webtyp.com/rpc` imports that only they used (`grep -rn "reg.Operation\|view.MountSave\|view.MountDelete\|router.Context\|rpc.WriteError" --include=*.go . | grep -v _test` → empty).

## Stage 4 — readers used by other modules

Signatures of every exported reader stay (other modules and apps call them). Bodies read through `m.store` (`ReadOne`, `ReadAll` with `crud.Query`), never `m.db` directly; a `tenantID` parameter different from the module's tenant answers «not found» (`found=false` / empty, `err=nil`). Keep every exported function other modules or the app call (`CreateReservation`, `CreateOverbooking`, `ChangeReservationStatus`, `ListAvailability`, `ListBookableServices`, `ListServiceAvailability`, `ExpirePendingReservations`, `RecomputeConflicts`, `ListConflictingReservations`, `SaveDayBlocks`, `SaveDateBlocks`, `MarkWorkingDays`, `UnmarkWorkingDays`, the charge reader `payment_manager` uses); writers run the same rules inside `m.store.Atomic`.

## Stage 5 — browser side (D15, D16)

- `view.go` (`//go:build browser`): every `view.NewCallerLister(caller, view.Ops{…}, newList)` takes
  `func() model.TableSlice { return &<Entity>List{} }` and `view.Ops` has no `Allow` (the screen reads `<Entity>List.Allow()`).
- `module/browser.go`, `module/server.go` (if present): `appkit.ModuleBrowser`/`appkit.BrowserDeps` → `view.ModuleBrowser`/`view.BrowserDeps`.
- Demo: `seed/demo.go` (`//go:build wasm`) returning `view.Demo{Label, Timezone: "America/Santiago", ActingAs: "u1",
  Modules: []crud.Module{mod}, Migrations: <the module's ddl.Step list, or nil if it has no migrations>,
  Screens: func(d view.BrowserDeps) []view.Screen { return ui.Screens(<what today's demo main passes>) }}`, built with
  `orm.New(mem.New())`, `New(db, Deps{…})` with the same fakes today's demo uses, and `seed.Load` (when the module has one).
  Delete the old demo: `web/browser/main.go` (`test ! -e web`).
- `grep -rn "webtyp.com/layout\|appkit.BrowserDeps\|appkit.ModuleBrowser\|router/loopback" --include=*.go . | grep -v "^./ui/"` → empty. `ui/` keeps `webtyp.com/layout/crudview` and `layout/rightpanel` until phase L6 of the architecture master; do not touch them here.

## Stage 6 — tests (`tests/`, through `crud.LoopbackActingAs(nil, "u1", mod)`; rows in `docs/TESTS.md`)

Delete tests of removed code. Operations are `router.Operation{Module: ModelName, Entity: "<entity>", Action: model.<Action>}`, never a string; fixtures follow `crud` v0.0.14 (`reg.AddStoredEntity`, `crud.CurrentUser`). Add `TestMount_MatchesDefinition` (`crud.Mount(reg, nil, nil, mod)` returns nil) and one test that an allowed write publishes a `*crud.Change` on `crud.Topic(ModelName, "<entity>", model.<Action>)`. Required:

| Test | Setup → call → expected |
|---|---|
| `TestReservation_NoDoubleBooking` | two reservations for the same staff and slot → second rejected; one row |
| `TestReservation_CreatorFromCaller` | create with `creator_user_id` from the client → `ServerOnly` error; without → creator = caller |
| `TestReservation_StatusTransitions` | an invalid transition → field error on `status` |
| `TestOverbooking_OwnPermission` | `reservation_overbooking_c` creates a reservation over capacity; `reservation_c` for the same slot is rejected |
| `TestOnline_HoldThenProcess` | public `online_reservation_c` (no session) → slot held; external call runs: fake patients and payments ports called; record has `patient_id`, `payment_url`, status done |
| `TestOnline_WithoutPaymentsPort` | `Deps.Payments` nil → record `done`, `patient_id` set, `payment_url` empty |
| `TestOnline_PaymentFailureRecorded` | fake payments fails → status failed, slot stays held |
| `TestForwardingOperationsGone` | `create_booking_counter_payment`, `find_booking_patient` → unknown operation |
| `TestExpiry_RecordsCount` | two pending reservations past their limit; `reservation_expiry_c` → `expired = 2` |
| `TestDemo_Valid` (`//go:build wasm`) | `seed.Demo()` → no error, `Validate()` nil, at least one screen |

## Stage 7 — translations and documentation (Spanish, veltylabs)

`langc sync .` (zero pending, zero reuse warnings). `README.md` (≤150 lines, 4 mandatory sections; «Sin/Con»: hand-written
operations vs `Allow` + rules) with the operations table of the mapping above. `docs/ARCHITECTURE.md`: rules vs persistence.
Translate to Spanish every README, `docs/*`, diagram and code comment still in English; exported symbols without a usage example get one.

## Delivery check

`gotest` passes; `langc sync .` clean; every grep of this plan empty. A tree that does not compile or whose tests did not run is a
failed delivery.
