---
PLAN: "refactor!: build destinations — web/browser.go and ui/browser.go with //go:build browser"
EXECUTOR: jules
REVIEWER: none
STATUS: running
SESSION: 3516746636369494847
---

> Read AGENTS.md at the repo root before any change.

# appointment_booking: build destinations migration (`web/browser.go`, `ui/browser.go`)

Part of [BUILD_DESTINATIONS_MASTER_PLAN.md](https://github.com/webtyp/docs/blob/main/BUILD_DESTINATIONS_MASTER_PLAN.md) (D1, D4, D6, D15):
- The browser entry for the demo page is `web/browser.go` (`//go:build browser`) which compiles into `browser.wasm`.
- `ui/browser.go` carries `//go:build browser`.
- `svg.go` and `ui/css.go` keep `//go:build !wasm` (D7, read by `sitec` at build time).
- Rename `web/client.go` to `web/browser.go` and replace its build tag `//go:build wasm` with `//go:build browser`.

## Tools

```bash
go install webtyp.com/devflow/cmd/gotest@latest
```

## Environment

```bash
export GOPRIVATE=webtyp.com/*,github.com/veltylabs/*
export GOFLAGS=-mod=mod
```

A PR that does not compile, or whose tests could not run, is a failed delivery. Never pause to ask permission or report progress: when the Delivery check passes, commit, push and open the PR. Stuck for 30 minutes: open the PR anyway listing what fails.

## Stage 1 — Rename `web/client.go` to `web/browser.go`

1. Rename `web/client.go` to `web/browser.go` (`git mv web/client.go web/browser.go`).
2. In `web/browser.go`, change the first line:
   ```go
   //go:build browser
   ```
3. In `ui/browser.go`, ensure the file carries:
   ```go
   //go:build browser
   ```
   at the top before the package declaration.

## Stage 2 — Update tests and docs

1. Check if any test or doc mentions `web/client.go` or `client.wasm`; update to `web/browser.go` and `browser.wasm`.
2. Run `gotest ./...` and ensure all tests pass.

## Deletions and acceptance criteria

- `grep -rn "web/client\.go" --include=*.go .` returns empty.
- `test -f web/browser.go && test ! -f web/client.go` succeeds.
- `gotest ./...` passes with zero failures.

## Stages

| # | Files | Goal |
|---|---|---|
| 1 | `web/browser.go`, `ui/browser.go` | Rename to `web/browser.go` with `//go:build browser`, add tag to `ui/browser.go` |
| 2 | tests, docs | Verify tests pass with `gotest ./...` |
