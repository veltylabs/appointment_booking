# AGENTS.md

<!-- webtyp:base — copied from devskills skill agents-md; do not edit here -->
## Ecosystem rules — read before any change

`docs/PLAN.md` is the spec: no design judgment; anything unclear is a question in the PR, not a guess.

- Public API: typed (no `any`/`interface{}`), explicit, sequential, minimal; no generics (`[T any]`) or `reflect` without benchmarked allocation/performance justification and owner approval; never export a symbol only a test uses.
- One intent, one path: the old path is deleted in the same change. No silent fallback, `TODO`, commented code or stub.
- Fail at compile time, else a loud diagnostic — never silently. Zero values deny.
- A defect in a dependency is reported, never wrapped or patched here.
- No string literals in logic: named constants. The same logic never lives in two repos: import the package the plan names; if you find an existing one the plan does not name that already does what you are about to write, say so in the PR instead of writing it.
- `cmd/*/main.go`: parse flags, inject, print/exit only. stdout = data, stderr = logs, exit codes are the contract.
- WASM code: `webtyp/fmt`, `webtyp/time`, `webtyp/json` instead of stdlib (imported without alias as `fmt`, `time`, `json` — never `tinytime`); no `map`; `dom.Element` embedded by value.
- `css.go`, `svg.go`, `js.go`, `html.go` carry `//go:build !wasm`; no `ssr.go`, no `front.go`.
- Build tags: `wasm`/`!wasm` only when the API does not exist on that platform; `browser`/`!browser` when an app's file belongs to another binary; a library carries no destination tag (the linker drops what a binary never calls). One main per folder, no destination tag: `web/browser/main.go` → `web/public/browser.wasm`, `web/server/main.go` → `web/server/server`, `web/edge/main.go` → `web/edge/edge.wasm`.
- Code and keys in English; UI text is `lang.Text`, translations live in `lang.json`. Comments and docs: English in `webtyp/*`; in `veltylabs/*` **every README, `docs/*` file, diagram and code comment is in Spanish**, written for a reader with little technical background, and each exported symbol's comment shows a short usage example.
- A translation key is reused, never duplicated as a near-identical phrase (`"not a valid integer"` + `"not a valid decimal number"` repeats `"not a valid"` three ways instead of composing it once) — every library ships to WASM/TinyGo, so each unreused phrase is dictionary rows AND binary bytes, in every consumer, forever. A token that doesn't need translating (a kind/type name like `int`) is passed as data (a variable), never as a string literal — a literal becomes a dictionary key whether or not it needed one.
- Tests cover behaviour and domain rules, never the look (CSS, colours, markup order, class strings). In `tests/` (external package), real stack with fakes only at the edge; run `gotest`.
- Docs (`README.md`, `docs/*`) updated with the API; permanent docs never link `docs/PLAN.md`.
- Before the PR: `git diff --stat main` lists every file of the plan's stages, and nothing the plan did not order is deleted.
<!-- /webtyp:base -->

## This repo — a veltylabs domain module

A domain module: business rules for one concern, published as its own Go library and assembled by an app under a
transport, a database and a renderer it never names. These rules apply to every `veltylabs/modules/*` repo.

- **Tables:** each table's `model.Definition` declares `Allow` (`model.Create | model.Read | …`; zero = not offered).
  `definitionc` generates `Mount<Entity>(reg, store, <Entity>Rules{…})`; `ops.go` calls it once per offered table. The module
  never writes persistence: id, `tenant_id`, timestamps, the transaction, partial update by `fields`, «not found», paging
  and schema checks are done by `crud` with the store (`orm.TenantStore(db, tenantID, ids)`, or `orm.GlobalStore` for a
  table without `tenant_id`). Domain rules live in `rules.go` as per-record hooks (`Create`, `Update(old, next, fields)`,
  `Delete`, `Search` with AND-only conditions, `Committed` for events after commit).
- **A resource that lives in another service** (not a table of this module) gets the same `Mount<Entity>` with a
  `crud.Store` the module writes over that service's API (Repository).
- **An action with its own permission** (approve, settle, receive stock) is a record **created** in its own table, or a
  **read model** (no `DB:` marks, `Allow: model.Read`) mounted with `crud.Registry.Read` — never a named operation.
- **Imports:** webtyp contracts only (`model`, `crud`, `orm`, `storage`, `ddl`, `view`, `events`, `fmt`, `time`, `form/input`,
  and the pure packages the plan names, e.g. `stock`, `money`, `network`). Never a concrete backend, transport, id
  generator, encoder or renderer (`webtyp.com/layout`), in any file, tests included.
- **Modules never import each other.** What a module needs from another it declares as a narrow interface with plain
  types in its own package; the app wires the real module, tests and the demo wire a fake. Readers other modules call
  keep their signatures and read through the module's store.
- **Layout of the repo:** `migrate/` (`Migrate(conn ddl.Execer, c ddl.Compiler) error`, run by the app, never at start);
  `ui/` (`Screens(...) []view.Screen`, contracts from `view` only); `seed/` (`Load(...)` through the module's own methods,
  and `Demo() (view.Demo, error)` in `seed/demo.go` with `//go:build wasm` — the webtyp daemon draws it; there is no `web/`).
- **Tests:** `tests/` (external package), `orm.New(mem.New())`, operations through `crud.LoopbackActingAs("u1", mod)`;
  a tenant-scoped module carries tenant-isolation tests.

### Domain notes of this module

- **Zero cross-module imports in any package** (root, `migrate/`, `ui/`, `seed/`, `module/`, `tests/`). All contracts required from other modules live in `interfaces.go` at the root of the module using plain types and `webtyp.com/time`.
- **`webtyp.com/time` is imported without alias (`time`)**: never `tinytime` or `tintime`.
- **`docs/PLAN_BOOKING_FORMS.md` is a separate parked plan**: ignore it, do not edit or delete it.
