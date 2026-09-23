---
PLAN: "feat: Reservation.Origin — quién confirma una reserva, y que el funcionario lo vea"
EXECUTOR: jules
REVIEWER: none
STATUS: review
SESSION: 15920676984437781603
PR: https://github.com/veltylabs/appointment_booking/pull/15
---

> This plan is dispatched via the CodeJob workflow. See skill: agents-workflow.

# Plan — el canal de una reserva, y de quién es el turno

## El problema

Una reserva nace `PENDING` y alguien la confirma. Hoy **nada en el modelo dice
quién es ese alguien**, y eso ya importa aunque la reserva online todavía no
exista:

- En el **mesón**, el funcionario crea la reserva y la confirma él mismo
  (pulsa "Confirmar reserva elegida"). Un `PENDING` de mesón es **trabajo
  suyo, pendiente**.
- En **online**, la confirma el paciente por su propia vía (correo u otra). Un
  `PENDING` online **no es trabajo del funcionario**: es espera.

Los dos se ven idénticos en la lista — `Description: r.Status`, el string crudo
`"PENDING"`. Cuando aterrice online, el funcionario va a confirmar reservas que
no le corresponden, porque nada en pantalla los distingue. Ese es el defecto
que este plan cierra, **antes** de que online llegue y lo haga visible en
producción.

De paso arregla algo que ya es cierto hoy: el estado se muestra en inglés y en
crudo (`PENDING`, `CONFIRMED`), sin pasar por `lang`.

## Decisiones tomadas — el ejecutor no elige ninguna

Vienen de un Q&A cerrado con el dueño del producto:

1. **Campo explícito**, no inferencia. Nada de deducir el canal de
   `creator_user_id` vacío: sería implícito, y el día que una reserva se cree
   sin usuario por otro motivo la inferencia miente sin avisar.
2. **El funcionario distingue "mía para confirmar" de "esperando al
   paciente"** de un vistazo, en la lista.
3. **El funcionario PUEDE confirmar una reserva online a mano** (el paciente
   llama por teléfono porque no le llegó el correo). El FSM ya deja registro
   de quién lo hizo en `updated_by` — no hace falta nada nuevo para eso.
4. **Sin caducidad**: una reserva online que nadie confirma no expira todavía.
   No se toca `EXPIRED` ni se agrega estado nuevo. No hay reservas online aún;
   inventar una ventana de tiempo sin un caso real que la valide es construir
   para una hipótesis.

## Design gate (skill: api-design)

### 1. Prior art

- **Stripe** — `PaymentIntent` lleva su propio campo de canal/origen
  (`payment_method_types` + metadata de la sesión) en la entidad, no derivado
  de quién la creó.
- **Shopify** — `Order.source_name` (`web`, `pos`, `shopify_draft_order`): un
  campo explícito del pedido, justo para que el operador sepa de dónde vino.
- **Calendly / Cal.com** — la reserva registra si la creó el anfitrión o el
  invitado, y la UI del anfitrión usa ese dato para no ofrecer acciones que no
  le tocan.

Los tres coinciden: **el canal es un dato de la entidad, no una deducción**.
Es la razón de la decisión 1.

### 2. La prueba del nombre novato

`Origin` con `OriginCounter` / `OriginOnline`. Un junior lee
`res.Origin == OriginCounter` y sabe qué pregunta sin abrir nada.

- `Channel` diría cómo viaja el mensaje (correo, SMS), que es otra cosa y la
  que vendrá después.
- `BookedBy` sugeriría un id de persona, no un canal.
- Los valores van en **inglés** porque esta librería es neutral de idioma y de
  cliente (`go.mod` con cero dependencias `veltylabs`, reutilizable entre
  clientes de Velty). "Mesón" es vocabulario de una clínica chilena; `COUNTER`
  es el mismo concepto sin casarse con un idioma. La app traduce.

### 3. El libro de complejidad

```
Conceptos que aprender              +1   (Origin y sus dos constantes)
Archivos que tocar para hacer X     +0   (quien ya crea reservas pasa un campo más)
Líneas en el call site              +1   (Origin: OriginCounter)
Formas de hacer lo mismo             0   (hoy no hay ninguna forma de saberlo)
```

### 4. Dónde vive

