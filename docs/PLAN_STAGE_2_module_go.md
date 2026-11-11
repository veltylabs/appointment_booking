← [Stage 1](PLAN_STAGE_1_definition_go.md) | Next → [Stage 3](PLAN_STAGE_3_generate_then_write_the_rule.md)

> Read AGENTS.md and the index `PLAN.md` (rules, tables, external calls) first. Commit when this stage compiles.

## Stage 2 — `module.go`

`Deps` keeps its fields; add `IDs model.IDGenerator` (required, `depErr("IDs")` when nil) if it is missing.
`Module` gains `store *orm.Store`, built once in `New`: `m.store = orm.TenantStore(db, deps.TenantID, deps.IDs)`.
Delete the hand-written persistence (create/update/upsert/patch/delete/list/get bodies that only move rows) once its rules moved to the rule files: the row-moving bodies of every create/upsert/save/mark/unmark/add/remove/change/expire/recompute function (their checks move to the rules); every handler of the 8 deleted forwarding operations and their `*Args`/`*Result` definitions.
