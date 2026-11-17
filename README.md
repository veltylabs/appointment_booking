# appointment-booking
<img src="docs/img/badges.svg">

Agenda y reservas de un establecimiento: qué horas existen y quién las ocupa.

## ¿Por qué existe?

Cuando un establecimiento atiende por hora (una clínica, una peluquería), alguien tiene que responder tres preguntas
sin equivocarse: ¿qué horas tiene libres cada profesional?, ¿esta hora ya está tomada? y ¿qué pasa con las reservas
cuando la agenda cambia? Este módulo es el dueño de esas respuestas: los
[calendarios de trabajo](docs/ARCHITECTURE.md#2-entidades), las [reservas y su máquina de estados](docs/diagrams/fsm.md),
los sobrecupos y la reserva online con retención.

| Sin este módulo | Con este módulo |
|---|---|
| Cada app calcula la disponibilidad a su manera y dos personas reservan la misma hora. | La regla «una hora, una reserva» corre dentro de la transacción: la segunda se rechaza. |
| Quitar un bloque de trabajo deja reservas que ya no caben, sin aviso. | Crear o borrar un bloque o una excepción recalcula en la misma transacción qué reservas pasan a `CONFLICTED`. |
| 38 operaciones escritas a mano, cada una con su permiso, su evento y su transacción. | Una [declaración](definition.go) (`Allow` + `Rules`): el framework `crud` hace el resto y el módulo escribe solo reglas de dominio. |

El módulo **no sabe** de pacientes ni de cobros: pregunta por ellos a través de [interfaces mínimas](interfaces.go)
que la app le entrega. La [pantalla de reservar](docs/diagrams/booking_flow.md) tampoco los importa: la app le pasa los
pasos «paciente» y «cobro» ([`ui.Steps`](ui/steps.go)).

## Antes de comenzar

- **Para usarlo en una app:** una base de datos (`orm.New(...)`) con las tablas creadas por [`migrate.Migrate`](migrate/migrate.go) (paso de despliegue, no del constructor), y un `model.IDGenerator`.
- **Lo que la app debe entregar** en [`Deps`](module.go): `TenantID`, `Zone` (zona horaria IANA del establecimiento), `Staff`, `Catalog`, `Patients`; opcionales `Hours` (horario del establecimiento; nil = sin límite), `Payments` (nil = sin cobro) y `Publisher`.
- **Para desarrollar:** `gotest` y `definitionc` en el `PATH` (`export PATH="$PATH:$(go env GOPATH)/bin"`). `definition_gen.go` lo escribe `definitionc`; nunca se edita.
- **Para ver la demo:** `seed.Demo()` (compila a wasm; la corre el MCP `webtyp`).

## Guía paso a paso

### Paso 1: crear el módulo

**Meta:** tener un módulo que responde sus operaciones sobre una base en memoria.

```go
package main

import (
	appointmentbooking "github.com/veltylabs/appointment_booking"
	"webtyp.com/fmt"
	"webtyp.com/orm"
	"webtyp.com/storage/mem"
	"webtyp.com/time"
	"webtyp.com/unixid"
)

// ports contesta lo mínimo a lo que el módulo pregunta de otros módulos; en una app real son sus módulos.
type ports struct{}

func (ports) StaffExists(string, string) (bool, error)                { return true, nil }
func (ports) StaffCard(string, string) (string, string, bool, error)  { return "Dra. Soto", "Pediatría", true, nil }
func (ports) OccupantOptions(string) ([]fmt.KeyValue, error)          { return nil, nil }
func (ports) ServiceExists(string, string) (bool, error)              { return true, nil }
func (ports) ServiceCard(string, string) (string, float64, string, bool, error) {
	return "Consulta", 35000, "CLP", true, nil
}
func (ports) GetPatient(string, string) (string, string, string, string, bool, error) {
	return "Juan", "1-9", "", "", true, nil
}
func (ports) FindPatient(string, string) (string, string, string, string, bool, error) {
	return "", "", "", "", false, nil
}
func (ports) CreatePatient(_, id, _, _, _, _ string) (string, error) { return id, nil }

func main() {
	ids, err := unixid.NewUnixID()
	if err != nil {
		fmt.Println(err)
		return
	}
	zone, err := time.LoadZone("America/Santiago")
	if err != nil {
		fmt.Println(err)
		return
	}
	mod, err := appointmentbooking.New(orm.New(mem.New()), appointmentbooking.Deps{
		IDs: ids, TenantID: "clinica-1", Zone: zone,
		Staff: ports{}, Catalog: ports{}, Patients: ports{},
	})
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(appointmentbooking.Definition.Name, mod != nil)
}
```

**Resultado:** imprime `appointment_booking true`. `New` falla con un error claro si falta un `Dep` obligatorio.

### Paso 2: montarlo en la app

**Meta:** que el servidor atienda sus operaciones. La app (nunca el módulo) escribe el `appkit.Module` y busca los
módulos que ya construyó con `appkit.FindBuiltModule`. Las [operaciones](#operaciones) salen de `Definition`.

**Resultado:** `crud.Mount(nil, nil, nil, mod)` devuelve `nil`: lo declarado coincide con lo implementado
([prueba](tests/reservation_test.go)).

### Paso 3: dar las pantallas

**Meta:** mostrar agenda, servicios y la reserva en 6 pasos. La app arma los pasos de otros módulos:

```go
screens := ui.Screens(deps, ui.Steps{Patient: patientStep, Payment: paymentStep}) // Payment puede faltar en una app sin cobros
```

**Resultado:** `Screens` devuelve «Reserva Hora», el grupo «Personal» (Horario, Servicios) y la reserva online. Sin `Steps.Patient` entrega `panic` con el mensaje de qué falta. La demo usa dobles ([seed/demo.go](seed/demo.go)).

## Operaciones

Cada fila es una entidad de [`Definition`](definition.go). `Allow` dice qué verbos existen; la columna «Regla» dice qué
decide el módulo (el resto lo hace `crud`). Las operaciones son `router.Operation{Module, Entity, Action}`.

| Entidad | Allow | Regla de dominio |
|---|---|---|
| `employee_service_config` | C R U | duración ≥ 1 min; lectura por `staff_id` |
| `work_calendar_config` | C R U | un calendario por profesional; lectura por `staff_id` |
| `work_calendar_block` | C R U D | sin solapes ni salirse del horario; recalcula conflictos |
| `work_calendar_exception` | C R D | recalcula conflictos al crear y al borrar |
| `reservation` | C R U | hora libre, creador = quien llama, solo cambia el `status` por la [FSM](docs/diagrams/fsm.md) |
| `reservation_overbooking` | C R | sobrecupo con su propio permiso, tope `max_overbook_per_day` |
| `online_reservation` | C R | retiene la hora y llama a `Process` (paciente y pago) en la misma petición |
| `reservation_expiry` | C R | vence las `PENDING` pasadas; guarda cuántas |
| `conflict_recompute` | C R | recalcula conflictos de un rango; guarda cuántas cambiaron |
| `time_slot`, `day_limits`, `bookable_service`, `bookable_slot`, `conflicting_reservation`, `staff_option`, `closure_option` | R | modelos de lectura sin tabla |

Los eventos de dominio (`appointment.reservation.*`, `appointment.schedule.changed`) están en [module.go](module.go).
La API en Go que usan otros módulos (`ConfirmedByStaff`, `Charge`, `ConfirmPaidReservation`, `ImportReservation`…) está en
[booking.go](booking.go), [import.go](import.go) y [service.go](service.go).

## Mapa de documentación

| Orden | Documento | Qué pregunta responde |
|---|---|---|
| 1 | [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) | ¿Qué decide el módulo y qué decide el framework? |
| 2 | [docs/diagrams/fsm.md](docs/diagrams/fsm.md) | ¿Qué estados tiene una reserva y cómo pasa de uno a otro? |
| 3 | [docs/diagrams/booking_flow.md](docs/diagrams/booking_flow.md) | ¿Cómo es la reserva en 6 pasos? |
| 4 | [docs/diagrams/booking_backend.md](docs/diagrams/booking_backend.md) | ¿Qué hace el servidor en cada paso? |
| 5 | [docs/diagrams/database.md](docs/diagrams/database.md) | ¿Qué tablas hay? |
| 6 | [docs/diagrams/sequence.md](docs/diagrams/sequence.md) | ¿Quién llama a quién? |
| 7 | [docs/TESTS.md](docs/TESTS.md) | ¿Qué prueba cada test y cómo se corre? |
