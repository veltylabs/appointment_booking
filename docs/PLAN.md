---
PLAN: "feat(ui): Reserva Hora y Personal (datos, horario, servicios), su semilla y su demo viven en el módulo"
EXECUTOR: jules
REVIEWER: none
STATUS: review
SESSION: 9473845396645637254
PR: https://github.com/veltylabs/appointment_booking/pull/16
---

> Este plan se despacha con el flujo CodeJob. Ver skill: agents-workflow.
> Todo lo que necesitas está en este plan y en `_temp/` (ver §2, nota final).

# Plan — `appointment_booking` trae su vista (`ui/`), sus datos de demo (`seed/`) y su demo (`web/`)

## 1. Por qué

La vista de este módulo vivía en `veltylabs/mjosefa-cms/modules/appointment_booking` (la app de
producción) y otra versión distinta en `webtyp/app-demo` (ya archivado). Cada pantalla
se escribía dos veces y probar una en la app obligaba a iniciar sesión. Decisión del
dueño (2026-09-24): **la vista vive en el módulo que posee los datos**, en un
subpaquete `ui/` del mismo repo, y el módulo trae su propia demo ejecutable en
`web/client.go`. Las apps solo inyectan base de datos y montan `ui.Browser`.

Reglas cerradas que este plan aplica (no se re-litigan):

- **Sin repos nuevos.** `ui/` es un paquete del mismo `go.mod`. El paquete raíz de
  dominio **sigue sin importar** `layout`/`components`: así el binario de un servidor
  (que importa el dominio) nunca enlaza la UI.
- **La fuente es la versión de mjosefa-cms**, no la de app-demo.
- **Grafo de dependencias entre módulos (DAG)**; un `ui/` importa el dominio y el `ui/`
  de sus módulos **upstream**, nunca de uno downstream:
  `device_manager, item_catalog, patient_directory, business_calendar` (hojas) ←
  `staff_manager` (→ device_manager) ← `appointment_booking` (→ staff_manager,
  item_catalog, patient_directory, business_calendar) ← `clinical_encounter`
  (→ appointment_booking, patient_directory, staff_manager).
- **Una pantalla que mezcla módulos vive en el módulo más downstream que toca.**
- **La demo corre entera en el navegador**: dominio real sobre `orm.New(mem.New())`
  (sin DDL: `storage/mem` no tiene esquema) + `router/loopback` + datos semilla. Sin
  servidor, sin login. `web/client.go` en la raíz de una librería es la convención que
  el daemon ya ejecuta (`layout/web/client.go`, `components/*/web/client.go`): se abre
  `webtyp` en la raíz del módulo y listo.

## 2. Contrato común (idéntico en los 7 módulos)

### 2.1 Estructura que queda en el repo

```
<modulo>/
├── (paquete de dominio, sin cambios salvo lo que diga §4)
├── ui/                 package ui — la vista (neutro: sin build tag)
│   ├── module.go       const ID, Label (identidad RBAC + ítem de nav)
│   ├── browser.go      func Browser(...) (platformd.UIModule, error) + helpers
│   ├── <otros>.go      componentes de la vista (nombres de §4)
│   ├── css.go          //go:build !wasm (solo si §4 lo trae)
│   └── svg.go          //go:build !wasm — ícono del ítem de nav
├── seed/               package seed — datos de demo (neutro: sin build tag)
│   └── seed.go         type Data struct{…}; func Load(...) (Data, error)
├── web/                package main — la demo
│   └── client.go       //go:build wasm
└── tests/              tests de la vista movidos desde mjosefa-cms (§4)
```

### 2.2 Reglas de código (obligatorias)

- **Build tags asimétricos, a propósito**: `ui/*.go` y `seed/*.go` **sin** tag (si
  `browser.go` llevara `wasm`, moriría el carril stdlib de los tests de vista);
  `ui/css.go` y `ui/svg.go` con `//go:build !wasm` (nunca llegan al binario wasm; el
  extractor SSR los descubre por import); `web/client.go` con `//go:build wasm`.
- `css.go` y `svg.go` se llaman **exactamente así**: otro nombre es invisible para el
  extractor SSR.
- Sin stdlib en código que compila a wasm (`ui/`, `seed/`, `web/`, dominio): usar
  `webtyp.com/fmt` en vez de `fmt`/`strings`/`strconv`/`errors`, `webtyp.com/time` en
  vez de `time`, `webtyp.com/json` en vez de `encoding/json`.
