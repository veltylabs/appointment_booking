# PLAN — extract DDL from `NewRepository` into `migrate/`

Next → [PLAN_BOOKING_FORM_RECORD.md](PLAN_BOOKING_FORM_RECORD.md)

> This plan is dispatched via the CodeJob workflow. See skill: **agents-workflow**.

You are an agent with **no prior context** and you have **only this repository**
(`github.com/veltylabs/appointment_booking`). Everything you need is inline.

## 1. The problem

`NewRepository` creates this module's five tables as a side effect of
construction:

```go
// repository.go — current code
// NewRepository crea un nuevo Repository y migra sus 5 tablas propias cuando el backend
// soporta DDL (no-op contra storage/mem, usado por las pruebas propias de este módulo).
func NewRepository(db *orm.DB, ids model.IDGenerator) (*Repository, error) {
	tables := []model.Model{
		&EmployeeServiceConfig{},
		&WorkCalendarConfig{},
		&WorkCalendarBlock{},
		&WorkCalendarException{},
		&Reservation{},
	}
	if ddlCompiler, ok := db.RawConn().(ddl.Compiler); ok {
		for _, t := range tables {
			if err := ddl.New(db.RawConn(), ddlCompiler).CreateTable(t); err != nil {
				return nil, err
			}
		}
	}
	return &Repository{db: db, ids: ids}, nil
}
```

Two defects follow:

1. **Every application boot runs DDL.** Schema reconciliation is a deploy-time
   step with its own ordering and its own failure mode. Running it on process
   start means a live server can alter a database, and an application has no
   way to construct the module without granting it DDL rights.
2. **`webtyp.com/ddl` is pulled into the WASM build graph.** A consuming app's
   client imports this package for `NewView`/`NewScheduleClient`. Because
   `repository.go` is in the root package and references `ddl`, that dependency
   is in the graph of every build importing the root package, regardless of
   build tags on the consumer's side.

Consumers of this ecosystem run migrations from a dedicated `cmd/migrate`
binary calling `<module>/migrate.Migrate(conn, compiler)` once per module, in a
fixed order. Four sibling modules already follow it. This one does not, so it
cannot be wired without breaking that contract.

## 2. The target shape — copy it exactly

This is the sibling `github.com/veltylabs/device_manager`'s `migrate/migrate.go`,
already published and in production. Reproduce its structure, its package name,
and the spirit of its doc comment:

```go
package migrate

import (
	"webtyp.com/ddl"

	devicemanager "github.com/veltylabs/device_manager"
)

// Migrate reconciles the database schema device_manager owns: Device.
//
// It is deliberately NOT called by New, and deliberately lives in its own
// package rather than a new file in the root package: nothing on a
// consuming app's WASM build path (its view.go, which imports the root
// devicemanager package for devicemanager.NewView) ever imports
// "github.com/veltylabs/device_manager/migrate" — so webtyp.com/ddl never
// enters that build graph, regardless of build tags on the consumer's side.
//
// conn is a ddl.Execer, not an *orm.DB, so a deploy-time transport that can
// only execute DDL satisfies it. An *orm.DB's RawConn() also satisfies it,
// for local/test callers:
//
//	conn, _ := postgres.Open(dsn)
//	compiler, _ := conn.(ddl.Compiler)
//	err := migrate.Migrate(conn, compiler)
func Migrate(conn ddl.Execer, ddlCompiler ddl.Compiler) error {
	return ddl.New(conn, ddlCompiler).CreateTable(&devicemanager.Device{})
}
```

## 3. Design gate

One exported symbol added (`migrate.Migrate`), one side effect removed.

1. **Prior art.** Django and Rails separate `migrate` from application boot into
   a dedicated command; Ent and Atlas expose schema reconciliation as an
   explicit API the application calls when it chooses. None reconciles schema
   inside a constructor. We follow the same split, and differ only in having no
   migration-history table — every statement is `CREATE TABLE IF NOT EXISTS`,
   so the step is idempotent and needs no version ledger.
2. **Novice-name test.** "Migrate the connection with this compiler" reads as a
   sentence. `migrate.Migrate` is the name four sibling modules already use.
3. **Complexity ledger.**
   - Concepts: `+0` — the `migrate` package is an existing ecosystem concept.
   - Files: `+2` (`migrate/migrate.go`, `migrate/migrate_test.go`).
   - Call-site lines: `−13` in `repository.go`, `+1` per deploying app.
   - Ways to do it: `−1` — schema creation had two homes; now one.
   - Net: negative.
4. **Where it belongs.** Its own package, not a file in the root package: the
   point is keeping `webtyp.com/ddl` out of the root package's import graph. A
   `migrate.go` in the root package would fix the boot-time side effect and
   leave the WASM graph defect untouched.
5. **What it deletes.** The `tables` slice and the `if ddlCompiler, ok := ...`
   block in `NewRepository`, and the `webtyp.com/ddl` import from
   `repository.go`.

## 4. Stages

### Stage 1 — create `migrate/migrate.go`

New file `migrate/migrate.go`, `package migrate`. One exported function:

```go
func Migrate(conn ddl.Execer, ddlCompiler ddl.Compiler) error
```

It creates the five tables this module owns, **in this exact order** — the same
order the deleted `tables` slice used:

