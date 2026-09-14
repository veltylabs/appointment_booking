---
PLAN: "feat: a public surface for EmployeeServiceConfig — service methods, ops, and a form"
EXECUTOR: jules
REVIEWER: none
STATUS: running
SESSION: 15053971493192888139
---

> This plan is dispatched via the CodeJob workflow. See skill: **agents-workflow**.

# Plan — expose `EmployeeServiceConfig` beyond the repository layer

You are an agent with **no prior context** and you have **only this repository**
(`github.com/veltylabs/appointment_booking`). Everything you need is inline.

## 1. The problem

`EmployeeServiceConfig` — which services a professional performs, at what
duration, with what price override — is a full table in this module
(`EmployeeServiceConfigModel`, migrated by `migrate.Migrate`), and
`Repository` already has complete CRUD for it:

```go
// repository.go — already exists
func (r *Repository) InsertEmployeeServiceConfig(cfg EmployeeServiceConfig) error
func (r *Repository) GetEmployeeServiceConfig(id string) (EmployeeServiceConfig, error)
func (r *Repository) ListEmployeeServiceConfigByStaff(tenantId, staffId string) ([]EmployeeServiceConfig, error)
func (r *Repository) UpdateEmployeeServiceConfig(cfg EmployeeServiceConfig) error
```

But **nothing above the repository reaches it**:

- `*Module` has no `CreateEmployeeServiceConfig`/`GetEmployeeServiceConfig`/
  `ListEmployeeServiceConfigByStaff`/`UpdateEmployeeServiceConfig` method. The
  only place these repository methods are called from is internally, deep
  inside `CreateReservation`/`ListAvailability` (`m.repo.GetEmployeeServiceConfig(...)`).
- `ops.go` mounts 19 operations and **none of them** touches
  `EmployeeServiceConfig` — an application cannot create, list, or edit one
  over MCP at all.
- `EmployeeServiceConfigModel`'s fields are **all base kinds**
  (`model.Text()`, `model.Int()`, `model.Float()`, `model.Bool()`) — no
  `input.*` widget anywhere. `webtyp/form`'s `New` skips any field whose
  `Type` does not assert to `input.Input`, so a form built on this model
  today would render **empty**.

