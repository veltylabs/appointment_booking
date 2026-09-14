# Arquitectura de appointment-booking

> **Nota de estado:** este documento describe la estructura actual del módulo tras la adopción
> del harness de módulos reutilizables (`router.OpModule`, `Deps.IDs model.IDGenerator`, `events.Publisher`,
> `ddl.CreateTable`, pruebas con `storage/mem`) y la adición del editor de horarios.
> `AGENTS.md` (en la raíz de este repositorio) es la autoridad sobre la lista blanca/negra que este módulo respeta.

## 1. Alcance del Dominio

El módulo `appointment-booking` gestiona el ciclo de vida completo de una cita de servicio agendada. Es responsable de:
- Configurar qué servicios ofrece cada miembro del personal (duración, precio, tiempo de amortiguación o buffer).
- Definir la disponibilidad del personal mediante **bloques** (semanales y datados) y excepciones puntuales (feriados, horarios especiales, intervalos bloqueados), delimitados por la ventana utilizable del establecimiento.
- Calcular huecos de tiempo libres y crear reservas con prevención atómica de conflictos.
- Aplicar las transiciones de estado de las reservas mediante una Máquina de Estados Finitos (FSM), incluyendo un estado `CONFLICTED` para citas que el horario actual ya no cubre.

## 2. Entidades Principales

- **`EmployeeServiceConfig` (`EmployeeServiceConfigModel`):** Mapea a un miembro del personal con un ítem de servicio, definiendo duración, tiempo de amortiguación (buffer) y anulación de precio. Es la fuente de verdad para la granularidad de los huecos. A diferencia de `Reservation`, esta tabla no contiene campos de auditoría ni instantáneas internas no editables; cada columna excepto `id` y `tenant_id` es legítimamente editable por un usuario, por lo que lleva widgets `input.*` directamente en `EmployeeServiceConfigModel` sin requerir un tipo separado de proyección de formulario.
- **`Reservation`:** La cita en sí (fila persistida en BD). Almacena instantáneas (snapshots) de personal, servicio, precio y moneda al momento de la creación para auditabilidad financiera — estos nunca cambian incluso si los datos de origen se modifican posteriormente. También rastrea `StatusBeforeConflict` para que una cita en conflicto pueda ser restaurada exactamente al estado que tenía antes del conflicto.
- **`ReservationForm` (`ReservationFormModel`):** La proyección de formulario de una reserva distinta de `Reservation`. `Reservation` contiene 22 campos (instantáneas, revisión, auditoría) con tipos base; `ReservationForm` expone los 6 campos visibles al mostrador (`id`, `client_id`, `day`, `hour`, `notes`, `status`) utilizando widgets `input.*` para que los generadores de formularios UI puedan construir formularios de creación sin exponer campos internos o de auditoría.
- **`WorkCalendarConfig`:** Una fila por miembro del personal. Única fuente de verdad para la zona horaria IANA del calendario del personal. Debe existir antes de que se puedan guardar bloques.
- **`WorkCalendarBlock`:** Una fila por bloque de tiempo de trabajo. Un día puede contener varios — "mañana 09:00–13:00, tarde 15:00–19:00" son dos filas, y el descanso para almorzar es la **brecha entre ellos** (deliberadamente no hay columna de descanso). `specific_date == 0` significa que el bloque es SEMANAL (aplica a `day_of_week`); `specific_date > 0` significa que el bloque es DATADO (aplica solo a esa fecha y **abre** el día incluso si ningún bloque semanal cubre ese día de la semana — cómo un profesional irregular marca los días que trabaja). No contiene zona horaria — la hereda de `WorkCalendarConfig`.
- **`WorkCalendarException`:** Anulaciones puntuales para una fecha específica: `HOLIDAY` (sin disponibilidad), `SPECIAL_HOURS` (estrecha un día que cubre un bloque), o `BLOCKED` (intervalo restado de las ventanas disponibles).