1. `&appointmentbooking.EmployeeServiceConfig{}`
2. `&appointmentbooking.WorkCalendarConfig{}`
3. `&appointmentbooking.WorkCalendarBlock{}`
4. `&appointmentbooking.WorkCalendarException{}`
5. `&appointmentbooking.Reservation{}`

Return the first error unwrapped; do not wrap it.

There is **no** `ddl.Compiler` type assertion in this function — the caller
passes the compiler explicitly. The optional-capability check existed only
because `NewRepository` received an `*orm.DB` that might be backed by
`storage/mem`; `Migrate` is never called against `storage/mem`.

Import the root package with the alias `appointmentbooking` (matching its
`package` clause).

Write the doc comment on `Migrate` explaining **both** reasons this lives in its
own package (deploy-time step, and keeping `webtyp.com/ddl` out of the WASM
build graph) and showing the three-line caller example. Do not copy
device_manager's comment verbatim — it names the wrong module and the wrong
tables.

**Anti-footgun — the five models are transport-adjacent siblings of many
others.** `model.go` declares ~20 `model.Definition` values, most of which are
transport-only (`*ArgsModel`, `TimeSlotModel`, `ConflictingReservationModel`,
`DayBoundsResultModel`). Only the five listed above are tables. A transport
model has `DB: nil` on every field; a table has a field with
`DB: &model.FieldDB{PK: true}`. Create tables for exactly the five named here
and no others.

### Stage 2 — strip the side effect from `NewRepository`

`repository.go`:

```go
// NewRepository crea un nuevo Repository sobre un *orm.DB ya conectado; el
// esquema se asume existente — ver el subpaquete migrate.
func NewRepository(db *orm.DB, ids model.IDGenerator) (*Repository, error) {
	return &Repository{db: db, ids: ids}, nil
}
```

Delete the `webtyp.com/ddl` import from `repository.go`. Do not leave a
deprecated wrapper, a flag, or a "migrate on first use" fallback — there is no
compatibility shim of any kind.

`NewRepository` now cannot fail, but **keep the `error` return**: it is part of
the published signature and `New` already threads it. Changing it is a separate
breaking change nobody asked for.

**Anti-footgun:** `webtyp.com/ddl` must stay in `go.mod` `require` — the new
`migrate` package uses it. Do not "clean up" that dependency.

### Stage 3 — `migrate/migrate_test.go`

`package migrate_test`. Mirror device_manager's test, adapted to five tables:

```go
type dummyExecer struct{ calls []string }

func (d *dummyExecer) Exec(query string, args ...any) error {
	d.calls = append(d.calls, query)
	return nil
}

type dummyCompiler struct{}

func (d *dummyCompiler) CompileDDL(stmt ddl.Stmt, m model.Model) (string, []any, error) {
	return stmt.Table, nil, nil
}
```

Two test functions:

- `TestMigrate_CreatesFiveTables` — asserts `len(execer.calls) == 5`.
- `TestMigrate_TableOrder` — asserts `execer.calls` equals the five table names
  in the order of stage 1. The dummy compiler returns `stmt.Table`, so each
  recorded call is the table name. The names are the `Name:` fields of
  `EmployeeServiceConfigModel`, `WorkCalendarConfigModel`,
  `WorkCalendarBlockModel`, `WorkCalendarExceptionModel` and
  `ReservationModel` in `model.go` — read them there, do not guess.

**Repo rule:** tests live in `tests/` — **except** this one. `migrate` is a
separate package and its test is that package's own unit test, exactly as
`device_manager/migrate/migrate_test.go` is. Place it in `migrate/`, not in
`tests/`.

### Stage 4 — existing tests must keep passing untouched

`tests/setup_test.go` builds the module over `webtyp.com/storage/mem`, which
does not implement `ddl.Compiler` — the deleted block was already a no-op
there. **No file under `tests/` changes.** If you find yourself editing one,
you have changed behaviour you were not asked to change; stop and re-read
stage 2.

Run `gotest ./...` — everything green.

### Stage 5 — documentation

In `README.md`, the "Composition Root (how to wire this module)" block must
show the migrate call as a separate deploy-time step, before `New`. Add a note
that `New` assumes the schema exists.

In `docs/ARCHITECTURE.md`, correct every statement saying construction creates
tables. Do **not** add a link to any plan file from a permanent document —
`docs/PLAN.md` is deleted when this lands.

## 5. Stages table

| # | Stage | Files | Acceptance |
|---|---|---|---|
| 1 | `migrate` package | `migrate/migrate.go` (new) | creates the five tables in order |
| 2 | Strip `NewRepository` | `repository.go` | `grep -n "ddl" repository.go` → empty |
| 3 | Unit test | `migrate/migrate_test.go` (new) | both tests pass |
| 4 | Regression | — | `gotest ./...` green, **zero** files changed under `tests/` |
| 5 | Docs | `README.md`, `docs/ARCHITECTURE.md` | no doc claims construction migrates |

## 6. Acceptance criteria

- `grep -rn "webtyp.com/ddl" *.go` at the repository root → **empty** (the only
  `ddl` import in the repo is inside `migrate/`).
- `grep -rn "CreateTable" *.go` at the repository root → **empty**.
- `webtyp.com/ddl` still present in `go.mod` `require`.
- `NewRepository` still returns `(*Repository, error)`.
- `gotest ./...` green.
- No file under `tests/` modified.
