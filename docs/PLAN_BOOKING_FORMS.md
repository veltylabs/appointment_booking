---
PLAN: "feat(ui): booking steps and overbooking use form.New; the patient is registered inline in step 4"
EXECUTOR: jules
REVIEWER: none
---

> **PARKED — not executable yet.** It becomes `docs/PLAN.md` when `docs/PLAN.md` (the layout migration) has
> closed **and** `veltylabs/patient_directory` has published a release whose models declare `Label` on every
> field that has a widget (prerequisite; see "Prerequisite"). Until then nobody executes this file.

> Read AGENTS.md at the repo root before any change.

## Tools

```
go install webtyp.com/devflow/cmd/gotest@latest
go install webtyp.com/lang/cmd/langc@latest
go install webtyp.com/ormc/cmd/ormc@latest
```

## Why

Rules of the UI-consistency wave that this module still breaks (the rules, in short, from
`webtyp/docs/UI_CONSISTENCY_DESIGN.md`): **a module never writes its own `<input>`** — fields are drawn by
`form.New(model)`; validation runs when the person leaves the field and on submit; every field has a `Label`; a
control that is a **value of the record** is a form field, not a loose control.

Today the booking flow draws its own inputs (`textField` and the `PartFlow*` field parts in
`ui/booking_flow.go`), the patient registration is a modal (`modaldialog`) in step 4, and the overbooking
number is a loose `<input type=number>` in `ui/scheduleview.go`.

## Prerequisite (another repo, another plan)

`patient_directory` v≥ the release that adds `Label: "RUT" | "Full name" | "Phone number" | "Email"` to the
`rut`, `name`, `phone` and `email` fields of `PatientModel`, `LinkPatientArgsModel` and
`FindPatientByRutArgsModel`. Without a `Label`, `form.New` raises a development diagnostic. Record the exact tag
here when it exists: `patient_directory@____`.

## Decisions already taken (do not reopen)

- **D5** — every form is `form.New` over an existing model. Step 4, counter: search = `FindPatientByRutArgs`
  with `SubmitLabel("Search")`; registration = `Patient` with `ShowField("rut", "name", "phone", "email")`;
  online = `LinkPatientArgs`.
- **D8** — the patient is registered **inline**: if the RUT does not exist, the registration form appears in
  the same step with the RUT already filled; the `modaldialog` of step 4 is deleted.
- **D15 (rest)** — "Máx. sobrecupo/día" becomes a field of a `form.New` over `WorkCalendarConfig`
  (`max_overbook_per_day`: widget `input.Number()`, `Label: "Max. overbooking per day"`), shown in the
  Horario screen under the staff picker; the loose `<input type=number>` and its `overbookVal` signal are deleted.

## Stages (to be detailed when unparked)

| # | Stage | Files |
|---|---|---|
| 1 | `WorkCalendarConfigModel`: widget + `Label` on `max_overbook_per_day` (regenerate with `ormc`); Horario uses `form.New` | `model.go`, `model_orm.go`, `ui/scheduleview.go` |
| 2 | step 4 counter: search form + inline registration form; delete the modal | `ui/step_patient.go` |
| 3 | step 4 online: `form.New(LinkPatientArgs)` | `ui/step_patient.go` |
| 4 | delete `textField` and the `PartFlowField/Check/Input` parts (grep-verifiable: `grep -rn "textField\|PartFlowInput" .` → empty) | `ui/booking_flow.go`, `ui/css.go` |
| 5 | translations: `langc sync`, translate the new keys | `config/lang.json` |

Each stage's exact edits are written when the prerequisite tag exists (versions must be exact).
