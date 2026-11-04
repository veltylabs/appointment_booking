---
PLAN: "feat: ImportReservation — store a past or future booking from a previous system with its real date and status"
EXECUTOR: jules
REVIEWER: none
STATUS: review
SESSION: 14647235089885463148
PR: https://github.com/veltylabs/appointment_booking/pull/24
REVIEWED_COMMIT: 3406e88313709ee3de9e378cd8b2f562c070963e
---

> Read AGENTS.md at the repo root before any change.
> **Finish by yourself:** when the delivery check passes, commit, push and open the pull request. Never ask whether to
> proceed or finalize — nobody answers, and a session that waits ends without your work.

# Plan — `ImportReservation`: bring the clinic's booking history

## Tools

```bash
go install webtyp.com/devflow/cmd/gotest@latest
go install webtyp.com/lang/cmd/langc@latest
```

Environment: `GOPRIVATE=webtyp.com/*,github.com/veltylabs/*`, `GOFLAGS=-mod=mod`.

## Why

The clinic's previous system holds 171,253 bookings since 2015. `CreateReservation` (`service.go:782`) is built for a
person booking **now**: it requires a free slot in today's schedule (`ListAvailability` → `ErrSlotTaken`), an
**active** service configuration, always starts in `PENDING`, and opens its own transaction (`m.db.Tx`). A booking
from 2019 with a professional who no longer works fails every one of those, although it is a real fact. The importer
(`mjosefa-cms/migrate/pastdb`) needs one method that stores such a booking as it was.

## Ecosystem search

- `CreateReservation` checks client, staff and service through `m.directory`, `m.staff`, `m.catalog` → **reuse the
  same ports**, skip only the live rules.
- Same contract already used by the importer for other modules (`patient_directory.ImportPatient`,
  `clinical_encounter.ImportVisit`, in their own plans) → **same shape**.

## Design gate

1. Prior art: Rails/Django data migrations write history bypassing UI-only rules but keep model validation
   (unverified links); followed — format and references validated, live rules skipped.
2. Name: `ImportReservation(r Reservation)` — "import this reservation as it is".
3. Ledger: +1 method, +1 error. Ways to book: unchanged (`CreateReservation` for people); history has its own path.
4. Belongs here: the module that owns `reservation`.
5. Deletes nothing.

## Stage 1 — `import.go` (new)

```go
// ImportReservation stores a booking from a previous system with its own id, date and status: it creates it, or
// updates it when that id already exists, so importing twice leaves the same row. It checks what CreateReservation
// checks about the data — id, tenant, a known status and origin, the client, the professional and the service exist,
// the configuration exists (active or not: a historical one may be inactive) — but none of the live rules: no free
// slot, no PENDING start, no hold, no event, and no transaction of its own (the importer already runs inside one).
//
// Example: r, err := booking.ImportReservation(Reservation{Id: "1650000000000000000", TenantId: t, ClientId: p,
//	EmployeeServiceConfigId: cfg, ReservationTime: int64(start), Status: StatusCompleted, Origin: OriginCounter})
func (m *Module) ImportReservation(r Reservation) (Reservation, error)
```

Body, in this order:
1. `r.Id`, `r.TenantId`, `r.ClientId`, `r.EmployeeServiceConfigId` non-empty and `r.ReservationTime > 0`, else `ErrMissingArgs`.
2. `r.Status` ∈ {`StatusConfirmed`, `StatusCancelled`, `StatusCompleted`, `StatusNoShow`} else `ErrImportStatus`;
   `r.Origin` ∈ {`OriginCounter`, `OriginOnline`} else `ErrMissingArgs`.
3. `cfg, err := m.repo.GetEmployeeServiceConfig(r.EmployeeServiceConfigId)`; error → return it; `cfg.TenantId != r.TenantId` → `ErrNotFound`. **Do not** check `cfg.IsActive`.
4. client / staff / service exist exactly as `CreateReservation` lines 29–53 (same errors).
5. Fill derived fields from the config and the instant (same as CreateReservation): `StaffIdsnapshot = cfg.StaffId`,
   `ServiceIdsnapshot = cfg.ServiceId`, `DurationMinSnapshot = cfg.DurationMin` **only when** `r.DurationMinSnapshot == 0`,
   `ReservationDate = tinytime.MidnightUTC(tinytime.Unix(r.ReservationTime))`, `LocalStringDate`/`LocalStringTime` with
   `tinytime.FormatDate`/`FormatTime`. Keep `r.PriceSnapshot`, `r.CurrencySnapshot`, `r.Notes`, `r.CreatorUserId` as given.
   `UpdatedAt = tinytime.NowNano()`, `UpdatedBy = r.CreatorUserId`, `Revision = 1`.
6. `existing, err := m.repo.GetReservation(r.Id)`: found and `existing.TenantId == r.TenantId` → `m.db.Update(&r,
   orm.Eq(Reservation_.Id, r.Id), orm.Eq(Reservation_.TenantId, r.TenantId))`; found with another tenant →
   `ErrIdTaken`; not found → `m.db.Create(&r)`.

New error in the file where `ErrIdTaken` lives, same style: `ErrImportStatus` = "an imported reservation must be confirmed, cancelled, completed or no-show".

## Stage 2 — tests (`tests/import_test.go`)

Reuse the fakes of `tests/booking_env_test.go` (directory, staff, catalog ports).

| Test | Setup → call | Expected |
|---|---|---|
| `TestImportReservationPast` | inactive config, time in 2019, `COMPLETED` | stored, status `COMPLETED`, `LocalStringDate == "2019-…"` |
| `TestImportReservationTwice` | same id twice, second with new notes | 1 row, notes updated, `Revision == 1` |
| `TestImportReservationNoSlotNeeded` | professional with no calendar blocks | stored (no `ErrSlotTaken`) |
| `TestImportReservationBadStatus` | `PENDING` | `ErrImportStatus` |
| `TestImportReservationUnknownClient` | client port says no | `ErrClientNotFound` |
| `TestImportReservationOtherTenant` | id exists in another tenant | `ErrIdTaken` |
| `TestImportReservationKeepsGivenDuration` | `DurationMinSnapshot: 45`, config 30 | 45 |

## Stage 3 — translations and docs

`langc sync .`, Spanish value for the new error. `README.md` + `docs/ARCHITECTURE.md`: one section "Importar historia"
**in Spanish**: what `ImportReservation` checks and what it deliberately skips, with the example.

## Delivery check

1. `gotest` passes (a PR that does not compile, or whose tests could not run, is a failed delivery).
2. `grep -n "m.db.Tx\|Publish\|ListAvailability" import.go` → empty.
3. `langc sync .` → 0 untranslated `es`.

Commit after every stage. Never edit a generated `*_orm.go`. Never run `git reset`, `git restore`, `git checkout --
<file>` or `git stash` on your own work. Stuck 30 minutes: open the PR anyway listing what fails.

## Stages

| Stage | Files | Done when |
|---|---|---|
| 1 | `import.go`, errors file | compiles |
| 2 | `tests/import_test.go` | tests pass |
| 3 | `lang.json`, `README.md`, `docs/ARCHITECTURE.md` | done |