This is the same defect the module already fixed once for `Reservation` (see
`docs/ARCHITECTURE.md`'s record of that decision): a model exists and is
migrated, but nothing above the repository can create or edit a row of it, and
its fields carry no form widgets. It surfaced now because a consuming
application (`mjosefa-cms`) needs a screen letting an admin say "Dr. X performs
these services, for this long, at this price" — the exact reason
`employee_service_config` exists — and found no op to call.

## 2. Design gate

### Prior art

- **Django REST Framework**'s `ModelViewSet` gives every model a full
  create/retrieve/list/update surface by default; a model with a migration but
  no viewset is the exception a maintainer has to notice and add, not the
  norm.
- **Rails** scaffolding generates the same four verbs for every resource as a
  matter of course.
- Inside this very repository, every OTHER table
  (`WorkCalendarConfig`, `WorkCalendarBlock`, `WorkCalendarException`,
  `Reservation`) already has this full surface. `EmployeeServiceConfig` is the
  one table that does not — an omission, not a design choice with its own
  rationale on record.

We match the shape those other four already established here: `*Module`
methods that wrap `Repository`, ops that wrap those methods, `input.*` widgets
on the fields a person edits.

### Novice-name test

- "Create a service config for this professional" → `CreateEmployeeServiceConfig`.
- "Get one" → `GetEmployeeServiceConfig`.
- "List a professional's services" → `ListEmployeeServiceConfigByStaff` — the
  exact name the `Repository` method already uses; the `*Module` method
  mirrors it rather than inventing a second name for the same question.
- "Update one" → `UpdateEmployeeServiceConfig`.
- The ops: `create_employee_service_config`, `get_employee_service_config`,
  `list_employee_service_configs_by_staff`, `update_employee_service_config` —
  the same `<verb>_<noun>` shape every other op in this file already uses.

### Complexity ledger

| | change |
|---|---|
| Concepts | `+0` — "a professional's service configuration is creatable and listable" is not a new concept, it is what the table's own name already claims and does not yet deliver |
| Files | `+0` new files (`service.go`, `ops.go`, `model.go`, `view.go` all exist); `+1` test file |
| Call-site lines | an application gains ~4 ops it can call instead of reaching into the database directly or inventing a local workaround |
| Ways to do it | `−1` — managing a professional's services had zero supported ways (only an internal, unreachable repository) |
| Net | negative |

### Where it belongs

Here. `EmployeeServiceConfig` is this module's own table, exactly like
`Reservation` and `WorkCalendarBlock`. Nothing about "which services does a
professional perform, at what price" is a concept `mjosefa-cms` or any other
consuming application should own or duplicate.

### What it deletes

Nothing. This is additive, like the `Reservation` form-view work before it.

## 3. Decisions already taken — do not revisit

1. **No separate form-projection type, unlike `Reservation`.**
   `Reservation` needed one (`ReservationForm`) because its persisted record
   carries audit fields (snapshots, `revision`, `status_before_conflict`) that
   must never be form-editable. `EmployeeServiceConfig` has no such fields —
   every column except `id` (machine-assigned) and `tenant_id`
   (machine-supplied) is something a person legitimately edits:
   `staff_id` (which professional — context-supplied by the host screen, not
   typed by hand, so `input.Text()` is still correct: it is a real column, the
   *value* just arrives from context rather than a free-typed field),
   `service_id`, `duration_min`, `buffer_min`, `price_override`,
   `payment_required`, `is_active`. So the fix is **adding `input.*` widgets
   directly to `EmployeeServiceConfigModel`**, not introducing a second type.
2. **No delete op.** A service config referenced by a past reservation's
   `EmployeeServiceConfigId` must not disappear — same reasoning
   `patient_directory` and `staff_manager` already apply to their own rows.
   `is_active` is how a professional stops offering a service.
3. **`UpdateEmployeeServiceConfig` takes the full record**, matching every
   other `Update*` in this module (`UpdateReservation`-shaped calls all work
   this way) — no partial-field PATCH semantics anywhere in this repository.

## 4. Stages

### Stage 1 — widgets on `EmployeeServiceConfigModel`, in `model.go`

Change the existing Definition to carry widgets, following the widget-policy
comment style already on every other `*ArgsModel` in this file:

```go
// EmployeeServiceConfigModel: which services a professional performs, for how
// long, and at what price. Widget policy is BY ROLE, same rule as every
// transport model below: input.X() on every field a person edits. Unlike
// Reservation, this table carries NO audit-only fields to protect from
// becoming form-editable — every column but id/tenant_id is legitimately
// user-facing, so the widgets go directly on the persisted model; there is no
// separate form projection.
var EmployeeServiceConfigModel = model.Definition{
	Name: "employee_service_config",
	Fields: model.Fields{
		{Name: "id", Type: model.Text(), DB: &model.FieldDB{PK: true}, OmitEmpty: true},
		{Name: "tenant_id", Type: model.Text(), NotNull: true},
		{Name: "staff_id", Type: input.Text(), NotNull: true},
		{Name: "service_id", Type: input.Text(), NotNull: true},
		{Name: "duration_min", Type: input.Number()},
		{Name: "buffer_min", Type: input.Number()},
		{Name: "price_override", Type: input.Number()},
		{Name: "payment_required", Type: input.Checkbox()},
		{Name: "is_active", Type: input.Checkbox()},
	},
}
```

Add the `webtyp.com/input` import if `model.go` does not already carry it
(check first — several other Definitions in this file already use `input.*`,
so it likely does).

**Anti-footgun.** `price_override` is a `model.Float()` in the ORIGINAL
Definition — `input.Number()` is the correct widget for it too (this
ecosystem's `input.Number()` covers both integer and float-backed fields; do
not invent a separate float widget).

Regenerate `model_orm.go` with `ormc` after this change. **Never hand-edit
`model_orm.go`.**

Also add transport-only Args Definitions, next to the other `*ArgsModel`
blocks:

```go
var CreateEmployeeServiceConfigArgsModel = model.Definition{
	Name: "create_employee_service_config_args",
	Fields: model.Fields{
		{Name: "tenant_id", Type: model.Text()}, // machine-supplied — never a form input
		{Name: "staff_id", Type: input.Text()},
		{Name: "service_id", Type: input.Text()},
		{Name: "duration_min", Type: input.Number()},
		{Name: "buffer_min", Type: input.Number()},
		{Name: "price_override", Type: input.Number()},
		{Name: "payment_required", Type: input.Checkbox()},
	},
}

var GetEmployeeServiceConfigArgsModel = model.Definition{
	Name: "get_employee_service_config_args",
	Fields: model.Fields{
		{Name: "id", Type: model.Text()}, // machine-supplied — never a form input
	},
}

var ListEmployeeServiceConfigsByStaffArgsModel = model.Definition{
	Name: "list_employee_service_configs_by_staff_args",
	Fields: model.Fields{
		{Name: "tenant_id", Type: model.Text()}, // machine-supplied — never a form input
		{Name: "staff_id", Type: input.Text()},
	},
}
```

`UpdateEmployeeServiceConfig`'s op reuses `EmployeeServiceConfig` itself as its
args (same pattern `opUpdateDevice`-shaped ops elsewhere in this ecosystem use
when the update takes the whole record) — no separate
`UpdateEmployeeServiceConfigArgsModel` needed.

### Stage 2 — `*Module` methods, in `service.go`

Place these near the other calendar-config methods (`UpsertCalendarConfig`,
`SaveDayBlocks`), following their exact error-handling shape:

```go
// CreateEmployeeServiceConfig registers that a professional performs a
// service, with its own duration/price override.
func (m *Module) CreateEmployeeServiceConfig(cfg EmployeeServiceConfig) (EmployeeServiceConfig, error) {
	if cfg.Id == "" {
		cfg.Id = m.ids.NewID()
	}
	if err := m.repo.InsertEmployeeServiceConfig(cfg); err != nil {
		return EmployeeServiceConfig{}, err
	}
	return cfg, nil
}

// GetEmployeeServiceConfig reads one by id.
func (m *Module) GetEmployeeServiceConfig(id string) (EmployeeServiceConfig, error) {
	return m.repo.GetEmployeeServiceConfig(id)
}

// ListEmployeeServiceConfigByStaff lists every service a professional
// performs, active or not — the editing screen needs to show and reactivate
// a disabled one, not just the active set.
func (m *Module) ListEmployeeServiceConfigByStaff(tenantId, staffId string) ([]EmployeeServiceConfig, error) {
	return m.repo.ListEmployeeServiceConfigByStaff(tenantId, staffId)
}

// UpdateEmployeeServiceConfig writes the full record.
func (m *Module) UpdateEmployeeServiceConfig(cfg EmployeeServiceConfig) error {
	return m.repo.UpdateEmployeeServiceConfig(cfg)
}
```

**Anti-footgun — check `m.ids` and `m.repo` are the module's actual field
names** before writing this (read the top of `service.go`'s `Module` struct
and `New`); do not guess the receiver's field names from this plan, copy them
from the real struct.

`m.ids.NewID()` on empty `Id` mirrors exactly how `CreateReservation`
assigns an id elsewhere in this same file — do not invent a different
id-assignment idiom for this one method.

### Stage 3 — ops, in `ops.go`

Add to the `const (...)` block:

```go
OpCreateEmployeeServiceConfig      = "create_employee_service_config"
OpGetEmployeeServiceConfig         = "get_employee_service_config"
OpListEmployeeServiceConfigsByStaff = "list_employee_service_configs_by_staff"
OpUpdateEmployeeServiceConfig      = "update_employee_service_config"
```

Add to `MountOperations`:

```go
reg.Operation(OpCreateEmployeeServiceConfig, m.opCreateEmployeeServiceConfig).Requires("employee_service_config", model.Create).Accepts(&CreateEmployeeServiceConfigArgs{})
reg.Operation(OpGetEmployeeServiceConfig, m.opGetEmployeeServiceConfig).Requires("employee_service_config", model.Read).Accepts(&GetEmployeeServiceConfigArgs{})
reg.Operation(OpListEmployeeServiceConfigsByStaff, m.opListEmployeeServiceConfigsByStaff).Requires("employee_service_config", model.Read).Accepts(&ListEmployeeServiceConfigsByStaffArgs{})
reg.Operation(OpUpdateEmployeeServiceConfig, m.opUpdateEmployeeServiceConfig).Requires("employee_service_config", model.Update).Accepts(&EmployeeServiceConfig{})
```

**Every op declares a `Resource` and an `Action` — no op without a policy.**
There is deliberately no delete op (decision 3.2) — do not add one.

Handlers, following the exact status-code shape every other handler in this
file already uses (`writeError(ctx, err)` for domain sentinels, `400` for a
bad decode, `200`/encoded body for success):

```go
func (m *Module) opCreateEmployeeServiceConfig(ctx router.Context) {
	var args CreateEmployeeServiceConfigArgs
	if err := ctx.Decode(&args); err != nil {
		ctx.WriteStatus(400)
		return
	}
	cfg, err := m.CreateEmployeeServiceConfig(EmployeeServiceConfig{
		TenantId: args.TenantId, StaffId: args.StaffId, ServiceId: args.ServiceId,
		DurationMin: args.DurationMin, BufferMin: args.BufferMin,
		PriceOverride: args.PriceOverride, PaymentRequired: args.PaymentRequired, IsActive: true,
	})
	if err != nil {
		writeError(ctx, err)
		return
	}
	if err := ctx.Encode(&cfg); err != nil {
		ctx.WriteStatus(500)
	}
}

func (m *Module) opGetEmployeeServiceConfig(ctx router.Context) {
	var args GetEmployeeServiceConfigArgs
	if err := ctx.Decode(&args); err != nil {
		ctx.WriteStatus(400)
		return
	}
	cfg, err := m.GetEmployeeServiceConfig(args.Id)
	if err != nil {
		writeError(ctx, err)
		return
	}
	if err := ctx.Encode(&cfg); err != nil {
		ctx.WriteStatus(500)
	}
}

func (m *Module) opListEmployeeServiceConfigsByStaff(ctx router.Context) {
	var args ListEmployeeServiceConfigsByStaffArgs
	if err := ctx.Decode(&args); err != nil {
		ctx.WriteStatus(400)
		return
	}
	rows, err := m.ListEmployeeServiceConfigByStaff(args.TenantId, args.StaffId)
	if err != nil {
		writeError(ctx, err)
		return
	}
	list := make(EmployeeServiceConfigList, len(rows))
	for i := range rows {
		list[i] = &rows[i]
	}
	if err := ctx.Encode(&list); err != nil {
		ctx.WriteStatus(500)
	}
}

func (m *Module) opUpdateEmployeeServiceConfig(ctx router.Context) {
	var cfg EmployeeServiceConfig
	if err := ctx.Decode(&cfg); err != nil {
		ctx.WriteStatus(400)
		return
	}
	if err := m.UpdateEmployeeServiceConfig(cfg); err != nil {
		writeError(ctx, err)
		return
	}
	ctx.WriteStatus(200)
}
```

**Anti-footgun — `EmployeeServiceConfigList`.** `ormc` generates it from
`model_orm.go` once the model exists (it already does — the model is not new,
only its widgets are). Confirm the generated name before using it; do not
guess a different plural.

### Stage 4 — a view, in `view.go`

```go
// Item implements view.Itemizer.
func (c *EmployeeServiceConfig) Item() view.Item {
	return view.Item{ID: c.Id, Label: c.ServiceId, Description: c.StaffId}
}

const titleEmployeeServiceConfig = "Services"

// NewEmployeeServiceConfigView builds a Presenter scoped to one professional —
// same shape as NewView(caller, tenantId, staffId) above: there is no
// "list every service config in the tenant" op, on purpose, mirroring why
// NewView itself is staff-scoped.
func NewEmployeeServiceConfigView(caller router.Caller, tenantId, staffId string) view.Presenter {
	return view.New(
		employeeServiceConfigLister{caller: caller, tenantId: tenantId, staffId: staffId},
		&EmployeeServiceConfig{},
		view.WithTitle(titleEmployeeServiceConfig),
	)
}
```

Add `employeeServiceConfigLister` to `lister.go`, mirroring `reservationLister`
exactly (a `List()` calling `OpListEmployeeServiceConfigsByStaff`, a `Save()`
calling `OpCreateEmployeeServiceConfig` when `Id == ""` else
`OpUpdateEmployeeServiceConfig` — same branch `opUpsertDevice`-shaped code
elsewhere in this ecosystem uses):

```go
type employeeServiceConfigLister struct {
	caller   router.Caller
	tenantId string
	staffId  string
}

func (l employeeServiceConfigLister) List() ([]model.Model, error) {
	out := &EmployeeServiceConfigList{}
	ch := make(chan error, 1)
	l.caller.Call(
		OpListEmployeeServiceConfigsByStaff,
		&ListEmployeeServiceConfigsByStaffArgs{TenantId: l.tenantId, StaffId: l.staffId},
		out,
		func(err error) { ch <- err },
	)
	if err := <-ch; err != nil {
		return nil, err
	}
	rows := make([]model.Model, 0, out.Len())
	for i := 0; i < out.Len(); i++ {
		rows = append(rows, out.At(i).(*EmployeeServiceConfig))
	}
	return rows, nil
}

func (l employeeServiceConfigLister) Save(recs ...model.Model) error {
	for _, rec := range recs {
		cfg, ok := rec.(*EmployeeServiceConfig)
		if !ok {
			return fmt.Err("appointment_booking: save: expected *EmployeeServiceConfig")
		}
		cfg.TenantId = l.tenantId
		cfg.StaffId = l.staffId
		op := OpCreateEmployeeServiceConfig
		if cfg.Id != "" {
			op = OpUpdateEmployeeServiceConfig
		}
		ch := make(chan error, 1)
		l.caller.Call(op, cfg, nil, func(err error) { ch <- err })
		if err := <-ch; err != nil {
			return err
		}
	}
	return nil
}

var (
	_ view.Lister = employeeServiceConfigLister{}
	_ view.Saver  = employeeServiceConfigLister{}
)
```

**No `Update`/`Delete` capability beyond `Save`** — same reasoning as
`reservationFormStore`: this presenter offers exactly List and Save, nothing
crudview would paint an extra button for.

### Stage 5 — tests, in `tests/`

New file `tests/employee_service_config_test.go`. Follow `tests/ops_test.go`'s
existing setup — read it first, reuse its harness, do not build a second one.

| Test | Asserts |
|---|---|
| `TestCreateEmployeeServiceConfig` | id assigned when empty, row readable back via `GetEmployeeServiceConfig` |
| `TestListEmployeeServiceConfigByStaff_ScopedToTenantAndStaff` | a config for tenant A / staff X is invisible when listing tenant A / staff Y, and invisible when listing tenant B / staff X |
| `TestUpdateEmployeeServiceConfig_PersistsChanges` | update then re-read shows the new `duration_min`/`price_override` |
| `TestOpsMountedWithPolicy` | all four ops are registered, each with a non-zero `Resource` and `Action` (mirror `tests/ops_test.go`'s existing assertion style for the other ops) |
| `TestNoDeleteOp` | `grep`-shaped assertion or a registry lookup: no `delete_employee_service_config` op exists |
| `TestEmployeeServiceConfigForm_HasWidgetsOnEveryEditableField` | `form.New` (or a direct `Fields()` walk) shows every field but `id`/`tenant_id` has an `input.Input`-asserting `Type` — the regression check for the defect this plan fixes |

Run `gotest ./...` — everything green, including every pre-existing test
untouched.

### Stage 6 — documentation

- `README.md`: add the four new ops to the "Available Ops" table (now 23
  total, not 19 — update that count too), and a short "Employee service
  configuration" section showing `NewEmployeeServiceConfigView` wired the same
  way `NewView`/`NewFormView` are documented.
- `docs/ARCHITECTURE.md`: record decision 3.1 (no separate form projection for
  this table, and why) so the reasoning outlives this plan.
- Do **not** link any permanent document to `docs/PLAN.md` — it is deleted
  when this lands.

## 5. Stages table

| # | Stage | Files | Acceptance |
|---|---|---|---|
| 1 | Widgets + Args | `model.go`, `model_orm.go` (ormc) | every editable field has `input.*`; `id`/`tenant_id` do not |
| 2 | Service methods | `service.go` | four methods, matching `m.ids`/`m.repo`'s real field names |
| 3 | Ops | `ops.go` | 4 ops, each Resource+Action; no delete op |
| 4 | View | `view.go`, `lister.go` | presenter has exactly List+Save |
| 5 | Tests | `tests/employee_service_config_test.go` | 6 cases green |
| 6 | Docs | `README.md`, `docs/ARCHITECTURE.md` | ops table updated, decision recorded |

## 6. Acceptance criteria

- `grep -n "model.Float()\|model.Int()\|model.Text()\|model.Bool()" model.go` on
  `EmployeeServiceConfigModel`'s own field list → only `id` and `tenant_id`
  remain base kinds; every other field is `input.*`.
- `grep -rn "delete_employee_service_config\|DeleteEmployeeServiceConfig" .` →
  **empty**.
- `grep -n "func (l employeeServiceConfigLister)" lister.go` → exactly two
  matches (`List`, `Save`).
- Every new op's `Requires` call names a non-empty `model.Resource` and a
  non-zero `model.Action`.
- `Reservation`, `ReservationForm`, `NewView`, `NewFormView`,
  `reservationLister`, `reservationFormStore` are **unchanged** — this plan is
  additive to a different table entirely.
- `model_orm.go` changes only through `ormc` regeneration.
- `gotest ./...` green.
