---
PLAN: "feat!: list rows declared on the model fields (Row/Options); Item() removed; screens as data"
EXECUTOR: jules
REVIEWER: none
STATUS: running
SESSION: 8484937785685557193
---

> Read AGENTS.md at the repo root before any change.

<!-- Dispatch only after webtyp.com/model v0.2.7, webtyp.com/view v0.6.47 and webtyp.com/layout v0.3.78 are published. -->

# Plan — `appointment_booking`: what is saved shows everywhere

## Why

`webtyp.com/view` v0.6.47 keeps one shared copy of each record per page and builds every list row from the
record's fields (`model.RowOf`, `webtyp.com/model` v0.2.7), so what a person saves shows at once in every list and
select that displays it. The hand-written `Item()` methods and `view.Itemizer` are gone: each listed model marks on
its fields where they show in the row (`Row: model.RowTitle` / `RowDetail` / `RowBadge`), and a select says which
list fills it with `Ref` (foreign key) or `Options` (no foreign key). `view.Lookup` is gone too. Breaking change,
no compatibility layer.

## Tools

```
go install webtyp.com/devflow/cmd/gotest@latest
go install webtyp.com/lang/cmd/langc@latest
go install webtyp.com/ormc/cmd/ormc@latest
go get webtyp.com/model@v0.2.7
go get webtyp.com/view@v0.6.47
go get webtyp.com/layout@v0.3.78
```

If a tag lacks a symbol of «APIs used», stop and say so in the PR.

## Ecosystem search

Nothing new is created: `model.RowSlot`, `Field.Row`, `Field.Options`, `model.RowOf` (model v0.2.7) and
`view.Records` (view v0.6.47) are reused.

## Design gate

1. **Prior art.** Laravel Nova (`->showOnIndex()` on the field), Strapi «main field», Payload `useAsTitle`:
   the row is declared on the fields.
2. **Novice-name test.** `Row: model.RowTitle` — «in the row, this is the title».
3. **Complexity ledger.** −3 `Item()` method(s); +1 `Row:` per shown field.
4. **Where it belongs.** The row is a fact about the model (`model.go`).
5. **What it deletes.** Every `Item()` below, and every `…Browser` of `ui/`.

## APIs used

| Symbol | Exact signature | Tag | File |
|---|---|---|---|
| `model.Field.Row` | `Row RowSlot`; constants `model.RowTitle`, `model.RowDetail`, `model.RowBadge` | model v0.2.7 | `field.go`, `row.go` |
| `model.Field.Options` | `Options *Definition` (a select fed by another list without a foreign key) | model v0.2.7 | `field.go` |
| `model.RowOf` | `func RowOf(f Fielder) Row`; `Row{ID, Title, Detail, Badge string}`; several `RowDetail` join with `" · "` in declaration order | model v0.2.7 | `row.go` |
| `view.NewRecords` / `(*Records).Caller` | `func NewRecords() *Records`; `func (r *Records) Caller(c router.Caller) router.Caller` — `view.NewCallerLister` panics with any other caller | view v0.6.47 | `records.go` |
| `view.Screen` | leaf: `Presenter` **or** `Component dom.Component` (+ `Help`); no `Lookups` | view v0.6.47 | `screen.go` |

If a signature in the published tag differs from this table, stop and name the symbol in the PR; never adapt it by guessing.

## Stage 0 — screens as data (`ui/`), replacing the cancelled plan

The previous `docs/PLAN.md` of this repo (dispatched 2026-10-27, cancelled by the owner) flattened this module's own
screens into plain lists. **Do not do that.** Every screen keeps what it shows today:

- Replace every exported `…Browser(...) (platformd.UIModule, error)` in `ui/` with ONE exported
  `func Screens(caller router.Caller, ids model.IDGenerator, tenantID string) []view.Screen` returning one
  `view.Screen` per former `…Browser`, with the same `ID`, label and icon it has today.
- A screen that today is a plain `crudview` over this module's presenter with no custom `crudview.Config` fields →
  a list leaf: `Presenter: <the presenter>`, `Permission: <resource>`, help via `view.WithDescription(...)`.
- Every other screen (a wizard, a chat, a `crudview` with a custom `List`/`Filter`/`NewRecord`/`Scope`, anything
  built with `rightpanel`) → a component leaf: `Component: <the same component it builds today>`,
  `Permission: <resource>`, `Help: <lang.Text>`. It may keep importing `webtyp.com/layout/chatview`,
  `webtyp.com/layout/crudview` or `webtyp.com/layout/rightpanel` to build that component (the one accepted
  exception); it must **not** import `webtyp.com/layout/platformd`.
- `Permission` of each screen = the resource the main list operation it calls `Requires` (read `ops.go`); name the
  pairs in the PR.
- `Help`: reuse the module's existing description text when there is one; otherwise write one English sentence
  saying what the screen is for, as a `lang.Text` constant, translated in `lang.json`.
- `web/browser/main.go` (demo): build the menu with `layout/app.BuildMenu(ui.Screens(...), ids, allowAll)` and pass
  `Menu.CanView` to the `platformd.Platform` (a `nil` CanView shows nothing since layout v0.3.77); `allowAll` is a
  demo func returning true.
