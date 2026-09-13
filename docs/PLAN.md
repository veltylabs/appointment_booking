---
PLAN: "refactor: deploy-time migrations and a form-capable booking surface"
EXECUTOR: jules
REVIEWER: none
---

> This plan is dispatched via the CodeJob workflow. See skill: **agents-workflow**.

# PLAN — execution queue for `appointment_booking`

> If you were told to "execute the plan described in docs/PLAN.md", execute
> **ALL the plans below, in order (top to bottom)**. Each plan is
> self-contained; finish one (its acceptance criteria green) before starting
> the next. Never mix changes from one plan into another.

| Order | Plan | Subject |
|-------|------|---------|
| 1 | [PLAN_MIGRATE_EXTRACTION.md](PLAN_MIGRATE_EXTRACTION.md) | Move schema creation out of `NewRepository` into a `migrate/` subpackage, so migrations are a deploy-time step and `webtyp.com/ddl` leaves the WASM build graph. |
| 2 | [PLAN_BOOKING_FORM_RECORD.md](PLAN_BOOKING_FORM_RECORD.md) | Add `NewFormView` — a booking surface that can both list and create — beside the existing list-only `NewView`. Additive; nothing in plan 1 is revisited. |

Plan 2 depends on plan 1 only for ordering hygiene (plan 1 touches
`repository.go`, plan 2 does not). Do not start plan 2 until plan 1's
acceptance criteria are green.

**Ignore these files.** `docs/PLAN_MODEL_MIGRATION.md`,
`docs/PLAN_REMOVE_TINYTIME_SHIM.md` and `docs/PLAN_TESTS_BACKUP.md` are
archived records of work already completed in this repository. They are not
part of this queue and must not be executed.

After completing both plans, run `gotest ./...` one final time: everything green.
