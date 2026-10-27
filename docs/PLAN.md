---
PLAN: "feat: export ui.Screens, drop layout dependency, resolve langc reuse warnings and update payment tests"
TAG: v0.3.50
EXECUTOR: jules
REVIEWER: none
STATUS: running
SESSION: 14325544441620928304
---

> Read AGENTS.md at the repo root before any change.

## Tools

```
go install webtyp.com/devflow/cmd/gotest@latest
go install webtyp.com/lang/cmd/langc@latest
go install webtyp.com/ormc/cmd/ormc@latest
```

## Why

Phase M of webtyp app composition (`webtyp/docs/APP_COMPOSITION_MASTER_PLAN.md`): modules declare their screens via `Screens(caller router.Caller) []view.Screen` instead of legacy `Browser(...) (platformd.UIModule, error)`, and drop `webtyp.com/layout` from `go.mod`.
Additionally, `payment_manager` was upgraded to `v0.4.15`, requiring test updates in `tests/e2e_payment_test.go` (`ReturnURL` removed from `pm.Deps`, and `CompleteOnlinePayment` taking `OnlineResult` via `gw.Result(token)`).
`langc sync .` detected reuse warnings in `model.go` and `lister.go` that must be resolved with explicit field labels and parameter separation in `fmt.Err`.

## APIs used

| symbol | exact signature | tag | file |
|---|---|---|---|
| `view.Screen` | `type Screen struct { ID string; Label lang.Text; Icon string; Permission model.Resource; Presenter Presenter; Lookups []Lookup; Help lang.Text; Children []Screen }` | `webtyp.com/view@v0.6.45` | `view/screen.go` |
| `view.WithDescription` | `func WithDescription(d lang.Text) Option` | `webtyp.com/view@v0.6.45` | `view/options.go` |
| `view.WithTitle` | `func WithTitle(t string) Option` | `webtyp.com/view@v0.6.45` | `view/options.go` |
| `view.New` | `func New(source Lister, target model.Model, opts ...Option) Presenter` | `webtyp.com/view@v0.6.45` | `view/view.go` |
| `router.Caller` | `type Caller interface { Call(op string, args, out any, done func(error)) }` | `webtyp.com/router@v0.4.8` | `router/caller.go` |
| `pm.New` | `func New(db *orm.DB, deps Deps) (*Module, error)` | `github.com/veltylabs/payment_manager@v0.4.15` | `module.go` |
| `pm.CompleteOnlinePayment` | `func (m *Module) CompleteOnlinePayment(r OnlineResult) (Payment, error)` | `github.com/veltylabs/payment_manager@v0.4.15` | `module.go` |

If a signature in the published tag differs from this table, stop and name the symbol in the PR; never adapt it by guessing.

## Stage 1: Explicit labels in model.go, ormc and save error helpers in lister.go

1. In `model.go`:
Assign explicit labels to disambiguate fields sharing 2+ words:
- `duration_min`: `Label: "Duration"`
- `duration_min_snapshot`: `Label: "Snapshot duration"`
- `local_string_date`: `Label: "Reservation date"`
- `local_string_time`: `Label: "Reservation time"`
- `config_id`: `Label: "Configuration"`
- `employee_service_config_id`: `Label: "Employee service config"`
- `creator_user_id`: `Label: "Creator"`
- `start_utc`: `Label: "Start time"`
- `slot_start_utc`: `Label: "Slot start"`

Run `ormc` to update `model_orm.go`.

2. In `lister.go`:
Disambiguate `appointment_booking: save:` errors by passing `"appointment_booking: save"` as the first argument to `fmt.Err`:
- Line 106: `fmt.Err("appointment_booking: save", "empty records")`
- Line 121: `fmt.Err("appointment_booking: save", "expected *ReservationForm")`
- Line 212: `fmt.Err("appointment_booking: save", "expected *EmployeeServiceConfig")`

Files: `model.go`, `model_orm.go`, `lister.go`

## Stage 2: Adapt tests/e2e_payment_test.go and update view.go

1. In `tests/e2e_payment_test.go`:
- In `newCircuit`:
Remove `ReturnURL: "https://clinic.example/return"` from `pm.Deps`.
- In `payOnline`:
Replace:
```go
	c.gw.Approve(started.Token)
	r, _ := c.gw.Result(started.Token)
	if _, err := c.pm.CompleteOnlinePayment(r); err != nil {
		t.Fatalf("CompleteOnlinePayment: %v", err)
	}
```
- In `TestE2E_PaidAfterHoldExpiredIsRefunded`:
Replace the second `CompleteOnlinePayment` call similarly:
```go
	c.gw.Approve(started.Token)
	r, _ := c.gw.Result(started.Token)
	if _, err := c.pm.CompleteOnlinePayment(r); err != nil {
		t.Fatal(err)
	}
```

2. In `view.go`:
Define constant:
```go
const ResourceReservation = "reservation"
```
Update `NewView` to accept `opts ...view.Option`:
```go
func NewView(caller router.Caller, tenantId, staffId string, opts ...view.Option) view.Presenter {
	allOpts := append([]view.Option{view.WithTitle(titleReservations)}, opts...)
	return view.New(
		reservationLister{caller: caller, tenantId: tenantId, staffId: staffId},
		&Reservation{},
		allOpts...,
	)
}
```

