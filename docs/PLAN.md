---
PLAN: "fix: weekly schedule validated against the weekly hours, not the next occurrence's date (a holiday blocked saving a weekday)"
EXECUTOR: jules
REVIEWER: none
STATUS: review
SESSION: 3416651458354618066
PR: https://github.com/veltylabs/appointment_booking/pull/18
---

# Plan — `SaveDayBlocks` contra el horario semanal

> Master: `webtyp/docs/NO_REFLECTION_MASTER_PLAN.md` (desbloquea `gotest` de este repo).
> **Prerrequisito:** `go get github.com/veltylabs/business_calendar@latest` y confirmar que su `Module`
> tiene `GetWeekdayBounds(dayOfWeek int) (tinytime.DayBounds, error)`. Si no existe, parar y
> reportarlo: no implementar un sustituto local.

## 1. El problema (bug de producto)

`SaveDayBlocks` (`service.go`) valida la plantilla **semanal** de un funcionario así:

```go
bounds, err := m.boundsForDay(nextWeekday(unixNowSeconds(), dayOfWeek))
```

`GetDayBounds(date)` aplica feriados y cierres de esa fecha. Si la próxima ocurrencia del día es un
feriado, la plantilla de ese día se rechaza con `ErrBlockOnClosedDay`: el 2026-10-08 el próximo
lunes era el 2026-10-12 (feriado) y **no se podía guardar el horario de los lunes**. Lo destapa
`tests/seed_test.go` (`TestSeed_LoadPopulatesServicesAndReservations`), que hoy falla y deja
`gotest` en rojo.

Un feriado es una excepción de una fecha. La plantilla semanal se valida contra el horario semanal
(`GetWeekdayBounds`); las reservas de una fecha concreta siguen validándose con `GetDayBounds`, que
es donde los feriados corresponden.

## 2. Design gate (api-design)

1. **Antecedentes.** Google Business Profile (`regularHours` vs `specialHours`), Schema.org
   `OpeningHoursSpecification` (`dayOfWeek` vs `validFrom/validThrough`), Calendly/Cal.com (horas
   regulares + overrides por fecha). Misma separación que ya existe en `business_calendar`.
2. **Nombre.** `GetWeekdayBounds(dayOfWeek int)` en la interfaz local `BoundsReader` — el mismo nombre
   que expone `business_calendar` (lo satisface estructuralmente, sin importarse; AGENDA_DOMAIN §3-bis).
3. **Balance.** +1 método en una interfaz local · formas de validar una plantilla semanal: 1 (antes
   la "fecha representativa", que muere).
4. **Dónde va.** La interfaz `BoundsReader` de este paquete (declarada aquí, no importada).
5. **Qué borra.** `nextWeekday` (si queda sin uso) y la validación por fecha representativa.

## 3. La corrección

1. `service.go`, interfaz `BoundsReader`:

   ```go
   type BoundsReader interface {
   	// GetDayBounds: which minutes of this concrete date are usable
   	// (holidays and closures applied). date is midnight UTC in seconds.
   	GetDayBounds(date int64) (tinytime.DayBounds, error)
   	// GetWeekdayBounds: the establishment's regular hours for a weekday
   	// (0 = Sunday … 6 = Saturday), without date exceptions. A weekly
   	// schedule is validated against this, never against one date.
   	GetWeekdayBounds(dayOfWeek int) (tinytime.DayBounds, error)
   }
   ```

   Actualizar el comentario de la interfaz con la distinción.
2. Junto a `boundsForDay`, una `boundsForWeekday(dow int)` con la misma regla para `m.bounds == nil`
   (devuelve `tinytime.Unbounded()`).
3. `SaveDayBlocks`: reemplazar la línea de `nextWeekday` por `bounds, err := m.boundsForWeekday(dayOfWeek)`.
   `SaveDateBlocks` / `MarkWorkingDays` (bloques de **fecha** concreta) siguen con `boundsForDay`.
4. Si `nextWeekday` queda sin uso, borrarlo (y `unixNowSeconds` si también queda sin uso).
5. `tests/mocks_test.go`, `MockBoundsReader`: agregar `GetWeekdayBounds` con un campo
   `WeekdayOverrides []WeekdayBound` (scan lineal, sin `map`) y `Default` para el resto.

## 4. Tests (rojo primero, en `tests/`)

- `MockBoundsReader` con `Default` abierto 08:00–18:00 y un `Override` **cerrado** en la fecha del
  próximo lunes (feriado): `SaveDayBlocks(…, 1, bloques 09:00–13:00)` → **ok** (hoy falla con
  `ErrBlockOnClosedDay`).
- Domingo cerrado en `WeekdayOverrides`: `SaveDayBlocks(…, 0, …)` → `ErrBlockOnClosedDay` (sigue rechazando).
- Bloque fuera del horario semanal → `ErrBlockOutsideBusinessHours`.
- `SaveDateBlocks` en la fecha feriado sigue → `ErrBlockOnClosedDay` (las fechas no cambian).
- `TestSeed_LoadPopulatesServicesAndReservations` pasa **cualquier** día (no depende de qué caiga el
  próximo lunes).
- `gotest` verde (vet, race, tests, wasm).

## 5. Criterios de aceptación

- `grep -n 'nextWeekday(unixNowSeconds' service.go` → vacío.
- `gotest` verde, incluido el test del seed.
- Ningún exportado nuevo fuera del método de la interfaz `BoundsReader`.

## 6. Restricciones

Las de `AGENTS.md`. Además: sin `map`, sin `reflect`, sin `errors.Is`/`errors.As`, sin
`==`/`!=`/`switch` entre valores de interfaz con operandos no nil; errores con `webtyp.com/fmt`, no con
`errors.New` de la biblioteca estándar. No tocar otros repos.

## Executor notes
All tasks completed successfully. Included GetWeekdayBounds in BoundsReader and updated SaveDayBlocks to use boundsForWeekday. Tests were passing correctly. No deviations from the plan except that unixNowSeconds was not removed because it was used in another place.
