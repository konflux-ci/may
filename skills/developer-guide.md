---
name: developer-guide
description: |
  Use when an agent needs MAY's architecture and entry points, needs to trace a Pod → Claim → Runner → Host flow, is adding or changing a driver, or is debugging a stuck Claim, Runner, or Host.
---

# MAY Developer Guide

Architecture and orientation for the MAY codebase, as of 2026-11-25. Repo layout and per-module commands live in `AGENTS.md` at the root and in each module; PR and CI process live in `skills/pr-workflow.md` and `skills/ci-troubleshooting.md`. Tests are required for every code change; do not report work as complete until the relevant tests pass. See section 7 and `skills/testing-standards.md`.

## 1. What MAY Does

MAY separates the request from the capacity: Claims connect a workload to a requested flavor, Runners provide the resources for that flavor, and Hosts supply the infrastructure that backs the Runners. Work arrives as Pods. MAY reacts to a Pod in a tenant namespace — one labelled `konflux-ci.dev/type=tenant` — when the Pod carries an annotation key beginning with `kueue.konflux-ci.dev/requests-`; the suffix after the prefix is the flavor.

The Claimer checks that label in code, and the Pod webhook scopes itself to tenant namespaces through `namespaceSelector`. A Claim requests a Runner for one workload. A Runner is the build environment reserved by at most one Claim.

StaticHost and DynamicHost are the hosts that back Runners, and drivers own them. One Kueue `ClusterQueue` is created per Runner that sets `spec.queue`. In the demo, Tekton PipelineRuns reach MAY as Pods through tekton-kueue's CEL mapping of the `build-platforms` param.

## 2. Modules and Entry Points

| Module | Entry point | Role | Status (2026-11-25) |
| --- | --- | --- | --- |
| `may/` | `may/cmd/main.go` | Core: Claimer, Gater, Scheduler, Binder, Provisioner. | Active. |
| `drivers/incluster/` | `drivers/incluster/cmd/main.go` | In-cluster host driver. | Reference implementation. Demo and development only. |
| `drivers/aws/` | `drivers/aws/cmd/main.go` | EC2-backed hosts. | Host reconcilers are scaffolds. The EC2 client exists but is not wired. |
| `drivers/ibm/` | `drivers/ibm/cmd/main.go` | IBM-backed hosts. | Scaffold only. |

Each module has its own `go.mod`, `Makefile`, `AGENTS.md`, and `Dockerfile`; there is no root `Makefile` and no `go.work` in the repository. All four modules currently require Go `1.26.0`.

The core binary reads `WATCH_NAMESPACE`. When the variable is empty, startup logs "missing required environment variable" and continues with an empty namespace. The Pod webhook is registered unless `ENABLE_WEBHOOKS` equals the string `false`.

## 3. CRDs and the Scheduling Contract

