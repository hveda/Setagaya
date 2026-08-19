# AGENTS.md

Go 1.26 load-testing platform (hexagonal architecture). Module: `github.com/heridotlife/honryu`.

## Commands

- `make test` — unit tests, no Docker needed (`go test -race -count=1 ./...`)
- `make integration` — adapter contract tests, needs Docker (`-tags=integration`)
- `make e2e` — full-stack tests, needs Docker (`-tags=e2e`)
- `make cover-gate` — ≥90% coverage gate over production packages (needs Docker)
- `make lint` — golangci-lint (CI pins v2.12.2; runs with `integration`+`e2e` build tags)
- `make engine` — real-engine tests (`-tags=engine`), needs `bzt` on PATH, 25m timeout
- Single package: `go test -race -count=1 ./internal/app/lifecycleapp/...`
- Single integration package: `go test -p 1 -count=1 -tags=integration ./internal/adapters/repo/mysql/...`

CI order (must all pass): gofmt clean → `go vet` → `golangci-lint run` → unit race tests; separate coverage-gate job. CI sets `TESTCONTAINERS_RYUK_DISABLED=true` — Ryuk is required to be disabled there; locally leave default unless it interferes.

## Architecture rules (enforced, not just convention)

- Hexagonal: `internal/domain` is pure (zero I/O imports), use-cases in `internal/app` depend only on `internal/ports` interfaces, adapters live in `internal/adapters/*`. No global state — config and collaborators are injected via `internal/config` (`HONRYU_*` env vars only; everything has local-dev defaults so `go run ./cmd/api` just works).
- Every port has an in-memory fake in `internal/ports/fake` and a reusable conformance suite in `internal/ports/<port>test`. A real adapter must pass the *same* conformance suite as the fake. Name new suites `<port>test` — `scripts/coverage.sh` excludes them from the coverage gate by that naming pattern (also excluded: `/test/dbtest`, `/test/e2e`, `/migrations`, `/internal/ports` itself).
- Docker-dependent tests are gated behind `integration`/`e2e` build tags (see `test/dbtest`); the unit lane must stay Docker-free. Container-backed test runs use `-p 1`.

## Migrations

- `migrations/00NN_name.sql`, sequential numbering, embedded via `migrations/embed.go`, auto-applied on startup when `HONRYU_DB_DRIVER=mysql`. Add new schema changes as the next number — never edit an applied migration.

## web/ (operator SPA)

- Built with **bun** (`bun run build`), embedded into the Go binary via `go:embed` (`web/embed.go`). Tests: `vitest run` from `web/`.
- `web/dist/.gitkeep` is committed intentionally so the embed compiles in fresh checkouts; a real build deletes it locally — do not "fix" or commit that deletion.
- **`bun run layout-check`** (from `web/`) asserts layout invariants a real browser can see and `vitest` cannot: `vitest` runs in jsdom, which computes no geometry. It caught nothing when the mobile nav drawer was hidden with `invisible` but left in flow, still reserving its height — `<nav>` measured 370px against `h-16`'s 64px, putting a ~306px empty band above every page's content. Checks nav height, `<main>`'s offset, horizontal overflow, and (at mobile widths) that the drawer stays absolutely positioned, opens, doesn't shift `<main>`, and closes on an outside tap — across 3 viewports × 5 routes. Exits non-zero, so it works as a gate.
  - Point it anywhere: `LAYOUT_CHECK_URL=https://honryu.pve.heri.life bun run layout-check` (default `http://localhost:4173`, i.e. `bun run preview`). Add `-- --screenshots` to dump PNGs to `web/.layout-check/`.
  - Needs a Chromium binary; Playwright's cache is auto-discovered, else set `CHROMIUM_PATH`. Provision with `bunx playwright install chromium`. Deliberately **not** in CI — it needs a running SPA, and CI has no browser or deployment to point at.

## Style / lint exceptions (do not undo)

- goimports with local prefix `github.com/heridotlife/honryu`; gofmt must be clean (CI fails on unformatted files).
- `revive` exported rule is ON: exported symbols need doc comments (exemptions only in `internal/ports/(fake|repositorytest|objectstoretest)/`).
- `ExecutionPlan`/`ExecutionCollection` (domain/execution) and `EngineName`/`EngineLabels` (domain/engine) stutter on purpose — renaming collides with existing helpers; the `stutters` lint exclusions in `.golangci.yml` document this.
- Test files are exempt from `errcheck`/`unparam`.
- YAML/Markdown formatted with Prettier, linted with yamllint: `npm run format` / `npm run check` (root `package.json` is dev-tools only).

## Cortex workflow (`.cortex/`, gitignored)

Phased development driven by `.cortex/<date>-<phase>/` containing `spec.md` → `plan.md` → `tasks.md`. Task numbers are global across phases (phase 10 starts at 112). Before implementing a phase, read its `tasks.md`; work tasks in dependency order. Check the latest phase directory for current context.

## Git / release flow

- Branches: `feat/*` → `develop` → `main`. Pushing to `develop` auto-opens/updates a draft PR to `main` — don't merge `feat/*` straight to `main`.
- **Closing a phase: `make phase-merge PHASE="phase N slug"`**, not a manual `git merge`. It runs the full bar (gofmt/vet/lint/test/coverage gate) and only merges `feat/*` into `develop` on success — the merge is the check, so a phase cannot land below the coverage constraint the way phases 10-12 did. Never pushes; prints the `git push` to run next. `PHASE_MERGE_DRY_RUN=1` runs every check and stops before merging.
- Conventional commits with scope, e.g. `feat(lifecycleapp): mint a fresh trace context once per Deploy`.