- **Sin `map[K]V`** en ningún archivo, tests incluidos: slice + búsqueda lineal o
  `fmt.KeyValue`.
- `dom.Element` se embebe **por valor**, nunca como puntero.
- Sin strings repetidos: los ids de tenant, ids de pestañas y etiquetas son
  constantes.
- **Datos semilla**: `seed.Load` escribe **por los métodos del módulo** (no con
  `db.Create` directo), así cada fila pasa la validación del modelo. Si una fila de §4
  no valida, `Load` devuelve ese error: **ajustar el valor semilla, nunca la
  validación del modelo**. Un `seed.Load` que falla hace `panic` en `web/client.go`.
- **Nunca** un `replace` en `go.mod`. Las dependencias nuevas se agregan con su
  **último tag publicado** (`go get <pkg>@latest`). Riesgo conocido: mjosefa-cms
  compila contra copias locales de `webtyp/components`, `webtyp/layout` y
  `webtyp/widget` (tiene `replace`). Si al portar falta un símbolo en el último tag
  publicado, **parar** y decirlo en la descripción del PR (qué símbolo, de qué paquete). Nunca
  copiar el símbolo aquí ni agregar un `replace`.
- Tests con `gotest` (nunca `go test`). `gotest` corre las dos suites: nativa y
  navegador (wasm).
- **Idioma**: si la sección "Domain-specific notes" del `AGENTS.md` de este repo fija
  uno, manda ese (`staff_manager` y `appointment_booking` exigen español en docs y
  comentarios Go). Si no fija ninguno, se usa el idioma del archivo que se edita (los
  `README.md`/`AGENTS.md` de estos módulos están en inglés). Los snippets de este plan
  traen comentarios en inglés: traducirlos cuando el repo exige español.
- **Anti-footgun**: la lista blanca de `AGENTS.md` sigue prohibiendo `layout`,
  `components`, drivers y transportes en el **paquete raíz de dominio**. No mover
  código de la vista al paquete raíz "para simplificar".

### 2.3 Cambios en `AGENTS.md` (primer paso de la etapa 1)

Aplicar al `AGENTS.md` de este repo **estos tres cambios, textuales** (en inglés, como
el resto de ese archivo por encima de la línea; la sección "Domain-specific notes" no
se toca). Es el mismo texto que ya tiene la plantilla canónica de todos los módulos:

1. En "Model definitions", la frase final `Cross-module references stay soft (a plain
   string id/SKU) — modules never import each other.` →
   `Cross-module references stay soft (a plain string id/SKU) — the root domain
   package never imports another module (see "ui/, seed/ and web/" for the DAG rule).`
2. En "Identity, persistence, …", la frase final de **Cross-module wiring**
   `The composition root wires concrete instances together. Modules never import each
   other.` → `The composition root wires concrete instances together. Root domain
   packages never import each other; only ui/, seed/ and web/ may, upstream only.`
3. Nueva sección, justo antes de "## Testing": el texto citado abajo sin los `>`, con
   su primera línea en negrita convertida en título `## ui/, seed/ and web/ — the
   module's own view and demo`.

> **`ui/`, `seed/` and `web/` — the module's own view and demo.** The whitelist and
> blacklist above apply to the **root domain package**. Three sub-packages are
> exempt, and only them:
>
> - `ui/` (package `ui`, no build tag; `css.go`/`svg.go` tagged `!wasm`) may import
>   `webtyp.com/layout/*`, `webtyp.com/components/*`, `dom`, `html`, `css`, `svg`,
>   `widget`. It is the module's screen: `const ID`, `const Label` and
>   `Browser(caller router.Caller, ids model.IDGenerator, tenantID string)
>   (platformd.UIModule, error)`.
> - `seed/` (package `seed`, no build tag) holds demo data: `Load(...) (Data, error)`,
>   which writes through the module's own methods so every row is validated, and
>   returns the rows it created so downstream demos can reference them.
> - `web/` (package `main`, `web/client.go` tagged `wasm`) is the runnable demo and
>   may additionally import the concrete `webtyp.com/storage/mem`,
>   `webtyp.com/router/loopback`, `webtyp.com/events/mock`, `webtyp.com/unixid` and
>   `webtyp.com/auth/trusted_ip` (for `ValidateRUT`).
>
> **Dependencies between modules form a DAG.** The root domain package still imports
> no sibling module. `ui/`, `seed/` and `web/` may import the domain, `ui/` and
> `seed/` packages of **upstream** modules only. Current graph: `device_manager`,
> `item_catalog`, `patient_directory`, `business_calendar` (leaves) ←
> `staff_manager` ← `appointment_booking` ← `clinical_encounter`
> (`appointment_booking` also depends on `item_catalog`, `patient_directory`,
> `business_calendar`; `clinical_encounter` also on `patient_directory` and
> `staff_manager`). A screen that mixes modules lives in the most-downstream module
> it touches.

