---
PLAN: "feat!: reservas offline-first — CreateReservationCmd.Id obligatorio y reintento idempotente; sobrecupo explícito con tope por profesional (cerrado por defecto)"
TAG: v0.2.0
EXECUTOR: local
REVIEWER: none
---

# Plan — `appointment_booking`: reservas sin servidor y sobrecupo explícito

## 0. Contexto (leer primero)

Una ola offline-first (master plan:
<https://github.com/veltylabs/mjosefa-cms/blob/main/docs/OFFLINE_FIRST_MASTER_PLAN.md>) permite
que la recepción de una clínica **reserve sin servidor**. El navegador ejecuta este mismo módulo
sobre su base local, guarda la operación en una cola (`webtyp/outbox`) y la reenvía al servidor
cuando vuelve la conexión, a `POST /api/appointment_booking/create_reservation`. Decisiones del
master que aplica este plan:

- **OF-5:** el id de la reserva lo genera quien llama. La reserva creada sin conexión llega al
  servidor con el mismo id, así que reenviarla **no duplica**.
- **OF-6:** sin retrocompatibilidad; no queda ningún camino viejo.
- **OF-7:** si la hora ya fue tomada por otro puesto, el servidor la rechaza con `ErrSlotTaken`
  (409; ya existe) y la reserva va a una bandeja de conflictos del puesto que la creó.
- **OF-16:** **sobrecupo explícito**. Es una operación propia con su propio permiso, con un
  **tope por profesional y día que por defecto es 0** (cerrado). Una reserva de sobrecupo se ve
  distinta en la agenda. Prior art: FHIR `Slot.overbooked`, el "overbook" con permiso de las
  agendas hospitalarias y el sobrecupo que autoriza el médico en Chile.

Estado actual (verificado en `main`, v0.1.46):

- `CreateReservation(cmd CreateReservationCmd)` (`service.go`) asigna
  `newReservation.Id = m.ids.NewID()`; `CreateReservationCmd` y `CreateReservationArgsModel`
  (`model.go`) no tienen `id`.
- Reenviar la misma creación hoy **falla** con `ErrSlotTaken`, porque la hora ya la ocupa la
  propia reserva.
- No existe sobrecupo.
- `patient_directory` **v0.1.0** quitó `Deps.IDs`; la demo (`web/client.go`) todavía lo pasa.

Los IDs internos de configuración (bloques de horario, excepciones, configuración de servicio)
siguen generándose con `Deps.IDs`. Son pantallas de administración que se usan en línea y no
entran en esta ola; `Deps.IDs` se mantiene para ellos.

## Design gate

### 1. Prior art
- **Replicache / Zero:** ids generados en el cliente; la mutación de creación es idempotente.
- **Stripe `Idempotency-Key` / HTTP `PUT` con id del cliente:** repetir una creación devuelve el
  mismo recurso.
- **FHIR `Slot.overbooked` / `Schedule`:** el sobrecupo es un estado explícito del cupo, no un
  choque silencioso; las agendas hospitalarias (Epic, Cerner) lo permiten solo con permiso.

### 2. Prueba del nombre para un novato
- `CreateReservation(CreateReservationCmd{Id: id, ...})`: "crea la reserva con este id".
- `CreateOverbooking(cmd)`: "crea un sobrecupo".
- `WorkCalendarConfig.MaxOverbookPerDay`: "máximo de sobrecupos por día de este profesional".
- `Reservation.Overbooked`: "es sobrecupo".
- `ErrOverbookNotAllowed`: "no se permite (más) sobrecupo".
- `ErrSlotNotInSchedule`: "la hora solicitada no pertenece a un bloque de horario habilitado".

### 3. Libro de complejidad
```
Conceptos que aprender              +1 (sobrecupo: un método, un campo de config, una marca)
Archivos que tocar para hacer X      0
Líneas en el sitio de llamada        +1 (Id en el comando)
Formas de hacer lo mismo             0 (sobrecupo es otra intención, no otra forma de reservar)
```

### 4. Dónde va
En este módulo: es su regla de agenda. La bandeja de conflictos y la decisión de ofrecer
"Sobrecupo" pertenecen a la app (mjosefa-cms), que llama a estas operaciones.
Se agrega simetría en la API exponiendo `GetCalendarConfig` para no tener lecturas destructivas en UI.

### 5. Qué borra
`newReservation.Id = m.ids.NewID()` en `CreateReservation`, e `IDs` de
`patientdirectory.Deps` en la demo.

## Code Quality Checklist

```
RULE: Every repeated string (env key, file path, prefix, flag name, URL) MUST be
a named constant in the library package. String literals are forbidden in logic.
```
- Errores como constantes tipadas `domainError` comparadas por aserción de tipo `err.(domainError)`.
- Sin uso de `reflectlite` o `==` directo entre interfaces de error.
- En WASM / paquete raíz: usar `webtyp.com/fmt`, nunca `errors`, `strconv`, `strings`.

## 1. Cambios (normativos)

### 1.1 `model.go` + regenerar `model_orm.go`
- `CreateReservationArgsModel`: agregar como **primer** campo
  `{Name: "id", Type: model.Text(), DB: &model.FieldDB{PK: true}, NotNull: true}`.
- `ReservationModel`: agregar `{Name: "overbooked", Type: model.Bool(), NotNull: true}`.
- `ReservationFormModel`: agregar `{Name: "overbooked", Type: model.Bool()}`.
- `WorkCalendarConfigModel`: agregar `{Name: "max_overbook_per_day", Type: model.Int(), NotNull: true}`
  (0 = sin sobrecupo por defecto).
- `UpsertCalendarConfigArgsModel`: agregar `{Name: "max_overbook_per_day", Type: input.Number()}`.
- Nuevo `GetCalendarConfigArgsModel`:
  `Name: "get_calendar_config_args"`, campos `tenant_id` (model.Text()) y `staff_id` (input.Text()).
- Nuevo `CreateOverbookingArgsModel`: **mismos campos** que `CreateReservationArgsModel`, con nombre
  `create_overbooking_args`.
- Regenerar con `go install webtyp.com/ormc/cmd/ormc@latest && ormc` en la raíz; commitear lo
  generado; nunca editar `model_orm.go` a mano.
- Migración: `migrate/migrate.go` ya reconcilia con `d.Sync(tables...)`. Asegurar que incluye todas las tablas de configuración y reservas.

### 1.2 `service.go`
- Interfaz `SchedulingService`: agregar:
  - `CreateOverbooking(cmd CreateReservationCmd) (Reservation, error)`
  - `GetCalendarConfig(tenantId, staffId string) (WorkCalendarConfig, error)`
- `CreateReservationCmd` gana `Id string` como primer campo.
- Nuevos errores de dominio junto a los existentes:
  - `ErrIdTaken domainError = "reservation id already used"`
  - `ErrOverbookNotAllowed domainError = "overbooking not allowed for this professional and day"`
  - `ErrSlotNotInSchedule domainError = "appointment_booking: slot is not within professional's scheduled blocks"`
- `GetCalendarConfig(tenantId, staffId string) (WorkCalendarConfig, error)` expuesto públicamente sobre `Module`.
- `CreateReservation`, **antes** de cualquier otra validación de disponibilidad:
  1. `cmd.Id == ""` → `ErrMissingArgs`.
  2. **Reintento idempotente:** buscar `repo.GetReservation(cmd.Id)`:
     - Si existe y tiene mismo `TenantId` y mismo `ClientId` → devolverla tal como está guardada, sin escribir en BD ni publicar eventos.
     - Si existe y tiene distinto `TenantId` o distinto `ClientId` → `ErrIdTaken`.
     - Si retorna `ErrNotFound` (evaluado con `e, ok := err.(domainError); ok && e == ErrNotFound`) → continuar.
     - Cualquier otro error de BD → retornarlo.
  3. El resto igual que hoy, con `newReservation.Id = cmd.Id` y `Overbooked: false`. (Eliminar `newReservation.Id = m.ids.NewID()`).
- **Refactor `scheduleSlots`:** extraer de `ListAvailability` una función:
  ```go
  func (m *Module) scheduleSlots(from, to int64, blocks []WorkCalendarBlock, exceptions []WorkCalendarException, cfg WorkCalendarConfig, empSvcCfg EmployeeServiceConfig, activeReservations []Reservation) ([]TimeSlot, error)
  ```
  `ListAvailability` la invoca con las reservas activas (comportamiento idéntico al actual).
- Nuevo `CreateOverbooking(cmd CreateReservationCmd) (Reservation, error)`:
  1. Id obligatorio y reintento idempotente (pasos 1 y 2 de `CreateReservation`).
  2. Mismas validaciones de precondición (origen válido, configuración de servicio activa, cliente, staff y catálogo existen).
  3. Comprobar que la hora solicitada coincide con el inicio de un slot habilitado del horario del día: invocar `scheduleSlots` con `activeReservations = nil`. Si no coincide → `ErrSlotNotInSchedule`.
  4. Leer `WorkCalendarConfig`: si `MaxOverbookPerDay == 0` → `ErrOverbookNotAllowed`.
  5. Contar reservas activas del profesional en ese día (`ReservationDate == targetDay`, `Overbooked == true`, excluyendo `StatusCancelled`, `StatusRescheduled`, `StatusExpired`). Si `int64(count) >= cfg.MaxOverbookPerDay` → `ErrOverbookNotAllowed`.
  6. Insertar en BD dentro de la transacción con `newReservation.Id = cmd.Id` y `Overbooked: true`. Emitir `EventReservationCreated`.
- `UpsertCalendarConfig`: validar que `cfg.MaxOverbookPerDay < 0` retorna `ErrMissingArgs`.

### 1.3 `ops.go`, `lister.go` y `view.go`
- En `ops.go`:
  - `OpCreateOverbooking = "create_overbooking"` registrado con `.Requires("overbooking", model.Create).Accepts(&CreateOverbookingArgs{})`.
  - `OpGetCalendarConfig = "get_calendar_config"` registrado con `.Requires("calendar", model.Read).Accepts(&GetCalendarConfigArgs{})`.
  - `writeError`: mapear `ErrIdTaken` y `ErrOverbookNotAllowed` a 409; `ErrSlotNotInSchedule` a 400.
  - `opCreateReservation`: mapear `args.Id` a `cmd.Id`.
  - `opUpsertCalendarConfig`: mapear `args.MaxOverbookPerDay` a `cfg.MaxOverbookPerDay`.
- En `lister.go`:
  - `reservationFormStore.List`: mapear `r.Overbooked` a `ReservationForm.Overbooked`.
  - `reservationFormStore.Save`: pasar `Id: r.Id` a `CreateReservationArgs`.
- En `view.go`:
  - Actualizar `ReservationForm.Item()`: si `r.Overbooked == true`, reflejar la marca de sobrecupo en `Description` (ej. añadiendo `· Sobrecupo` vía `lang.Translate`).

### 1.4 `ui/`
- En `ui/bookingview.go`: una reserva sobrecupada refleja la etiqueta de sobrecupo en el slot de la agenda.
- En `ui/scheduleview.go`:
  - `reload()` consulta `OpGetCalendarConfig`. Si no existe, invoca `OpUpsertCalendarConfig` con defaults sin pisar datos existentes.
  - Muestra un control numérico para editar `max_overbook_per_day` y guardarlo.
- En `ui/css.go`: estilos asociados al panel de horario.

### 1.5 `seed/` y `web/`
- `seed/seed.go`: asignar IDs determinísticos a las reservas de demostración (`"demo-reservation-1"` y `"demo-reservation-2"`).
- `web/client.go`: actualizar dependencia de `patient_directory` y remover `IDs` de `patientdirectory.Deps`.

### 1.6 Documentación
- Actualizar `README.md` y `docs/ARCHITECTURE.md` detallando id generado por el cliente, reintento idempotente y la operación de sobrecupo con su permiso `overbooking:c`.

## 2. Tests (`tests/`, `gotest`)

- Actualizar `tests/helpers_test.go:createBooked` para generar o recibir un ID determinístico.
- En `tests/setup_test.go`:
  - Actualizar `CreateReservation_SlotTaken` para que dos llamadas con IDs distintos a la misma hora fallen con `ErrSlotTaken`.
  - Agregar prueba de reintento idempotente con el mismo ID.
- Crear `tests/overbooking_test.go` cubriendo los 10 casos de aceptación:
  1. Sin `Id` → `ErrMissingArgs`.
  2. **Reintento:** `CreateReservation` dos veces con el mismo comando → la segunda devuelve la misma reserva sin error; 1 fila en BD; 1 evento emitido.
  3. Mismo ID con distinto cliente → `ErrIdTaken` (409 en op).
  4. Dos IDs distintos para la misma hora → `ErrSlotTaken` (409).
  5. Sobrecupo cerrado por defecto: `MaxOverbookPerDay == 0` → `ErrOverbookNotAllowed`.
  6. Con `MaxOverbookPerDay == 1`: un sobrecupo sobre hora tomada → OK y `Overbooked == true`; segundo sobrecupo el mismo día → `ErrOverbookNotAllowed`; al día siguiente permitido.
  7. Sobrecupo en hora fuera del horario → `ErrSlotNotInSchedule`.
  8. Sobrecupo cancelado no cuenta para el tope.
  9. RBAC: `create_overbooking` declara `overbooking`/Create.
  10. `ListAvailability` no sufre regresiones tras el refactor de `scheduleSlots`.

## 3. Criterios de aceptación y Entrega

- `gotest ./...` en verde (race, tests y wasm).
- `grep -n "newReservation.Id = m.ids.NewID()" service.go` → vacío.
- `grep -n "IDs:" web/client.go` no muestra `patientdirectory.Deps` con `IDs`.
- **Delivery check:** Antes de finalizar la ejecución: `git diff --stat main` lista los archivos correspondientes a las etapas planificadas, y ningún archivo es eliminado que no haya sido ordenado eliminar.

| Etapa | Archivos | Listo cuando |
|---|---|---|
| 1 | `model.go`, `model_orm.go`, `migrate/migrate.go` | Definiciones de modelos actualizadas y código generado por ormc al día |
| 2 | `service.go`, `lister.go`, `view.go` | Idempotencia, `scheduleSlots`, `CreateOverbooking`, `GetCalendarConfig` y mapeo de `ReservationForm` |
| 3 | `ops.go` | Nuevas operaciones registradas y mapeo de errores HTTP |
| 4 | `ui/bookingview.go`, `ui/scheduleview.go`, `ui/css.go` | Visualización en agenda y configuración de sobrecupo en horario |
| 5 | `seed/seed.go`, `web/client.go` | Semillas con IDs y demo actualizada |
| 6 | `tests/helpers_test.go`, `tests/setup_test.go`, `tests/overbooking_test.go`, etc. | Suite completa en verde con todos los casos nuevos |
| 7 | `README.md`, `docs/ARCHITECTURE.md` | Documentación técnica sincronizada |
