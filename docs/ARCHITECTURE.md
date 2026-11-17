# Arquitectura de appointment-booking

> `AGENTS.md` (raíz del repositorio) es la autoridad sobre qué puede importar este módulo.

## 1. Alcance del Dominio

El módulo `appointment-booking` gestiona el ciclo de vida completo de una cita de servicio agendada. Es responsable de:
- Configurar qué servicios ofrece cada miembro del personal (duración, precio, tiempo de amortiguación o buffer).
- Definir la disponibilidad del personal mediante **bloques** (semanales y datados) y excepciones puntuales (feriados, horarios especiales, intervalos bloqueados), delimitados por la ventana utilizable del establecimiento.
- Calcular horas libres y crear reservas sin doble reserva (la regla corre dentro de la transacción).
- Aplicar las transiciones de estado de las reservas mediante una Máquina de Estados Finitos (FSM), incluyendo un estado `CONFLICTED` para citas que el horario actual ya no cubre.

## 2. Entidades Principales

- **`EmployeeServiceConfig` (`EmployeeServiceConfigModel`):** Mapea a un miembro del personal con un ítem de servicio, definiendo duración, tiempo de amortiguación (buffer) y anulación de precio. Es la fuente de verdad para la granularidad de los huecos. A diferencia de `Reservation`, esta tabla no contiene campos de auditoría ni instantáneas internas no editables; cada columna excepto `id` y `tenant_id` es legítimamente editable por un usuario, por lo que lleva widgets `input.*` directamente en `EmployeeServiceConfigModel` sin requerir un tipo separado de proyección de formulario.
- **`Reservation`:** La cita en sí (fila persistida en BD). Almacena instantáneas (snapshots) de personal, servicio, precio y moneda al momento de la creación para auditabilidad financiera — estos nunca cambian incluso si los datos de origen se modifican posteriormente. También rastrea `StatusBeforeConflict` para que una cita en conflicto pueda ser restaurada exactamente al estado que tenía antes del conflicto.
- **`ReservationForm` (`ReservationFormModel`):** La proyección de formulario de una reserva distinta de `Reservation`. `Reservation` contiene 22 campos (instantáneas, revisión, auditoría) con tipos base; `ReservationForm` expone los 6 campos visibles al mostrador (`id`, `client_id`, `day`, `hour`, `notes`, `status`) utilizando widgets `input.*` para que los generadores de formularios UI puedan construir formularios de creación sin exponer campos internos o de auditoría.
- **`WorkCalendarConfig`:** Una fila por miembro del personal. Única fuente de verdad para la zona horaria IANA del calendario del personal. Debe existir antes de que se puedan guardar bloques.
- **`WorkCalendarBlock`:** Una fila por bloque de tiempo de trabajo. Un día puede contener varios — "mañana 09:00–13:00, tarde 15:00–19:00" son dos filas, y el descanso para almorzar es la **brecha entre ellos** (deliberadamente no hay columna de descanso). `specific_date == 0` significa que el bloque es SEMANAL (aplica a `day_of_week`); `specific_date > 0` significa que el bloque es DATADO (aplica solo a esa fecha y **abre** el día incluso si ningún bloque semanal cubre ese día de la semana — cómo un profesional irregular marca los días que trabaja). No contiene zona horaria — la hereda de `WorkCalendarConfig`.
- **`WorkCalendarException`:** Anulaciones puntuales para una fecha específica: `HOLIDAY` (sin disponibilidad), `SPECIAL_HOURS` (estrecha un día que cubre un bloque), o `BLOCKED` (intervalo restado de las ventanas disponibles).

Este módulo posee el esquema de sus nueve tablas (las cinco anteriores más `reservation_overbooking`, `online_reservation`,
`reservation_expiry` y `conflict_recompute`, las órdenes de §3). La migración es el subpaquete `migrate` (`migrate.Migrate`),
un paso de despliegue y no del constructor.

## 3. Quién decide qué: reglas del módulo, persistencia del framework

El módulo **declara** en [definition.go](../definition.go) (`Allow`, `Rules`, `ExternalCalls` por entidad) y escribe solo
**reglas de dominio** (`rule_<entidad>_<verbo>.go`). `crud` hace lo demás.

| Lo hace `crud` (el módulo no lo escribe) | Lo decide el módulo (una regla) |
|---|---|
| id, `tenant_id`, `revision`, `updated_at`; la transacción; cuerpos de crear/actualizar/borrar; «no existe»; lectura paginada; permisos y eventos `*crud.Change` | Hora libre dentro de la transacción: sin doble reserva ([rule_reservation_create.go](../rule_reservation_create.go)) |
| Validar esquema, campos `ServerOnly` y límites antes de llamar a la regla | Quién crea es quien llama; precio y duración congelados (snapshots) |
| Registrar y ejecutar una llamada externa (`WaitForReply`, reintentos) | Qué transición de estado permite la [FSM](diagrams/fsm.md) |
| | Recalcular conflictos al crear o borrar un bloque o una excepción |