Este módulo posee el esquema para las cinco entidades anteriores (a diferencia de p. ej. `work_schedule`,
que solo lee tablas de solo lectura pertenecientes a otros lugares). La migración del esquema se realiza a través del
subpaquete `migrate` (`migrate.Migrate`) como un paso en tiempo de despliegue — ver §7.

## 3. Máquina de Estados Finitos (FSM)

Las transiciones de estado de las reservas se aplican en código — no hay tabla de BD `reservation_status`.

Ver: [Diagrama FSM](diagrams/fsm.md)

Decisiones clave:
- `RESCHEDULED` es un estado terminal distinto (no `CANCELLED`) para preservar la claridad de la traza de auditoría en analítica.
- `EXPIRED` es activado exclusivamente por un programador externo a través de la operación `expire_pending_reservations` — el módulo no ejecuta goroutines en segundo plano.
- `CONFLICTED` **no** es terminal: un recomputo entra a este estado desde `PENDING`/`CONFIRMED` cuando el horario actual (bloques del profesional o ventana del establecimiento) ya no cubre la cita, y sale de él — restaurando exactamente lo que registró `StatusBeforeConflict` — cuando el horario la vuelve a cubrir. Por qué un día está cerrado es asunto del establecimiento: este módulo solo razona sobre "¿todavía encaja el instante?", nunca sobre la razón (la neutralidad se hereda de `time.DayBounds`).

## 4. Patrones Arquitectónicos

1. **Inyección de Dependencias:** el módulo recibe lectores externos (`StaffReader`,
   `CatalogReader`, `DirectoryReader`), un `BoundsReader` opcional (la ventana diaria utilizable del establecimiento; nil = sin límites, correcto para una aplicación sin institución por encima del profesional)
   y un `events.Publisher` a través de `Deps` en la construcción (`New(db, deps)`). Sin estado global, sin importaciones directas de otros módulos.

2. **Acceso directo a ORM (sin interfaces de almacenamiento):** el módulo mantiene `*orm.DB` directamente (a través de un
   `*Repository` interno) y llama a funciones de ORM de `model_orm.go`. No hay interfaz intermedia `ReservationStore`, `CalendarStore` o `ConfigStore` — `Repository` es una estructura simple, no un límite de abstracción que una prueba pueda cambiar por un mock. Esto mantiene el límite interno delgado y permite que las propias pruebas del módulo ejerciten el generador de consultas real de `github.com/webtyp/orm` contra `github.com/webtyp/storage/mem` — el backend de referencia en memoria — detectando errores reales de restricciones y concurrencia (conflictos de bloqueo optimista, violaciones de unicidad) en lugar de ocultarlos detrás de mocks, sin ningún controlador concreto de base de datos en el grafo de dependencias del módulo. Solo las interfaces entre módulos (`StaffReader`, `CatalogReader`, `DirectoryReader`) y el `events.Publisher` inyectado son mockeables.

3. **Referencias Blandas (sin FK física):** `client_id`, `staff_id`, `service_id`, `creator_user_id` y `payment_id` referencian entidades en otros módulos solo por ID. La existencia entre módulos se valida en la capa de aplicación a través de lectores inyectados, no mediante restricciones de BD.

4. **Instantáneas (Snapshotting):** Precio, moneda, duración, ID de personal e ID de servicio se guardan en instantáneas al crear la reserva. Los cambios posteriores en el catálogo o datos del personal no alteran las reservas existentes.

5. **Tiempo Entero Local + Zona Horaria IANA (Única Fuente de Verdad):** Las horas de trabajo en `WorkCalendarBlock` se almacenan como minutos enteros locales desde la medianoche (p. ej., `540 = 09:00`). La zona horaria IANA se almacena exclusivamente en `WorkCalendarConfig` (una fila por personal) — los bloques y excepciones no llevan campos de zona horaria. Esto evita la inconsistencia de zonas horarias por fila por construcción. El algoritmo `ListAvailability` carga primero `WorkCalendarConfig` para obtener la zona horaria, luego convierte los límites locales a Unix UTC utilizando `webtyp.com/time` (`LocalMinutesToUnixUTC`). Este diseño garantiza que los horarios recurrentes se mantengan correctos a través de las transiciones de horario de verano (DST).

