---
PLAN: "feat!: reservas offline-first — CreateReservationCmd.Id obligatorio y reintento idempotente; sobrecupo explícito con tope por profesional (cerrado por defecto)"
TAG: v0.2.0
EXECUTOR: jules
REVIEWER: none
---

> This plan is dispatched via the CodeJob workflow. See skill: agents-workflow.

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

Estado actual (verificado en `main`, v0.1.43):

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

### 5. Qué borra
`newReservation.Id = m.ids.NewID()` en `CreateReservation`, e `IDs` de
`patientdirectory.Deps` en la demo.

## 1. Cambios (normativos)

### 1.1 `model.go` + regenerar `model_orm.go`
- `CreateReservationArgsModel`: agregar como **primer** campo
  `{Name: "id", Type: model.Text(), DB: &model.FieldDB{PK: true}, NotNull: true}`. `webtyp/form`
  oculta un PK de texto y le asigna `IDGenerator.NewID()` al enviar, conservándolo entre
  reintentos.
- `ReservationModel`: agregar `{Name: "overbooked", Type: model.Bool(), NotNull: true}`.
- `WorkCalendarConfigModel`: agregar `{Name: "max_overbook_per_day", Type: model.Int(), NotNull: true}`
  (0 = sin sobrecupo).
- `UpsertCalendarConfigArgsModel`: agregar `{Name: "max_overbook_per_day", Type: input.Number()}`.
- Nuevo `CreateOverbookingArgsModel`: **mismos campos** que `CreateReservationArgsModel`, con nombre
  `create_overbooking_args`.
- Regenerar con `go install webtyp.com/ormc/cmd/ormc@latest && ormc` en la raíz; commitear lo
  generado; nunca editar `model_orm.go` a mano.
- Migración: `migrate/` crea las tablas con `ddl`; las columnas nuevas entran por la misma vía
  (`Sync` agrega columnas). Verificar que `migrate.Migrate` usa `Sync` y no solo `CreateTable`;
  si usa `CreateTable`, cambiarlo a `Sync` para que una base existente reciba las columnas.

### 1.2 `service.go`
- `CreateReservationCmd` gana `Id string` como primer campo.
- Nuevos errores de dominio junto a los existentes:
  `ErrIdTaken domainError = "reservation id already used"` y
  `ErrOverbookNotAllowed domainError = "overbooking not allowed for this professional and day"`.
- `CreateReservation`, **antes** de cualquier otra validación de disponibilidad:
  1. `cmd.Id == ""` → `ErrMissingArgs`.
  2. **Reintento:** si existe una reserva con ese `Id` (`repo.GetReservation(id)`):
     - mismo `TenantId` y mismo `ClientId` → devolverla tal como está guardada, sin escribir ni
       publicar eventos (es la misma operación repetida);
     - distinto → `ErrIdTaken`.
     - `ErrNotFound` → seguir. Distinguir por aserción de tipo `err.(domainError)`, nunca con `==`
       entre interfaces (en TinyGo arrastra `reflectlite`).
  3. El resto igual que hoy, con `newReservation.Id = cmd.Id` y `Overbooked: false`.
- **Refactor sin cambio de comportamiento:** extraer de `ListAvailability` una función no
  exportada `scheduleSlots(...)` que genera los slots del horario **recibiendo la lista de
  reservas activas como parámetro**. `ListAvailability` la llama con las reservas activas (igual
  que hoy) y el sobrecupo con una lista vacía.
- Nuevo `CreateOverbooking(cmd CreateReservationCmd) (Reservation, error)`:
  1. Pasos 1 y 2 de arriba (id obligatorio, reintento idempotente).
  2. Las mismas validaciones de `CreateReservation` (origen, configuración de servicio activa,
     cliente, profesional y servicio existen).
  3. La hora pedida tiene que ser el inicio de un slot **del horario** de ese día
     (`scheduleSlots` con la lista de reservas vacía); si no lo es → `ErrIncompleteSlot` (error que
     ya existe).
  4. Leer el `WorkCalendarConfig` del profesional: `MaxOverbookPerDay == 0` → `ErrOverbookNotAllowed`.
  5. Contar las reservas del profesional en ese día con `Overbooked == true` y estado activo
     (excluir cancelada, reprogramada y vencida, como en `ListAvailability`); si el conteo es
     `>= MaxOverbookPerDay` → `ErrOverbookNotAllowed`.
  6. Insertar como `CreateReservation` (misma transacción y eventos) con `Overbooked: true`.
     **No** se valida el choque con reservas existentes: esa es la definición de sobrecupo.
- `UpsertCalendarConfig` guarda `MaxOverbookPerDay`; un valor negativo → `ErrMissingArgs`.

### 1.3 `ops.go`
- `opCreateReservation` copia `args.Id` al comando.
- Nueva op `OpCreateOverbooking = "create_overbooking"`:
  `.Requires("overbooking", model.Create).Accepts(&CreateOverbookingArgs{})`. Es un recurso
  **propio**: quien puede reservar no puede, por eso, dar sobrecupos.
