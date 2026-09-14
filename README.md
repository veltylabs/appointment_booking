# appointment-booking
<img src="docs/img/badges.svg">

Gestiona la configuración de servicios agendables, calendarios de trabajo del personal (bloques) y reservas de clientes.

## Entidades principales

- `employee_service_config`: configuración de qué servicios maneja cada profesional (duración, anulación de precio).
- `reservation`: la cita programada (fecha/hora, cliente, profesional, servicio) con un estado (`status`) controlado por máquina de estados finitos (FSM).
- `workcalendar_block`: una fila por bloque de tiempo de trabajo — SEMANAL (`specific_date == 0`, aplica a `day_of_week`) o DATADO (`specific_date > 0`, aplica solo a esa fecha y la abre). Varios bloques por día = el descanso para almorzar es el espacio entre ellos.
- `workcalendar_exception`: excepciones puntuales (feriados personales, horarios especiales, intervalos bloqueados).
- `workcalendar_config`: la zona horaria IANA del calendario de un miembro del personal (heredada por bloques y excepciones).

## Documentación

- [Arquitectura](docs/ARCHITECTURE.md)
- Plan local ejecutado más recientemente: [docs/LAST_PLAN_EXECUTED.md](docs/LAST_PLAN_EXECUTED.md)
- [Diagrama de Base de Datos](docs/diagrams/database.md)
- [Diagrama FSM](docs/diagrams/fsm.md)
- [Diagramas de Secuencia](docs/diagrams/sequence.md)

## Notas de diseño / desacoplamiento

Sin claves foráneas (FK) físicas hacia otros módulos:
- `reservation.client_id` referencia a un cliente (Directorio/Clínica) por ID.
- `reservation.creator_user_id` referencia a un usuario de IAM.
- `employee_service_config.service_id` referencia a un ítem del módulo de Catálogo.
- Los campos `staff_id` referencian al módulo de Personal (Staff).

Las reglas de disponibilidad (bloques + excepciones + la ventana diaria del establecimiento, cruzadas como `DayBounds` de `webtyp.com/time`) se aplican en la capa de servicio, no a través de FKs entre módulos.
`appointment_booking/go.mod` mantiene **cero dependencias de veltylabs** — el límite del establecimiento es un puerto declarado aquí (`BoundsReader`), satisfecho estructuralmente por `veltylabs/business_calendar`.
El estado de la reserva se controla mediante una FSM en código — sin tabla `reservation_status`.

## Reglas de desarrollo (SKILL)

### Restricciones y reglas principales

- **Cambios de estado solo vía FSM:** `Reservation.Status` DEBE cambiar únicamente mediante `FSM.Transition(current, event)` — excepto al entrar o salir de `CONFLICTED`, que un recomputo escribe directamente y restaura a través de `StatusBeforeConflict`. Eventos válidos: `CONFIRM`, `CANCEL`, `COMPLETE`, `NO_SHOW_EVENT`, `EXPIRE`, `RESCHEDULE`, `CONFLICT`, `RESOLVE`.
- **RESCHEDULED ≠ CANCELLED:** Cuando una reserva es reemplazada por una nueva, la original se marca como `RESCHEDULED` (no `CANCELLED`) para preservar la integridad de la traza de auditoría. Estos son estados terminales distintos. `CONFLICTED` **no** es terminal.
- **La zona horaria está en WorkCalendarConfig:** `WorkCalendarBlock` y `WorkCalendarException` NO tienen campo de zona horaria. Siempre carga primero `WorkCalendarConfig` para obtener la zona horaria IANA. Las horas de los bloques son minutos locales desde la medianoche (p. ej., `540 = 09:00`), convertidos a UTC en tiempo de consulta mediante `LocalIntToUnixUTC`.
- **Instantáneas (Snapshotting):** Al crear una reserva, el precio, la moneda, la duración, el staffID y el serviceID se guardan en instantáneas. Nunca mutes los campos de instantánea después de la creación. `StaffIdsnapshot`/`ServiceIdsnapshot` usan los nombres de columna históricos `staff_idsnapshot`/`service_idsnapshot` — nunca renombrar.
- **Un cambio de agenda reporta sus conflictos:** Cada edición profesional (`save_day_blocks`, `save_date_blocks`, `mark_working_days`, `unmark_working_days`, excepciones) recomputa las reservas futuras en el rango afectado y marca/limpia `CONFLICTED`. Los cambios a nivel de establecimiento van a través de `RecomputeConflicts`. Cuando nadie resulta afectado, **no se publica nada** (CU-19).
- **Sin RBAC aquí:** Este módulo confía en `actorID` como una cadena ya autorizada. La autorización es aplicada por el gateway antes del servicio. Este módulo solo almacena `actorID` como campo de auditoría.
- **La publicación de eventos es de tipo "dispara y olvida" (fire-and-forget)** a través del `events.Publisher` inyectado (`Deps.Publisher`). Un publisher `nil` es seguro.
- **Sin importaciones entre módulos.** Las dependencias externas se acceden únicamente mediante interfaces inyectadas: `StaffReader`, `CatalogReader`, `DirectoryReader`, `BoundsReader`.