| Kind | Purpose | Key spec fields | Status signals | Anchor |
| --- | --- | --- | --- | --- |
| `Claim` | Request for a Runner for one workload. | `spec.for` (name/kind/apiVersion/uid), `spec.flavor` | `status.conditions[type=Claimed]` with reasons `Scheduled`, `Pending`, `Unclaimable`. | `may/api/v1alpha1/claim_types.go` → `ClaimSpec`; `may/pkg/claim/claim.go` → `ConditionReasonScheduled`, `ConditionReasonPending`, `ConditionReasonUnclaimable` |
| `Runner` | Build environment reserved by at most one Claim. | `spec.flavor`, `spec.resources`, `spec.inUseBy`, `spec.queue.cohort`, `spec.hooks` | `status.conditions[type=Ready]` with reasons `Ready`, `Initializing`, `Cleaning`, `InitializationFailed`, `CleaningFailed`. Also `status.hooksStatus`. | `may/api/v1alpha1/runner_types.go` → `RunnerSpec`; `may/pkg/runner/runner.go` → `ConditionReasonReady`, `ConditionReasonInitializing`, `ConditionReasonCleaning`, `ConditionReasonInitializationFailed`, `ConditionReasonCleaningFailed` |
| `StaticHost` | Host serving a fixed number of Runners. | `HostCoreSpec` (`flavor`, `queue`, `status`, `rootKey`), `spec.runners.instances`, `spec.runners.resources`, `spec.runners.hooks` | `status.state`, `status.runners.ready`, `status.runners.stopped`, `status.pipelines` | `may/api/v1alpha1/statichost_types.go` → `StaticHostSpec`; `may/api/v1alpha1/host_types.go` → `HostCommonStatus` |
| `DynamicHost` | Host backing exactly one Runner. | `HostCoreSpec`, `spec.runner.resources`, `spec.runner.hooks` | `status.state`, `status.runners`, `status.pipeline` | `may/api/v1alpha1/dynamichost_types.go` → `DynamicHostSpec`; `may/api/v1alpha1/host_types.go` → `HostCommonStatus` |
| `DynamicHostAutoscaler` | Template that creates a DynamicHost per Pending Claim of one flavor. | `spec.flavor`, `spec.template.spec` | `status.conditions` | `may/api/v1alpha1/dynamichostautoscaler_types.go` → `DynamicHostAutoscalerSpec` |

All five CRDs are namespaced, while the Kueue `ClusterQueue` that `may` creates per Runner is cluster-scoped.

The Pod annotation key is `kueue.konflux-ci.dev/requests-<flavor>`. A Runner matches when `<flavor>` is a key of `spec.resources`. The flavors `localhost`, `local`, and `linux-x86-64` are hardcoded exclusions.

