# MAY InCluster Driver

MAY InCluster Driver allows MAY to run workloads in cluster.

## Commands

| Action | Command |
|--------|---------|
| test | `make test` |
| e2e | `make test-e2e` (isolated Kind) |
| run local | `make run` |
| CRD/types changed | `make manifests generate` |
| lint | `make lint` / `make lint-fix` |

## Project Layout

- `internal/controller/` — controllers
- `config/` — manifests

## Key Conventions

- Do not edit `**/zz_generated.*`, `config/crd/bases/*`, `config/rbac/role.yaml`, `PROJECT` — regenerate with `make manifests` / `make generate`.
- Keep `// +kubebuilder:scaffold:*` markers.
- Do not move files.

## Gotchas

- `Host` type is defined in `../../may`.
- This driver is for development only.
