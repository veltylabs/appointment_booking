← [Index](PLAN.md) | Next → [Stage 2](PLAN_STAGE_2_module_go.md)

> Read AGENTS.md and the index `PLAN.md` (rules, tables, external calls) first. Commit when this stage compiles.

## Stage 1 — `definition.go`

- Set `Allow` on each table exactly as in «Tables, stores and rules»; a table not listed there gets no `Allow` (internal).
- Mark `ServerOnly: true`: `tenant_id`, `created_at`, `updated_at`, and every field the server fills (`creator_user_id`, `actor_id`, `status` on create, `reservation_id`, `patient_id`, `payment_id`, `payment_url`, `failure`, counts).
- Delete every `*Args` / `*Result` definition that only carried a removed operation (`grep -n "ArgsModel\|ResultModel" model.go` → only the ones this plan keeps).
- New tables (add to `migrate/`): `ReservationOverbookingModel` (`reservation_overbooking`: id, tenant_id, the fields of `CreateOverbookingArgsModel`, `reservation_id`, `actor_id`, `created_at`), `OnlineReservationModel` (`online_reservation`: id, tenant_id, the fields of `CreateReservationArgsModel` a visitor sends incl. the patient's document, name, phone, email, plus `return_to`, `reservation_id`, `patient_id`, `payment_id`, `payment_url`, `status`, `failure`, `created_at`; constants in `const.go`: `OnlinePending = "pending"`, `OnlineDone = "done"`), `ReservationExpiryModel` (`reservation_expiry`: id, tenant_id, `expired` int, `actor_id`, `created_at`), `ConflictRecomputeModel` (`conflict_recompute`: id, tenant_id, `from`, `to`, `recomputed` int, `actor_id`, `created_at`). Server-filled fields `ServerOnly`.
- `RecomputeConflicts` stays a manual action (`conflict_recompute.create`): nothing in today's code runs it after a block or exception is saved, so no call is declared for them.
- Declare `ExternalCalls: []model.ExternalCallDefinition{...}` on:
  - `OnlineReservationModel`: `{Name: "Process", Mode: model.WaitForReply, MaxWaitSeconds: 10}`.
- Remove the `DB:` marks of `ReservationFormModel` and of every result/view definition the migration does not create (`TimeSlot`…).
- `ResourceBookingCatalog`, `ResourceReservationOnline`, `ResourceOverbooking` are deleted (each table/read model is its own resource; the app opens the public ones).
- `Deps.Payments` and `Deps.Patients` stay (used by `online_reservation`'s external call); the methods of those ports only the deleted operations used are removed from the port interfaces.
- **Keep every `*Args` model that an exported reader's signature still uses** (`grep -n "^func (m \*Module) [A-Z][A-Za-z]*([^)]*Args" *.go` lists them): mark each `Wire: true`; delete only the `*Args` no remaining signature uses.
- D52 (the entity is its own filter): a filter model (`*Filter`, `*FilterModel`, an `*ArgsModel` that only carried filter values) is not kept: its fields become fields of the read model or table (`OmitEmpty`), and the rule reads them from its receiver; delete the filter type and its `Wire`/`NewFilter` plumbing.
- Add `var Definition = model.ModuleDefinition{Name: "appointment_booking", Entities: []model.Definition{…every table and read model of this plan…}}` and, on every table and read model, `Rules` as «Tables, stores and rules» says (`AddMyBusinessRule` where a rule is named, `HaveNoBusinessRule` for the other allowed actions; a read model: `Read: model.AddMyBusinessRule`). Delete the hand topics (`Topic*`) and `Err*`/resources no code uses any more.
