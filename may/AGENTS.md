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

- **gomega `MatchError()`** — use `MatchError()` for error assertions instead of comparing `err.Error()` strings or using bare `Equal(someErr)`. `MatchError` accepts a matcher, error value, or string and produces clear failure output. See `internal/controller/scheduler/suite_test.go` (`MatchError(context.Canceled)`) and `internal/controller/claimer/claimer_controller_test.go` (`MatchError(apierrors.IsNotFound, "IsNotFound")`) for exemplars.
- **Minimal test infrastructure** — use the simplest test setup that covers the behaviour under test. When tests only need cached client reads with field indexers, prefer a standalone `cache.Cache` + `client.New` with `CacheOptions.Reader` over a full `ctrl.NewManager`. See `internal/controller/scheduler/suite_test.go` for the cache-only pattern. Suites that do not need cached reads at all can use a direct `client.New` without a cache (see `internal/controller/claimer/suite_test.go`).
- **Halt-vs-propagate error handling** — test assertions on reconciler return values must reflect the production controller's error semantics. Application-level validation errors (e.g., wrong flavor, unclaimable claim) cause the controller to *halt* (`return ctrl.Result{}, nil` — no requeue), while transient infrastructure errors (e.g., API failures) are *propagated* (`return ctrl.Result{}, err` — requeue). Tests should verify the correct category: assert `Expect(err).ShouldNot(HaveOccurred())` for halt cases and `Expect(err).Should(HaveOccurred())` (or `MatchError()`) for propagation cases.

## Gotchas

- Host *implementation* is in `drivers/*`.
- E2E tests uses `drivers/incluster`.