| Constant | Value | Set by / read by | Anchor |
| --- | --- | --- | --- |
| `RunnerTypeLabel` | `may.konflux-ci.dev/runner-type` | Values `static` and `dynamic`. Gates the Provisioner's Runner predicate. | `may/pkg/constants/constants.go` → `RunnerTypeLabel`; `may/internal/controller/provisioner/runner/runner_controller.go` → `SetupWithManager` |
| `RunnerIdLabel` | `may.konflux-ci.dev/runner-id` | StaticHost reconciler writes the instance index. | `may/pkg/constants/constants.go` → `RunnerIdLabel`; `may/internal/controller/provisioner/statichost_controller.go` → `ensureHostRunnersExists` |
| `RunnerNameLabel` | `may.konflux-ci.dev/runner` | Hook Pods. | `may/pkg/constants/constants.go` → `RunnerNameLabel`; `may/internal/controller/provisioner/runner/runner_controller.go` → `runNextHookPod` |
| `RunnerHookNameLabel` | `may.konflux-ci.dev/hook` | Hook Pods. | `may/pkg/constants/constants.go` → `RunnerHookNameLabel`; `may/internal/controller/provisioner/runner/runner_controller.go` → `runNextHookPod` |
| `RunnerHookPhaseLabel` | `may.konflux-ci.dev/hook-phase` | Values `provisioning` and `cleanup`. | `may/pkg/constants/constants.go` → `RunnerHookPhaseLabel`; `may/internal/controller/provisioner/constants/constants.go` → `RunnerHookPhaseLabelProvisioningValue`, `RunnerHookPhaseLabelCleanupValue` |
| `RunnerUIDLabel` | `may.konflux-ci.dev/runner-uid` | Hook Pods. Set to the Runner UID. | `may/pkg/constants/constants.go` → `RunnerUIDLabel`; `may/internal/controller/provisioner/runner/runner_controller.go` → `runNextHookPod` |
| `RunnerUserLabel` | `may.konflux-ci.dev/runner-user` | Hook Pods receive it set to the Runner UID. The Binder reads it from the Runner's labels to build `user-dir`, so the Runner itself must carry it. | `may/pkg/constants/constants.go` → `RunnerUserLabel`; `may/internal/controller/provisioner/runner/runner_controller.go` → `runNextHookPod`; `may/internal/controller/binder/runnerbinder_controller.go` → `Reconcile` |
| `HostLabel` | `may.konflux-ci.dev/host` | Runner → owning host. | `may/pkg/constants/label.go` → `HostLabel`; `may/internal/controller/provisioner/statichost_controller.go` → `ensureHostRunnersExists`; `may/internal/controller/provisioner/dynamichost_controller.go` → `ensureRunnerExists` |
| `MayPodSchedulingGate` | `may.konflux-ci.dev/scheduling` | Webhook adds. Gater removes. | `may/pkg/constants/pod.go` → `MayPodSchedulingGate`; `may/internal/webhook/v1/pod_webhook.go` → `Default`; `may/internal/controller/gater/gater_controller.go` → `ensurePodCanBeScheduled` |
| `ClaimControllerFinalizer` | `may.konflux-ci.dev/reservation-controller` | Claim and reserved Runner. | `may/pkg/constants/constants.go` → `ClaimControllerFinalizer`; `may/internal/controller/scheduler/claim_controller.go` → `Reconcile`; `may/pkg/scheduler/scheduler.go` → `setInUseBy` |
| `HostControllerFinalizer` | `may.konflux-ci.dev/host-controller` | Core host finalizer. | `may/pkg/constants/label.go` → `HostControllerFinalizer`; `may/internal/controller/provisioner/statichost_controller.go` → `Reconcile`; `may/internal/controller/provisioner/dynamichost_controller.go` → `Reconcile` |
| `RunnerControllerFinalizer` | `may.konflux-ci.dev/runner-controller` | Provisioner Runner finalizer. | `may/internal/controller/provisioner/constants/constants.go` → `RunnerControllerFinalizer`; `may/internal/controller/provisioner/runner/runner_controller.go` → `Reconcile`, `finalize` |
| `InClusterDriverFinalizer` | `drivers.may.konflux-ci.dev/incluster` | Incluster driver's own finalizer. | `drivers/incluster/internal/controller/statichost_controller.go` → `InClusterDriverFinalizer` |
| Pipeline label | `tekton.dev/pipeline` | Claimer copies Pod → Claim; the Scheduler copies Claim → Runner. Host controllers mirror it into `status.pipelines` / `status.pipeline`. | `may/internal/controller/claimer/claimer_controller.go` → `pipelineLabelKey`; `may/pkg/runner/runner.go` → `SetInUseBy`; `may/internal/controller/provisioner/statichost_controller.go` → `ensureRunnersInfoIsInStatus`; `may/internal/controller/provisioner/dynamichost_controller.go` → `ensureRunnersPipelinesAreInStatus` |
| `FieldOwnerOperator` | `may.konflux-ci.dev` | Declared in `may/pkg/constants/constants.go`. No caller today. | `may/pkg/constants/constants.go` → `FieldOwnerOperator` |

### Status semantics

`spec.status` is the requested `HostRequestedStatus`; the CRD enum is `Pending;Ready;Drain;Halted`. `status.state` is the observed `HostActualState`; its values are `Pending`, `Ready`, `Draining`, and `Drained`. `Drain` and `Halted` are accepted by the API but implemented by no current driver.

On a Runner, the reason `Stopped` is read by the host controllers but never set: the generic `SetNotReady` has no caller, and the specific setters use other reasons, so `status.runners.stopped` stays `0` unless a Runner's `Ready` condition is written out of band. On a Claim, the reason `Unclaimable` is declared and read by `IsUnclaimable`, but no controller sets it today.

## 4. Reconciler Map