6. **Concurrencia Optimista:** `Reservation.revision` se incrementa en cada actualización de estado. `UpdateReservationStatus` aplica `WHERE revision = N` — un desacuerdo devuelve `ErrConflict`, evitando sobreescrituras silenciosas.

7. **Reprogramación Atómica:** Reprogramar no es un estado — es una operación transaccional: crear una nueva reserva + marcar la original como `RESCHEDULED` dentro de una sola transacción de BD.

8. **Los datos desactualizados se recomputan, nunca se revierten:** un cambio de horario (edición del profesional o del establecimiento mediante `RecomputeConflicts`) reevalúa todas las reservas futuras en el rango afectado contra el horario y límites ACTUALES. `CONFLICTED` se marca y limpia únicamente por esa regla de resolución única, compartida con `ListAvailability` (`availableRanges`) — nunca se altera por el evento que se disparó, por lo que dos causas independientes no pueden ocultar la segunda.

## 5. Contrato de Identidad y RBAC

Este módulo **no** implementa autorización ni control de acceso basado en roles (RBAC). Opera bajo el siguiente contrato:

- `actorID` es una cadena simple — ya autenticada y autorizada por el llamador.
- El adaptador de transporte o capa de middleware es responsable de verificar que el usuario autenticado tenga permiso para realizar la operación **antes** de que se ejecute el manejador de la op (`router.Route.Requires(resource, action)` es donde se declara ese filtro).
- Este módulo almacena `actorID` únicamente como campo de auditoría (`creator_user_id`, `updated_by`).
- **El RBAC pertenece a un módulo IAM separado.** Los cambios en roles o permisos no requieren cambios en este módulo.

## 6. Publicación de Eventos y Comunicación entre Módulos

Este módulo se comunica hacia afuera a través del `events.Publisher` inyectado de `github.com/webtyp/events`.

Tras cada mutación exitosa de estado, el módulo publica un evento de dominio con un payload tipado:

| Operación | Constante de evento |
|---|---|
| `CreateReservation` | `appointment.reservation.created` |
| `ChangeStatus` CONFIRM | `appointment.reservation.confirmed` |
| `ChangeStatus` CANCEL | `appointment.reservation.cancelled` |
| `ChangeStatus` COMPLETE | `appointment.reservation.completed` |
| `ChangeStatus` NO_SHOW | `appointment.reservation.no_show` |
| `ChangeStatus` EXPIRE | `appointment.reservation.expired` |
| Reprogramación (original) | `appointment.reservation.rescheduled` |
| Edición de agenda de un profesional que genera conflicto en ≥1 reserva | `appointment.schedule.changed` (una vez por personal) **+** `appointment.reservation.conflicted` (una vez por reserva en conflicto) |
| Recomputo a nivel de establecimiento (`RecomputeConflicts`) que genera conflicto en ≥1 | `appointment.schedule.changed` (una vez por personal afectado) |

El evento `appointment.schedule.changed` lleva un **`ScheduleChangedPayload{TenantId, StaffId, FromDate, ToDate, ConflictCount}`** — el rango que cambió (para que un consumidor recompute un rango acotado en lugar de todo) y cuántas reservas puso en conflicto el cambio. Según CU-19, **no se publica nada cuando el cambio no genera conflictos en nadie** — el payload nunca es "noticia vacía".

`appointment.reservation.conflicted` se emite por reserva **únicamente** cuando el cambio provino de la edición del propio profesional, donde la cantidad es pequeña y un notificador orientado al paciente necesita el registro individual. El caso a nivel de establecimiento (un feriado que afecta a cientos de citas) nunca inunda el broker: los consumidores leen `ListConflictingReservations`, que es la lista de trabajo del administrador.

