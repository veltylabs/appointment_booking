---
PLAN: "fix!: FreeSlots blocks on a channel, deadlocking wasm when called from an async callback — make it callback-shaped"
EXECUTOR: jules
REVIEWER: none
STATUS: review
SESSION: 2874502215171401177
PR: https://github.com/veltylabs/appointment_booking/pull/14
---

> This plan is dispatched via the CodeJob workflow. See skill: agents-workflow.

# PLAN — `FreeSlots`: no channel, callback-shaped like everything else in this package

You are an external agent with **zero prior context** about this project. Everything you need is
in this file. Read `AGENTS.md` at the repo root first, then this file fully before writing code.

## 0. Prerequisite — run this first

```bash
go install webtyp.com/devflow/cmd/gotest@latest
```

All tests run with `gotest`, never `go test` directly.

## 1. The bug, proven by failing tests already in this repo

`tests/booking_form_test.go` (already committed on this branch) now has two tests reproducing a
real production bug — a **silent, hard crash** of the whole wasm app:

```bash
gotest
# tests/booking_form_test.go:371:46: too many arguments in call to ab.FreeSlots
#         have (loopbackCaller, appointmentbooking.FormConfig, string, func(slots []string, err error))
#         want (router.Caller, appointmentbooking.FormConfig, string)
```

It does not compile yet — that is the intended red state (see §3, this is a signature change).
The acceptance criterion: **`gotest` must go fully green**, including
`TestFreeSlots_EmptyWithoutServiceConfig` and `TestFreeSlots_NeverBlocksOnAChannel`, with no other
test regressing.

## 2. Root cause

`FreeSlots` (`view.go`) has this shape today:

```go
func FreeSlots(caller router.Caller, cfg FormConfig, day string) ([]string, error) {
	...
	out := &TimeSlotList{}
	ch := make(chan error, 1)
	caller.Call(qualifiedOp(OpListAvailability), &ListAvailabilityArgs{...}, out,
		func(err error) { ch <- err },
	)
	if err := <-ch; err != nil {
		return nil, err
	}
	...
}
```

It fakes a synchronous return by blocking on a channel until `caller.Call`'s async callback fills
it. Downstream, `mjosefa-cms`'s booking screen calls it from exactly the place this breaks:

```go
// mjosefa-cms/modules/appointment_booking/bookingview.go
OnAfterReload: func(list crudview.ListView) {
	if t, ok := list.(*targethour.TargetHour); ok {
		t.FreeSlots = freeSlotsCache(v.caller, cfg, v.day.Get()) // calls ab.FreeSlots
	}
},
```

`OnAfterReload` runs **inside** `crudview.Reload()`'s own callback chain
(`Presenter.Reload → callerLister.list → caller.Call`) — which is itself already running inside an
async browser callback (a real `fetch` resolution). Under `GOOS=js/wasm` there is no OS thread to
park a blocked goroutine on: the callback that would fill `FreeSlots`' internal `ch` can never run
until the *current* call stack unwinds, and the current stack cannot unwind because it is blocked
on `<-ch`. The Go/wasm runtime detects zero runnable goroutines and calls `runtime.wasmExit(0)`
**silently** — no panic text reaches the page, the whole app just dies with no explanation.
Confirmed reproducible against a real Postgres row: any professional with ≥1 service configured
hits this the instant "Reserva Hora" loads.

