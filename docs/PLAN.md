---
PLAN: "feat!: clinic zone (Deps.Zone), single instant per booking, and decouple from all sibling modules via interfaces.go"
EXECUTOR: jules
REVIEWER: none
STATUS: dispatch
---

> Read AGENTS.md at the repo root before any change.
> **Finish by yourself:** when the delivery check passes, commit, push and open the pull request. Never ask whether to
> proceed or finalize — nobody answers, and a session that waits ends without your work.

# Plan — clinic zone (`Deps.Zone`), one instant per booking, and zero sibling module imports

> `docs/PLAN_BOOKING_FORMS.md` is another, parked plan: ignore it, do not edit or delete it.

## Tools

```bash
go install webtyp.com/devflow/cmd/gotest@latest
go install webtyp.com/ormc/cmd/ormc@latest
go get webtyp.com/time@v0.6.4 webtyp.com/appkit@v0.2.7
```

Environment: `GOPRIVATE=webtyp.com/*,github.com/veltylabs/*`, `GOFLAGS=-mod=mod`, `go mod tidy` before `ormc`.

## Why

1. **One zone for the clinic (`Deps.Zone`), one instant per booking (`reservation_time`):**
   A booking previously stored the same hour four times (`reservation_time`, `reservation_date`, `local_string_date`, `local_string_time`) and a per-professional `work_calendar_config.timezone`. `webtyp.com/time` v0.6.4 removed the implicit offset (`FormatDate`, `FormatTime`, `LocalMinutesToUnixUTC` no longer exist) and renamed `DayBounds`/`Unbounded` to `DayLimits`/`Unlimited`. A booking keeps **only** `reservation_time` (UTC seconds), and every local date or clock string is derived through `Deps.Zone` (`time.Zone`).
2. **Decouple `appointment_booking` from all 6 sibling modules:**
   `appointment_booking` currently imports `business_calendar`, `device_manager`, `patient_directory`, `payment_manager`, `service_catalog`, and `staff_manager` in `ui/`, `seed/`, `web/`, and `tests/`. A domain module must never import another module in any package: all external contracts live in `interfaces.go` at the module root with plain types, `ui/` calls only `appointment_booking`'s own operations, and the consuming app wires dependencies via `appkit.Find` in `module/`.

## Ecosystem search

- `webtyp.com/time` (v0.6.4): `time.Zone` (`At`, `Day`, `Date`, `Clock`, `Minutes`, `Weekday`), `time.DayLimits`, `time.Unlimited()`, `time.ParseDateUTC`, `time.CivilUTC`, `time.WeekdayUTC`, `time.MidnightUTC` → **reuse** (always import without alias as `time`, never `tinytime` or `tintime`).
- `webtyp.com/appkit` (v0.2.7): `appkit.Module`, `appkit.ModuleServer`, `appkit.ModuleBrowser`, `appkit.Deps.Zone`, `appkit.BrowserDeps{Caller, Zone}`, `appkit.Find[T](d)` → **reuse**.
- Reference decoupled module: `veltylabs/modules/room_layout` (`interfaces.go`, `module/{module.go,server.go,browser.go}`, `web/browser/main.go`).

## Design gate

1. Prior art: RFC 5545 `TZID` + Google Calendar + HL7 FHIR `Appointment.start` for single-instant bookings; Hexagonal Architecture ports and Go consumer-owned interfaces for module decoupling.
2. Novice-name test:
   - `OpeningHours` (`Deps.Hours`) with `DayLimits(date time.Unix) (time.DayLimits, error)`, `WeekdayLimits(dayOfWeek int) (time.DayLimits, error)`, `ClosedDates(from, to time.Unix) (holidays, closures []string, err error)` — replaces `BoundsReader`.
   - `PatientDirectory` (`Deps.Patients`) with **only** `FindPatient(tenantID, documentNumber string) (id, name, phone, email string, err error)` and `RegisterPatient(tenantID, id, documentNumber, name, phone, email string) (patientID string, err error)` — replaces `DirectoryReader` (no `ClientExists`, no `LinkPatient`).
   - `StaffReader` (`Deps.Staff`) adds `OccupantOptions(tenantID string) ([]fmt.KeyValue, error)`.
   - `PaymentGateway` (`Deps.Payments`) extends `PaymentReader` with charge/counter/terminal/online methods so `ui/` calls `appointment_booking`'s own ops.
   - `ConfirmedByStaff` method on `*Module` satisfies `clinical_encounter.AgendaReader` with plain slices.
