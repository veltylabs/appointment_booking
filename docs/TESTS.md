# Pruebas de appointment-booking

Se corren con `gotest` (vet, estilo, carrera, pruebas y wasm). Los tests viven en [tests/](../tests) y llaman al módulo como
lo hace una pantalla: por `crud.LoopbackActingAsWithQueue(nil, "u1", &callQueue{}, mod)` y `router.Operation`, nunca por
funciones internas. El doble de la cola de llamadas externas es [queue_double_test.go](../tests/queue_double_test.go).

| Test | Rule |
|---|---|
| [TestMount_MatchesDefinition](../tests/reservation_test.go) | Lo declarado en `Definition` coincide con lo implementado (`crud.Mount` devuelve `nil`) |
| [TestReservation_PublishesChange](../tests/reservation_test.go) | Una escritura permitida publica un `*crud.Change` en su tópico |
| [TestReservation_NoDoubleBooking](../tests/reservation_test.go) | Dos reservas de la misma hora: la segunda se rechaza y queda una fila |
| [TestReservation_CreatorFromCaller](../tests/reservation_test.go) | Un `creator_user_id` del cliente se rechaza; sin él, el creador es quien llama |
| [TestReservation_StatusTransitions](../tests/reservation_test.go) | Una transición inválida se rechaza y el estado no cambia; una válida se aplica |
| [TestOverbooking_OwnPermission](../tests/reservation_test.go) | El sobrecupo pasa por `reservation_overbooking` hasta el tope del día; por `reservation` no |
| [TestOnline_HoldThenProcess](../tests/online_test.go) | La reserva online retiene la hora y `Process` resuelve paciente y pago en la petición |
| [TestOnline_WithoutPaymentsPort](../tests/online_test.go) | Sin puerto de pagos el registro queda `done`, con paciente y sin `payment_url` |
| [TestOnline_PaymentFailureKeepsHold](../tests/online_test.go) | Si el pago falla, la cola guarda la causa, el registro sigue `pending` y la hora sigue retenida |
| [TestForwardingOperationsGone](../tests/reservation_test.go) | `booking_patient` ya no está montada |
| [TestExpiry_RecordsCount](../tests/reservation_test.go) | `reservation_expiry` vence las `PENDING` pasadas y guarda cuántas |
| [TestBlockDelete_ConflictsTheReservation](../tests/schedule_test.go) | Borrar el bloque que cubría una reserva la deja en `CONFLICTED` |
| [TestBlockCreate_ReleasesTheConflict](../tests/schedule_test.go) | Crear un bloque que la cubre la devuelve a su estado anterior |
| [TestExceptionCreate_ConflictsAndDeleteReleases](../tests/schedule_test.go) | Una excepción `HOLIDAY` la pone en conflicto; borrarla la libera |
| [TestConflictRecompute_CountsChanges](../tests/schedule_test.go) | La orden `conflict_recompute` cuenta lo que cambió |
| [TestAvailability_ExcludesBookedSlot](../tests/service_test.go) | Una hora reservada deja de ofrecerse |
| [TestBlockCreate_RejectsOverlap](../tests/service_test.go) | Un bloque que se solapa con otro del día se rechaza |
| [TestBookableServices_PriceOverrideWins](../tests/service_test.go) | El precio de la configuración gana al de lista |
| [TestCharge_PendingReservationIsPayable](../tests/service_test.go) | Una reserva `PENDING` con precio es cobrable |
| [TestConfirmPaidReservation_IsIdempotent](../tests/service_test.go) | Confirmar dos veces el mismo pago es seguro; otro pago se rechaza |
| [TestImportReservation_TwiceLeavesOneRow](../tests/service_test.go) | Importar dos veces deja una fila; un `PENDING` se rechaza |
| [TestTenantIsolation_OtherTenantRejected](../tests/service_test.go) | Otro tenant no puede leer la reserva |
| [TestFSM](../tests/fsm_test.go) | La máquina de estados y sus transiciones |
| [TestErrorStrings](../tests/errors_test.go) | Los textos de error no cambian sin aviso |
| [TestSlotsAcrossDST](../tests/timezone_test.go) | Las horas se mantienen a través del cambio de horario de verano |
| [TestNightBookingInItsDay](../tests/timezone_test.go) | Una reserva de noche cae en su día local |
| [TestSeed_LoadPopulatesServicesAndReservations](../tests/seed_test.go) | La semilla crea servicios y reservas |
| [TestDemo_Valid](../tests/demo_test.go) | `seed.Demo()` es válida y muestra pantallas |
| [TestMigrate_CreatesEveryTableInOrder](../migrate/migrate_test.go) | Las nueve tablas, en orden de dependencia |
| [TestMigrate_NoCompilerIsAnError](../migrate/migrate_test.go) | Sin compilador de SQL, `Migrate` devuelve un error y no termina en silencio. |
| [TestActiveByStaff_StatusesWithoutCancelled](../tests/agenda_watch_test.go) | La agenda activa por profesional excluye reservas canceladas. |
| [TestWatchStaffAgenda_ReportsCreateAndStatusChange](../tests/agenda_watch_test.go) | El observador de agenda notifica creaciones y cambios de estado. |
