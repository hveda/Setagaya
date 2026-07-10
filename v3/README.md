# Setagaya v3

A ground-up, test-driven rebuild of the Setagaya load-testing platform using a
hexagonal (ports-and-adapters) architecture. Built alongside the existing
`setagaya/` module so it can be adopted incrementally (strangler pattern).

See the full plan for the phased roadmap and decisions.

## Architecture

```
cmd/            entrypoints (api; controller and agent to follow)
internal/
  domain/       pure business types + rules, zero I/O imports
  ports/        interfaces the app depends on (Repository, ...)
    fake/       in-memory port implementations for fast tests
    repositorytest/  reusable conformance suite run against every adapter
  app/          use-cases orchestrating domain over ports
  adapters/
    httpapi/    inbound REST adapter (net/http)
    repo/mysql/ MySQL repository adapter
  config/       typed, injected configuration (no globals)
migrations/     embedded, ordered SQL migrations
test/
  dbtest/       testcontainers MySQL helper (integration/e2e only)
  e2e/          full-stack end-to-end tests
```

**Principles**

- The domain imports no infrastructure; use-cases depend on ports, not adapters.
- Every port ships an in-memory fake. Real adapters must pass the *same*
  conformance suite as the fake (`internal/ports/repositorytest`), keeping them
  interchangeable.
- Configuration and collaborators are injected — there is no global state.

## Testing

Three layers, gated in CI at **≥90%** coverage over production packages:

| Lane | Command | Needs Docker |
|------|---------|:---:|
| Unit | `make test` | no |
| Integration (adapter contract tests) | `make integration` | yes |
| End-to-end (real HTTP → services → MySQL) | `make e2e` | yes |
| Coverage gate (all of the above) | `make cover-gate` | yes |

Integration/e2e tests are guarded by the `integration` / `e2e` build tags, so
the default build and unit lane stay free of Docker dependencies.

## Run locally

```bash
# Walking skeleton against the in-memory repository:
go run ./cmd/api                 # serves :8080

curl localhost:8080/healthz      # {"status":"ok"}
curl localhost:8080/api/projects # []
curl localhost:8080/metrics      # Prometheus exposition
```

Configuration is read from `SETAGAYA_*` environment variables (see
`internal/config`), e.g. `SETAGAYA_HTTP_PORT`, `SETAGAYA_DB_DRIVER=mysql`,
`SETAGAYA_DB_DSN=...`.