3. Complexity ledger:
   - Redundant time/zone fields on models: −4 (`reservation_date`, `local_string_date`, `local_string_time`, `work_calendar_config.timezone`)
   - Sibling module imports in `go.mod`: 0 (−6)
   - Duplicate staff CRUD tab in `ui/screens.go`: −1 (`staffScreen` removed; `staff_manager` owns it)
4. Where it belongs: `interfaces.go` at root of `appointment_booking`; `module/` subpackage for `appkit.Module`.
5. Deletes: 4 model fields, `LocalIntToUnixUTC`, `BoundsReader`, `DirectoryReader`, `staffScreen` in `ui/screens.go`, `web/browser.go` (replaced by `web/browser/main.go`), and all 6 `github.com/veltylabs/*` dependencies in `go.mod`.

## Stage 1 — `interfaces.go` and `model.go` + `ormc`

Create `interfaces.go` at the root of `appointment_booking` (move all cross-module interfaces and `SchedulingService` out of `service.go` into `interfaces.go`):

```go
package appointmentbooking

import (
	"webtyp.com/fmt"
	"webtyp.com/time"
)

// StaffReader consulta existencia, ficha y opciones de profesionales clínicos.
// Ejemplo: ok, err := s.StaffExists("t1", "staff1")
type StaffReader interface {
	StaffExists(tenantId, staffId string) (bool, error)
	StaffCard(tenantId, staffId string) (name, specialty string, active bool, err error)
	OccupantOptions(tenantID string) ([]fmt.KeyValue, error)
}

// CatalogReader consulta existencia y datos comerciales de un servicio clínico.
// Ejemplo: name, price, curr, ok, err := c.ServiceCard("t1", "svc1")
type CatalogReader interface {
	ServiceExists(tenantId, serviceId string) (bool, error)
	ServiceCard(tenantId, serviceId string) (name string, price float64, currency string, active bool, err error)
}

// PatientDirectory busca y registra pacientes por su número de documento.
// Ejemplo: id, name, phone, email, err := p.FindPatient("t1", "12.345.678-5")
type PatientDirectory interface {
	FindPatient(tenantID, documentNumber string) (id, name, phone, email string, err error)
	RegisterPatient(tenantID, id, documentNumber, name, phone, email string) (patientID string, err error)
}

// OpeningHours informa las ventanas horarias y días cerrados del establecimiento.
// Ejemplo: lim, err := h.DayLimits(date)
type OpeningHours interface {
	DayLimits(date time.Unix) (time.DayLimits, error)
	WeekdayLimits(dayOfWeek int) (time.DayLimits, error)
	ClosedDates(from, to time.Unix) (holidays, closures []string, err error)
}

// PaymentGateway verifica y procesa cobros asociados a una reserva.
// Ejemplo: amt, curr, err := p.PaidAmount("t1", "pay1", "appointment_booking", "res1")
type PaymentGateway interface {
	PaidAmount(tenantId, paymentId, referenceModule, referenceId string) (amount int64, currency string, err error)
	ChargeDetail(tenantID, referenceModule, referenceID string) (payable, terminal bool, amount int64, currency string, err error)
	RegisterCounterPayment(tenantID, id, referenceModule, referenceID, method, actorID string) (paymentID string, err error)
	StartTerminalPayment(tenantID, id, referenceModule, referenceID, method string) (paymentID string, err error)
	RefreshTerminalPayment(tenantID, paymentID string) (status string, err error)
	StartOnlinePayment(tenantID, id, referenceModule, referenceID, returnTo string) (redirectURL, tokenField, token string, err error)
}
```
Also include `SchedulingService` in `interfaces.go` (updating `GetDayBounds` → `GetDayLimits(date time.Unix) (time.DayLimits, error)` and replacing `tinytime` with `time`).

