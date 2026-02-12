# May E2E Test Cases

This document summarizes the end-to-end test cases for the may project. Tests cover the controller manager, webhooks, and the full Pod → Claim → Runner reservation → scheduling flow.

## 1. Manager & Infrastructure

| Case | Description |
|------|-------------|
| Controller runs | Controller-manager pod is running and healthy |
| Metrics endpoint | Metrics service is reachable and returns HTTP 200 |
| Cert-manager | Webhook certificate Secret is provisioned |
| Webhook CA | Mutating webhook configuration has CA bundle injected |

## 2. Pod Webhook ✅

Tests run in two contexts: a **tenant** namespace (labeled `konflux-ci.dev/type=tenant`) and a **non-tenant** namespace (no such label).
The webhook’s `namespaceSelector` matches only tenant namespaces, so only pods in tenant namespaces can receive the scheduling gate.

| Case | Description |
|------|-------------|
| Gate on flavor (tenant ns) | Pod with flavor annotation (`kueue.konflux-ci.dev/requests-*`) in a tenant namespace gets `may.konflux-ci.dev/scheduling` in `spec.schedulingGates` |
| No gate without flavor (tenant ns) | Pod without flavor annotation in a tenant namespace is left unchanged (no scheduling gate added) |
| No gate with flavor (non-tenant ns) | Pod with valid flavor annotation in a non-tenant namespace is not gated; no scheduling gate is added |
| No gate without annotation (non-tenant ns) | Pod without annotation in a non-tenant namespace is not gated; no scheduling gate is added by the webhook |

## 3. Claimer (Pod → Claim) ✅

Tests run in two contexts: a **tenant** namespace (labeled `konflux-ci.dev/type=tenant`) and a **non-tenant** namespace (no such label). The Claimer only creates Claims for Pods in tenant namespaces (same as the webhook).

| Case | Description |
|------|-------------|
| Claim created (tenant ns) | Pod with flavor annotation in a tenant namespace gets a Claim (same name/namespace, correct `spec.for`, `spec.flavor`, ownerReference to Pod) |
| No Claim without flavor (tenant ns) | Pod without flavor annotation in a tenant namespace does not get a Claim |
| No Claim with flavor (non-tenant ns) | Pod with valid flavor annotation in a non-tenant namespace does not get a Claim |
| No Claim without annotation (non-tenant ns) | Pod without flavor annotation in a non-tenant namespace does not get a Claim |

## 4. Claim / Scheduler (Claim → Runner) ✅

| Case | Description |
|------|-------------|
| Claim scheduled | With a free Runner (matching flavor), Claim gets Claimed and Runner gets `spec.inUseBy` set |
| Claim pending | Claim stays Pending when no Runner exists for the flavor |
| Single Runner, two Claims | One Claim Claimed, one Pending until the first is released |
| Claim deletion | Deleting a Claim releases the Runner (finalizer clears reservation or deletes Runner) |

## 5. Gater (Claim Claimed → Pod ungated) ✅

| Case | Description |
|------|-------------|
| Gate removed when Claimed | When Claim is Claimed, Pod’s scheduling gate is removed |
| Gate remains when Pending | Pod keeps the gate while Claim is Pending |

## 6. Runner Lifecycle (Provisioner) ✅

Tests do **not** involve StaticHost; Runners are created by the test (simulating an external driver). May only observes and uses Runners.

| Case | Description |
|------|-------------|
| Runner with no condition left untouched | A Runner with no Ready condition is left unchanged by may; may waits for a driver to set the Runner to Ready before considering it for scheduling |
| Runner reserved | Runner status reflects reservation when `inUseBy` is set |
| Runner provisioning hooks | Provisioning hook pods run and status is updated |
| Runner cleanup hooks | Cleanup hook pods run and status is updated before Runner is deleted |
| Runner not deleted when cleaning failed | When cleanup (e.g. release/finalizer) fails, the Runner is not deleted; may leaves it for the driver or operator to handle; test cleanup removes finalizers so the Runner can be deleted |

## 7. RunnerBinder (Runner → Pod credentials) ✅

