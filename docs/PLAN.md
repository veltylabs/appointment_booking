---
PLAN: "feat!: one zone for the clinic (Deps.Zone), one instant per booking — drop reservation_date, local strings and per-professional timezone"
EXECUTOR: jules
REVIEWER: none
STATUS: running
SESSION: 3228069055081192423
---

> Read AGENTS.md at the repo root before any change.
> **Finish by yourself:** when the delivery check passes, commit, push and open the pull request. Never ask whether to
> proceed or finalize — nobody answers, and a session that waits ends without your work.

# Plan — a booking is one instant; every local day and hour comes from the clinic's zone

> `docs/PLAN_BOOKING_FORMS.md` is another, parked plan: ignore it, do not edit or delete it.

## Tools

```bash
go install webtyp.com/devflow/cmd/gotest@latest
go install webtyp.com/ormc/cmd/ormc@latest
go get webtyp.com/time@v0.6.3 webtyp.com/appkit@v0.2.7
```

Environment: `GOPRIVATE=webtyp.com/*,github.com/veltylabs/*`, `GOFLAGS=-mod=mod`, `go mod tidy` before `ormc`.

## Why (read `docs/TIEMPO.md` first)

A booking stores the same hour four times: `reservation_time` (the instant, correct), `reservation_date` (midnight
**UTC** of the instant: a 21:30 winter booking in Santiago is 01:30 UTC, so it lands on the **next** day and the day's
agenda, which filters by this field in `repository.go:79`, does not show it), and `local_string_date` /
`local_string_time` (text made with a fixed machine offset, one hour off half of the year). And the zone comes from two
places: the installation and each professional's `work_calendar_config.timezone`. `webtyp.com/time` v0.6.3 removed the
implicit offset (`FormatDate`, `FormatTime`, `LocalMinutesToUnixUTC` no longer exist) and `appkit` v0.2.7 hands the
installation's zone to modules. Industry practice (RFC 5545 `TZID`, Google Calendar `{dateTime, timeZone}`, HL7 FHIR
`Appointment.start`, unverified links): store **one instant**, derive the local day and hour with the place's zone.

## Ecosystem search

- `time.Zone` (time v0.6.3 `zone.go`): `At`, `Day`, `Date`, `Clock`, `Minutes`, `Weekday` → **reuse**.
- `appkit.Deps.Zone`, `appkit.BrowserDeps{Caller, Zone}` (appkit v0.2.7) → the zone's only source.
- A day key stays what it is today: midnight UTC of a civil date (`work_calendar_block.specific_date`, `boundsForDay`).

## Design gate

1. Prior art: above; followed.
2. Names: `Deps.Zone`; helpers `dayKey(u)` and `dateOf(dayKey)` (unexported).
3. Ledger: fields −3 (`reservation_date`, `local_string_date`, `local_string_time`) and −1
   (`work_calendar_config.timezone`, plus `Timezone` in `UpsertCalendarConfigArgs`); sources of a zone 2 → 1.
4. Belongs here: owner of the booking.
5. Deletes: those four fields, `LocalIntToUnixUTC`, every use of the removed time functions.

## APIs used (time v0.6.3, appkit v0.2.7)

| Symbol | Signature |
|---|---|
| `(time.Zone) At` | `At(date string, minutes int) (Unix, error)` — local date "YYYY-MM-DD" + minutes → instant |
| `(time.Zone) Day` | `Day(date string) (from, to Unix, err error)` — `[from, to)` |
| `(time.Zone) Date`, `Clock`, `Minutes`, `Weekday` | of an instant, in the zone ("YYYY-MM-DD", "HH:MM", int, 0 = Sunday) |
| `time.ParseDateUTC`, `time.CivilUTC`, `time.WeekdayUTC`, `time.MidnightUTC` | UTC, for day keys |
| `time.LoadZone` (tests, seed, demo) | `func LoadZone(name string) (Zone, error)` |
| `appkit.Deps.Zone` / `appkit.BrowserDeps.Zone` | `time.Zone` |

If a signature in the published tag differs from this table, stop and name the symbol in the PR; never adapt it by guessing.

## Stage 1 — model (`model.go`, then `ormc`; never edit `model_orm.go` by hand)

- `ReservationModel`: delete the fields `reservation_date` (line ~121), `local_string_date`, `local_string_time`
  (lines ~123–124).
- `WorkCalendarConfigModel`: delete `timezone` (line ~57). `UpsertCalendarConfigArgsModel` (line ~302): delete `timezone`;
  `ops.go:376` stops passing it.

## Stage 2 — the zone and two helpers (`service.go`)

`Deps` gets `Zone time.Zone // la zona de la instalación (appkit.Deps.Zone); obligatoria`; `Module` keeps `m.zone`.

```go
// dayKey es el día local de un instante en la zona de la clínica, como clave (su medianoche UTC): una cita a las
// 21:30 de invierno en Santiago (01:30 UTC del día siguiente) queda en SU día.
func (m *Module) dayKey(u tinytime.Unix) tinytime.Unix {
	d, _ := tinytime.ParseDateUTC(m.zone.Date(u))
	return d
}

// dateOf es la fecha "YYYY-MM-DD" de una clave de día.
func dateOf(dayKey tinytime.Unix) string {
	y, mo, d, _, _, _ := tinytime.CivilUTC(dayKey)
	return fmt.Sprintf("%04d-%02d-%02d", y, mo, d)
}

// at es el instante de un minuto local de un día clave, en la zona de la clínica.
func (m *Module) at(dayKey tinytime.Unix, minutes int) tinytime.Unix {
	u, _ := m.zone.At(dateOf(dayKey), minutes)
	return u
}
```