In `model.go`:
- `ReservationModel`: delete `reservation_date`, `local_string_date`, `local_string_time`.
- `WorkCalendarConfigModel` and `UpsertCalendarConfigArgsModel`: delete `timezone`.
- Rename `GetDayBoundsArgsModel` (`get_day_bounds_args`) → `GetDayLimitsArgsModel` (`get_day_limits_args`) and `DayBoundsResultModel` (`day_bounds_result`) → `DayLimitsResultModel` (`day_limits_result`).
- Add transport models for the decoupled UI operations:
  - Staff options (`OpListStaffOptions = "list_staff_options"`):
    `ListStaffOptionsArgsModel` (`list_staff_options_args`: `tenant_id`), `StaffOptionModel` (`staff_option`: `id`, `name`).
  - Calendar closures (`OpListCalendarClosures = "list_calendar_closures"`):
    `ListCalendarClosuresArgsModel` (`list_calendar_closures_args`: `tenant_id`, `from` int, `to` int), `CalendarClosuresResultModel` (`calendar_closures_result`: `holidays` `model.StringSlice()`, `closures` `model.StringSlice()`).
  - Patient step 4 (`OpFindBookingPatient = "find_booking_patient"`, `OpRegisterBookingPatient = "register_booking_patient"`, `OpResolveOnlinePatient = "resolve_online_patient"`):
    `FindBookingPatientArgsModel` (`find_booking_patient_args`: `tenant_id`, `document_number` `NotNull: true`), `BookingPatientModel` (`booking_patient`: `id`, `tenant_id`, `document_number` `NotNull: true`, `name` `NotNull: true`, `phone`, `email`).
  - Payment step 6 (`OpGetBookingCharge = "get_booking_charge"`, `OpCreateBookingCounterPayment = "create_booking_counter_payment"`, `OpStartBookingTerminalPayment = "start_booking_terminal_payment"`, `OpRefreshBookingTerminalPayment = "refresh_booking_terminal_payment"`, `OpStartBookingOnlinePayment = "start_booking_online_payment"`):
    - `BookingChargeArgsModel` (`booking_charge_args`: `tenant_id`, `reservation_id`)
    - `BookingChargeResultModel` (`booking_charge_result`: `payable` bool, `terminal` bool, `amount` int, `currency` text)
    - `BookingPaymentArgsModel` (`booking_payment_args`: `tenant_id`, `id`, `reservation_id`, `method`, `return_to`)
    - `BookingPaymentResultModel` (`booking_payment_result`: `payment_id`, `status`, `redirect_url`, `token_field`, `token`)
- Run `ormc` and `gofmt -w .` (never edit `model_orm.go` by hand).

## Stage 2 — `service.go`, `repository.go`, `import.go`, `lister.go`, `booking.go`, and `ops.go`

Replace every `tinytime "webtyp.com/time"` import with `"webtyp.com/time"` across all files.

In `service.go`:
- Update `Deps`:
  ```go
  type Deps struct {
  	Staff     StaffReader
  	Catalog   CatalogReader
  	Patients  PatientDirectory
  	Hours     OpeningHours // opcional: nil = time.Unlimited()
  	Payments  PaymentGateway
  	IDs       model.IDGenerator
  	Publisher events.Publisher
  	Zone      time.Zone // obligatoria
  	TenantID  string
  }
  ```
  Validate in `New`: `Staff != nil`, `Catalog != nil`, `Patients != nil`, `IDs != nil`, `Zone.Name() != ""` (return descriptive error if missing).
- Add zone helpers on `*Module`:
  ```go
  func (m *Module) dayKey(u time.Unix) time.Unix {
  	d, _ := time.ParseDateUTC(m.zone.Date(u))
  	return d
  }
  func dateOf(dayKey time.Unix) string {
  	y, mo, d, _, _, _ := time.CivilUTC(dayKey)
  	return fmt.Sprintf("%04d-%02d-%02d", y, mo, d)
  }
  func (m *Module) at(dayKey time.Unix, minutes int) time.Unix {
  	u, _ := m.zone.At(dateOf(dayKey), minutes)
  	return u
  }
  ```
- Delete `LocalIntToUnixUTC`. Replace all `LocalIntToUnixUTC(d, x, cfg.Timezone)` with `m.at(d, x)`. Replace `time.Weekday(<dayKey>)` with `time.WeekdayUTC(<dayKey>)`.
- Rename `GetDayBounds` → `GetDayLimits(date time.Unix) (time.DayLimits, error)` and `boundsForDay` → `limitsForDay(date time.Unix) (time.DayLimits, error)` using `m.hours.DayLimits` / `m.hours.WeekdayLimits` and `time.Unlimited()`.
- In `CreateReservation`, `CreateOverbooking`, `RescheduleReservation`, and `import.go`:
  - Remove `ReservationDate`, `LocalStringDate`, `LocalStringTime` assignments.
  - Remove `m.directory.ClientExists(...)` (patient lookup/registration happens through `PatientDirectory`; require `ClientId != ""` as part of `ErrMissingArgs`).
- In `repository.go`:
  - `ListReservationsByStaff` and `ListReservationsByTenantRange` take `(..., fromUtc, toUtc time.Unix)` and filter `Where(Reservation_.ReservationTime).Gte(int64(fromUtc)).Where(Reservation_.ReservationTime).Lt(int64(toUtc))`.
  - Callers that start from day keys convert them via `fromUtc := m.at(from, 0)` and `_, toUtc, _ := m.zone.Day(dateOf(to))`.
