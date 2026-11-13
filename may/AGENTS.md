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

## Test Conventions

- **Ginkgo `By()` annotations** — every logical step inside an `It()` block must be annotated with `By("description of step")` to improve test output readability. See `internal/controller/provisioner/statichost_controller_test.go` and `internal/controller/provisioner/dynamichost_controller_test.go` for exemplars.
- **Test package** — test files use `package provisioner` (same package), not `package provisioner_test`.
- **Assertion descriptions** — each `Expect` assertion's failure message or surrounding `By()` must accurately describe what is being checked. Reviewers should verify assertion messages match the actual check and are not copy-pasted from a different test case.
- **Test command** — always use `make test` to run tests, not `go test ./...`.

## Gotchas

- Host *implementation* is in `drivers/*`.
- E2E tests uses `drivers/incluster`.