### 2.4 La demo `web/client.go` — esqueleto común

Todas las demos tienen esta forma. §4 dice qué módulos construye y qué `seed.Load`
llama:

```go
//go:build wasm

package main

import (
	. "webtyp.com/dom"
	"webtyp.com/events/mock"
	"webtyp.com/layout/platformd"
	"webtyp.com/orm"
	"webtyp.com/router/loopback"
	"webtyp.com/storage/mem"
	"webtyp.com/unixid"
	// + los paquetes de dominio, seed y ui que diga §4
)

// demoTenantID is the only tenant of this in-browser demo.
const demoTenantID = "demo"

// demoUser is the fixed identity the demo shell shows: the demo has no login.
type demoUser struct{}

func (demoUser) UserName() string    { return "Demo" }
func (demoUser) UserAvatar() string  { return "" }
func (demoUser) UserRoles() []string { return []string{"Administrador"} }

func main() {
	ids, err := unixid.NewUnixID()
	if err != nil {
		panic(err)
	}
	db := orm.New(mem.New())
	broker := &mock.Broker{}

	// 1. construir los módulos (§4) — panic en error: una demo que no arma no sirve
	// 2. seed.Load de cada uno, upstream primero (§4) — panic en error;
	//    un seed downstream recibe el seed.Data de sus upstream
	// 3. caller := loopback.WithTenant(demoTenantID, <módulos>...)
	// 4. v, err := ui.Browser(caller, ids, demoTenantID) — panic en error

	p := &platformd.Platform{
		AppName:   "<Label del módulo> — demo",
		User:      demoUser{},
		Modules:   []platformd.UIModule{v},
		DefaultID: ui.ID,
	}
	Append("body", p)
	select {}
}
```

`loopback.WithTenant(tenantID string, mods ...router.OperationModule) router.Caller`
monta los módulos en proceso; `Brand` de `platformd` es opcional y se omite. Las
funciones que bloquean esperando una llamada (`blockingCall` en clinical_encounter)
son válidas aquí por la misma razón que en la app: `ui.Browser` corre en `main`, antes
de que arranque el event loop del navegador.

### 2.5 Design gate (común)

1. **Prior art.** Odoo: cada módulo trae modelos + **vistas** + **datos de demo**
   (`'demo': [...]` en el manifiesto, cargados solo en modo demo) y declara `depends`.
   Django: cada app trae `views`/`templates` junto a sus modelos. Storybook: cada
   componente trae su propia historia ejecutable. Aquí: `ui/` = vistas, `seed/` = datos
   de demo, `web/client.go` = la historia ejecutable, con la convención de entrada que
   el daemon ya tiene.
2. **Prueba del nombre.** `ui.Browser(caller, ids, tenantID)` → "la vista de este módulo
   para el navegador" (el mismo nombre que ya usa `mjosefa-cms/modules/browser.go`).
   `seed.Load(...)` → "cargar los datos semilla"; devuelve `seed.Data` (las filas
   creadas). `ui.ID`, `ui.Label` sin cambios.
3. **Contabilidad de complejidad.** Conceptos +1 (`seed`). Archivos que una app toca
   para mostrar este módulo: de ~4 (`module.go`, `browser.go`, `svg.go`, helpers) a 1
   línea en su registro. Formas de construir esta pantalla: de 2 (mjosefa + app-demo) a
   1. Costo honesto: el `go.mod` del módulo gana `layout`/`components`, que un consumidor
   solo-servidor descarga pero nunca enlaza.
4. **Dónde vive.** En el módulo que posee los datos (D1 del plan maestro).
5. **Qué borra.** `mjosefa-cms/modules/appointment_booking/` completo (lo borra el plan de
   mjosefa-cms, fase C, después del tag de este repo) y los tests de vista de
   mjosefa-cms que se mueven aquí.