- In `lister.go`:
  - `NewReservationLister` receives `zone time.Zone` (stored on `ReservationLister`).
  - Derive `Day: s.zone.Date(time.Unix(r.ReservationTime))` and `Hour: s.zone.Clock(time.Unix(r.ReservationTime))`.
  - Replace `LocalIntToUnixUTC` with `s.zone.At(r.Day, m)` and `ParseDate` with `ParseDateUTC`.

In `booking.go` and `ops.go`:
- Add `ConfirmedByStaff(tenantID, staffID string, from, to time.Unix) (reservationIDs, patientIDs, serviceIDs []string, startTimes []time.Unix, err error)`: queries `m.repo.ListReservationsByStaff(tenantID, staffID, from, to)` and returns the 4 slices for reservations with `Status == StatusConfirmed`.
- Add domain methods and RPC handlers in `ops.go` for:
  - `OpGetDayLimits = "get_day_limits"` (replaces `OpGetDayBounds`)
  - `OpListStaffOptions = "list_staff_options"` (calls `m.staff.OccupantOptions`)
  - `OpListCalendarClosures = "list_calendar_closures"` (calls `m.hours.ClosedDates`; if `m.hours == nil`, returns empty slices)
  - `OpFindBookingPatient = "find_booking_patient"` (calls `m.patients.FindPatient`)
  - `OpRegisterBookingPatient = "register_booking_patient"` (calls `m.patients.RegisterPatient`)
  - `OpResolveOnlinePatient = "resolve_online_patient"`: calls `m.patients.FindPatient(args.TenantId, args.DocumentNumber)`; if `err == nil`, returns `BookingPatient{Id: existingID}` (without modifying or exposing stored personal data); if `model.KindOf(err) == model.ErrorNotFound`, calls `m.patients.RegisterPatient` and returns `BookingPatient{Id: newID}` (if `RegisterPatient` fails with conflict/duplicate, re-runs `FindPatient` once and returns its ID).
  - `OpGetBookingCharge`, `OpCreateBookingCounterPayment`, `OpStartBookingTerminalPayment`, `OpRefreshBookingTerminalPayment`, `OpStartBookingOnlinePayment`: delegate to `m.payments` (`PaymentGateway`).

## Stage 3 — `ui/*`, `view.go`, `seed/seed.go`, `module/`, and `web/browser/main.go`

Remove all imports of `business_calendar`, `patient_directory`, `payment_manager`, `service_catalog`, and `staff_manager` from `ui/`, `seed/`, and `web/`.

- `view.go`: `NewEmployeeServiceConfigView` and slot formatters use `zone.Clock(ts.StartUtc)`.
- `ui/screens.go`:
  - `func Screens(caller router.Caller, zone time.Zone, ids model.IDGenerator, tenantID string) []view.Screen`
  - Delete `staffScreen` (D6: `staff_manager` owns the staff CRUD screen). `personalGroup.Children` has only `scheduleScreen` and `servicesScreen`.
- `ui/staffpicker.go`:
  - Remove `staffmanager` import; call `ab.ModelName + "." + ab.OpListStaffOptions` (`&ab.ListStaffOptionsArgs{TenantId: p.tenantID}`) into `&ab.StaffOptionList{}`.
- `ui/scheduleview.go`:
  - Pass `zone time.Zone` to `NewScheduleView(caller, zone, tenantID)` and set `Zone: s.zone` on `&scheduleeditor.ScheduleEditor{...}`.
  - Remove `businesscalendar` import; call `ab.ModelName + "." + ab.OpListCalendarClosures` instead of `OpListHolidays`/`OpListClosures`.
  - Remove `Timezone` from `UpsertCalendarConfigArgs`; replace `ParseDate` → `ParseDateUTC` and `CivilDate` → `CivilUTC`.
- `ui/step_patient.go`:
  - Remove `patientdirectory` import; rename `rut` signal/fields to `documentNumber` (`DocumentNumber`).
  - Counter search calls `ab.ModelName + "." + ab.OpFindBookingPatient`; counter registration calls `ab.ModelName + "." + ab.OpRegisterBookingPatient`; online continue calls `ab.ModelName + "." + ab.OpResolveOnlinePatient`.
