# AGENTS.md — veltylabs/modules (plantilla canónica)

Notas de trabajo para agentes IA que operan en cualquier repositorio `veltylabs/<module>`. Cada módulo recibe su propia
copia de este archivo en su raíz, ajustada únicamente en la sección "Notas específicas del dominio" al final —
las reglas sobre esa línea son **textuales en todos los módulos**, no las modifiques ni ramifiques. Si una regla aquí
resulta ser errónea para un módulo, la corrección pertenece aquí (y se replica hacia afuera), no como una excepción local
en la copia de un módulo.

Plan maestro (estado entre repositorios, justificación de lista blanca, orden de despacho):
[`app-releases/docs/REUSABLE_MODULES_MASTER_PLAN.md`](https://github.com/webtyp/app/blob/main/docs/REUSABLE_MODULES_MASTER_PLAN.md).
Implementación de referencia (el patrón que cada módulo replica): `github.com/veltylabs/item_catalog`.

## Misión de un repositorio `veltylabs/modules/*`

Un **módulo de dominio**: lógica de negocio para un dominio acotado (catálogo, programación/agendamiento, pagos, …),
publicado como una librería Go independiente, importable por cualquier aplicación en el ecosistema Velty. Es una pieza
de lego — debe ensamblarse bajo un transporte, una base de datos, un generador de IDs, una codificación y un
renderizador que nunca ha visto, elegidos por cualquiera que sea la aplicación que lo componga.

## La lista blanca — lo que un módulo puede importar

Los archivos Go de un módulo que **no sean de prueba** pueden importar, desde `github.com/webtyp/*`:

| Paquete | Rol | Por qué es un puerto y no una dependencia concreta |
|---|---|---|
| `model` | `Model`/`Fielder`/`Encodable`/`Decodable`/`IDGenerator`/`Definition` | *Interfaces* de esquema + codec; los codificadores concretos (`json`, `jsvalue`) viven fuera |
| `router` | `OpModule`/`OpRegistry`/`Context`/`Caller` | Agnóstico del transporte; un módulo implementa `OpModule`, nunca un servidor concreto |
| `view` | `Presenter`, `view.New(...)` | Contrato de UI; el renderizador (`layout/crudview` o cualquier otro) es inyectado por la aplicación |
| `events` | `Publisher`/`Subscriber`/`Event` | Contrato de pub/sub; el broker (en proceso, `sse`, una cola) es inyectado |
| `orm` | `*orm.DB`, constructor de consultas (`Create`/`Update`/`Delete`/`Query`) | Capa ergonómica sobre `storage.Conn` — el equivalente de `database/sql`, agnóstico del backend por construcción |
| `storage` | `Conn`/`Condition`/`Query` (mayormente transitivo, vía las re-exportaciones de `orm` en `reexport.go`) | El **puerto** de almacenamiento real — `orm` nunca lo redefine |
| `ddl` | `CreateTable`/`Sync` (esquema en tiempo de ejecución) | Hermano de `orm` sobre `storage.Conn` — sigue siendo agnóstico del backend |
| `form/input` | `input.Text()`, `input.Number()`, … (`Kind` con un widget de UI) | Solo cuando un campo de `model.Definition` necesita un widget para un formulario; sigue siendo solo un `Kind`, sin renderizador |
| `fmt` | manejo de cadenas/números/errores, `Err(...)` | El reemplazo de stdlib del ecosistema — ver la sección de stdlib a continuación |
| `time` | ayudantes de fecha/hora | El reemplazo de `time` del ecosistema (isomórfico, seguro para wasm) |

**"Puerto" vs "implementación concreta" es toda la prueba.** `orm`/`storage`/`ddl` son agnósticos — funcionan
sin cambios contra backends `sqlt`, `postgres`, `mem` o un futuro backend `indexdb`, seleccionados por quien
llame a `orm.New(conn)`. Un módulo que los importa **no** sabe ni le importa qué backend está detrás del
`storage.Conn` que recibe. Eso es diferente a importar `webtyp/sqlite` (un controlador) o
`webtyp/mcp` (un transporte) directamente — esos nombran una implementación, descartando todas las demás.

## La lista negra — lo que un módulo nunca puede importar, en ningún archivo, incluidas las pruebas

- **Cualquier backend de almacenamiento concreto**: `webtyp/sqlite`, `webtyp/sqlt`, `webtyp/postgres`,
  `webtyp/indexdb`, o cualquier controlador `database/sql`. Las propias pruebas del módulo ejercitan
  `storage/mem` (`github.com/webtyp/storage/mem`) vía `orm.New(mem.New())` — el backend de referencia en memoria
  construido exactamente para esto. **Ni siquiera en archivos `_test.go`** — un controlador concreto
  incorporado "solo para pruebas" sigue siendo una dependencia que el módulo envía, y es exactamente el acoplamiento
  que esta lista blanca existe para prevenir. Las pruebas de integración de backend pertenecen al repositorio de la aplicación (raíz de composición), nunca al módulo.
- **Un transporte concreto**: `webtyp/mcp`, `webtyp/server`/`httpd`, o cualquier cosa que importe
  `net/http`. Un módulo habla `router.OpModule`; la aplicación decide qué transporte lo cosecha.
- **Un generador de ID concreto**: `webtyp/unixid`. En su lugar, acepta `model.IDGenerator` vía `Deps` —
  nunca construyas uno dentro del módulo.
- **Un codificador concreto**: `webtyp/json`, `webtyp/jsvalue`. Los modelos de un módulo implementan
  `model.Encodable`/`Decodable` (generados por `ormc`); qué `FieldWriter`/`FieldReader` concreto
  los recorre (texto JSON, valores JS, un futuro codec binario) es elección de la aplicación, hecha en el límite
  del transporte, nunca dentro del módulo.
- **Un renderizador concreto**: `webtyp/layout` (o cualquier otro kit de UI). Un módulo construye su
  `view.Presenter` con `view.New(caller, ...)` — solo `view`+`model`+`router`. La aplicación elige el
  renderizador que dibuja ese `Presenter`.
- **Un puerto autodeclarado que duplica un contrato del ecosistema**: ninguna interfaz local `EventPublisher`,
  `UIAdapter`, `IDGenerator` o `CatalogService`-como-shim-de-transporte que se cruce con
  `events.Publisher`/`view.Presenter`/`model.IDGenerator`/`router.OpModule`. Si un límite necesita un
  contrato que esta lista no nombra, eso es un defecto **aguas arriba** (en `model`/`router`/`view`/`events`/
  `orm`), corregido allí y consumido aquí — nunca parcheado localmente. Un módulo aún puede declarar sus propias
  interfaces de lector estrechas entre módulos (`CatalogReader`, `StaffReader`, …) para datos de **dominio** que
  necesita de un módulo hermano — ese patrón se mantiene (ver Cableado entre módulos a continuación).
- **Ningún paquete `internal/` que bifurque o venda un repositorio del ecosistema.** Si a un paquete aguas arriba le
  falta una función, la solución es un `docs/PLAN.md` contra *ese* repositorio, publicado aguas arriba — nunca una
  copia local con una directiva `replace` en `go.mod`. Un `replace` apuntando a una ruta local siempre es un
  defecto a cerrar, no una solución alternativa a mantener.

## El límite de stdlib

`github.com/webtyp/fmt` **es** el reemplazo de stdlib para este ecosistema: nada en él, y
nada construido sobre él, importa `fmt`, `strings`, `strconv` o `errors` — `webtyp/fmt` ya
proporciona manipulación de cadenas, conversión de números y construcción de errores (`fmt.Err(...)`),
libre de reflexión y de tamaño TinyGo. Un módulo apunta a `wasm`/TinyGo primero, por lo que sigue la misma regla:

- **Prohibido, usa el reemplazo de webtyp en su lugar:** `errors`, `strings`, `strconv`, stdlib `fmt` →
  `github.com/webtyp/fmt`. `encoding/json` → `model.Encodable`/`Decodable` (el módulo nunca
  elige el codificador concreto). `database/sql` → `orm`/`storage`. `net/http` → `router`. `time` →
  `github.com/webtyp/time`.
- **Aceptable para usar directamente:** cualquier cosa que no sea un contrato que este ecosistema ya reemplace —
  `testing`, `sort`, `context` (stdlib, cuando es genuinamente cancelación/plazos límite, no el
  `webtyp/context` a nivel de transporte), etc. En caso de duda: si `webtyp/fmt` (u otro paquete en
  la lista blanca anterior) ya lo cubre, úsalo; si no lo hace, stdlib puro está bien.
- **Idioma del repositorio:** Toda la documentación del repositorio (archivos markdown, comentarios en código, etc.) debe estar escrita en español.
- **Sin `map[K]V` de Go en ninguna parte**, código de prueba incluido — y **sin excepciones para "estado privado"**: un
  mapa dentro de un cierre o campo no exportado envía el entorno de ejecución de mapas de TinyGo en el binario wasm
  exactamente igual. Usa `fmt.KeyValue{Key, Value string}` para un par cadena→cadena, o una pequeña rebanada (slice) de estructuras
  escaneada linealmente para cualquier otra cosa (el caché de escaneo lineal `byID []*X` en `item_catalog/view.go` es
  la referencia) — las colecciones de módulos (los elementos de un inquilino, los campos de un esquema) son siempre lo suficientemente pequeñas
  como para que un escaneo lineal no cueste nada medible.
- **Sin `reflect`.** La introspección de estructuras es una preocupación en tiempo de compilación (`ormc`); el módulo consume
  `Schema()`/`Pointers()`/`EncodeFields`/`DecodeFields` generados, nunca se inspecciona a sí mismo en
  tiempo de ejecución.

## Definiciones de modelo — restricciones, enumeraciones, política de widgets

- **Las restricciones se declaran en la `Definition`, y la validación se ejecuta antes de cada escritura.** Los campos requeridos
  llevan `NotNull: true`; los formatos llevan un piso `Permitted` (p. ej., moneda ISO-4217:
  `Permitted{Letters: true, Minimum: 3, Maximum: 3}`; una columna `"HH:MM"`: dígitos + `':'`, exactamente
  5). Cada ruta de creación/actualización llama al `Validate(action)` generado (o
  `model.ValidateFields`) **antes** de `db.Create`/`db.Update` — con fallo cerrado: los datos que nunca fueron
  validados nunca llegan a la BD. Las comprobaciones manuales `if x == ""` pueden existir como defensa en profundidad, pero nunca
  reemplazan la restricción declarada.
- **Columnas con valor de enumeración: los literales de cadena viven SOLO en constantes exportadas** (`ItemTypeService`,
  `PayoutStatusPending`, …) — nunca en línea en un punto de llamada, nunca documentados únicamente en un
  comentario `// "a" | "b"`. Un valor que debe *recordarse* es un agujero en el arnés. Un
  campo de enumeración editable por el usuario obtiene adicionalmente un widget de opciones cerradas: un widget personalizado local del paquete
  que envuelve `input.Radio()`/`input.Select()` + `SetOptions(fmt.KeyValue{Key: TheConstant, ...})` —
  el patrón `form/input/gender.go`. Agregar un valor de enumeración = una nueva constante + una línea de opción,
  buscable con grep. (Brecha conocida aguas arriba: `input.Base.Validate` aún no exige la pertenencia en
  `Options`; ese es un defecto de `webtyp/form` que se está corrigiendo aguas arriba — nunca lo parches localmente).
- **Los widgets se asignan por ROL, nunca se copian de un archivo generado anterior**: `input.X()` SOLO en
  campos que un usuario edita en un formulario. Tipos base (`model.X()`) en ids, `tenant_id`, marcas de tiempo y cada
  modelo de resultado/respuesta **solo de salida** — la salida nunca se renderiza como un formulario editable, y un widget
  allí hace que `form.New` produce entradas editables para datos que el usuario no debe tocar. Omitir un widget
  de un campo genuinamente vinculado a un formulario renderiza silenciosamente un formulario vacío — así que la regla corta en ambos sentidos.
- **Las claves foráneas intramódulo están declaradas** (`Ref: &OtherModel` + `DB: &model.FieldDB{RefColumn:
  "id"}` — impulsa la generación de restricciones DDL; el tipo Go se mantiene como escalar plano). Las referencias
  entre módulos se mantienen blandas (un ID/SKU de cadena plana) — los módulos nunca se importan entre sí.

## Controladores de ops — decodificar → validar → responder

- Forma del controlador, en orden: `ctx.Decode(&args)` (error ⇒ 400) → `args.Validate(action)` (error ⇒
  400) → método de servicio → codificar/estado.
- **Convención de estado** (nunca colapsar todo a 500 — un uso indebido que produce un 500 genérico
  es el "misterio en tiempo de ejecución" que el arnés prohíbe): `400` decodificación/validación/precondición inválida ·
  `403` denegación de RBAC (el enrutador lo escribe) · `404` no encontrado (centinelas de la clase `ErrNotFound`) ·
  `409` conflicto (`ErrAlreadyExists`, espacio tomado, desajuste de revisión) · `500` solo errores internos genuinos. Los centinelas siempre se distinguen de los errores reales antes del mapeo — y un error de BD real
  **nunca** se traga en un no encontrado (`err == orm.ErrNotFound` se mapea al
  centinela de dominio; cualquier otra cosa se propaga como el error interno que es).
- **`.Requires(resource, action)` declara cada acción que la op realmente puede realizar.**
  `model.Action` es una máscara de bits: un upsert que crea en la rama de no encontrado y actualiza en caso contrario
  requiere `model.Create|model.Update` — declarar solo una permite que un principal parcialmente autorizado realice
  la otra (violación de cerrado por defecto).
- **Una op sin argumentos declara `.Accepts(nil)`** ("nil significa 'sin argumentos'", según `router.Route`) — nunca una
  estructura/definición de argumentos vacía inventada.

## Multi-inquilino (Multi-tenancy) — el alcance se aplica en la condición, no en la lectura previa

- Cada tabla con alcance de inquilino lleva `tenant_id` (`NotNull: true`), y **cada condición de UPDATE/DELETE
  incluye la columna de inquilino** — `orm.Eq(X_.Id, id)` solo es una escritura entre inquilinos
  esperando suceder; el patrón leer-luego-escribir (obtener con verificación de inquilino, luego mutar solo por ID)
  es una ventana TOCTOU, no una defensa. Nunca mutes lo que no pudiste leer: si la búsqueda con alcance de inquilino
  falla, devuelve el error — sin alternativas de "eliminación simulada".
- Un módulo que deliberadamente **no** tiene alcance de inquilino lo establece en sus "Notas específicas del dominio" y
  `docs/ARCHITECTURE.md`, como una decisión explícita firmada — nunca por omisión silenciosa.

## Identidad, persistencia, transporte, vista, eventos — la forma que toma cada módulo

- **Identidad**: `Deps.IDs model.IDGenerator`, requerido. El módulo llama a `m.ids.NewID()`; nunca
  construye un generador.
- **Persistencia**: `New(db *orm.DB, deps Deps)` recibe un `*orm.DB` ya conectado (respaldado por
  cualquiera que sea el `storage.Conn` que eligió la aplicación) y posee su propia migración de esquema vía
  `github.com/webtyp/ddl`, reemplazando el eliminado `orm.DB.CreateTable`. `ddl.New` toma **dos**
  argumentos — `ddl.New(conn storage.Conn, ddlCompiler ddl.Compiler)` — y `ddl.Compiler` es una
  capacidad que solo los backends SQL (`sqlt`, `postgres`) implementan; el backend de pruebas en memoria
  (`storage/mem`) no lo hace, porque crea tablas perezosamente en el primer `Exec` y no necesita DDL en absoluto.
  Por lo tanto, el módulo hace una aserción de tipo para la capacidad en lugar de asumirla (el mismo idioma
  que `storage.TxExecutor` ya usa para transacciones opcionales):
  ```go
  if ddlCompiler, ok := db.RawConn().(ddl.Compiler); ok {
      if err := ddl.New(db.RawConn(), ddlCompiler).CreateTable(&CatalogItem{}); err != nil {
          return nil, err
      }
  }
  ```
  Contra `storage/mem` (pruebas del módulo) esto es una operación nula (no-op) — nada que crear. Contra un backend SQL real
  migra el esquema, exactamente como lo hacía el antiguo `orm.DB.CreateTable`. El módulo nunca
  recibe una cadena de conexión sin procesar ni elige un controlador.
- **Transporte**: el módulo implementa `router.OpModule` — `ModelName() string` +
  `MountOps(reg router.OpRegistry)`, registrando cada operación con `.Requires(resource, action)`
  y `.Accepts(&ArgsType{})`. Nunca implementa `router.APIModule`/`Router`, y nunca ve
  `mcp.Tool`/`mcp.ToolProvider`.
- **Vista**: `NewView(caller router.Caller) view.Presenter`, construido con `view.New(...)` — importando
  solo `view`+`model`+`router`. La aplicación proporciona tanto el `router.Caller` como el renderizador que dibuja
  el `Presenter` resultante.
- **Eventos**: `Deps.Publisher events.Publisher`, opcional — `nil` deshabilita la publicación silenciosamente. El
  módulo publica `events.Event{Topic: ..., Payload: &typedRecord}` después de cada mutación exitosa,
  nunca un payload `map` o `any` desnudo.
- **Cableado entre módulos**: cuando el módulo A necesita datos del módulo B, A declara la interfaz estrecha que
  necesita (`CatalogReader`, `StaffReader`, …) en su propio paquete; el `*Module` de B la satisface estructuralmente
  (sin importación de A). La raíz de composición conecta instancias concretas. Los módulos nunca se importan entre sí.

## Pruebas

- Ejecutor: `gotest`, nunca `go test` directamente (una vez instalado vía
  `go install github.com/webtyp/devflow/cmd/gotest@latest`).
- Las propias pruebas de un módulo construyen su `*orm.DB` sobre `storage/mem` (`orm.New(mem.New())`), ejecutan
  `MountOps` contra `router/mock` (satisface `router.OpRegistry`), y ejercitan el `view.Presenter`
  contra el `FakeCaller` de `view/conformance` o un `router.Caller` simulado hecho a mano — nunca una BD, transporte o renderizador concreto.
- Las pruebas viven en `tests/` (paquete `tests`, externo — ejercita solo la API exportada), según la
  convención del ecosistema. `tests/` es un directorio plano **dentro del módulo raíz** — **nunca un módulo Go anidado**: sin `tests/go.mod`, sin `replace` apuntando de vuelta al padre (un `replace` de ruta local siempre es un defecto, ver la lista negra). Las dependencias solo de prueba se resuelven mediante un `go mod tidy` en la raíz del módulo.
- Las suites de conformidad viven en su propio archivo con el mismo nombre (`tests/conformance_test.go`) — los archivos de prueba
  se mantienen pequeños y modulares, nunca un solo archivo gigante.
- Cada módulo con alcance de inquilino incluye **pruebas de aislamiento de inquilinos** (el inquilino A no puede leer/actualizar/
  eliminar filas del inquilino B a través de ningún método de servicio u op) — en la propia suite del módulo, no
  diferido a una lista de pendientes.
- Las pruebas hacen aserciones sobre resultados — una prueba que llama a métodos y descuarta cada retorno
  (`_ = m.Validate(0)`) infla la cobertura mientras no prueba nada; la cobertura obtenida de esa manera no
  cuenta. Prefiere viajes de ida y vuelta (codificar→decodificar igualdad campo por campo) y aserciones de comportamiento.
- Un módulo cuyas Definiciones llevan widgets de formulario incluye la prueba de regresión de widgets: `form.New(id,
  &GeneratedArgs{})` rinde exactamente las entradas esperadas — detecta una regeneración que silenciosamente
  pierde widgets.
- Las verificaciones de contrato en tiempo de compilación pertenecen al lado de la implementación: `var _ router.OpModule =
  (*Module)(nil)`.

## Publicación / despacho

Los cambios a un repositorio `veltylabs/modules/*` pasan por el flujo de trabajo CodeJob (ver skill
**agents-workflow**): escribe un `docs/PLAN.md` autocontenido con encabezado (`PLAN`/`TAG`/
`EXECUTOR`/`REVIEWER`), el humano lo despacha (`codejob`), un agente ejecutor abre un PR, un
agente revisor opcional lo juzga, y cerrar el ciclo (`codejob 'msg'` o fusionar el PR) llama a
`gopush` internamente. Un agente de planificación/autoría **escribe** el plan;
nunca ejecuta `codejob` o `gopush` él mismo — el despacho y el cierre son decisión del humano.

## Documentación que debe llevar un módulo

| Archivo | Propósito |
|---|---|
| `AGENTS.md` | Este archivo, copiado textualmente + una sección de "Notas específicas del dominio" debajo de la línea |
| `docs/ARCHITECTURE.md` | Alcance del dominio, entidades, los patrones anteriores aplicados a este módulo, tabla de Ops, ejemplo de raíz de composición |
| `docs/PLAN.md` | Presente mientras hay un cambio en curso — autocontenido (nunca borrar este archivo) |
| `docs/diagrams/database.md` | ERD de Mermaid |
| `README.md` | Inicio rápido, tabla de Ops, archivos clave |

---

## Notas específicas del dominio (editar por módulo — nada por encima de esta línea)

`appointment_booking` es el módulo más complejo del lote: 5 entidades propias, una FSM de estado
de reserva aplicada en código, instantáneas (snapshotting), concurrencia optimista y un algoritmo
de disponibilidad consciente de la zona horaria. A partir de este pase de solo documentación (17-07-2026), su código **aún no**
ha adoptado el arnés — ver `docs/PLAN.md` para la migración completa. Hechos específicos de este módulo:

- **FSM (`fsm.go`)**: Las transiciones de `Reservation.Status` (`PENDING → CONFIRMED/CANCELLED/EXPIRED/
  RESCHEDULED`, `CONFIRMED → CANCELLED/COMPLETED/NO_SHOW/RESCHEDULED`) se aplican enteramente en código
  vía `Transition(current, event string) (string, error)` — no hay tabla de BD `reservation_status`.
  `RESCHEDULED` es un estado terminal distinto (no `CANCELLED`) para preservar la claridad de la traza de auditoría.
  `EXPIRED` es activado exclusivamente por un programador externo llamando a
  `expire_pending_reservations` — el módulo nunca ejecuta goroutines en segundo plano. `fsm.go` en sí usa un
  `map[string]map[string]string` para la tabla de transiciones; esto precede a la rectificación del arnés
  y es una violación estrecha y preexistente de la regla de límite de stdlib "sin mapas" no abordada por
  `docs/PLAN.md` (ese plan está acotado a puertos de infraestructura — db/transporte/ids/vista/eventos/ddl —
  no a reescribir lógica de negocio ya correcta y probada). Señalado aquí para visibilidad; un
  plan futuro e independiente puede reemplazarlo con un escaneo lineal si esta regla se aplica
  retroactivamente.
- **Instantáneas (Snapshotting)**: `Reservation` congela `StaffIDSnapshot`, `ServiceIDSnapshot`,
  `DurationMinSnapshot`, `PriceSnapshot` y `CurrencySnapshot` al momento de la creación. Cambios posteriores
  a `EmployeeServiceConfig`, personal o datos de catálogo nunca alteran retroactivamente una reserva existente
  — esta es la fuente de verdad financiera/de auditoría de lo que realmente se reservó.
- **Nombres de columna irregulares — no "corregir" estos**: las columnas de BD son `staff_idsnapshot` y
  `service_idsnapshot` (sin guión bajo entre `id` y `snapshot` — una peculiaridad histórica de un convertidor de
  snake_case antiguo, ya en vivo en producción). La Etapa 1 de `docs/PLAN.md` preserva estos exactamente
  como `Field.Name` en la migración de `model.Definition`. Renombrarlos a `staff_id_snapshot`/
  `service_id_snapshot` forzaría un renombrado destructivo de columna en cada base de datos desplegada — nunca
  hagas esto como efecto secundario de la migración del arnés.
- **Concurrencia optimista**: `Reservation.Revision` se incrementa en cada actualización de estado.
  `UpdateReservationStatusTx` (y la rama que lleva pago de `ChangeReservationStatus`) aplican
  `WHERE revision = N` antes de escribir — un desacuerdo devuelve `ErrConflict` en lugar de sobreescribir
  silenciosamente un cambio concurrente. Este patrón no está relacionado con la migración del arnés y se mantiene como está.
- **3 interfaces de lector entre módulos que este módulo DEFINE por sí mismo — patrón correcto, se mantiene**:
  `StaffReader{StaffExists}`, `CatalogReader{ServiceExists}`, `DirectoryReader{ClientExists}`. Este es
  el patrón "Cableado entre módulos" de las reglas anteriores, no un puerto de infraestructura autodeclarado —
  son datos de dominio que este módulo necesita de módulos hermanos (staff, item_catalog, directory), y cada
  `*Module` hermano satisface la interfaz estructuralmente sin importar de vuelta
  `appointment_booking`. `item_catalog.Module.ServiceExists` existe únicamente para satisfacer el
  `CatalogReader` de este módulo. Solo la **cuarta** interfaz que este módulo declara hoy — `EventPublisher` — es
  la violación real (duplica `events.Publisher`); ver Etapa 2 de `docs/PLAN.md`.
- **Shim `internal/tinytime` — AÚN NO corregido.** A partir de este pase de solo documentación, `internal/tinytime/` aún
  existe en este repositorio y `go.mod` aún tiene `replace github.com/webtyp/time => ./internal/tinytime`
  fijado en `webtyp/time v0.4.0`. El prerrequisito (`Weekday`/`MidnightUTC`/`LocalMinutesToUnixUTC`
  en el `github.com/webtyp/time` real) ha sido publicado en `v0.5.0` (confirmado: estas tres
  funciones existen en el paquete aguas arriba hoy) — eliminar el shim está desbloqueado. Ver Etapa 0 de `docs/PLAN.md`.
- **`webtyp/context` (`tinyctx`) es importado hoy por `service.go` y cada archivo de prueba** — **no**
  está en la lista blanca anterior (la lista de los Cinco Contratos es exhaustiva: "model + router + view +
  events + orm + ddl — y nada más de webtyp/\* en código que no sea de prueba"). Su único uso en este
  módulo es pasar un parámetro no utilizado `ctx *tinyctx.Context` a través de `SchedulingService` puramente
  para reenviarlo a la llamada `EventPublisher.Publish(ctx, ...)` autodeclarada — la Etapa 2 de `docs/PLAN.md`
  elimina ambos juntos.