`veltylabs/appointment_booking`: es dueño de `Reservation`, de su FSM y de la
proyección que la lista lee (`Item()`). Un consumidor no puede resolverlo —
tendría que agregar una tabla paralela para anotar el canal de una reserva que
no es suya, que es exactamente el fork que la regla lego prohíbe. Y la
distinción la quiere **toda** app que use este módulo, no solo mjosefa-cms.

### 5. Qué borra este cambio

- Borra el string crudo en inglés que hoy ve el funcionario (`Description:
  r.Status` sin traducir).
- No borra estados ni eventos del FSM: la tabla de transiciones **no se toca**.

## ATENCIÓN — este plan activa el defecto D10

`migrate/migrate.go` llama a `d.CreateTable(t)`, que compila a
`CREATE TABLE IF NOT EXISTS`: contra una base **que ya existe** es un no-op, y
la columna nueva **nunca llegaría**. En `veltylabs/mjosefa-cms` la base de
producción ya está creada, así que sin esto cada escritura fallaría después con
"columna desconocida", en silencio hasta el primer INSERT.

`webtyp.com/ddl` ya trae la pieza: `(*ddl.DB).Sync(models...)` hace
`CreateTable` **más** un `OpAddColumn` aditivo por cada columna que falte,
dentro de una transacción. Este plan **debe** cambiar `migrate/migrate.go` a
`Sync`. No es alcance extra: sin eso, la funcionalidad no llega a ninguna base
desplegada y el plan no cumple lo que promete.

## Qué construir

### a) El campo y sus constantes

En `model.go`, junto a las constantes de estado del FSM (misma regla
anti-magic-string: los literales viven SOLO acá):

```go
// Origin dice por qué vía se confirma una reserva, y por lo tanto de quién es
// el turno mientras está PENDING. No se deduce de creator_user_id: una
// inferencia miente en silencio el día que una reserva se cree sin usuario por
// otro motivo.
const (
	OriginCounter = "COUNTER" // la crea y la confirma el personal, en el mostrador
	OriginOnline  = "ONLINE"  // la crea el paciente y la confirma él mismo por su vía
)
```

y en `ReservationModel`, después de `status`:

```go
{Name: "origin", Type: model.Text(), NotNull: true},
```

`model.Text()`, no `input.X()`: es un dato que se fija al crear y **nadie
edita en un formulario** — misma política por rol que ya siguen `status` y las
instantáneas.

### b) Se exige al crear, no se adivina

`CreateReservationCmd` y `CreateReservationArgs` ganan `Origin string`, y
`CreateReservation` lo valida junto al resto:

```go
if cmd.Origin != OriginCounter && cmd.Origin != OriginOnline {
	return Reservation{}, ErrMissingArgs
}
```

**Deliberadamente sin default.** Un `Origin` vacío que cayera a `COUNTER`
marcaría como "de mesón" cada reserva online que olvidara declararlo — el
fallo silencioso que la skill prohíbe. Que el llamador lo diga es una línea; el
dato equivocado en una ficha no se descubre hasta que alguien reclama.

> Esto **rompe a propósito** a los consumidores actuales de `create_reservation`
> hasta que pasen el campo: es un 400 ruidoso, no un dato mal guardado.
> `veltylabs/mjosefa-cms` lo cablea al subir este tag.

### c) La migración backfillea lo que ya existe

Toda reserva anterior a este cambio se hizo en el mostrador — es un hecho, no
una suposición. La migración las deja en `COUNTER`; una columna `NOT NULL`
sobre filas existentes lo necesita de todas formas.

### d) El funcionario lo ve en la lista

`ReservationForm` es la proyección que la pantalla del personal lee (6 campos,
D8). Gana `origin` como campo de solo lectura (`model.Text()`, igual que
`status`), y **su `Item()` deja de mostrar el estado crudo**:

```go
func (r *ReservationForm) Item() view.Item {
	return view.Item{
		ID:          r.Id,
		LeadMain:    r.Hour,
		Label:       r.ClientId,
		Description: reservationStanding(r.Status, r.Origin),
	}
}
```

donde `reservationStanding` responde **de quién es el turno**, no solo el
estado, y pasa por `lang.Translate` con claves en inglés (esta librería no
fija idioma; la app registra el diccionario — mismo patrón que
`business_calendar` ya usa):

| Status | Origin | Clave (EN) |
|---|---|---|
| `PENDING` | `COUNTER` | `Pending confirmation` |
| `PENDING` | `ONLINE` | `Awaiting patient` |
| cualquier otro | — | la clave del propio estado (`Confirmed`, `Cancelled`, …) |

