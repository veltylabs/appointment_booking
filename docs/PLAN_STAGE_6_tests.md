← [Stage 5](PLAN_STAGE_5_browser_side.md) | Next → [Stage 7](PLAN_STAGE_7_translations_and_documentati.md)

> Read AGENTS.md and the index `PLAN.md` (rules, tables, external calls) first. Commit when this stage compiles.

## Stage 6 — tests (`tests/`, through `crud.LoopbackActingAsWithQueue(nil, "u1", &callQueue{}, mod)`; rows in `docs/TESTS.md`)

Delete tests of removed code.

`tests/queue_double_test.go` (copy it as is; it is the only queue the tests use):

```go
package tests

import (
	"webtyp.com/crud"
	"webtyp.com/fmt"
	"webtyp.com/model"
)

// callQueue is the test double of crud.CallQueue: it records what crud asks and stores nothing.
type callQueue struct {
	added  []crud.CallRequest
	done   []string
	failed []string // the reasons
}

func (q *callQueue) Add(tx crud.Tx, r crud.CallRequest) (string, error) {
	q.added = append(q.added, r)
	return fmt.Sprintf("call-%d", len(q.added)), nil
}
func (q *callQueue) MarkDone(id string) error { q.done = append(q.done, id); return nil }
func (q *callQueue) MarkFailedAttempt(id string, call model.ExternalCallDefinition, reason string) error {
	q.failed = append(q.failed, reason)
	return nil
}
func (q *callQueue) ConfirmCall(id string) error        { return nil }
func (q *callQueue) RejectCall(id, reason string) error { return nil }
```

The caller of every test that touches a call is `crud.LoopbackActingAsWithQueue(nil, "u1", &callQueue{}, mod)` (keep the pointer in a variable to assert `added`, `done`, `failed`).

Operations are `router.Operation{Module: ModelName, Entity: "<entity>", Action: model.<Action>}`, never a string; fixtures follow `crud` v0.0.14 (`reg.AddStoredEntity`, `crud.CurrentUser`). Add `TestMount_MatchesDefinition` (`crud.Mount(reg, nil, nil, mod)` returns nil) and one test that an allowed write publishes a `*crud.Change` on `crud.Topic(ModelName, "<entity>", model.<Action>)`. Required:

| Test | Setup → call → expected |
|---|---|
| `TestReservation_NoDoubleBooking` | two reservations for the same staff and slot → second rejected; one row |
| `TestReservation_CreatorFromCaller` | create with `creator_user_id` from the client → `ServerOnly` error; without → creator = caller |
| `TestReservation_StatusTransitions` | an invalid transition → field error on `status` |
| `TestOverbooking_OwnPermission` | create on `reservation_overbooking` makes a reservation over capacity; create on `reservation` for the same slot is rejected |
| `TestOnline_HoldThenProcess` | create on `online_reservation` → slot held, the call runs inside the request: fake patients and payments ports called once; read the row: `patient_id`, `payment_url` set, status `done`; `queue.done` has 1 id |
| `TestOnline_WithoutPaymentsPort` | `Deps.Payments` nil → record `done`, `patient_id` set, `payment_url` empty |
| `TestOnline_PaymentFailureKeepsHold` | fake payments fails → `queue.failed` has the reason, the row stays `pending`, the slot stays held |
| `TestForwardingOperationsGone` | `router.Operation{Module: ModelName, Entity: "booking_patient", Action: model.Read}` through the loopback → a non-nil error (no such entity is mounted) |
| `TestExpiry_RecordsCount` | two pending reservations past their limit; create on `reservation_expiry` → `expired = 2` |
| `TestDemo_Valid` (`//go:build wasm`) | `seed.Demo()` → no error, `Validate()` nil, at least one screen |