**A previous fix attempt for the same symptom, at the wrong layer, was rejected.** An earlier
session traced this same crash and proposed patching `webtyp.com/dom`'s reconciler to defer a
reactively-mounted component's `Init()` to a JS microtask **run on a new goroutine**
(`go fn()`). That was rejected: it introduces real goroutine concurrency into a reconciler that is
single-threaded by construction, for a problem that is actually local to this one function. **Do
not touch `webtyp.com/dom`, `webtyp.com/view`, or `webtyp.com/layout/crudview` for this fix** —
`callerLister.list()`, `reservationLister.List()`, and `reservationFormStore.List()` (this very
package's own `lister.go`) are already correctly callback-shaped and never block. `FreeSlots` is
the **one exception** in this codebase — every other transport call already looks like this.

## 3. The fix

In `view.go`, change `FreeSlots` to take a trailing callback, exactly like `router.Caller.Call`
itself and every other lister in this package:

```go
// FreeSlots devuelve los huecos reservables de un día como cadenas "HH:MM" en
// cfg.Timezone, listos para que un widget de lista los renderice como filas vacías.
//
// day es "YYYY-MM-DD". done recibe (nil, nil) cuando el alcance está incompleto —
// sin staff o sin configuración de servicio significa que no hay nada que calcular, no un
// fallo. done siempre se invoca exactamente una vez, nunca de forma síncrona antes de que
// FreeSlots retorne cuando hay una llamada de red real en curso — igual que
// router.Caller.Call.
func FreeSlots(caller router.Caller, cfg FormConfig, day string, done func([]string, error)) {
	if cfg.StaffId == "" || cfg.ServiceConfigId == "" {
		done(nil, nil)
		return
	}
	daySec := dayToUnix(day)
	if daySec == 0 {
		done(nil, nil)
		return
	}
	out := &TimeSlotList{}
	caller.Call(
		qualifiedOp(OpListAvailability),
		&ListAvailabilityArgs{
			TenantId: cfg.TenantId,
			StaffId:  cfg.StaffId,
			ConfigId: cfg.ServiceConfigId,
			From:     daySec,
			To:       daySec,
		},
		out,
		func(err error) {
			if err != nil {
				done(nil, err)
				return
			}
			slots := make([]string, 0, out.Len())
			for i := 0; i < out.Len(); i++ {
				ts := out.At(i).(*TimeSlot)
				tStr := tinytime.FormatTime(ts.StartUtc * 1000000000)
				if len(tStr) >= 5 {
					tStr = tStr[:5]
				}
				slots = append(slots, tStr)
			}
			done(slots, nil)
		},
	)
}
```

No `ch`, no `make(chan`, no blocking wait anywhere in the function.

## Design gate (required — this changes public API)

**1. Prior art.** Every other transport-touching function in this exact package is already
callback-shaped: `router.Caller.Call(op, args, into, done func(error))` itself, and this package's
own `reservationLister.List(done func([]model.Model, error))` and
`reservationFormStore.List(done func([]model.Model, error))` (`lister.go`). More broadly: every
browser-async API (`fetch`, `addEventListener`, `IndexedDB`) is callback/Promise-shaped, never a
function that blocks until data it doesn't have yet arrives — because a single-threaded event loop
(exactly what Go/wasm is) cannot service the callback that would end the wait while something else
already occupies the call stack waiting for it. Node.js's own convention for any I/O-bound
function (`fn(args..., callback)`) exists for the identical reason. `FreeSlots` is the sole
function in this codebase that fought that convention by hiding a channel behind a
synchronous-looking return — this plan removes the one exception, it does not invent a new shape.

**2. Novice-name test.** `FreeSlots(caller, cfg, day, done func([]string, error))` reads as "get
free slots, call done with the result" — the exact same tail-callback shape a developer has
already seen on `router.Caller.Call` and this package's own listers. No abbreviation, no boolean
parameter, no new vocabulary.

**3. Complexity ledger.**
```
Concepts the developer must learn   +0  (matches the done-callback convention already used by
                                          every other transport call in this package)
Files touched                       +1 in this repo (view.go) — the one caller in mjosefa-cms
                                     (bookingview.go) is a separate, downstream fix, not part of
                                     this plan's file count
Lines at the call site              ~+2 (wrap the two-line body in a closure instead of
                                          `slots, err :=`; the one real call site — OnAfterReload —
                                          is already a closure, so this is nearly a wash)
Ways to fetch free slots            1 → 1  (never ends positive — FreeSlots stays the ONE way;
                                            this changes its shape, it does not add a second)
```

**4. Where it belongs.** In `view.go`, same file, same function name — it is the same capability
(compute a day's free slots over the wire), only its shape changes to match the async transport it
already wraps via `caller.Call`. Not a new file, not a new package.

**5. What this deletes.** The `ch := make(chan error, 1)` / `<-ch` blocking pair is deleted
entirely — the exact mechanism that makes this function unsafe to call from any already-async call
site, which is precisely how `mjosefa-cms`'s `OnAfterReload` calls it today.

### What NOT to do

- **Do not touch `webtyp.com/dom`, `webtyp.com/view`, or `webtyp.com/layout/crudview`.** See §2 —
  the bug is local to this one function; those libraries' own listers are already correct.
- **Do not add a goroutine, a channel, `sync.WaitGroup`, or any other blocking-wait primitive
  anywhere in this fix.** The whole point is a non-blocking callback; reintroducing a wait
  primitive under a different name reproduces the exact bug this plan closes.
- **Do not change `ListAvailabilityArgs`, `OpListAvailability`, or anything about the wire
  contract.** Only `FreeSlots`' own Go-level shape changes; the op it calls is untouched.

## 4. Verification

```bash
gotest
# vet ✅, race ✅, tests ✅ — TestFreeSlots_EmptyWithoutServiceConfig and
# TestFreeSlots_NeverBlocksOnAChannel now PASS, and every other existing test still passes.
```

Confirm no blocking construct survives anywhere in the fixed function:
```bash
grep -n "make(chan" view.go   # must be empty
```

## 5. Downstream consumers (informational — not part of this plan's scope)

Once this ships as a new tagged version, `github.com/veltylabs/mjosefa-cms` needs: a version bump
(`go get github.com/veltylabs/appointment_booking@<new-version>`), and its own
`modules/appointment_booking/bookingview.go` updated to call the new callback-shaped `FreeSlots`
from inside `OnAfterReload` — setting `t.FreeSlots` and re-calling `t.SetItems(t.Items())` inside
the callback (so `targethour.TargetHour`'s existing reactive `rows` signal picks up the free-slot
rows once they arrive), instead of assigning a synchronously-returned value. Both are the consuming
app's own job, tracked there, not here.

## Stages

| # | Stage | File(s) | Acceptance |
|---|---|---|---|
| 1 | Change `FreeSlots` to the callback shape shown above | `view.go` | No `make(chan` left in the file |
| 2 | Verify | — | `gotest` green, both new tests pass, no other test changed |
