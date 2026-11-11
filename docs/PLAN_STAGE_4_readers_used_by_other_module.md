← [Stage 3](PLAN_STAGE_3_generate_then_write_the_rule.md) | Next → [Stage 5](PLAN_STAGE_5_browser_side.md)

> Read AGENTS.md and the index `PLAN.md` (rules, tables, external calls) first. Commit when this stage compiles.

## Stage 4 — readers used by other modules

Signatures of every exported reader stay (other modules and apps call them). Bodies read through `m.store` (`ReadOne`, `ReadAll` with `crud.Query`), never `m.db` directly; a `tenantID` parameter different from the module's tenant answers «not found» (`found=false` / empty, `err=nil`). Keep every exported function other modules or the app call (`CreateReservation`, `CreateOverbooking`, `ChangeReservationStatus`, `ListAvailability`, `ListBookableServices`, `ListServiceAvailability`, `ExpirePendingReservations`, `RecomputeConflicts`, `ListConflictingReservations`, `SaveDayBlocks`, `SaveDateBlocks`, `MarkWorkingDays`, `UnmarkWorkingDays`, the charge reader `payment_manager` uses); writers run the same rules inside `m.store.Atomic`.
