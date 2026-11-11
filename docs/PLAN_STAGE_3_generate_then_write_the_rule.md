← [Stage 2](PLAN_STAGE_2_module_go.md) | Next → [Stage 4](PLAN_STAGE_4_readers_used_by_other_module.md)

> Read AGENTS.md and the index `PLAN.md` (rules, tables, external calls) first. Commit when this stage compiles.

## Stage 3 — generate, then write the rules & external calls

1. Delete `ops.go`, `crud.go`, `queries.go`, `model_crud.go`, `model_orm.go` and `definition_crud.go` (those that exist). Run `definitionc`: it writes one `definition_gen.go` (the types, schemas, external call type `OnlineReservationProcess`, and on `Module`, `ModelName()`, `Definition()`, `MountOperations()`) and the skeletons:
   - `rule_<entity>_<verb>.go` for every action declared `AddMyBusinessRule`.
   - `call_<entity>_<call>.go` for every call in `ExternalCalls`.
2. Add `store.go`: for each table of «Tables, stores and rules» that has a Store, `// Store<Entity> is the store of <entity>.` `func (m *Module) Store<Entity>() crud.Store { return m.store }`.
3. Fill each rule skeleton:
   - `OnlineReservation.Create`: holds slot, verifies no collision with `tx`, returns `crud.ApplyAndPublish(OnlineReservationProcess{})`.
   - `WorkCalendarBlock.Create/Delete`: checks block hours/overlap, returns `crud.ApplyAndPublish()`.
   - Rejection: `return crud.RuleAnswer{}, err`.
4. Fill `call_online_reservation_process.go` (the receiver `p` is the stored row; `FindPatient`/`CreatePatient`/`StartOnlinePayment` are the methods of `PatientDirectory` and `PaymentGateway` in `interfaces.go`; the queue id `key` makes the patient and payment ids idempotent):

```go
func (p *OnlineReservation) Process(a crud.CurrentUser, m *Module, key crud.IdempotencyKey) (crud.FieldsToSave, error) {
	id, _, _, _, found, err := m.deps.Patients.FindPatient(p.TenantId, p.DocumentNumber)
	if err != nil {
		return nil, err
	}
	if !found {
		if id, err = m.deps.Patients.CreatePatient(p.TenantId, string(key), p.DocumentNumber, p.Name, p.Phone, p.Email); err != nil {
			return nil, err
		}
	}
	p.PatientId = id
	p.Status = OnlineDone
	if m.deps.Payments == nil {
		return crud.Fields("patient_id", "status"), nil
	}
	redirectURL, _, _, err := m.deps.Payments.StartOnlinePayment(p.TenantId, string(key), ModelName, p.ReservationId, p.ReturnTo)
	if err != nil {
		return nil, err
	}
	p.PaymentId, p.PaymentUrl = string(key), redirectURL
	return crud.Fields("patient_id", "payment_id", "payment_url", "status"), nil
}
```

   If `m.deps.Patients` is nil the module's `New` already refuses to start with an online flow; do not add a nil check here.
Delete every `Op*` constant, `res*`/`Resource*` variable, every `op*` handler, every `view.MountSave`/`view.MountDelete`/`reg.Operation` call and the `webtyp.com/router`, `webtyp.com/rpc` imports that only they used (`grep -rn "reg.Operation\|view.MountSave\|view.MountDelete\|router.Context\|rpc.WriteError" --include=*.go . | grep -v _test` → empty).