- `writeError`: `ErrIdTaken` y `ErrOverbookNotAllowed` → 409.
- `opUpsertCalendarConfig` copia `max_overbook_per_day`.

### 1.4 `ui/`
- Donde se crea una reserva (formulario o `CreateReservationArgs` armado a mano), el id lo pone el
  formulario (PK oculto) o, si se arma a mano, `ids.NewID()` **una sola vez por intento**,
  conservado mientras se reintenta.
- En la agenda (`bookingview.go`), una reserva con `Overbooked` muestra la etiqueta "Sobrecupo"
  (texto en el `lang.json` del módulo si existe; si no, en el mismo mecanismo de etiquetas que ya
  use la vista) con un estilo distinto definido en `ui/css.go` (DSL `widget/style`, sin colores
  literales).
- La pantalla de horario (`scheduleview.go`) permite editar `max_overbook_per_day` donde ya edita la
  zona horaria y si está activo.
- **No** se agrega un botón de "dar sobrecupo" en esta etapa: lo ofrece la bandeja de conflictos
  de la app.

### 1.5 `seed/` y `web/`
- `seed/seed.go`: cada `CreateReservation` de demo lleva un id determinístico
  (`"demo-reservation-1"`, `-2`, … como constantes del paquete `seed`).
- `web/client.go`: `go get github.com/veltylabs/patient_directory@v0.1.0` y quitar `IDs` de
  `patientdirectory.Deps`. Si `seed` de `patient_directory` cambió de firma, adaptarlo.

### 1.6 Documentación
`README.md` y `docs/ARCHITECTURE.md`: el id de la reserva lo genera quien llama y repetir la
creación devuelve la guardada; sección "Sobrecupo" (qué es, permiso `overbooking:c`, tope por
profesional, por defecto cerrado); tabla de ops con `create_overbooking`.

## 2. Tests (`tests/`, paquete externo; `gotest`)

Actualizar los existentes (todo `CreateReservation` lleva `Id`). Agregar:

1. Sin `Id` → `ErrMissingArgs`.
2. **Reintento:** `CreateReservation` dos veces con el mismo comando → la segunda devuelve la misma
   reserva sin error (no `ErrSlotTaken`); hay **una** fila; **un** evento `EventReservationCreated`.
3. Mismo id con otro cliente → `ErrIdTaken`; por la op → 409.
4. Dos ids distintos para la misma hora → la segunda da `ErrSlotTaken` (OF-7; ya existía, que siga).
5. Sobrecupo cerrado por defecto: con `MaxOverbookPerDay == 0` → `ErrOverbookNotAllowed`.
6. Con `MaxOverbookPerDay == 1`: un sobrecupo sobre una hora **ya tomada** → OK y `Overbooked == true`;
   un segundo sobrecupo ese día → `ErrOverbookNotAllowed`; al día siguiente vuelve a permitirse.
7. Sobrecupo en una hora fuera del horario → `ErrIncompleteSlot`.
8. Un sobrecupo **cancelado** no cuenta para el tope.
9. RBAC: `create_overbooking` declara `overbooking`/Create (probarlo con el registro de ops de los
   tests existentes, como ya se prueba `create_reservation`).
10. `ListAvailability` devuelve exactamente lo mismo que antes del refactor (los tests existentes de
    disponibilidad siguen verdes sin tocarlos).

## 3. Reglas de código (no negociables)
- Paquete raíz compila a TinyGo WASM: `webtyp.com/fmt`; nada de `errors`, `strconv`, `strings`; sin
  `map`.
- Errores como constantes `domainError`, comparadas por aserción de tipo.
- Tests solo en `tests/`; nunca exportar un símbolo para un test.

## 4. Criterios de aceptación
- `gotest ./...` en verde (stdlib y wasm).
- `grep -n "newReservation.Id = m.ids.NewID()" service.go` → vacío.
- `grep -n "IDs:" web/client.go` no muestra `patientdirectory.Deps` con `IDs`.

| Etapa | Archivos | Listo cuando |
|---|---|---|
| 1 | `model.go`, `model_orm.go`, `migrate/` | campos nuevos; columnas nuevas llegan a bases existentes |
| 2 | `service.go` | id obligatorio + reintento; `scheduleSlots`; `CreateOverbooking` |
| 3 | `ops.go` | op `create_overbooking`; 409 para los errores nuevos |
| 4 | `ui/` | etiqueta "Sobrecupo"; tope editable; id por intento |
| 5 | `seed/`, `web/client.go` | demo funciona con `patient_directory` v0.1.0 |
| 6 | `tests/` | 10 casos + existentes en verde |
| 7 | `README.md`, `docs/ARCHITECTURE.md` | id del cliente y sobrecupo documentados |

**Consumidores (no son trabajo de este plan):** `clinical_encounter` y `mjosefa-cms` arman
`CreateReservationCmd`; se actualizan cuando suban de versión (etapa I1 del master).