Delete `LocalIntToUnixUTC`. Replace every `LocalIntToUnixUTC(d, x, cfg.Timezone)` (`service.go:637,638,660,661,668,669`)
with `m.at(d, x)`. `service.go:422,473,644` `tinytime.Weekday(<day key>)` → `tinytime.WeekdayUTC(...)`.

## Stage 3 — one instant per booking

- `CreateReservation` (`service.go:~882-885`) and `CreateOverbooking` (`~1072-1075`): delete the `ReservationDate`,
  `LocalStringDate`, `LocalStringTime` assignments.
- `checkReservation` (`service.go:1249`) `d := r.ReservationDate` → `d := m.dayKey(tinytime.Unix(r.ReservationTime))`;
  `service.go:1357,1464` `m.boundsForDay(r.ReservationDate)` → `m.boundsForDay(m.dayKey(tinytime.Unix(r.ReservationTime)))`.
- `repository.go:74-112` `ListReservationsByStaff` / `ListReservationsByTenantRange` (`from`, `to` are **day keys**,
  inclusive): the repository receives instants instead — change their signature to `(…, fromUtc, toUtc time.Unix)` meaning
  `fromUtc <= reservation_time < toUtc`, and filter `Where(Reservation_.ReservationTime).Gte(int64(fromUtc)).Where(Reservation_.ReservationTime).Lt(int64(toUtc))`.
  Every caller converts its day-key range: `fromUtc := m.at(from, 0)`; `_, toUtc, _ := m.zone.Day(dateOf(to))`.
- `import.go`: delete lines 69–79 (`ReservationDate`, local strings). The importer passes the exact instant.
- `lister.go:99-100` `Day: r.LocalStringDate, Hour: r.LocalStringTime` → `Day: zone.Date(…ReservationTime)`,
  `Hour: zone.Clock(…)` with the lister's zone (Stage 4). `lister.go:146` `LocalIntToUnixUTC(d, m, s.cfg.Timezone)` →
  `zone.At(r.Day, m)` (it already has the "YYYY-MM-DD" text). `lister.go:264` `time.ParseDate(` → `time.ParseDateUTC(`.

## Stage 4 — screens get the zone

- `ui/screens.go:30` `Screens(caller router.Caller, ids model.IDGenerator, tenantID string)` → add `zone time.Zone` after
  `caller`; give it to every lister/view that shows a day or hour.
- `view.go:120` `tinytime.FormatTime(ts.StartUtc)` → `zone.Clock(ts.StartUtc)`; `ui/booking_flow.go:316`
  `tinytime.FormatDate(startUtc)` → `zone.Date(startUtc)`; `ui/booking_flow.go:320` `tinytime.FormatTime(startUtc)` (and
  its cut) → `zone.Clock(startUtc)`; `ui/scheduleview.go:352` `ParseDate(` → `ParseDateUTC(`; `ui/scheduleview.go:357`
  `CivilDate(` → `CivilUTC(` (day keys).
- `seed/seed.go:130` `Weekday(` → `WeekdayUTC(`; `seed/seed.go:139` `ParseDate(` → `ParseDateUTC(`.
- The `web/` demo loads `time.LoadZone("America/Santiago")` and passes it to `New` (`Deps.Zone`) and to `ui.Screens`.

## Stage 5 — tests

Every test builds the module with `Deps{…, Zone: santiago}` (`santiago, _ := time.LoadZone("America/Santiago")`) and
drops the removed fields. New tests:

| Test | Setup → call | Expected |
|---|---|---|
| `TestNightBookingInItsDay` | config + block 20:00–23:00 on 2019-07-15 (Monday); `CreateReservation` at 21:30 local | `ListReservationsByStaff` for day 2019-07-15 returns it; for 2019-07-16 does not |
| `TestSlotsAcrossDST` | weekly block 09:00 on a Monday in January and in July | slot instants 12:00 UTC (summer) and 13:00 UTC (winter) |
| `TestImportKeepsInstant` | `ImportReservation` with `ReservationTime` = 2019-07-16 01:30 UTC | listed on 2019-07-15; lister shows "21:30" |

## Stage 6 — docs (Spanish)

Rewrite `docs/TIEMPO.md`: a booking keeps **only** `reservation_time`; the day and hour are derived with the clinic's
zone (`Deps.Zone`); the per-professional zone is gone (a clinic has one). Remove the sections about local strings and
the fixed offset. `README.md`/`docs/ARCHITECTURE.md`: `Deps.Zone` is required.

## Delivery check

1. `gotest` passes (a PR that does not compile, or whose tests could not run, is a failed delivery).
2. `grep -rn "ReservationDate\|LocalStringDate\|LocalStringTime\|LocalIntToUnixUTC\|cfg.Timezone" --include=*.go . | grep -v _orm.go` → empty.
3. `grep -rn -E "time\.(FormatDate|FormatTime|IsToday|LocalMinutesToUnixUTC|CivilDate|ParseDate|Weekday)\(" --include=*.go .` → empty.

Commit after every stage. Never edit `model_orm.go` by hand; a build error there means the hand-written code is wrong.
Never run `git reset`, `git restore`, `git checkout -- <file>` or `git stash` on your own work. Stuck 30 minutes: open
the PR anyway listing what fails.

## Stages

| Stage | Files | Done when |
|---|---|---|
| 1 | `model.go`, `model_orm.go` (ormc), `ops.go` | ormc ran clean |
| 2 | `service.go` | compiles |
| 3 | `service.go`, `repository.go`, `import.go`, `lister.go` | compiles |
| 4 | `ui/*`, `view.go`, `seed/seed.go`, `web/*` | compiles for browser |
| 5 | `tests/*` | tests pass |
| 6 | `docs/TIEMPO.md`, `README.md`, `docs/ARCHITECTURE.md` | done |