> **La fuente está en `_temp/` de este mismo repo.** `mjosefa-cms` es un repo privado
> al que no tienes acceso, así que el dueño copió aquí **solo** los archivos que este
> plan porta: `_temp/mjosefa-cms/modules/...` (el código de la vista) y
> `_temp/mjosefa-cms/tests/...` (los tests a mover y sus helpers `idgen_test.go` →
> `testIDGen`, `item_catalog_view_mock_test.go` → `mockCaller`). Go ignora los
> directorios que empiezan con `_`, así que `_temp/` no compila ni rompe `go build
> ./...`. Esos archivos importan `github.com/veltylabs/mjosefa-cms/...`: esos imports
> son justo lo que este plan reescribe al copiarlos fuera de `_temp/`.
> **La última etapa borra `_temp/` completo**: el PR no debe contenerlo.

## 3. Fuente

Espera los tags de `staff_manager` (B2) y de las cuatro hojas (B1).

Por la regla "la pantalla mixta vive en el módulo más downstream", este módulo recibe
**dos** pantallas: "Reserva Hora" (la suya) y "Personal" (hoy
`_temp/mjosefa-cms/modules/personal/`, que mezcla el panel de `staff_manager` con el horario y los
servicios de este módulo).

| Origen | Destino | Cambios |
|---|---|---|
| `modules/appointment_booking/module.go` | `ui/module.go` | `package ui`. Conservar `ID = "appointment_booking"` y `NavLabel = "Reserva Hora"` (se llama `NavLabel` porque `html.Label` ya existe en el scope por el dot-import de `webtyp.com/html`; el comentario lo explica, conservarlo). |
| `modules/appointment_booking/browser.go` | `ui/browser.go` | `package ui`. |
| `modules/appointment_booking/bookingview.go` | `ui/bookingview.go` | `package ui`. |
| `modules/appointment_booking/scheduleview.go` | `ui/scheduleview.go` | `package ui`. |
| `modules/appointment_booking/serviceconfigview.go` | `ui/serviceconfigview.go` | `package ui`. |
| `modules/appointment_booking/staffpicker.go` | `ui/staffpicker.go` | `package ui`. |
| `modules/appointment_booking/css.go` | `ui/css.go` | `package ui`, conserva `//go:build !wasm`. |
| `modules/appointment_booking/svg.go` | `ui/svg.go` | `package ui`, conserva `//go:build !wasm`. |
| `modules/personal/module.go` | `ui/personal.go` | `package ui`. `ID`/`Label` → `PersonalID = "personal"` (**no** cambiar el valor: es recurso RBAC), `PersonalLabel = "Personal"`. |
| `modules/personal/browser.go` | `ui/personal.go` (mismo archivo) | `Browser` → `PersonalBrowser`. El crudview de staff se reemplaza por `staffui.StaffPanel(caller, ids, PersonalID+".staff")`. `appointmentbooking.NewScheduleView` / `NewServiceConfigView` (el wrapper) → `NewScheduleView` / `NewServiceConfigView` del propio paquete. Ids de pestaña y textos a constantes. |
| `modules/personal/svg.go` | se fusiona en `ui/svg.go` (el extractor SSR solo ve un archivo llamado `svg.go`) | copiar su `sprite.Define(svg.Icon(PersonalID), …)` en `ui/svg.go`, dentro del mismo `IconSvg()`, como segunda definición del sprite. |
| `modules/appointment_booking/docs/reference/demo-reservation.png`, `modules/personal/docs/reference/demo-agenda.png` | `docs/reference/` | capturas de referencia, tal cual. |

Todos los orígenes están bajo `_temp/mjosefa-cms/`. `server.go` **no** se copia: su `NewBackend`
(que arma los cuatro readers) es composition root; en la demo lo hace
`web/client.go` (etapa 4).

Imports a reescribir en todos los archivos copiados:
`github.com/veltylabs/mjosefa-cms/modules/<x>` → nada (lo que venía del propio wrapper
queda en el mismo paquete `ui`), salvo `modules/staff_manager` →
`staffui "github.com/veltylabs/staff_manager/ui"`. Los imports de dominio
(`staffmanager`, `itemcatalog`, `patientdirectory`, `businesscalendar`) se quedan: los
permite la regla del DAG en `ui/`. El paquete **raíz** de este módulo sigue sin
importar ningún módulo hermano (sus cuatro readers se satisfacen estructuralmente).


## 4. Etapas

### Etapa 1 — `AGENTS.md`

Aplicar §2.3.