- `ui/step_payment.go`, `ui/online.go`, `ui/booking_flow.go`:
  - Remove `paymentmanager` and `paymentmanagerui` imports.
  - Pass `zone time.Zone` into `BookingFlow` (`NewBookingView(caller, zone, ids, tenantID)` and `newBookingFlow(caller, zone, ids, tenantID, true)`), using `f.zone.Date(startUtc)` and `f.zone.Clock(startUtc)` in `slotLabel`.
  - Format prices with a helper `FormatAmount(amount int64, currency string) string` in `appointment_booking` (CLP `"$15.000"`, USD `"US$15,00"`).
  - In `ui/online.go`, read query params `"status"` and `"payment"` via `dom.GetQuery` and `dom.ClearQuery` using `appointment_booking`'s own payment status constants (`PaymentStatusPaid = "PAID"`, `PaymentStatusPending = "PENDING"`, `PaymentStatusCancelled = "CANCELLED"`, `PaymentStatusExpired = "EXPIRED"`, `PaymentStatusFailed = "FAILED"`).
  - In `ui/step_payment.go`, render the counter/terminal/online payment UI calling `ab.OpGetBookingCharge`, `ab.OpCreateBookingCounterPayment`, `ab.OpStartBookingTerminalPayment`, `ab.OpRefreshBookingTerminalPayment`, and `ab.OpStartBookingOnlinePayment`.
- `seed/seed.go`:
  - Remove `patientseed`, `catalogseed`, `staffseed` imports.
  - Define plain seed types `StaffSeed{ID, Name, Specialty string}`, `ServiceSeed{ID, Name, Specialty string}`, `LoadOptions{Staff []StaffSeed, Services []ServiceSeed, PatientIDs []string}` (with sensible default demo values when `opts` is omitted).
  - Use `m.Zone()` (`zone.At(isoDate, 540)` and `zone.At(isoDate, 570)`) instead of `LocalIntToUnixUTC`, and `time.WeekdayUTC` / `time.ParseDateUTC`.
- Create `module/module.go`, `module/server.go` (`//go:build !browser`), `module/browser.go` (`//go:build browser`) using `appkit.Find` for `StaffReader`, `CatalogReader`, `PatientDirectory`, and optional `OpeningHours` and `PaymentGateway`.
- Replace `web/browser.go` with `web/browser/main.go` (delete `web/browser.go`, no build tag on `web/browser/main.go`) using in-memory fakes of `StaffReader`, `CatalogReader`, `PatientDirectory`, `OpeningHours`, and `PaymentGateway`.

## Stage 4 — `tests/*`, docs, and `go.mod`

- Update all files in `tests/*`:
  - Remove imports of `business_calendar`, `device_manager`, `patient_directory`, `payment_manager`, `service_catalog`, and `staff_manager`.
  - Pass `Zone: santiago` (`santiago, _ := time.LoadZone("America/Santiago")`), `Patients: ...` (`PatientDirectory` fake), `Hours: ...` (`OpeningHours` fake), `Payments: ...` (`PaymentGateway` fake).
  - Add the 3 timezone tests (`TestNightBookingInItsDay`, `TestSlotsAcrossDST`, `TestImportKeepsInstant`) and test `ConfirmedByStaff` and `OpResolveOnlinePatient`.
- Rewrite `docs/TIEMPO.md`, `README.md`, and `docs/ARCHITECTURE.md` in Spanish documenting `Deps.Zone`, single instant `reservation_time`, `interfaces.go`, and `module.Module`.
- Run `go mod tidy` and verify zero `github.com/veltylabs/*` dependencies remain in `go.mod`.

## Delivery check

1. `gotest` passes.
2. `grep -rn "github.com/veltylabs/" --include="*.go" go.mod . | grep -v "github.com/veltylabs/appointment_booking"` → empty.
3. `grep -rn "ReservationDate\|LocalStringDate\|LocalStringTime\|LocalIntToUnixUTC\|cfg.Timezone\|BoundsReader\|GetDayBounds\|GetWeekdayBounds\|DirectoryReader\|ClientExists\|tinytime\|tintime" --include="*.go" . | grep -v _orm.go` → empty.
4. `grep -rn -E "time\.(FormatDate|FormatTime|IsToday|LocalMinutesToUnixUTC|CivilDate|ParseDate|Weekday)\(" --include="*.go" .` → empty.

Commit after every stage. Never edit `model_orm.go` by hand.

## Stages

| Stage | Files | Done when |
|---|---|---|
| 1 | `interfaces.go`, `model.go`, `model_orm.go` (ormc) | ormc ran clean |
| 2 | `service.go`, `repository.go`, `import.go`, `lister.go`, `booking.go`, `ops.go` | root package compiles |
| 3 | `ui/*`, `view.go`, `seed/seed.go`, `module/*`, `web/browser/main.go` (delete `web/browser.go`) | compiles for server & browser |
| 4 | `tests/*`, `docs/TIEMPO.md`, `README.md`, `docs/ARCHITECTURE.md`, `go.mod`, `go.sum` | `gotest` passes & zero sibling imports |