Files: `tests/e2e_payment_test.go`, `view.go`

## Stage 3: Declarative Screens and drop layout from go.mod

1. In `ui/browser.go`:
Replace with:
```go
//go:build browser

package ui

import (
	appointmentbooking "github.com/veltylabs/appointment_booking"
	"webtyp.com/lang"
	"webtyp.com/router"
	"webtyp.com/view"
)

const (
	ID       = "appointment_booking"
	NavLabel = "Agenda"
)

const (
	descReservations lang.Text = "Patient appointment reservations, confirmed bookings and scheduling calendar."
)

// Screens declara las pantallas que este módulo ofrece al menú de la app.
func Screens(caller router.Caller) []view.Screen {
	return []view.Screen{
		{
			ID:         ID,
			Label:      NavLabel,
			Icon:       ID,
			Permission: appointmentbooking.ResourceReservation,
			Presenter:  appointmentbooking.NewView(caller, "", "", view.WithDescription(descReservations)),
		},
	}
}
```

2. In `ui/scheduleview.go`:
Remove `import "webtyp.com/layout/rightpanel"`.
Replace lines 201-204 (the `rightpanel.RightPanel` wrapper):
```go
	return Div().Set(clsRoot.AsAttr()).Child(
		Div().Child(controls, Div().BindChildren(s.editor)),
	)
```

3. In `ui/serviceconfigview.go`:
Remove `import "webtyp.com/layout/crudview"`. If `ServiceConfigView` is kept, replace `crudview.New` with a simple container rendering `ab.NewEmployeeServiceConfigView`.

4. In `ui/personal.go`:
Remove `import "webtyp.com/layout/platformd"`. If `PersonalBrowser` is no longer needed, remove it or adapt to return `Component`.

5. In `ui/online.go`:
Remove `import "webtyp.com/layout/platformd"`.

6. In `web/browser.go`:
Rewrite to use `ui.Screens(caller)` and standard `dom`/`html` without importing `platformd`:
```go
//go:build browser

package main

import (
	"github.com/veltylabs/appointment_booking/ui"
	. "webtyp.com/dom"
	"webtyp.com/html"
	"webtyp.com/router/loopback"
)

func main() {
	caller := loopback.New()
	screens := ui.Screens(caller)
	appNode := html.Div().Child(
		html.H1().Text(string(screens[0].Label)),
		html.P().Text("Demo"),
	)
	Append("body", appNode)
	select {}
}
```

7. In `go.mod`:
Remove `webtyp.com/layout`. Run `go mod tidy`.

Files: `ui/browser.go`, `ui/scheduleview.go`, `ui/serviceconfigview.go`, `ui/personal.go`, `ui/online.go`, `web/browser.go`, `go.mod`, `go.sum`

## Stage 4: Test and Translations

1. In `tests/`:
Replace `tests/personal_group_test.go` with `tests/ui_screens_test.go`:
```go
package tests

import (
	"testing"

	"github.com/veltylabs/appointment_booking/ui"
	"webtyp.com/router/mock"
)

func TestUIScreens(t *testing.T) {
	caller := mock.NewCaller()
	screens := ui.Screens(caller)
	if len(screens) == 0 {
		t.Fatal("expected at least 1 screen")
	}
	for _, s := range screens {
		if err := s.Validate(); err != nil {
			t.Fatalf("screen %s invalid: %v", s.ID, err)
		}
		if s.Presenter == nil {
			t.Fatalf("screen %s expected Presenter", s.ID)
		}
		if s.Presenter.Description() == "" {
			t.Fatalf("screen %s expected non-empty description", s.ID)
		}
	}
}
```
Delete `tests/personal_group_test.go`.

2. Run `langc sync .`.
3. Complete 100% of Spanish translations `(es)` in `lang.json` (or `config/lang.json`).
4. Ensure `langc sync .` reports:
   - 0 untranslated `(es)`
   - 0 reuse warnings
   - 0 identical values

Files: `tests/ui_screens_test.go`, `tests/personal_group_test.go` (deleted), `config/lang.json`

## Delivery check

Run:
```
gotest
langc sync .
```
Both commands must exit 0 with 0 errors, 0 untranslated `(es)`, and 0 reuse warnings.
Commit after every stage. When all checks pass, push and open the PR.

## Stages

| # | Stage | Files |
|---|---|---|
| 1 | Model labels, ormc and lister errors | `model.go`, `model_orm.go`, `lister.go` |
| 2 | E2E payment tests & NewView options | `tests/e2e_payment_test.go`, `view.go` |
| 3 | Declarative screens & drop layout | `ui/browser.go`, `ui/scheduleview.go`, `ui/serviceconfigview.go`, `web/browser.go`, `go.mod`, `go.sum` |
| 4 | Test and translations | `tests/ui_screens_test.go`, `config/lang.json` |
