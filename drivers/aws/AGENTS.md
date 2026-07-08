# MAY AWS Driver

MAY AWS Driver manages AWS Instances for MAY.

The driver intended to perform all kind of operations with EC2 instances in Amazon cloud. It should support two modes or operations - static (long-running instances) and dynamic (one-time instances). This includes, but not limited to - on demand EC2 instances creation, accessibility verification, and disposal.

## Commands

| Action | Command |
|--------|---------|
| test | `make test` |
| e2e | `make test-e2e` (isolated Kind) |
| run local | `make run` |
| CRD/types changed | `make manifests generate` |
| lint | `make lint` / `make lint-fix` |

## Project Layout

- `config/` — manifests
- `internal/controller/` — controllers
- `internal/config/` — internal configuration structs and parsers
- `internal/client/` — EC2 client factories (token and service account auth)

## Key Conventions

- Do not edit `**/zz_generated.*`, `config/crd/bases/*`, `config/rbac/role.yaml`, `PROJECT` — regenerate with `make manifests` / `make generate`.
- Keep `// +kubebuilder:scaffold:*` markers.
- Do not move files.

## Gotchas

- `Host` type is defined in `../../may`.
