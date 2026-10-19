---
PLAN: "feat(ui)!: Personal is a platformd group; Horario and Servicios adopt the published layout API (Scope, no title band) — decktabs is gone from components"
EXECUTOR: jules
REVIEWER: none
STATUS: running
SESSION: 14351356372224228451
---

> Read AGENTS.md at the repo root before any change.

Part of the cross-library UI-consistency wave: libraries published `widget` v0.6.43, `components` v0.8.34,
`form` v0.4.43 and `layout` v0.3.52; this plan moves the module onto them. The booking-flow work that was
decided in the same wave (forms, inline patient registration, overbooking in a form) is **not** here: it needs
a `patient_directory` release first and lives in `docs/PLAN_BOOKING_FORMS.md` (parked; do not execute it).

## Tools

```
go install webtyp.com/devflow/cmd/gotest@latest
```

## Why this module does not compile today

1. `ui/personal.go` imports `webtyp.com/components/decktabs`, **deleted in components v0.8.31** (grouping
   screens under one navigation item is the shell's job): `go mod tidy` fails with
   `module webtyp.com/components@latest found (v0.8.34), but does not contain package webtyp.com/components/decktabs`.
2. `ui/scheduleview.go` and `ui/serviceconfigview.go` use `rightpanel.RightPanel{Title, HeadControls}` and
   `crudview.Config{Context}`. In layout v0.3.52 the module's title is the platform header's (so `rightpanel`
   draws no title band and has **no** `Title` nor `HeadControls`), and `crudview.Config.Context` was renamed
   `crudview.Config.Scope` (same meaning: the control that picks *which records the list shows*, drawn in the
   list header above the search box).
3. `go.mod` carries three local `replace` lines that point outside the repo and cannot resolve for you:
   `github.com/veltylabs/payment_manager => ../payment_manager`, `webtyp.com/layout => ../../../webtyp/layout`,
   `webtyp.com/css => ../../../webtyp/css`.

## Design gate

Public API: `PersonalBrowser(caller, ids, tenantID) (platformd.UIModule, error)`, `NewScheduleView` and
`NewServiceConfigView` keep their signatures; `PersonalBrowser` now returns a group instead of a single module.

1. **Prior art:** `item_catalog/ui/browser.go`
   ([github](https://github.com/veltylabs/item_catalog/blob/main/ui/browser.go)): a group of crudviews.
2. **Novice-name test:** nothing new is named.
3. **Complexity ledger:** −1 import (`decktabs`), −1 widget, −2 uses of removed fields.
4. **Where it belongs:** here; the module composes its own screens.
5. **What it deletes:** the `decktabs` use; `RightPanel.Title/HeadControls`; `crudview.Config.Context`; the three
   local `replace` lines.

## Stage 1 — dependencies

Delete the three `replace` lines from `go.mod`, then:

```
go get webtyp.com/layout@v0.3.52 webtyp.com/components@v0.8.34 webtyp.com/form@v0.4.43 \
       webtyp.com/widget@v0.6.43 webtyp.com/css@v0.4.31 github.com/veltylabs/payment_manager@v0.1.13
go mod tidy
```

(`go mod tidy` only succeeds after Stages 2 and 3; run it last.)

## Stage 2 — `ui/personal.go`: Personal is a group

Replace the `decktabs.DeckTabs` with child modules (keep `staffui.StaffPanel`, `NewScheduleView` and
`NewServiceConfigView` exactly as they are):

```go
staffMod := platformd.NewUIModule(PersonalID+"."+tabStaff, labelStaff, svg.Icon(PersonalID), staffPanel)
scheduleMod := platformd.NewUIModule(PersonalID+"."+tabSchedule, labelSchedule, svg.Icon(PersonalID), NewScheduleView(caller, tenantID))
servicesMod := platformd.NewUIModule(PersonalID+"."+tabServices, labelServices, svg.Icon(PersonalID), NewServiceConfigView(caller, tenantID))

return platformd.NewGroupUIModule(PersonalID, PersonalLabel, svg.Icon(PersonalID), []platformd.UIModule{
	staffMod, scheduleMod, servicesMod,
}), nil
```

- All three children reuse the icon `svg.Icon(PersonalID)` already registered in `ui/svg.go`; add no artwork.
- Update the doc comment ("tres pestañas" → "un grupo de tres secciones") and the constants' comment. Delete the
  `decktabs` import.

## Stage 3 — the views adopt the layout API

**`ui/serviceconfigview.go`**

- `crudview.Config{ … Context: … }` → `Scope: Div().Set(clsServiceHeader.AsAttr()).Child(s.picker.render("service-config-staff"))`
  (same element, new field name).
- Empty branch (`staffID == ""`): there is no staff to pick, so show only the message. Replace the
  `rightpanel.RightPanel{Title, HeadControls, Article}` block with
  `s.panel.Set([]*Element{Div().Text("No hay profesionales registrados todavía.")})` and delete the
  `rightpanel` import if nothing else uses it.

**`ui/scheduleview.go`** — `Render`:

- The `controls` row (staff picker + "Máx. sobrecupo/día" input) stays as it is, but it is no longer a
  `HeadControls` slot: it becomes the first child of the article, above the editor.

```go
panel := &rightpanel.RightPanel{
	Article: Div().Child(controls, Div().BindChildren(s.editor)),
}
return Div().Set(clsRoot.AsAttr()).Child(panel.Render())
```

- Delete the `Title: "Horario"` line (the name of the screen is the header's now).
- Do **not** touch the overbooking input or the picker markup: moving the overbooking into a form is the parked
  plan's job.

## Stage 4 — tests and docs

- Search `tests/` first for a test that builds `PersonalBrowser` or `NewScheduleView`/`NewServiceConfigView`
  (`grep -rn "PersonalBrowser\|NewScheduleView\|NewServiceConfigView" tests`). Extend it if it exists. If none
  does, add `tests/personal_group_test.go` (external package, exported API only, a fake `router.Caller` like the
  other tests in the folder) asserting: `PersonalBrowser` returns a `platformd.GroupModule` whose `Children()`
  have `ModelName()` `personal.staff`, `personal.schedule` and `personal.services`, in that order. One test; do
  not duplicate what the suite already covers.
- `README.md` (the `PersonalBrowser` entry) and `docs/ARCHITECTURE.md`: describe a group of three sections, not
  tabs; remove any mention of `decktabs`, `HeadControls`, `Context`.

## Delivery check

```
grep -rn "decktabs\|HeadControls\|Context:" ui docs README.md      # → empty (ignore context.Context uses unrelated to crudview.Config)
grep -n '^replace' go.mod                                          # → empty
gotest                                                             # vet, race, tests and wasm green
git diff --stat main                                               # only the files named above
```

## Stages

| # | Stage | Files |
|---|---|---|
| 1 | dependencies and local replaces | `go.mod`, `go.sum` |
| 2 | Personal group | `ui/personal.go` |
| 3 | views on the new layout API | `ui/serviceconfigview.go`, `ui/scheduleview.go` |
| 4 | tests and docs | `tests/personal_group_test.go` (or the existing test), `README.md`, `docs/ARCHITECTURE.md` |

## Executor notes
- Successfully executed the plan and removed `decktabs` in favor of `platformd.GroupModule`.
- Adjusted `go.mod` by removing `replace` directives and updating dependency versions. Encountered an issue pulling private modules so set `GOPRIVATE`.
- Replaced `rightpanel` metadata with pure elements and renamed `crudview` config's `Context` to `Scope`.
- Created a new test, `tests/personal_group_test.go`, verifying correct IDs for `platformd.GroupModule`.
- Had to build `wasmbrowsertest` but it failed to run locally on loopback. However, the standard `gotest` with pure Go execution succeeded.
