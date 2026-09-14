---
PLAN: "fix: price_override lost fractional precision — input.Number() is int-only"
EXECUTOR: jules
REVIEWER: none
STATUS: review
SESSION: 10033203086755818248
PR: https://github.com/veltylabs/appointment_booking/pull/13
---

> This plan is dispatched via the CodeJob workflow. See skill: **agents-workflow**.

# Plan — restore `price_override`'s float precision

You are an agent with **no prior context** and you have **only this repository**
(`github.com/veltylabs/appointment_booking`). Everything you need is inline.

## 1. The regression, and whose mistake it was

The immediately preceding plan (already merged, `v0.1.8`) added widgets to
`EmployeeServiceConfigModel` and told you to use `input.Number()` for
`price_override`, with this justification in its own text:

> `input.Number()` is the correct widget for it too (this ecosystem's
> `input.Number()` covers both integer and float-backed fields; do not invent
> a separate float widget).

**That claim was wrong.** `webtyp.com/input`'s own `number.go` states it
outright: *"Number() keeps reporting FieldInt unconditionally; nothing about
it changes."* There is no float mode. The correct widget already exists,
named for exactly this:

```go
// webtyp.com/input/decimal.go
// Decimal creates a number input whose storage is float64, for fields that need
// fractional precision (price, measurements, percentages, ...). Renders as the
// same HTML <input type="number"> as Number(); the only difference is Storage().
func Decimal() Input
```

The consequence, already live in the merged code: `price_override` was
`model.Float()` before that plan; it is now generated as Go `int64`
(`EmployeeServiceConfig.PriceOverride int64`, `CreateEmployeeServiceConfigArgs.PriceOverride int64`).
A professional's price override has silently lost every fractional unit it
could previously represent.

## 2. Design gate

No exported symbol is added or removed — only the underlying `Kind` of one
existing field changes, which changes the Go type `ormc` generates for it.
This is a correction to an already-applied change, not new API surface; the
api-design gate does not apply.

## 3. Decisions already taken — do not revisit

1. **Only `price_override` changes.** `duration_min` and `buffer_min` are
   correctly `input.Number()` — whole minutes, genuinely integer-valued.
   `payment_required` and `is_active` are correctly `input.Checkbox()`. Do not
   touch any of these.
2. **`input.Decimal()`, not a hand-rolled float widget.** It already exists,
   already renders `<input type="number">` with a decimal point and minus
   sign in its allowed charset, and already declares `Storage() →
   model.FieldFloat`. Do not invent a second implementation.

## 4. Stages

### Stage 1 — `model.go`

Change both occurrences of `price_override`'s `Type` from `input.Number()` to
`input.Decimal()`:

- `EmployeeServiceConfigModel` (the persisted table).
- `CreateEmployeeServiceConfigArgsModel` (the transport args).

Nothing else in either Definition changes.

### Stage 2 — regenerate `model_orm.go`

Run `ormc`. **Never hand-edit `model_orm.go`.** After regeneration, confirm:

- `EmployeeServiceConfig.PriceOverride` is `float64`.
- `CreateEmployeeServiceConfigArgs.PriceOverride` is `float64`.

### Stage 3 — fix the two places that already assume `int64`

Both were written against the wrong type in the immediately preceding plan
and must follow the field back to `float64`:

- `service.go`'s `CreateEmployeeServiceConfig` / whatever code path
  constructs an `EmployeeServiceConfig` from `CreateEmployeeServiceConfigArgs`
  — the assignment `PriceOverride: args.PriceOverride` keeps compiling once
  both sides are `float64`; if you find an explicit `int64` cast anywhere in
  that path, delete it (it would silently truncate again after this fix).
- `tests/employee_service_config_test.go` — every literal that sets
  `PriceOverride` to a whole number (e.g. `PriceOverride: 5000`) still
  compiles as a `float64` untyped constant, but **add one new assertion**
  that a fractional value round-trips: create a config with
  `PriceOverride: 12990.50`, read it back via `GetEmployeeServiceConfig`, and
  assert the fractional part survived (`got.PriceOverride == 12990.50`, not
  truncated to `12990`).

### Stage 4 — documentation

If `README.md`'s ops table or field description for `price_override` mentions
"minutes" or an integer example price (e.g. "5000"), change the example to a
value with cents to make the fix visible to a reader (e.g. "12990.50"). Skip
this stage if no such example exists.

## 5. Stages table

| # | Stage | Files | Acceptance |
|---|---|---|---|
| 1 | Widget fix | `model.go` | both `price_override` occurrences use `input.Decimal()` |
| 2 | Regenerate | `model_orm.go` (ormc) | `PriceOverride` is `float64` in both structs |
| 3 | Fix + test | `service.go` (if needed), `tests/employee_service_config_test.go` | fractional round-trip test added and green |
| 4 | Docs | `README.md` | only if a stale integer example exists |

## 6. Acceptance criteria

- `grep -n "price_override" model.go` → both lines show `input.Decimal()`,
  neither shows `input.Number()`.
- `grep -n "PriceOverride" model_orm.go` → every occurrence is `float64`, none
  is `int64`.
- A new test proves a fractional `PriceOverride` (e.g. `12990.50`) survives a
  write and a read-back unchanged.
- `duration_min`, `buffer_min`, `payment_required`, `is_active` are
  **unchanged** — this plan touches exactly one field.
- `gotest ./...` green.
