---
name: testing-standards
description: >
  Use when writing or reviewing Ginkgo/Gomega tests in MAY to apply the project's
  conventions for structure, style, organization, and coverage.
---

# Testing Standards

Ginkgo/Gomega conventions for MAY. Apply them to every test change. Tests are
required for every behavior change; this file covers how to write them.

## Required Workflow

- Run `make -C may test` for core changes and `make -C drivers/<name> test` for driver changes. When a change spans modules, run tests in each affected module.
- Update existing specs when behavior changes. Add specs for every new behavior, branch, and error path.
- Run the tests yourself; do not hand the check back to the user.
- Do not report work as complete until the relevant tests pass.
- The module's `make test` writes `cover.out` in the module. For the core module, `make -C may report-coverage` writes `cover.html`.
- Run the module's e2e target when the change affects cluster behavior and the environment is available.
- If `controller-gen` fails on a parent `go.work`, prefix the command with `GOWORK=off`.

## Style Rules

### 1. `When()` for New Scenario Blocks

Both `When()` and `Context()` appear in MAY. Prefer `When()` for new scenario
blocks because it reads naturally with `It()`; do not rewrite existing
`Context()` blocks unless you are already editing that spec.

```go
When("the Runner has no provisioning hooks", func() { ... }) // Do
Context("when the Runner has no provisioning hooks", func() { ... }) // Prefer not to add
```

### 2. `Should()`/`ShouldNot()` for New Assertions

```go
Expect(err).ShouldNot(HaveOccurred())
Expect(count).Should(Equal(3))
```

Both `Should()` and `To()` appear in MAY. Prefer `Should()`/`ShouldNot()` in new
tests; convert existing `To()`/`NotTo()` calls only when editing that spec.

### 3. Sentence Coherence — Describe/When/It Must Read as a Sentence

```go
Describe("findBestRunner", func() {
    When("no Runner has the requested flavor", func() {
        It("should return ErrNoAvailableRunnerForFlavor", func() { ... })
    })
})
// Reads: "findBestRunner, when no Runner has the requested flavor, should return ErrNoAvailableRunnerForFlavor"
```

### 4. Separate `When()` Blocks for Scenarios

Group related scenarios (happy/error/edge) into distinct `When()` blocks.

```go
Describe("findBestRunner", func() {
    When("no free Runners exist", func() { ... })
    When("the flavor is missing from every Runner", func() { ... })
    When("a matching Runner exists", func() { ... })
})
```

### 5. Inline With Forethought

```go
// Single-return — inline freely
Expect(claim.IsClaimed(*c)).Should(BeTrue())

// (value, error) — must capture both, can't inline
r, err := s.Schedule(ctx, c)
Expect(err).ShouldNot(HaveOccurred())
Expect(r).ShouldNot(BeNil())

// Error-only — be cautious: inlining panics if the call unexpectedly returns a nil error
err := k8sClient.Delete(ctx, pod)
Expect(err).ShouldNot(HaveOccurred())
```

Also consider line length: if inlining creates an overly long line due to large
inputs, split the input setup from the call.

```go
// Long input — split for readability
req := reconcile.Request{NamespacedName: client.ObjectKeyFromObject(pod)}
res, err := reconciler.Reconcile(ctx, req)
Expect(err).ShouldNot(HaveOccurred())
Expect(res).Should(Equal(reconcile.Result{}))
```

### 6. `DescribeTable` for Repeated Patterns

When test cases share identical `Expect()` structure, use `DescribeTable`.

```go
DescribeTable("excluded flavors",
    func(flavor string, excluded bool) {
        Expect(pod.IsExcludedFlavor(flavor)).Should(Equal(excluded))
    },
    Entry("localhost", "localhost", true),
    Entry("local", "local", true),
    Entry("custom AWS flavor", "aws-linux-amd64", false),
)
```

### 7. Test Your Logic, Not Stdlib

Test what *your* function does, not that `context.WithCancel` or `json.Marshal` work.

```go
// Do — verify MAY's behavior
When("the namespace is not a tenant", func() {
    It("should not create a Claim", func() { ... })
})
// Don't — verify stdlib behavior
Expect(ctx.Err()).Should(Equal(context.Canceled))
```

## When to Use `SpecContext`

Ginkgo passes a `SpecContext` to any setup, subject, or cleanup node that declares one. It satisfies `context.Context` and is canceled when the spec times out, the suite is interrupted, or the node's own timeout expires. Prefer it over `context.Background()` whenever the test calls cancellable code.