**Reglas:**
- La publicación de eventos es de tipo **dispara y olvida (fire-and-forget)** — `events.Publisher.Publish` no devuelve error; un fallo del lado del broker es asunto del broker, nunca del módulo.
- Pasar `nil` como `Deps.Publisher` deshabilita de forma segura la emisión de eventos (útil en pruebas o herramientas CLI).
- El broker concreto (en proceso, `github.com/webtyp/sse`, una cola) es decidido por la raíz de composición, nunca por este módulo.
- **El módulo NO se suscribe** al calendario del establecimiento. `RecomputeConflicts` es la recomputación exportada; la aplicación —que legítimamente conoce ambos módulos— se suscribe a `business.calendar.changed` y lo llama. No existe dependencia de `Subscriber` aquí.

## 7. Transporte, Identidad, Vista — Raíz de Composición

El módulo implementa `router.OpModule` (`ModelName() string` + `MountOps(reg router.OpRegistry)`). Las 23 operaciones (8 de reservas + 11 de calendario + 4 de configuración de servicios) son registradas por un único `*Module`.

### Ops (vía `MountOps`)

| Op | Acción | Recurso | Descripción |
|---|---|---|---|
| `create_employee_service_config` | `c` | `employee_service_config` | Registra que un profesional realiza un servicio con su duración y anulación de precio |
| `get_employee_service_config` | `r` | `employee_service_config` | Lee una configuración de servicio por ID |
| `list_employee_service_configs_by_staff` | `r` | `employee_service_config` | Lista todos los servicios que realiza un profesional (activos e inactivos) |
| `update_employee_service_config` | `u` | `employee_service_config` | Actualiza la configuración completa de servicio de un profesional |
| `create_reservation` | `c` | `reservation` | Crea una nueva reserva (reprogramación atómica si `RescheduledFromId` está configurado) |
| `get_reservation` | `r` | `reservation` | Obtiene una reserva por ID |
| `list_reservations_by_staff` | `r` | `reservation` | Lista reservas por ID de personal y rango de fechas |
| `list_reservations_by_client` | `r` | `reservation` | Lista reservas por ID de cliente |
| `change_reservation_status` | `u` | `reservation` | Cambia el estado de una reserva mediante un evento FSM |
| `expire_pending_reservations` | `u` | `reservation` | Expira reservas pendientes no confirmadas (llamado por un programador externo) |
| `list_conflicting_reservations` | `r` | `reservation` | La lista de trabajo del administrador: reservas futuras que el horario actual ya no cubre |
| `recompute_conflicts` | `u` | `reservation` | Reevalúa cada reserva futura en un rango; marca/limpia `CONFLICTED`; idempotente — el disparador para cambios a nivel de establecimiento |
| `upsert_calendar_config` | `u` | `calendar` | Configura la zona horaria IANA para un miembro del personal |
| `save_day_blocks` | `u` | `calendar` | Reemplaza CADA bloque semanal de un día de la semana — la edición de día completo |
| `save_date_blocks` | `u` | `calendar` | Reemplaza los bloques datados de UNA fecha (un día marcado que diverge de su ventana común) |
| `mark_working_days` | `u` | `calendar` | Marca fechas como trabajadas con una ventana común |
| `unmark_working_days` | `d` | `calendar` | Elimina los bloques datados de esas fechas — el día vuelve a la plantilla semanal o a no trabajado |
| `list_blocks` | `r` | `calendar` | Lista todos los bloques de un miembro del personal (semanales + datados; para el editor de horarios) |
| `get_day_bounds` | `r` | `calendar` | Sirve de proxy para `Deps.Bounds` de modo que el editor limite sus propios controles a través de este módulo |
| `add_calendar_exception` | `c` | `calendar` | Agrega una excepción de calendario para una fecha específica |
| `remove_calendar_exception` | `d` | `calendar` | Elimina una excepción de calendario |
| `list_availability` | `r` | `calendar` | Lista horarios disponibles para un miembro del personal |
| `list_exceptions` | `r` | `calendar` | Lista excepciones de calendario en un rango de fechas (para el editor de horarios) |

### Vista

`NewView(caller router.Caller, tenantId, staffId string) view.Presenter` construye un `view.Presenter` **solo de lista/selección** sobre `Reservation`, delimitado al horario de un miembro del personal.

