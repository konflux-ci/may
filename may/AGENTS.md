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
  - `provisioner/dynamichost/` — DynamicHost and DynamicHostAutoScaler controllers
  - `provisioner/runner/` — Runner controller
  - `provisioner/runnerhook/` — RunnerHook controller
  - `provisioner/statichost/` — StaticHost controller
  - `provisioner/constants/` — shared provisioner constants
- `internal/webhook/` — webhooks
- `config/` — manifests

## Key Conventions

- Do not edit `**/zz_generated.*`, `config/crd/bases/*`, `config/rbac/role.yaml`, `PROJECT` — regenerate with `make manifests` / `make generate`.
- Keep `// +kubebuilder:scaffold:*` markers.
- Do not move or rename files referenced by kubebuilder scaffolding markers (`// +kubebuilder:scaffold:*`) or generated manifests — regenerate with `make manifests` / `make generate` instead.

## Gotchas

- Host *implementation* is in `drivers/*`.
- E2E tests uses `drivers/incluster`.