### Interfaces inyectadas (parámetros del constructor)

El servicio mantiene `*orm.DB` directamente — sin interfaces de almacenamiento intermedias. Solo se inyectan dependencias entre módulos:

```go
type Deps struct {
    Staff     StaffReader     // provisto por el módulo staff
    Catalog   CatalogReader   // provisto por el módulo catalog
    Directory DirectoryReader // provisto por el módulo directory
    IDs       model.IDGenerator // requerido — nunca se construye dentro del módulo
    Publisher events.Publisher  // nil = eventos deshabilitados
    Bounds    BoundsReader      // nil = sin límites (un profesional independiente sin establecimiento)
}

func New(db *orm.DB, deps Deps) (*Module, error)
```

### Eventos de dominio publicados

| Constante de evento | Cuándo |
|---|---|
| `appointment.reservation.created` | Después de que `CreateReservation` hace commit |
| `appointment.reservation.rescheduled` | Para la reserva original durante la reprogramación |
| `appointment.reservation.confirmed` | Después de la transición CONFIRM |
| `appointment.reservation.cancelled` | Después de la transición CANCEL |
| `appointment.reservation.completed` | Después de la transición COMPLETE |
| `appointment.reservation.no_show` | Después de la transición NO_SHOW |
| `appointment.reservation.expired` | Después de la transición EXPIRE |
| `appointment.schedule.changed` | Una vez por profesional cuyo cambio de agenda puso ≥1 reserva en conflicto (payload `ScheduleChangedPayload{TenantId, StaffId, FromDate, ToDate, ConflictCount}`) |
| `appointment.reservation.conflicted` | Por reserva en conflicto, solo cuando el cambio provino de la edición del propio profesional |

### Errores centinela clave

| Error | Cuándo |
|---|---|
| `ErrSlotTaken` | Horario no disponible o carrera de reserva concurrente |
| `ErrConflict` | Mismatched de concurrencia optimista en actualizaciones de reservas |
| `ErrCalendarConfigNotFound` | Guardar bloques antes de `upsert_calendar_config` |
| `ErrInvalidBlock` | `start_min` no es `< end_min`, o fuera de 0..1439 |
| `ErrBlocksOverlap` | Dos bloques del mismo día se solapan |
| `ErrBlockOutsideBusinessHours` | Un bloque cae fuera de la ventana de apertura del establecimiento |
| `ErrBlockOnClosedDay` | El establecimiento está cerrado en esa fecha |
| `ErrInvalidTransition` | La FSM rechaza el evento para el estado actual |

### Raíz de composición (cómo conectar este módulo)

```go
// La creación del esquema es un paso en tiempo de despliegue (ver subpaquete migrate):
// err := migrate.Migrate(conn, ddlCompiler)

// New / NewRepository asume que el esquema de la base de datos ya existe.
scheduling, _ := appointmentbooking.New(db, appointmentbooking.Deps{
    Staff:     staffmodule.New(db),        // implementa StaffReader
    Catalog:   catalogmodule.New(db),      // implementa CatalogReader
    Directory: directorymodule.New(db),    // implementa DirectoryReader
    IDs:       idGen,                      // model.IDGenerator
    Publisher: eventBus,                   // nil = eventos deshabilitados
    Bounds:    businesscalendarModule,     // implementa BoundsReader; nil = sin límites
})
scheduling.MountOps(opRegistry)            // el transporte recolecta las ops
reservationsView := scheduling.NewView(caller, tenantId, staffId)
```

La recomputación a nivel de establecimiento es conectada por la aplicación (conoce ambos módulos):

```go
broker.Subscribe(businesscalendar.EventCalendarChanged, func(ev events.Event) {
    var p businesscalendar.CalendarChangedPayload
    // decodificar; abrir más temprano nunca puede invalidar una reserva
    if !p.Closed {
        return
    }
    _, _ = book.RecomputeConflicts(config.TenantID, p.FromDate, p.ToDate)
})
```

### Operaciones disponibles (23 en total)

`create_reservation`, `get_reservation`, `list_reservations_by_staff`, `list_reservations_by_client`,
`change_reservation_status`, `expire_pending_reservations`, `list_conflicting_reservations`,
`recompute_conflicts`, `upsert_calendar_config`, `save_day_blocks`, `save_date_blocks`,
`mark_working_days`, `unmark_working_days`, `list_blocks`, `get_day_bounds`, `add_calendar_exception`,
`remove_calendar_exception`, `list_availability`, `list_exceptions`,
`create_employee_service_config`, `get_employee_service_config`,
`list_employee_service_configs_by_staff`, `update_employee_service_config`