Un `PENDING` sin origin reconocible cae en la clave del estado: degradar a
"Pending" es correcto, inventar un turno que no se sabe no lo es.

## Casos de uso y sus tests (TDD — el test primero, y en rojo antes del fix)

Cada caso es un test en `tests/`. Escribirlos **antes** del código y
comprobarlos en rojo: un test que nunca falló no prueba nada.

| # | Caso de uso | Test |
|---|---|---|
| **CU-1** | El mesón crea una reserva declarando su canal. | `CreateReservation` con `Origin: OriginCounter` guarda `COUNTER` en la fila. |
| **CU-2** | Nadie puede crear una reserva sin declarar el canal. | `CreateReservation` con `Origin: ""` ⇒ `ErrMissingArgs`. **Y con un valor inventado** (`"WHATSAPP"`) ⇒ `ErrMissingArgs` también: la validación es una lista blanca, no un chequeo de no-vacío. |
| **CU-3** | El funcionario ve que una reserva de mesón es trabajo suyo. | `(&ReservationForm{Status: PENDING, Origin: COUNTER}).Item().Description` ⇒ la clave `Pending confirmation`. |
| **CU-4** | El funcionario ve que una online no le toca. | Igual con `Origin: ONLINE` ⇒ `Awaiting patient`. **Y el test afirma que CU-3 y CU-4 dan textos DISTINTOS** — es la razón de ser del plan; dos claves que colapsaran al mismo string dejarían al funcionario igual de ciego con los tests en verde. |
| **CU-5** | Un estado que no es PENDING se lee por su estado, sin importar el canal. | `Status: CONFIRMED` con ambos origins ⇒ la misma clave `Confirmed`. |
| **CU-6** | El funcionario confirma a mano una reserva online (el paciente llamó). | `ChangeReservationStatus(EventConfirm)` sobre una `ONLINE` en `PENDING` ⇒ `CONFIRMED`, y `updated_by` queda con el actor. Ninguna regla nueva lo impide: este test **fija** que sigue siendo legal. |
| **CU-7** | El FSM no cambió. | La tabla de transiciones sigue teniendo exactamente las mismas filas; `Origin` no aparece en ninguna guarda. |
| **CU-8** | Una base ya creada recibe la columna nueva. | Con `storage/mem` u otro backend: crear el esquema con el modelo **sin** `origin`, correr `migrate` con el modelo **con** `origin`, e insertar y leer una fila con el campo. Sin el cambio a `Sync` este test falla — es el que prueba que D10 quedó cerrado para este módulo. |

## Criterios de aceptación

- `gotest` en verde (nunca `go test`).
- `migrate/migrate.go` usa `Sync`, y CU-8 lo demuestra.
- Los literales `"COUNTER"`/`"ONLINE"` aparecen **solo** en las dos constantes.
- La tabla `transitions` del FSM es idéntica a la de antes del cambio.
- Ninguna librería llama a `lang.RegisterWords` ni a `lang.OutLang` — eso es
  del consumidor (regla de `layout/AGENTS.md`); acá solo se usa
  `lang.Translate`.
- `grep -rn "TODO\|FIXME" --include='*.go' .` sin entradas nuevas.

## Fuera de alcance

- **Nada de correo, tokens ni portal del paciente.** Este plan solo deja el
  dato y la visibilidad; la vía online se construye después.
- **No tocar el FSM** (estados, eventos ni transiciones).
- **No agregar caducidad** para reservas sin confirmar (decisión 4).
- **No traducir desde esta librería** — solo `lang.Translate` con claves en
  inglés.

## Etapas

| # | Etapa | Entregable |
|---|---|---|
| 1 | Tests CU-1 a CU-8 escritos y **en rojo** | `tests/` |
| 2 | Constantes + campo en `ReservationModel` + regenerar `model_orm.go` (ormc) | `model.go`, `model_orm.go` |
| 3 | `Origin` en `CreateReservationCmd`/`Args` + validación de lista blanca | `service.go`, `ops.go` |
| 4 | `migrate` a `Sync` + backfill a `COUNTER` | `migrate/migrate.go` |
| 5 | `origin` en `ReservationForm` + `reservationStanding` vía `lang` | `model.go`, `view.go` |
| 6 | `README.md`: fila de `Origin` y la tabla de claves de traducción | `README.md` |
