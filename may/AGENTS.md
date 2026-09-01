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

## Controller Reconciliation Conventions

### Return after mutation

When a reconciler creates, updates, or patches a Kubernetes resource
(including adding finalizers), it should return and let the next
reconciliation event handle subsequent logic. Continuing after a
mutation risks operating on stale state because the in-memory object
no longer matches what the API server persisted. Only continue past a
mutation if there is an explicit, documented justification.

This pattern is used consistently in `DynamicHostReconciler`,
`RunnerReconciler`, and `StaticHostReconciler`. Treat deviations as
high-severity review findings, not style nits.

### Cross-controller consistency

Changes to reconciliation control flow in one controller should be
evaluated against the patterns used in `DynamicHostReconciler`,
`RunnerReconciler`, and `StaticHostReconciler` for consistency. A
control-flow change in one controller that diverges from the others is
an architectural concern, not a cosmetic inconsistency.

### Owner references

Created sub-resources (e.g., Runners, Pods) must have controller owner
references set via `controllerutil.SetControllerReference` with the
`WithBlockOwnerDeletion(true)` option. This ensures proper cascading
deletion when the parent resource is removed.

## Gotchas

- Host *implementation* is in `drivers/*`.
- E2E tests uses `drivers/incluster`.
