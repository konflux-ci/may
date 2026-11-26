---
name: go-coding-standards
description: >
  Use when writing or reviewing Go code in MAY to apply the project's standards
  for simplicity, readability, performance, and maintainability.
---

# Go Coding Standards

Team preferences for writing Go in MAY. Apply them to every code change.

## Code Style

### 1. Fewer Lines, Better Readability

Eliminate unnecessary variables and boilerplate.
Avoid comments on unexported helpers whose name already explains intent.

```go
// Prefer
return ctrl.Result{RequeueAfter: time.Minute}, nil

// Avoid
result := ctrl.Result{RequeueAfter: time.Minute}
return result, nil
```

### 2. DRY — Extract Repeated Logic into Helpers

```go
func labelSelector(key, value string) client.MatchingLabels {
    return client.MatchingLabels{key: value}
}
```

### 3. Modularize — Small, Reusable Pieces

Keep one controller or cohesive concern per file. Separate reconciliation flow
from reusable helpers.

```go
// statichost_controller.go — StaticHost reconciliation
// dynamichost_controller.go — DynamicHost reconciliation
// Each file owns one controller; shared helpers stay with the package.
```

Keep controller tests next to the controller in `*_test.go`; shared envtest
setup belongs in the package's `suite_test.go`.

### 4. Single Responsibility — One Function, One Task

```go
func (r *StaticHostReconciler) ensureHostIsReady(ctx context.Context, h maykonfluxcidevv1alpha1.StaticHost) error {
    return errors.Join(
        r.ensureHostRunnersExists(ctx, &h),
        r.ensureExtraHostRunnersAreDeleted(ctx, &h),
    )
}
```

The helper performs one step; the reconciler decides when to call it.

### 5. Return Early — Guard Clauses Over Nesting

Fail fast at the top; keep the happy path at the lowest indentation level.

```go
func (r *RunnerReconciler) finalize(ctx context.Context, u maykonfluxcidevv1alpha1.Runner) error {
    if runner.IsNotReadyWithReason(u, runner.ConditionReasonCleaningFailed) {
        return nil
    }
    if runner.SetNotReadyCleaning(&u) {
        return r.Status().Update(ctx, &u)
    }
    // ... happy path continues at base indentation
}
```

## Performance and Memory

Reconcile loops run frequently under load — these patterns matter.

| Guideline | Detail |
|-----------|--------|
| **Avoid allocations in hot paths** | Don't create slices/maps inside tight loops; reuse or lift outside. |
| **Pre-allocate when size is known** | `make([]corev1.Pod, 0, len(items))` instead of `var pods []corev1.Pod`. |
| **Pointer receivers for large structs** | `RunnerReconciler` methods use pointer receivers to avoid copying. |
| **Respect `ctx.Done()`** | Check context cancellation in long-running cloud provider calls. |
| **Prevent goroutine leaks** | Always pair goroutines with context or done-channel cleanup. |
| **Bounded retries** | Never retry indefinitely. Use `RequeueAfter` with backoff or a max-attempt cap. |

## Quick Reference

| Principle | Do | Don't |
|-----------|----|-------|
| Brevity | `return ctrl.Result{}, err` | Assign to temp var, then return |
| DRY | Helper for repeated label selectors | Copy-paste the same 3 lines |
| Modularity | One controller per file | 500-line god function |
| Single responsibility | Reconciler dispatches, helper executes | Reconciler does everything inline |
| Guard clauses | `if err != nil { return }` at top | Deeply nested if/else chains |
| Allocations | `make([]corev1.Pod, 0, count)` | `var pods []corev1.Pod` inside a loop |
| Context | `select { case <-ctx.Done(): }` | Blocking call with no cancellation |
| Retries | `RequeueAfter: backoff` | `for { retry() }` |

For Go-specific pitfalls to avoid (nil interfaces, nil map writes, unsafe type assertions, etc.),
see the Go-Specific Pitfalls table in `skills/testing-standards.md`.

## Review Checklist

Before submitting code, verify:

- [ ] No unnecessary comments on unexported functions with clear names
- [ ] No duplicated logic that should be a helper
- [ ] Functions each have a single, clear responsibility
- [ ] Guard clauses used instead of deep nesting
- [ ] Slices/maps pre-allocated when capacity is known
- [ ] Context cancellation respected in cloud/network calls
- [ ] Retry loops are bounded (max attempts or `RequeueAfter`)
- [ ] Existing tests updated and new behavior covered by new tests
- [ ] No references to specific Jira tasks in comments that are not `TODO`
- [ ] `make -C may test` and `make -C drivers/<name> test` pass