| Controller (Named) | Watches | Writes | Anchor |
| --- | --- | --- | --- |
| Pod webhook `mpod-v1.kb.io` | Pod CREATE in tenant namespaces. | Adds the scheduling gate. | `may/internal/webhook/v1/pod_webhook.go` → `PodCustomDefaulter` |
| `claimer` | Pods on create only, in tenant namespaces, with a flavor annotation present and not excluded. | Creates the Claim. | `may/internal/controller/claimer/claimer_controller.go` → `ensureClaimExists` |
| `gater` | Claim updates. | Removes the scheduling gate from the Pod. | `may/internal/controller/gater/gater_controller.go` → `ensurePodCanBeScheduled` |
| `claim` | Claims. Pods in `Succeeded`/`Failed`. Runner updates. | Runner `spec.inUseBy`, Claim conditions and finalizer. Deletes the Claim and linked Runners. | `may/internal/controller/scheduler/claim_controller.go` → `ClaimReconciler` |
| `runnerbinder` | Runners where `runner.IsReserved`. | The workload Secret named after the Claim. | `may/internal/controller/binder/runnerbinder_controller.go` → `RunnerBinderReconciler` |
| `runner` | Runners labelled `runner-type` `static` or `dynamic`. Owns Pods. Claim delete events requeue static Runners. | Runner `Ready` condition, hook Pods, and `ClusterQueue`. | `may/internal/controller/provisioner/runner/runner_controller.go` → `RunnerReconciler` |
| `runnerhooks` | Runners labelled `runner-type=static`. Owns Pods. | Runner `status.hooksStatus`. | `may/internal/controller/provisioner/runnerhook_controller.go` → `RunnerHookReconciler` |
| `statichost` | StaticHosts whose `status.state` is set and not `Pending`. Owns Runners. | Creates and deletes Runners. Updates host status. | `may/internal/controller/provisioner/statichost_controller.go` → `StaticHostReconciler` |
| `dynamichost` | DynamicHosts. Owns Runners. | Creates and deletes exactly one Runner. Updates host status. | `may/internal/controller/provisioner/dynamichost_controller.go` → `DynamicHostReconciler` |
| `dynamichost-provisioner` | Claims, DynamicHostAutoscalers, DynamicHosts. | Creates a DynamicHost for a Pending Claim. | `may/internal/controller/provisioner/dynamichost_provisioner.go` → `DynamicHostProvisioner` |
| `dynamichost-gc` | DynamicHosts whose `status.state` is `Drained`. | Deletes the DynamicHost. | `may/internal/controller/provisioner/dynamichost_gc.go` → `DynamicHostGarbageCollector` |
| `dynamichostautoscaler` | DynamicHostAutoscalers. | Nothing. No-op reconciler today. | `may/internal/controller/provisioner/dynamichostautoscaler_controller.go` → `DynamicHostAutoscalerReconciler` |

Controller registration is fixed in `may/cmd/main.go`; the provisioner's reconcilers come from a map, so their relative order is not guaranteed. Field indexers in `may/pkg/indexer` give the scheduler and finalizers direct lookups for free Runners, reserved Runners, and pending Claims.

## 5. End-to-End Flow

1. A tenant Pod with a flavor annotation is gated by the webhook.
2. The Claimer creates a Claim; the scheduler reserves a free Runner whose resources include the flavor.
3. The Binder writes the workload Secret with OTP credentials, and the Gater removes the scheduling gate.
4. The Provisioner runs provisioning hooks, marks the Runner `Ready`, and creates its `ClusterQueue` when requested.
5. When the Pod finishes, the Claim and its Runner are deleted after cleanup hooks run.
6. For a pending Claim with a matching `DynamicHostAutoscaler`, MAY provisions a DynamicHost and one Runner instead of reusing an existing Runner, then garbage-collects the host once drained.

Executable examples live in `may/test/e2e/full_flow_test.go`, `may/test/e2e/dynamichost_test.go`, and `may/test/e2e/dynamichostautoscaler_test.go`.

## 6. Driver Contract and Adding a Driver

A driver owns `StaticHost` and `DynamicHost` reconciliation and writes `status.state`; the core creates and deletes Runners. The core waits while `status.state` is nil or `Pending`. A driver sets `status.state=Ready` once its host resources exist; the incluster driver treats resource creation as acceptance and does not probe runner availability, while a production driver should probe acceptance before setting Ready.

A driver adds its own finalizer and removes it after its resources are gone, while the core's `may.konflux-ci.dev/host-controller` finalizer is separate and independently managed. Requested state is `spec.status`; observed state is `status.state`. The core reacts to observed `Draining` and `Drained` on StaticHosts.