### Etapa 2 — `ui/`

Copiar y adaptar lo de §3. Resultado exportado:

```go
const ID, NavLabel, PersonalID, PersonalLabel
func Browser(caller router.Caller, ids model.IDGenerator, tenantID string) (platformd.UIModule, error)          // Reserva Hora
func PersonalBrowser(caller router.Caller, ids model.IDGenerator, tenantID string) (platformd.UIModule, error)  // Personal
func NewBookingView(caller router.Caller, tenantID string) dom.Component
func NewScheduleView(caller router.Caller, tenantID string) dom.Component
func NewServiceConfigView(caller router.Caller, tenantID string) dom.Component
```

### Etapa 3 — `seed/seed.go`

```go
type Upstream struct {
	Staff    staffseed.Data
	Catalog  catalogseed.Data
	Patients patientseed.Data
}

type Data struct {
	ServiceConfigs []appointmentbooking.EmployeeServiceConfig
	Reservations   []appointmentbooking.Reservation
}

func Load(m *appointmentbooking.Module, tenantID string, up Upstream) (Data, error)
```

Portar la lógica de semilla de app-demo (archivado; copia en
`_temp/app-demo/config/env.go`, también en
`https://github.com/webtyp/app-demo/blob/165ad5e/config/env.go`): funciones
`upsertCalendarConfigs`, `upsertBlocks`, `seedEmployeeServiceConfig`,
`seedReservations`, **reemplazando** sus datos inventados por los reales de `up`:

1. Por cada funcionario de `up.Staff.Staff`: `m.UpsertCalendarConfig(WorkCalendarConfig{
   TenantId, StaffId, Timezone: "America/Santiago", IsActive: true})`.
2. Bloques: `m.SaveDayBlocks(tenantID, staffID, dow, blocks)` lunes a viernes
   (1–5), 09:00–13:00 y 15:00–18:00, con el mismo formato de bloque que usa
   `upsertBlocks` de app-demo.
3. Servicio por funcionario: `m.CreateEmployeeServiceConfig(EmployeeServiceConfig{
   TenantId, StaffId, ServiceId: <ver abajo>, DurationMin: 30, BufferMin: 0,
   IsActive: true})`. Cómo elegir `ServiceId`: `StaffMember.Specialty` guarda el
   **nombre visible** de la especialidad (ej. `Medicina General`; el campo es
   `input.Text()` y no admite el `-` de un slug). Buscar en `up.Catalog.Specialties`
   la especialidad cuyo `Name` es igual al `Specialty` del funcionario, y usar el ítem
   de `up.Catalog.Items` cuyo `SpecialtyId` es el `Id` de esa especialidad. Si no hay
   coincidencia, `Load` devuelve error (nunca un servicio elegido al azar).
4. Dos reservas del **próximo día hábil** (hoy + 1, saltando sábado y domingo), con
   `m.CreateReservation(CreateReservationCmd{TenantId, ClientId: up.Patients.Patients[i].Id,
   EmployeeServiceConfigId: <el primero>, SlotStartUtc: <09:00 y 09:30 de ese día, hora
   de Santiago, en UTC — igual que `localToUTC` de app-demo>, Origin:
   appointmentbooking.OriginCounter})`, y luego confirmarlas con
   `m.ChangeReservationStatus` usando el evento de confirmación del FSM del módulo
   (ver `fsm.go`; mismo camino que "Confirmar reserva elegida" de `bookingview.go`).

**No** portar de app-demo: `stubStaff`/`stubCatalog`/`stubDirectory`, la lista
`StaffOption` ni `holidaysCL2026` (los datos vienen de los seeds de los módulos
upstream), ni nada de `work_schedule` (módulo descartado).

### Etapa 4 — `web/client.go`

Esqueleto de §2.4 construyendo, en este orden, `device_manager`, `staff_manager` (con
`devicemanager.IPLocator`, como la demo de staff_manager), `item_catalog`,
`patient_directory` (con `trustedip.ValidateRUT`), `business_calendar`, y este módulo:

```go
ab, err := appointmentbooking.New(db, appointmentbooking.Deps{
	Staff: sm, Catalog: ic, Directory: pd, Bounds: bc,
	IDs: ids, Publisher: broker,
})
```

Semillas, upstream primero: `deviceseed.Load`, `staffseed.Load`, `catalogseed.Load`,
`patientseed.Load`, `calendarseed.Load(bc)`, y `seed.Load(ab, demoTenantID,
seed.Upstream{...})`. `caller := loopback.WithTenant(demoTenantID, ab, sm, dm, ic, pd,
bc)`. **Dos** módulos en el shell:

```go
booking, err := ui.Browser(caller, ids, demoTenantID)
personal, err := ui.PersonalBrowser(caller, ids, demoTenantID)
// Modules: []platformd.UIModule{booking, personal}, DefaultID: ui.ID
```

Además, igual que `mjosefa-cms/config/auth.go` hoy: suscribir el broker a
`businesscalendar.EventCalendarChanged` para llamar `ab.RecomputeConflicts` cuando
`Closed` es `true` (así la demo muestra conflictos al agregar un feriado).

### Etapa 5 — tests movidos desde mjosefa-cms

De `_temp/mjosefa-cms/tests/modules_view_wasm_test.go` mover `TestWASM_AppointmentBooking_ViewBuildsAndLoadsStaff`
y `TestWASM_Personal_ViewBuildsWithItsTabs` a `tests/ui_view_wasm_test.go`
(`//go:build wasm`), con los helpers que usan. Imports →
`github.com/veltylabs/appointment_booking/ui`; `personal.Browser` →
`ui.PersonalBrowser`. Aserciones sin cambios. Agregar `tests/seed_test.go`: con las
seis semillas sobre `orm.New(mem.New())`, `seed.Load` devuelve 3 configuraciones de
servicio y 2 reservas confirmadas, y `ListAvailability` del primer funcionario para
ese día devuelve huecos libres (la demo de reserva no puede salir vacía: es el
bloqueante B3 de producción, "Reserva Hora no puede emitir una sola hora").

### Etapa 6 — `go.mod` y README

1. `go get webtyp.com/layout@latest webtyp.com/components@latest webtyp.com/dom@latest
   webtyp.com/html@latest webtyp.com/svg@latest webtyp.com/css@latest
   webtyp.com/widget@latest webtyp.com/storage@latest webtyp.com/events@latest
   webtyp.com/unixid@latest webtyp.com/auth@latest webtyp.com/time@latest github.com/veltylabs/staff_manager@latest github.com/veltylabs/device_manager@latest github.com/veltylabs/item_catalog@latest github.com/veltylabs/patient_directory@latest github.com/veltylabs/business_calendar@latest` y `go mod tidy`. Sin `replace` (ver §2.2).
2. `README.md`, sección nueva **"View and demo"** (en inglés, como el README): qué
   exporta `ui` (`ID`, `Label`, `Browser`, `PersonalBrowser`, `NewBookingView`, `NewScheduleView`, `NewServiceConfigView`), qué carga `seed.Load`, y "run
   `webtyp` at the repository root to open the demo — in-browser, in-memory, no login".

### Etapa 7 — borrar `_temp/`

Cuando todo lo anterior está verde: `rm -rf _temp` y commitear ese borrado en el mismo
PR. El PR no debe contener ningún archivo bajo `_temp/`.

## 5. Verificación

```bash
gotest                                                        # verde (nativo + navegador)
GOOS=js GOARCH=wasm go build -o /dev/null ./web/              # la demo compila
go list -deps . | grep -c 'webtyp.com/layout\|webtyp.com/components'   # 0: el dominio no arrastra UI
grep -rln 'webtyp.com/layout\|webtyp.com/components' --include=*.go . \
  | grep -v '^./ui/\|^./web/\|^./tests/'                      # vacío
grep -rn 'mjosefa-cms' --include=*.go .                        # vacío
grep -rn 'map\[' --include=*.go ui seed web                    # vacío
```

```bash
test ! -e _temp && echo ok                                     # _temp/ ya no existe
```

Prueba visual (la hace el dueño al revisar el PR): `webtyp` en la raíz del repo → el
navegador muestra la pantalla de §4 con los datos semilla, sin login.

## 6. Tabla de etapas

| # | Etapa | Archivos |
|---|---|---|
| 1 | AGENTS.md | `AGENTS.md` |
| 2 | vistas | `ui/*.go` (8 del booking + `personal.go`), `docs/reference/` |
| 3 | semilla | `seed/seed.go` |
| 4 | demo | `web/client.go` |
| 5 | tests | `tests/ui_view_wasm_test.go`, `tests/seed_test.go` |
| 6 | deps + README | `go.mod`, `go.sum`, `README.md` |
| 7 | borrar `_temp/` | `_temp/` |
