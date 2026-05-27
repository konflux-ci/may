# MAY

## Commands

| Action | Command |
|--------|---------|
| test | `make test` |
| e2e | `make test-e2e` (isolated Kind) |
| e2e smoke | `make test-e2e-smoke` (isolated Kind) |
| run local | `make run` |
| CRD/types changed | `make manifests generate` |
| lint | `make lint` / `make lint-fix` |

## Project Layout

- `api/` — types
- `internal/controller/` — controllers
- `internal/webhook/` — webhooks
- `config/` — manifests

## Key Conventions

- Do not edit `**/zz_generated.*`, `config/crd/bases/*`, `config/rbac/role.yaml`, `PROJECT` — regenerate with `make manifests` / `make generate`.
- Keep `// +kubebuilder:scaffold:*` markers.
- Do not move files.

## Gotchas

- Host *implementation* is in `drivers/*`.
- E2E tests uses `drivers/incluster`.