The e2e suite installs the **OTP server** from [konflux-ci/multi-platform-controller](https://github.com/konflux-ci/multi-platform-controller) (see `demo/dependencies/multi-platform-controller/config/otp`) in `BeforeSuite` and uninstalls it in `AfterSuite`. Use `OTP_SERVER_INSTALL_SKIP=true` to skip install when the server is already present.

| Case | Description |
|------|-------------|
| Secret and Pod config | When Runner is bound, Secret/OTPServer entry exists and Pod receives credentials (Secret in Pod namespace with otp-ca, otp, otp-server, host, user-dir); see `binder_test.go` |

## 8. Full Flow (Pod → Claim → Schedule → Run) ✅

| Case | Description |
|------|-------------|
| Happy path | One Runner exists; create Pod with flavor → Claim created → Claimed → gate removed → Pod scheduled |
| Pod completion | When Pod reaches Succeeded/Failed, Claim is deleted and Runner is released |

Both cases are covered in a single test; see `full_flow_test.go`.

## 9. StaticHost (in may) ✅

StaticHost is reconciled only when `status.state` is set and not `Pending`; the driver (e.g. incluster) sets state (e.g. `Ready`). May creates and owns Runners for each index in `spec.runners.instances`.

| Case | Description |
|------|-------------|
| No Runners when status.state is unset | When `status.state` is unset, the controller does not create Runners; it waits for the driver to set state; see `statichost_test.go` |
| No Runners when status.state is Pending | When `status.state` is `Pending`, the controller does not create Runners; it waits for the driver to set Ready; see `statichost_test.go` |
| Runners created when state is Ready | For each index in `spec.runners.instances`, May creates a Runner named `{host}-{index}` with flavor, resources, queue, hooks from spec; labels `may.konflux-ci.dev/host`, `may.konflux-ci.dev/runner-id`, `may.konflux-ci.dev/runner-type=static`; ownerReference to StaticHost; see `statichost_test.go` |
| Finalizer set when reconciled | When StaticHost is reconciled (e.g. state is Ready), May adds the host-controller finalizer; see `statichost_test.go` |
| Status reflects Runner counts | `status.runners.ready` and `status.runners.stopped` are updated from owned Runners (Ready vs Stopped) |
| Status reflects pipelines | `status.pipelines` is updated from owned Runners’ `tekton.dev/pipeline` labels (if any) |
| Runners created when instances increased | When `spec.runners.instances` is increased, May creates additional Runners for new indices (e.g. Runner `{host}-2` when scaling from 2 to 3); see `statichost_test.go` |
| Excess Runners deleted when instances reduced | When `spec.runners.instances` is decreased, Runners with `runner-id` outside `[0..instances-1]` are deleted; see `statichost_test.go` |
| Finalizer and deletion | When StaticHost is deleted, May deletes all Runners with the host label; when no Runners remain, the StaticHost finalizer is removed so the host can be deleted; see `statichost_test.go` |

## 10. DynamicHost (in may) ✅

DynamicHost is reconciled only when `status.state` is set and not `Pending`; the driver (e.g. incluster) sets state (e.g. `Ready`). May creates and owns a single Runner with the same name as the DynamicHost.

| Case | Description |
|------|-------------|
| No Runner when status.state is unset | When `status.state` is unset, the controller does not create a Runner; it waits for the driver to set state; see `dynamichost_test.go` |
| No Runner when status.state is Pending | When `status.state` is `Pending`, the controller does not create a Runner; it waits for the driver to set Ready; see `dynamichost_test.go` |
| Creates Runner when state is Ready | When `status.state` is Ready, May creates a single Runner named like the DynamicHost with flavor, resources, queue, hooks from spec; labels `may.konflux-ci.dev/host`, `may.konflux-ci.dev/runner-type=static`; ownerReference to DynamicHost; see `dynamichost_test.go` |
| Sets finalizer when reconciled | When DynamicHost is reconciled (e.g. state is Ready), May adds the host-controller finalizer; see `dynamichost_test.go` |
| Status reflects Runner counts | `status.runners.ready` and `status.runners.stopped` are updated from the owned Runner (Ready vs Stopped); see `dynamichost_test.go` |
| Status reflects pipeline | `status.pipeline` is updated from the Runner’s `tekton.dev/pipeline` label (if any); see `dynamichost_test.go` |
| Runner not recreated when deleted | When the DynamicHost’s Runner is deleted, May does not recreate the Runner; the host remains in Draining (and can transition to Drained once the Runner is gone); see `dynamichost_test.go` |
| DynamicHost marked Draining when Runner is deleted | When the DynamicHost’s Runner is deleted (or marked for deletion), May sets DynamicHost `status.state` to Draining; see `dynamichost_test.go` |
| Finalizer and deletion | When DynamicHost is deleted, the finalizer is removed so the host can be deleted (Runner already gone or deleted by finalizer); see `dynamichost_test.go` |

## 11. DynamicHostAutoscaler (in may) ✅

The DynamicHost provisioner watches Pending Claims and creates a DynamicHost per Claim when an Autoscaler matches the Claim’s flavor. The GC deletes DynamicHosts in state Drained (scale-down when idle).

| Case | Description |
|------|-------------|
| Creates DynamicHost when Claim is Pending and Autoscaler matches flavor | Provisioner creates a DynamicHost named like the Claim, with spec from the Autoscaler template (flavor, runner resources, rootKey, etc.); see `dynamichostautoscaler_test.go` |
| Does not create DynamicHost when no Autoscaler matches Claim flavor | When a Claim is Pending but no DynamicHostAutoscaler has the same flavor, no DynamicHost is created; see `dynamichostautoscaler_test.go` |
| One DynamicHost per Pending Claim | When multiple Pending Claims have matching Autoscalers, the provisioner creates one DynamicHost per Claim (by CreationTimestamp order); see `dynamichostautoscaler_test.go` |
| GC deletes DynamicHost after Runner is deleted | After the Pod (and thus Claim and Runner) is released, the DynamicHost transitions to Drained and the GC deletes it; test verifies the DynamicHost is deleted once the Runner is gone; see `dynamichostautoscaler_test.go` |

## 12. Edge Cases & Failures

| Case | Description |
|------|-------------|
| Claim for deleted Pod | Claim with non-existent Pod reference; no crash, optional cleanup |
| Invalid flavor | Claim for flavor with no Runners stays Pending or Unclaimable |
| Immutable inUseBy | Runner `inUseBy` cannot be changed after reservation (if enforced) |

## 13. Multi-namespace / Configuration

| Case | Description |
|------|-------------|
| Scheduler namespace | Claims and Runners in configured scheduler namespace; Claimer/scheduler use it correctly |
| Namespace selectors | Webhook/Claimer only act in namespaces matching selector (if configured) |

## 14. Integration (Optional)

| Case | Description |
|------|-------------|
| May + incluster driver | Full stack: StaticHost → Ready → Runners → Pod with flavor → Claim → Claimed → Pod scheduled |

---

## Implementation status

- **Done:** Manager run, metrics, cert-manager, webhook CA, Pod webhook (tenant and non-tenant namespace cases; see `e2e_test.go` and `pod_webhook_test.go`), Claimer (tenant and non-tenant cases; see `claimer_test.go`), Claim/Scheduler (Claim scheduled, Claim pending, single Runner two Claims, Claim deletion; see `scheduler_test.go`), Gater (gate removed when Claimed, gate remains when Pending; see `gater_test.go`), Runner Lifecycle (Runner untouched without condition, Runner reserved, provisioning hooks, cleanup hooks, Runner not deleted when cleaning failed; see `runner_lifecycle_test.go`), RunnerBinder (Secret and Pod config; see `binder_test.go`), Full Flow (happy path and Pod completion in one test; see `full_flow_test.go`), StaticHost (all cases; see `statichost_test.go`), DynamicHost (all cases; see `dynamichost_test.go`), DynamicHostAutoscaler (no DynamicHost when no Autoscaler matches flavor, creates DynamicHost when Autoscaler matches, one DynamicHost per Pending Claim, GC deletes Drained DynamicHosts; see `dynamichostautoscaler_test.go`). OTP server from multi-platform-controller is installed in the e2e cluster for RunnerBinder.
- **TODO:** Edge cases, multi-namespace / configuration, integration.

## Running E2E tests

```bash
make test-e2e
```

This creates a Kind cluster (`may-test-e2e` by default), builds the may controller image, loads it into Kind, installs cert-manager if needed, installs the OTP server from multi-platform-controller (see `demo/dependencies/multi-platform-controller/config/otp`), runs the tests, then tears down the cluster. Requires Kind and `kubectl`. Optional: `CERT_MANAGER_INSTALL_SKIP=true`, `OTP_SERVER_INSTALL_SKIP=true` to skip installing cert-manager or the OTP server when already present.