Una acción con permiso propio es un **registro creado** en su tabla (`reservation_overbooking`, `reservation_expiry`,
`conflict_recompute`, `online_reservation`) o un **modelo de lectura** (`bookable_slot`…): no hay operaciones por verbo.

- **Lectura de tabla:** `Search(a, filter, m)` solo agrega condiciones (`staff_id` del filtro). El filtro es la propia entidad (D52).
- **Reserva online:** `online_reservation.Create` retiene la hora (PENDING, `HoldUntil`) y pide la llamada `Process`, que resuelve al paciente
  y empieza el cobro en la misma petición; si falla, la reserva sigue retenida y el registro sigue `pending`.
- **Conflictos:** `recomputeConflictsFor` ([service.go](../service.go)) reevalúa las reservas futuras del rango contra la agenda **actual**, con la
  misma `availableRanges` que la disponibilidad (anti-deriva). Las reglas de bloque y excepción lo corren dentro de su transacción sumando el cambio
  que `crud` todavía no escribió (`scheduleChange`). Los eventos de dominio se publican desde la regla, antes del commit (decisión conocida).
- **Datos de otros módulos:** solo por las [interfaces](../interfaces.go) de `Deps` (`StaffReader`, `CatalogReader`, `PatientDirectory`, `OpeningHours`,
  `PaymentGateway`), con tipos simples; ningún import de otro módulo, ni en `ui/` ni en `seed/` ni en las pruebas.
- **Pantallas:** `ui.Screens(deps, ui.Steps{...})`. Los pasos «paciente» y «cobro» los implementa la app con la `ui` de cada módulo ([steps.go](../ui/steps.go)).
- **Instante UTC único + zona inyectada:** `reservation_time` es un instante UTC; los bloques son minutos locales (`540 = 09:00`) que `Deps.Zone`
  convierte con `zone.At(fecha, minutos)`; así el horario de verano no duplica campos ([TIEMPO.md](TIEMPO.md)).
- **Sin RBAC aquí:** `crud` comprueba el permiso de cada entidad; el módulo solo guarda quién actuó (`creator_user_id`, `updated_by`).

## 4. Máquina de Estados Finitos (FSM)

Las transiciones se aplican en código ([fsm.go](../fsm.go)), sin tabla `reservation_status`. Ver [diagrama FSM](diagrams/fsm.md).

- `RESCHEDULED` es un estado terminal distinto de `CANCELLED`: conserva la traza de auditoría.
- `EXPIRED` lo escribe la orden `reservation_expiry` (un programador externo la crea); el módulo no lanza goroutines.
- `CONFLICTED` **no** es terminal: un recálculo entra a él desde `PENDING`/`CONFIRMED` cuando la agenda ya no cubre la cita y sale
  restaurando lo que guardó `StatusBeforeConflict`. Por qué un día está cerrado no es asunto del módulo (neutralidad heredada de `time.DayLimits`).

## 5. Cálculo de Disponibilidad

Los horarios libres se derivan en tiempo de consulta de la intersección de:
- La ventana utilizable del establecimiento (`OpeningHours.DayLimits`, resuelta una vez a `time.Unlimited()` cuando `Deps.Hours` es nil). Un edificio cerrado cierra a todos antes de consultar a cualquier profesional.
- Por día, **bloques datados primero** (ABREN su día independientemente de si la plantilla semanal cubre el día de la semana), luego **bloques semanales** activos para ese día de la semana — cada uno recortado a la ventana del establecimiento.
- Excepciones puntuales aplicadas sobre esas ventanas: `HOLIDAY` anula el día, `SPECIAL_HOURS` lo estrecha a una sola ventana, `BLOCKED` resta un intervalo de las ventanas del día.
- Reservas no terminales existentes (bloquean intervalos ocupados incluyendo tiempo de amortiguación o buffer).

Prioridad de excepciones: `HOLIDAY` > `SPECIAL_HOURS` > `BLOCKED`.

## 6. Documentos Relacionados

- [Diagrama de Base de Datos](diagrams/database.md)
- [Diagrama FSM](diagrams/fsm.md)
- [Diagramas de Secuencia](diagrams/sequence.md)
- [Pruebas](TESTS.md)

## Importar historia

`ImportReservation` almacena una reserva de un sistema anterior con su propio id, fecha y estado. Comprueba lo que comprueba `CreateReservation` sobre los datos: el id, el inquilino, un estado y origen conocidos, que el cliente, el profesional y el servicio existen, y que existe la configuración (activa o no: un historial puede ser inactivo). Sin embargo, omite deliberadamente todas las reglas en vivo: no requiere un espacio libre, no obliga a un inicio `PENDING`, no tiene reserva, no emite eventos, y no abre su propia transacción (el importador ya ejecuta dentro de una).

Ejemplo:
```go
r, err := booking.ImportReservation(Reservation{Id: "1650000000000000000", TenantId: t, ClientId: p,
	EmployeeServiceConfigId: cfg, ReservationTime: int64(start), Status: StatusCompleted, Origin: OriginCounter})
```

Cómo se guardan la fecha y la hora (UTC, textos locales, horario de verano): [TIEMPO.md](TIEMPO.md).