Use `SpecContext` when:

- The node makes Kubernetes, HTTP, SSH, or OTP calls. Cancellation lets the call return instead of leaking a goroutine.
- The spec has `SpecTimeout`, or the node has `NodeTimeout`. Pass the same `ctx` to the work and to `Eventually(...).WithContext(ctx)` so polling and the underlying call stop together.
- A cleanup node calls cancellable code. Declare a fresh `SpecContext` in `DeferCleanup` or `AfterEach`; do not capture the `BeforeEach` context because it is already canceled when cleanup runs.
- You want Gomega progress reports from a running `Eventually`; `SpecContext` carries Ginkgo's extension point.

```go
It("waits for the Claim", func(ctx SpecContext) {
    Eventually(func() error {
        return k8sClient.Get(ctx, key, fetched)
    }).WithContext(ctx).Should(Succeed())
}, SpecTimeout(10*time.Second))
```

`context.Context` still works; Ginkgo accepts either. Use plain `context.Context` for helpers shared outside Ginkgo that do not need Ginkgo's cancellation or progress reporting.


## DRY Test Setup

Consolidate shared variables into `BeforeEach`. Extract helpers when `BeforeEach`
isn't flexible enough. Never duplicate setup across `It()` blocks.

```go
BeforeEach(func(ctx SpecContext) {
    reconciler = &ClaimerController{Client: k8sClient, Scheme: k8sClient.Scheme()}
    tenantNS = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{GenerateName: "tenant-"}}
    Expect(k8sClient.Create(ctx, tenantNS)).Should(Succeed())
})
```

## Go-Specific Pitfalls

| Pitfall | What Happens | Prevention |
|---------|-------------|------------|
| Nil interface comparison | `var e error; e == nil` is true, but an interface holding a nil pointer is not nil | Compare concrete types or use `Expect().Should(BeNil())` |
| Nil map write | `var m map[string]string; m["k"] = "v"` panics | Initialize with `make()` or a literal |
| Closed channel send | `close(ch); ch <- v` panics | Guard sends with select or sync |
| JSON unknown fields | `json.Unmarshal` silently ignores unknown keys | Use `DisallowUnknownFields()` when strictness matters |
| Race conditions | Concurrent map/slice access passes locally, fails under `-race` | Enable `-race` manually for concurrency changes; `make test` does not |
| Unsafe type assertion | `v := i.(MyType)` panics if wrong | Use `v, ok := i.(MyType)` |

Keep regression tests for these cases. Do not remove them once the code
stabilizes; they are the evidence that the pitfall stays fixed.

## Quick Reference

| Do | Don't |
|----|-------|
| `When("the Claim is not pending", ...)` | `Context("when ...", ...)` in new tests |
| `Expect(x).Should(Equal(y))` | `Expect(x).To(Equal(y))` in new tests |
| `func(ctx SpecContext)` for cancellable nodes | `context.Background()` inside a timeout-capable spec |
| `Eventually(...).WithContext(ctx)` | `Eventually(...)` without `ctx` when the node can time out |
| Sentence-readable Describe/When/It | `It("test case 1", ...)` |
| `DescribeTable` for repeated patterns | Copy-paste `It` blocks with one param changed |
| `BeforeEach` for shared setup | Same 5-line setup in every `It` |
| Test MAY's behavior | Test that `context.WithCancel` works |
| Capture `(value, error)` returns | Inline multi-return calls into `Expect()` |
| Think before inlining error-only returns | Blindly inline everything |

## Test Organization

- **Suite files**: Packages bootstrap Ginkgo in a `*suite_test.go`. Controller packages use envtest where the test needs a real API server; pure helper packages can bootstrap Ginkgo in a normal `_test.go`.
- **Helpers and mocks**: Reusable test utilities live next to the package. E2E helpers live in `may/test/e2e/utils_test.go`.
- **Controller tests**: Use the package's `*suite_test.go` and envtest for reconciliation behavior that needs an API server. Full cluster flows live in `may/test/e2e/`.
- **Coverage**: The module's `make test` writes `cover.out`. New behavior needs a spec; changed behavior needs its existing specs updated. Do not remove regression tests to make a change pass.
- **Run tests**: `make -C may test` for the core module and `make -C drivers/<name> test` for a driver. Use `make -C may test-e2e` or `make -C drivers/<name> test-e2e` for cluster behavior; AWS uses `make -C drivers/aws test-e2e-aws`.
