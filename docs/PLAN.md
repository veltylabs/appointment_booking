---
PLAN: "refactor!: appointment_booking on the crud-by-declaration framework — Allow, Rules and ExternalCalls in definition.go, persistence by crud, only domain rules in rule files"
EXECUTOR: jules
REVIEWER: none
---

> Read AGENTS.md at the repo root before any change.
> **Finish by yourself:** when the delivery check passes, commit, push and open the pull request. Never ask whether to
> proceed or finalize — nobody answers, and a session that waits ends without your work. Commit after every stage.
> Never run `git reset`, `git restore`, `git checkout -- <file>` or `git stash` on your own work.
> **Publishing:** you publish only by OPENING THE PULL REQUEST (a `git push` from your sandbox does not reach GitHub). Open it after Stage 3 with whatever compiles and keep committing to it.
> **Never ask a yes/no question about progress**; ask only when a signature in this plan is wrong, and name the symbol.
> Never edit `definition_gen.go`: a build error there means `definition.go`, a rule file or a call file is wrong; fix that and run `definitionc` again.

# Plan: `appointment_booking` on the crud-by-declaration framework

Today 38 operations. D26: the 8 that only forward to `patient_directory` and `payment_manager` are deleted (the app's booking screen uses those modules' tables); the visitor's online flow is one public record, `online_reservation` orchestrated with `ExternalCalls` (D36, D58, D60). Actions with their own permission or that work in batch become records (D18): overbooking, expiring pending reservations, recomputing conflicts. `ReservationForm` is a screen form (not migrated): remove its `DB:` marks; the screen converts it to `Reservation`. The client never sends `creator_user_id` (D9): it is `a.UserID`.

Reference implementation of the same migration (read it first, copy its shape): `patient_directory` — `definition.go` (`Definition`, `Allow`, `Rules`), `rule_patient_create.go`/`rule_patient_update.go`/`rule_patient_search.go` (the rules), `store.go`, `readers.go` (signatures kept, bodies on `m.store`).

## Tools

```bash
go install webtyp.com/devflow/cmd/gotest@latest
go install webtyp.com/lang/cmd/langc@latest
go install webtyp.com/definitionc/cmd/definitionc@v0.1.43
go get webtyp.com/model@v0.2.20 webtyp.com/crud@v0.0.14 webtyp.com/orm@v0.12.33 webtyp.com/storage@v0.2.2 webtyp.com/ddl@v0.0.49 webtyp.com/router@v0.4.22 webtyp.com/definitionc@v0.1.43 webtyp.com/view@v0.6.74 webtyp.com/appkit@v0.2.36 webtyp.com/rpc@v0.2.21 webtyp.com/externalcalls@v0.2.0
```
Remove every `replace` from `go.mod`; `go mod tidy`.
`definitionc` is installed in `$(go env GOPATH)/bin`, which is not on the PATH of a fresh shell: write `export PATH="$PATH:$(go env GOPATH)/bin"` in the SAME command that runs it. **`definitionc` rewrites `go.mod` to older versions on every run: after EVERY run repeat the `go get` line above and `go mod tidy`.** If a module cannot be fetched, stop and say so in the PR; never write a hash by hand.
Before any change check, with `go doc`, that these exist: `model.ModuleDefinition`, `model.Rules` (`model.AddMyBusinessRule`, `model.HaveNoBusinessRule`), `model.ExternalCallDefinition`, `crud.CurrentUser`, `crud.RuleAnswer`, `crud.ApplyAndPublish`, `crud.ApplyWithoutPublishing`, `crud.Topic`, `crud.LoopbackActingAsWithQueue`, and that `definitionc` writes a `definition_gen.go` containing `func (m *Module) MountOperations(`. If one is missing, stop and say so in the PR.

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

A rule ends with `return crud.ApplyAndPublish(calls...), nil` (saves, announces, and enqueues external calls) or `crud.ApplyWithoutPublishing(calls...)` (saves and enqueues without announcing); a rejection is `return crud.RuleAnswer{}, err`. It queries with `tx`, never with `m.db`. To write another row it calls `tx.Create`/`tx.UpdateFields`. The old `Committed` hook does not exist.

**External calls (verified against `crud` v0.0.14).** A rule asks for a call with `return crud.ApplyAndPublish(<Entity><Call>{}), nil`; `definitionc` writes the type `<Entity><Call>` and the skeleton `call_<entity>_<call>.go`. The call is a **method on the stored record** `p`: it **sets fields on `p`** and returns their column names, `p.Status = StatusDone; return crud.Fields("status"), nil` (`crud.FieldsToSave` is `[]string`, never a map; `crud` saves exactly those columns). On failure return `nil, err`: `crud` marks the attempt failed in the queue and the record keeps its old values (a call cannot write a `failed` status; the reason lives in the queue row). A `WaitForReply` call runs inside the request. A `RetryInBackground` call runs later with an **empty `crud.CurrentUser{}`**: it never uses `a.UserID`. A module that declares calls needs a queue: the app passes `appkit.NewCallQueue(q)`; the tests pass the double of Stage 6.


## Tables, stores and rules

An action with a rule below is declared `model.AddMyBusinessRule` in the table's `Rules`; every other allowed action is `model.HaveNoBusinessRule`. A `Search` below is the rule of `Read`.

| Table (`<X>Model`) | Store | `Allow` | Rules (`rule_<entity>_<verb>.go`) & ExternalCalls |
|---|---|---|---|
| `EmployeeServiceConfig` | `orm.TenantStore` | `model.Create \| model.Read \| model.Update` | checks of `CreateEmployeeServiceConfig` / `UpdateEmployeeServiceConfig` |
| `WorkCalendarConfig` | `orm.TenantStore` | `model.Create \| model.Read \| model.Update` | checks of `UpsertCalendarConfig` |
| `WorkCalendarBlock` | `orm.TenantStore` | `model.Create \| model.Read \| model.Update \| model.Delete` | checks of `SaveDayBlocks` / `SaveDateBlocks` / `MarkWorkingDays` per row (inside working hours from `Deps.Hours`, no overlap within the staff/day). |
| `WorkCalendarException` | `orm.TenantStore` | `model.Create \| model.Read \| model.Delete` | checks of `AddCalendarException`. |
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

## Stages (do them in order; commit after each)

| Stage | File |
|---|---|
| 1 — `definition.go` | [PLAN_STAGE_1_definition_go.md](PLAN_STAGE_1_definition_go.md) |
| 2 — `module.go` | [PLAN_STAGE_2_module_go.md](PLAN_STAGE_2_module_go.md) |
| 3 — generate, then write the rules & external calls | [PLAN_STAGE_3_generate_then_write_the_rule.md](PLAN_STAGE_3_generate_then_write_the_rule.md) |
| 4 — readers used by other modules | [PLAN_STAGE_4_readers_used_by_other_module.md](PLAN_STAGE_4_readers_used_by_other_module.md) |
| 5 — browser side (D15, D16) | [PLAN_STAGE_5_browser_side.md](PLAN_STAGE_5_browser_side.md) |
| 6 — tests (`tests/`, through `crud.LoopbackActingAsWithQueue(nil, "u1", &callQueue{}, mod)`; rows in `docs/TESTS.md`) | [PLAN_STAGE_6_tests.md](PLAN_STAGE_6_tests.md) |
| 7 — translations and documentation (Spanish, veltylabs) | [PLAN_STAGE_7_translations_and_documentati.md](PLAN_STAGE_7_translations_and_documentati.md) |

## Delivery check

`gotest` passes; `langc sync .` clean; every grep of this plan empty. A tree that does not compile or whose tests did not run is a
failed delivery.
