---
PLAN: "feat!: PatientDirectory uses patient_directory's six-verb names (GetPatient, FindPatient, CreatePatient)"
EXECUTOR: jules
REVIEWER: none
---

> Read AGENTS.md at the repo root before any change.
> **Finish by yourself:** when the delivery check passes, commit, push and open the pull request. Never ask whether to
> proceed or finalize — nobody answers, and a session that waits ends without your work.

# Plan — the booking asks for patients with the same names patient_directory exports

## Tools

```bash
go install webtyp.com/devflow/cmd/gotest@latest
```

## Why

`patient_directory` now exports only plain `<Verb><Entity>` methods (rule module-layout «Six verbs»). This module's
`PatientDirectory` (`interfaces.go`) asks for `PatientExists`, `FindPatient` (without `found`) and `RegisterPatient`,
which no longer exist there. Replace it with exactly:

```go
// PatientDirectory es la parte de patient_directory que este módulo usa.
// Ejemplo: id, name, phone, email, ok, err := p.FindPatient("t1", "12.345.678-5")
type PatientDirectory interface {
	GetPatient(tenantID, id string) (name, documentNumber, phone, email string, found bool, err error)
	FindPatient(tenantID, documentNumber string) (id, name, phone, email string, found bool, err error)
	CreatePatient(tenantID, id, documentNumber, name, phone, email string) (patientID string, err error)
}
```
Absent → `found == false`, `err == nil`. `CreatePatient` with a registered document → an error whose message is
`"patient rut already exists in this tenant"` (a `model.Conflict`; check it with `model.KindOf(err) == model.ErrorConflict`).

## Stage 1 — call sites

| Where | Today | Becomes |
|---|---|---|
| `service.go:768`, `service.go:917`, `import.go:42` | `ok, err := m.patients.PatientExists(t, id)` | `_, _, _, _, ok, err := m.patients.GetPatient(t, id)` |
| `ops.go:726` (`opFindBookingPatient`) | `id, name, phone, email, err := m.patients.FindPatient(...)` | add `found`; `!found` → `rpc.WriteError(ctx, ErrClientNotFound)` |
| `ops.go:754` (`opRegisterBookingPatient`) | `m.patients.RegisterPatient(...)` | `m.patients.CreatePatient(...)` (same args) |
| `ops.go:~771-810` (`opResolveOnlinePatient`) | find, register, re-find on error | body below |

```go
	id, name, phone, email, found, err := m.patients.FindPatient(args.TenantId, args.DocumentNumber)
	if err != nil {
		rpc.WriteError(ctx, err)
		return
	}
	if !found {
		id = args.Id
		if id == "" {
			id = m.ids.NewID()
		}
		if id, err = m.patients.CreatePatient(args.TenantId, id, args.DocumentNumber, args.Name, args.Phone, args.Email); err != nil {
			rpc.WriteError(ctx, err)
			return
		}
		name, phone, email = args.Name, args.Phone, args.Email
	}
	res := BookingPatient{Id: id, TenantId: args.TenantId, DocumentNumber: args.DocumentNumber, Name: name, Phone: phone, Email: email}
	if err := ctx.Encode(&res); err != nil {
		rpc.WriteError(ctx, err)
	}
```

## Stage 2 — fakes

`tests/mocks_test.go` (`MockDirectoryReader`) and `web/browser/main.go` (`fakePatientDirectory`): the three methods with
the new signatures (the fake finds "12.345.678-5"/"12345678-5" → `"pat-1","Juan Pérez","+56911223344","juan@example.com",true,nil`;
other → `found=false`). Tests that asserted "patient not found" from FindPatient now expect `ErrClientNotFound`.

## Stage 3 — docs (Spanish)

`README.md`/`docs/ARCHITECTURE.md`: `PatientDirectory{GetPatient, FindPatient, CreatePatient}`.

## Delivery check

1. `gotest` passes.
2. `grep -rn "PatientExists\|RegisterPatient" --include=*.go --include=*.md .` → empty.

Commit after every stage. Never edit `model_orm.go` by hand; a build error there means the hand-written code is wrong.
Never run `git reset`, `git restore`, `git checkout -- <file>` or `git stash` on your own work. Stuck 30 minutes: open
the PR anyway listing what fails. A PR that does not compile, or whose tests could not run, is a failed delivery.
Environment: `GOPRIVATE=webtyp.com/*,github.com/veltylabs/*`, `GOFLAGS=-mod=mod`, `go mod tidy` before `ormc`.
If a signature in the published tag differs from what this plan states, stop and name the symbol in the PR; never adapt it by guessing.

## Stages

| Stage | Files | Done when |
|---|---|---|
| 1 | `interfaces.go`, `service.go`, `import.go`, `ops.go` | compiles |
| 2 | `tests/mocks_test.go`, `web/browser/main.go` | tests pass |
| 3 | `README.md`, `docs/ARCHITECTURE.md` | done |