`drivers/incluster` is the reference implementation for demo and development only. It creates one Secret, Pod, and Service per host and reaches runners over SSH.

To add a new driver, copy `drivers/incluster` as the starting point, then:
1. Create a new module under `drivers/<name>/` with its own `go.mod`, `Makefile`, `Dockerfile`, and `AGENTS.md`.
2. Add an entry point `cmd/main.go` under the module root. Register the manager and reconcilers.
3. Add `StaticHostReconciler` and `DynamicHostReconciler` in the module's `internal/controller/`.
4. Implement the status contract: initialize `status.state=Pending`. Set `Ready` once host resources exist. A production driver should probe acceptance first.
5. Add your own finalizer `drivers.may.konflux-ci.dev/<name>`. Remove it after provider resources are gone.
6. Add RBAC through `+kubebuilder:rbac` markers. Run `make -C drivers/<name> manifests`.
7. Add an envtest suite under `internal/controller/` and a Kind e2e suite under `test/e2e/`.
8. Document the module in its `AGENTS.md`. Add it to the module table in this guide.

## 7. Testing Is Required

Tests are part of every code change, not a follow-up. Run them yourself; do not ask the user to run them for you. Do not report a change as complete until the relevant tests pass.

- Run `make -C may test` for core changes and `make -C drivers/<name> test` for driver changes. Add the module's e2e target when the change affects cluster behavior and the environment is available.
- When a change crosses modules, run tests in every affected module.
- Update existing specs when behavior changes. Add specs for every new behavior, branch, and error path.
- Coverage is part of the change: new code needs tests, and changed code needs its tests updated. The module's `make test` writes `cover.out`; inspect it and make sure new branches are covered. `make -C may report-coverage` builds the HTML report for the core module.
- Follow `skills/testing-standards.md` for Ginkgo/Gomega structure, style, and organization.

If a test cannot run, say so explicitly and state what remains unverified. Do not claim completion from a code read or a compile-only check. If `controller-gen` fails because a parent `go.work` excludes the module, prefix the test command with `GOWORK=off`.

## 8. Load-Bearing Decisions

| Decision                                                           | Enforced at                                                                                                                           |
| ------------------------------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------- |
| Claim, Runner, and Host are separate kinds.                        | `may/api/v1alpha1/claim_types.go`, `may/api/v1alpha1/runner_types.go`, `may/api/v1alpha1/host_types.go`                               |
| Core owns Runners. Drivers own Hosts.                              | `may/internal/controller/provisioner/statichost_controller.go` → `Reconcile` waits on `status.state`                                  |
| Flavor matches a Runner resource key, not a field.                 | `may/pkg/scheduler/scheduler.go` → `findBestRunner`                                                                                   |
| Gate is added by the webhook and removed by a separate controller. | `may/internal/webhook/v1/pod_webhook.go` → `Default`; `may/internal/controller/gater/gater_controller.go` → `ensurePodCanBeScheduled` |
| One ClusterQueue per Runner. Borrowing is disabled.                | `may/internal/controller/provisioner/runner/runner_controller.go` → `ensureClusterQueue` sets `BorrowingLimit` to zero                |
| Hooks run as Pods. `CleaningFailed` Runners are retained.          | `may/internal/controller/provisioner/runner/runner_controller.go` → `runNextHookPod`, `finalize` returns early on `CleaningFailed`    |
| Credentials are handed over as an OTP, not a mounted key.          | `may/internal/controller/binder/runnerbinder_controller.go` → `Reconcile` POSTs to `/store-key`                                       |
| Each component is its own Go module.                               | `may/go.mod`, `drivers/aws/go.mod`, `drivers/ibm/go.mod`, `drivers/incluster/go.mod`                                                  |

## 9. Troubleshooting

For CI-only failures, use `skills/ci-troubleshooting.md` instead of the following table:

| Symptom | Likely cause | Where to look |
| --- | --- | --- |
| Pod stays `SchedulingGated`. | The Claim is not yet `Claimed`, or the Gater is not running. | `may/internal/controller/gater/gater_controller.go` → `ensurePodCanBeScheduled`; `may/pkg/claim/claim.go` → `IsClaimed` |
| No Claim exists for a gated Pod. | The Claimer is not running or failed to create the Claim. A gate added earlier can remain after the tenant label or flavor annotation is removed. | `may/internal/controller/claimer/claimer_controller.go` → `Reconcile`, `SetupWithManager`; `may/internal/webhook/v1/pod_webhook.go` → `Default` |
| Claim `Pending`, message `no available runner`. | No Runner is both Ready and unreserved in `WATCH_NAMESPACE`. | `may/pkg/scheduler/scheduler.go` → `listFreeRunners` |
| Claim `Pending`, message `no available runner for flavor <f>`. | Ready Runners exist, but none has `<f>` as a key of `spec.resources`. | `may/pkg/scheduler/scheduler.go` → `findBestRunner`; compare `Claim.spec.flavor` with `Runner.spec.resources` |
| Runner stuck `Initializing`. | A provisioning hook Pod has not reached `Succeeded`. | `may/internal/controller/provisioner/runner/runner_controller.go` → `ensureRunnerIsProvisioned`; Pod `p-<runner>-<hook>` |
| Runner `InitializationFailed`. | A provisioning hook Pod failed. The message carries the Pod name. | `may/internal/controller/provisioner/runner/runner_controller.go` → `ensureRunnerIsProvisioned`; Pods labelled `may.konflux-ci.dev/hook-phase=provisioning` |
| Runner `CleaningFailed` and never deleted. | A cleanup hook failed. `finalize` returns early and keeps the finalizer, by design. | `may/internal/controller/provisioner/runner/runner_controller.go` → `finalize` |
| Runner ignored by the Provisioner. | `may.konflux-ci.dev/runner-type` is missing or outside `{static, dynamic}`. | `may/internal/controller/provisioner/runner/runner_controller.go` → `SetupWithManager` predicate |
| Host `status.state` nil or `Pending`, and no Runners appear. | The driver has not marked the host Ready. | Driver `StaticHostReconciler` / `DynamicHostReconciler` |
| Host stays `Ready` after `spec.status: Drain`. | The incluster driver has no `Drain` case. Drain is not wired end to end. | `drivers/incluster/internal/controller/statichost_controller.go` → `Reconcile` |
| No ClusterQueue for a Ready Runner. | The Runner has no `spec.queue`. | `may/internal/controller/provisioner/runner/runner_controller.go` → `ensureClusterQueueExists` |
| Workload Secret never created. | The OTP server is unreachable, or `otp-tls-secrets` / the Runner Secret is missing. | `may/internal/controller/binder/runnerbinder_controller.go` → `Reconcile` |
| Pod schedules with no gate and no Claim. | The flavor is excluded (`localhost`, `local`, `linux-x86-64`), the namespace is not a tenant, or the Pod webhook is disabled (`ENABLE_WEBHOOKS=false`). | `may/pkg/pod/excluded.go`; `may/config/webhook/manifests_patch.yaml`; `may/cmd/main.go` → `main` |
| Controller lists cluster-wide or finds nothing. | `WATCH_NAMESPACE` is unset. Startup logs the error and continues with an empty namespace. | `may/cmd/main.go` → `main` |
| `git status` is dirty after `make test`. | The target regenerated CRD, RBAC, or deepcopy files because the committed output did not match the generated result. | `may/Makefile` → `test`; review and commit the regenerated files |
| `controller-gen` fails with `directory prefix . does not contain modules listed in go.work`. | A parent workspace `go.work` does not include this module. | Prefix the command with `GOWORK=off` |

## 10. Code Standards

- For Go coding standards used in this repo, see `skills/go-coding-standards.md`.
- For testing and test-coding standards used in this repo, see `skills/testing-standards.md`.