- Check: `grep -rn "layout/platformd" --include=*.go . | grep -v web/` → empty.

## Stage 1 — mark the row on the fields (`model.go`), then regenerate

These are the `Item()` methods of this repo today (`view.go`), verbatim:

```go
func (c *EmployeeServiceConfig) Item() view.Item {
	return view.Item{ID: c.Id, Label: c.ServiceId, Description: c.StaffId}
}

func (r *Reservation) Item() view.Item {
	desc := reservationStanding(r.Status, r.Origin)
	if r.Overbooked {
		desc += " · " + lang.Translate("Overbooked").String()
	}
	return view.Item{
		ID:          r.Id,
		Label:       r.LocalStringDate + " " + r.LocalStringTime,
		Description: desc,
	}
}

func (r *ReservationForm) Item() view.Item {
	desc := reservationStanding(r.Status, r.Origin)
	if r.Overbooked {
		desc += " · " + lang.Translate("Overbooked").String()
	}
	return view.Item{
		ID:          r.Id,
		LeadTop:     r.Day,
		LeadMain:    r.Hour,
		Label:       r.ClientId,
		Description: desc,
	}
}
```

For each one, in the `model.Definition` of that type (`<Type>Model` in `model.go`):

- the field read into `Label` → `Row: model.RowTitle`;
- each field read into `Description` → `Row: model.RowDetail` (several, joined with « · » in field order: if the
  method concatenates them in another order, reorder nothing — say so in the PR);
- the field read into `LeadTop`/`LeadMain`/`LeadBottom` (a date, an hour) → `Row: model.RowBadge` on that ONE
  field (the widget formats it now);
- `ID` must be the primary-key field (`DB: &model.FieldDB{PK: true}`); if it is not, stop and say so in the PR.
- A text that is not a plain field (a translated status, a computed label, a constant, a call) cannot be declared:
  mark the field it is computed from and list each such case in the PR.

Then run `ormc` (it rejects an invalid row: two `RowTitle`, details without a title, `Ref` with `Options`). Never
edit `model_orm.go` by hand. Note: `ormc` only validates rows at generation time via `model.ValidateRow` — it does
NOT generate code for `Row` into `model_orm.go`. `model.RowOf` evaluates `Schema()` dynamically at runtime. If `ormc .`
exits with 0, that is expected and complete. Do not expect `model_orm.go` to contain `Row` references.

## Stage 2 — delete the `Item()` methods (`view.go`)

Delete every function shown in Stage 1 and any `var _ view.Itemizer = …` line, plus helpers only they used (e.g.
date-abbreviation arrays). Check: `grep -rn "Item() view.Item\|view.Itemizer\|LeadTop\|LeadMain\|LeadBottom" --include=*.go .` → empty.

## Stage 3 — selects
No `Lookups` in this repo: nothing to do.
Check: `grep -rn "view.Lookup\|Lookups:" --include=*.go .` → empty.

## Stage 4 — saving answers the saved record

Every operation named as `Save` in a `view.Ops{…}` of this repo must answer the record it stored
(`ctx.Encode(&out)` with the saved record, the last one for a batch) — `view` now puts that answer in the shared
copy. Read each handler in `ops.go`; add the `ctx.Encode` where it is missing and name those handlers in the PR.

## Stage 5 — tests

- Every test that called `Item()` asserts `model.RowOf(&rec)` instead (`Title`/`Detail`/`Badge`).
- Every test that builds a presenter over `view.NewCallerLister(caller, …)` wraps the caller once:
  `view.NewRecords().Caller(caller)`.
- Add `TestRowsDeclaredOnFields` in `tests/`: for each listed model, build one record with known values and assert
  the exact `model.RowOf` result (one table row per model).

## Stage 6 — Spanish

Translate to Spanish every README, `docs/*` file, diagram and code comment of this repo still in English (never
rewrite what is already Spanish), and add a short usage example to exported-symbol comments that lack one. Run
`langc sync .` and fill every empty value of `lang.json`; zero untranslated keys, zero reuse warnings.

## Delivery

- `gotest` green. A PR that does not compile, or whose tests could not run, is a failed delivery.
  Environment: `GOPRIVATE=webtyp.com/*,github.com/veltylabs/*`, `GOFLAGS=-mod=mod`, `go mod tidy` before `ormc`.
- Never edit a generated file (`model_orm.go`): a build error there means the hand-written code is wrong.
- Never pause to ask permission or report progress: when the check passes, commit, push and open the PR. Stuck 30
  minutes → open the PR listing what fails. Commit after every stage; never `git reset`, `git restore`,
  `git checkout -- <file>` or `git stash` on your own work.

| Stage | File | Action |
|---|---|---|
| 0 | `ui/*`, `web/browser/main.go` | `Screens(...)`; own screens as `Component`; no `platformd` |
| 1 | `model.go`, `model_orm.go` (regenerated), `go.mod`, `go.sum` | `Row:`/`Options:` on the fields |
| 2 | `view.go` | `Item()` gone |
| 3 | `ui/browser.go` | `Lookups` gone |
| 4 | `ops.go` | save answers the record |
| 5 | `tests/*` | `model.RowOf` |
| 6 | docs, comments, `lang.json` | Spanish |