> `expire_pending_reservations` es el **único disparador para el evento FSM EXPIRE**; debe ser llamado por un
> programador externo — el módulo no tiene procesos en segundo plano internos.
>
> `list_blocks` y `list_exceptions` son las lecturas directas que necesita un editor de horarios; la vista orientada al llamador
> es `NewScheduleClient` (ver abajo).

## ScheduleClient — la cara para el editor de horarios

`NewScheduleClient(caller router.Caller, tenantId, staffId string) *ScheduleClient` es un cliente tipado
del lado del llamador sobre las operaciones de calendario, destinado a ser adaptado por una aplicación a un
componente de interfaz `scheduleeditor`. Importando únicamente `router` + los tipos de este módulo, se mantiene agnóstico del renderizador:

```go
cl := appointmentbooking.NewScheduleClient(caller, "t1", "s1")
cl.Blocks(func(rows []appointmentbooking.WorkCalendarBlock, err error) { /* … */ })
cl.SaveDayBlocks(dayOfWeek, blocks, func(err error) { /* … */ })
cl.Exceptions(from, to, func(rows []appointmentbooking.WorkCalendarException, err error) { /* … */ })
cl.AddException(exc, func(err error) { /* … */ })
cl.RemoveException(exceptionID, func(err error) { /* … */ })
```

Una mutación de agenda publica `appointment.schedule.changed` (con el rango afectado y recuento de conflictos)
únicamente cuando realmente puso reservas en conflicto.

## NewEmployeeServiceConfigView — la cara para gestionar servicios por profesional

`NewEmployeeServiceConfigView(caller router.Caller, tenantId, staffId string) view.Presenter` construye un presenter
acotado a un profesional para listar y guardar la configuración de sus servicios (`employee_service_config`).

```go
configView := appointmentbooking.NewEmployeeServiceConfigView(caller, tenantId, staffId)
```

## NewFormView — la cara para pantallas de reserva

`NewFormView(caller router.Caller, cfg FormConfig) view.Presenter` construye un presenter que tanto
lista las reservas de un profesional como crea nuevas. `NewView` se mantiene como la superficie de solo lista para
consumidores de solo lectura.

```go
formView := appointmentbooking.NewFormView(caller, appointmentbooking.FormConfig{
    TenantId:        tenantId,
    StaffId:         staffId,
    ServiceConfigId: serviceConfigId,
    Timezone:        "America/Santiago",
    From:            fromUnixSec,
    To:              toUnixSec,
    ActorId:         actorId,
    LabelFor: func(clientId string) string {
        // Punto de extensión opcional: traducir clientId a un nombre visible en la lista
        return directoryClientName(clientId)
    },
})
```

`FreeSlots(caller, cfg, day)` devuelve los horarios reservables para una fecha (p. ej. `"2026-09-08"`) como
cadenas `"HH:MM"` en la zona horaria configurada.

## Interfaz de Servicio (SchedulingService)

```go
type SchedulingService interface {
    // Gestión de calendario
    UpsertCalendarConfig(cfg WorkCalendarConfig) error
    SaveDayBlocks(tenantId, staffId string, dayOfWeek int, blocks []WorkCalendarBlock) error
    SaveDateBlocks(tenantId, staffId string, date int64, blocks []WorkCalendarBlock) error
    MarkWorkingDays(tenantId, staffId string, dates []int64, startMin, endMin int) error
    UnmarkWorkingDays(tenantId, staffId string, dates []int64) error
    ListBlocks(tenantId, staffId string) ([]WorkCalendarBlock, error)
    AddException(exc WorkCalendarException) error
    RemoveException(tenantId, exceptionId string) error
    ListExceptions(tenantId, staffId string, from, to int64) ([]WorkCalendarException, error)
    GetDayBounds(date int64) (tinytime.DayBounds, error)

    // Disponibilidad
    ListAvailability(tenantId, staffId, configId string, from, to int64) ([]TimeSlot, error)

    // Reservas
    CreateReservation(cmd CreateReservationCmd) (Reservation, error)
    GetReservation(tenantId, id string) (Reservation, error)
    ListReservationsByStaff(tenantId, staffId string, from, to int64) ([]Reservation, error)
    ListReservationsByClient(tenantId, clientId string) ([]Reservation, error)
    ChangeReservationStatus(cmd ChangeStatusCmd) error
    ExpirePendingReservations(tenantId string, before int64) (int, error)

    // Conflictos
    ListConflictingReservations(tenantId, staffId string, from int64) ([]ConflictingReservation, error)
    RecomputeConflicts(tenantId string, from, to int64) (int, error)
}
```

Esta interfaz depende de los lectores inyectados:
- `DirectoryReader` — valida la existencia del cliente
- `StaffReader` — valida la existencia del personal
- `CatalogReader` — valida la existencia del servicio
- `BoundsReader` — responde "qué minutos de una fecha son utilizables" (opcional)