`NewEmployeeServiceConfigView(caller router.Caller, tenantId, staffId string) view.Presenter` construye un `view.Presenter` (Lista + Guardado) acotado a un profesional sobre `EmployeeServiceConfig`.

`NewFormView(caller router.Caller, cfg FormConfig) view.Presenter` construye un **presenter capaz de formularios** (Lista + Guardado) sobre `ReservationForm`. `FormConfig` recibe `Timezone` explícitamente porque la zona horaria autorizada vive en `work_calendar_config.timezone` en el servidor y actualmente no existe op de lectura para el cliente (limitación conocida).

**Sin segundo `view.Presenter` para la configuración del calendario, pero existe una interfaz para el editor.** `WorkCalendarConfig` (una fila por personal), `WorkCalendarWeekly` (a lo sumo 7 filas por personal) y `WorkCalendarException` (unas pocas filas puntuales) son datos de configuración reducidos. En su lugar, este módulo expone la interfaz del editor de horarios como un **cliente tipado del lado del llamador**:

- `list_blocks` + `list_exceptions` — las lecturas directas que necesita un `scheduleeditor`.
- `NewScheduleClient(caller, tenantId, staffId)` — cliente tipado del lado del llamador (`Blocks`, `SaveDayBlocks`, `Exceptions`, `AddException`, `RemoveException`) sobre esas ops más las ops de escritura existentes.

### Ejemplo de Raíz de Composición

```go
staffSvc     := staffmodule.New(db, staffmodule.Deps{IDs: idGen})            // implementa StaffReader
directorySvc := directorymodule.New(db, directorymodule.Deps{IDs: idGen})   // implementa DirectoryReader

catalogSvc, _ := itemcatalog.New(db, itemcatalog.Deps{
    IDs:       idGen,       // model.IDGenerator
    Publisher: eventBroker, // events.Publisher, nil deshabilita la publicación
})

scheduling, _ := appointmentbooking.New(db, appointmentbooking.Deps{
    Staff:     staffSvc,
    Catalog:   catalogSvc,   // *itemcatalog.Module satisface CatalogReader
    Directory: directorySvc,
    IDs:       idGen,        // model.IDGenerator
    Publisher: eventBroker,  // events.Publisher, nil deshabilita la publicación
    Bounds:    calendarSvc,  // *business_calendar.Module satisface BoundsReader; nil = sin límites
})

scheduling.MountOps(opRegistry)               // router.OpRegistry
reservationsView := scheduling.NewView(caller, tenantId, staffId) // router.Caller -> view.Presenter
```

Ningún módulo importa a otro directamente — `appointment_booking` define `StaffReader`, `CatalogReader` y `DirectoryReader`; los módulos hermanos los satisfacen estructuralmente sin importar de vuelta este paquete.

## 8. Cálculo de Disponibilidad

Los horarios libres se derivan en tiempo de consulta de la intersección de:
- La ventana utilizable del establecimiento (`BoundsReader.GetDayBounds`, resuelta una vez a `tinytime.Unbounded()` cuando `Deps.Bounds` es nil). Un edificio cerrado cierra a todos antes de consultar a cualquier profesional.
- Por día, **bloques datados primero** (ABREN su día independientemente de si la plantilla semanal cubre el día de la semana), luego **bloques semanales** activos para ese día de la semana — cada uno recortado a la ventana del establecimiento.
- Excepciones puntuales aplicadas sobre esas ventanas: `HOLIDAY` anula el día, `SPECIAL_HOURS` lo estrecha a una sola ventana, `BLOCKED` resta un intervalo de las ventanas del día.
- Reservas no terminales existentes (bloquean intervalos ocupados incluyendo tiempo de amortiguación o buffer).

Prioridad de excepciones: `HOLIDAY` > `SPECIAL_HOURS` > `BLOCKED`.

## 9. Documentos Relacionados

- [Diagrama de Base de Datos](diagrams/database.md)
- [Diagrama FSM](diagrams/fsm.md)
- [Diagramas de Secuencia](diagrams/sequence.md)
