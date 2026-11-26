---
name: developer-guide
description: >
  Use when an agent needs MAY's architecture and entry points, needs to trace a
  Pod → Claim → Runner → Host flow, is adding or changing a driver, or is debugging
  a stuck Claim, Runner, or Host.
---

# MAY Developer Guide

Architecture and orientation for the MAY codebase, as of 2026-11-25.
Repo layout and per-module commands live in `AGENTS.md` at the root and in each module.
PR and CI process live in `skills/pr-workflow.md` and `skills/ci-troubleshooting.md`.

Anchors below are paths plus symbol names, not line numbers.

## 1. What MAY Does

Pods are the workload interface.
MAY reacts to a tenant Pod carrying the annotation prefix `kueue.konflux-ci.dev/requests-`. — `may/pkg/pod/annotation.go` → `KueueFlavorLabelPrefix`
A Claim requests a Runner for one workload. — `may/api/v1alpha1/claim_types.go` → `ClaimSpec`
A Runner is the build environment reserved by at most one Claim. — `may/api/v1alpha1/runner_types.go` → `RunnerSpec`
StaticHost and DynamicHost back Runners.
Drivers own them. — `may/api/v1alpha1/statichost_types.go` → `StaticHostSpec`; `may/api/v1alpha1/dynamichost_types.go` → `DynamicHostSpec`

One Kueue `ClusterQueue` is created per Runner that sets `spec.queue`. — `may/internal/controller/provisioner/runner/runner_controller.go` → `ensureClusterQueue`
In the demo, Tekton PipelineRuns reach MAY as Pods through tekton-kueue's CEL mapping of the `build-platforms` param. — `demo/dependencies/tekton-kueue/config/config.yaml`
The Claimer and Pod webhook target namespaces labelled `konflux-ci.dev/type=tenant`. — `may/pkg/constants/label.go` → `TenantNamespaceLabelKey`/`TenantNamespaceLabelValue`

## 2. Modules and Entry Points

| Module | Entry point | Role | Status (2026-11-25) |
| --- | --- | --- | --- |
| `may/` | `may/cmd/main.go` | Core: Claimer, Gater, Scheduler, Binder, Provisioner. | Active. |
| `drivers/incluster/` | `drivers/incluster/cmd/main.go` | In-cluster host driver. | Reference implementation. Demo and development only. |
| `drivers/aws/` | `drivers/aws/cmd/main.go` | EC2-backed hosts. | Host reconcilers are scaffolds. The EC2 client exists but is not wired. |
| `drivers/ibm/` | `drivers/ibm/cmd/main.go` | IBM-backed hosts. | Scaffold only. |

Each module has its own `go.mod`, `Makefile`, `AGENTS.md`, and `Dockerfile`. — `may/go.mod`, `drivers/aws/go.mod`, `drivers/ibm/go.mod`, `drivers/incluster/go.mod`, `may/Makefile`, `drivers/*/Makefile`
There is no root `Makefile` and no `go.work` in the repository. — `may/go.mod`, `drivers/aws/go.mod`, `drivers/ibm/go.mod`, `drivers/incluster/go.mod`
All four modules require Go `1.26.0`. — `may/go.mod`, `drivers/aws/go.mod`, `drivers/ibm/go.mod`, `drivers/incluster/go.mod`

`may/cmd/main.go` → `main` reads `WATCH_NAMESPACE`.
When it is empty, startup logs "missing required environment variable" and keeps running with an empty namespace.
The Pod webhook is registered unless `ENABLE_WEBHOOKS` equals the string `false`. — `may/cmd/main.go` → `main`

## 3. CRDs and the Scheduling Contract

| Kind | Purpose | Key spec fields | Status signals | Anchor |
| --- | --- | --- | --- | --- |
| `Claim` | Request for a Runner for one workload. | `spec.for` (name/kind/apiVersion/uid), `spec.flavor` | `status.conditions[type=Claimed]` with reasons `Scheduled`, `Pending`, `Unclaimable`. | `may/api/v1alpha1/claim_types.go` → `ClaimSpec`; `may/pkg/claim/claim.go` → `ConditionReasonScheduled`, `ConditionReasonPending`, `ConditionReasonUnclaimable` |
| `Runner` | Build environment reserved by at most one Claim. | `spec.flavor`, `spec.resources`, `spec.inUseBy`, `spec.queue.cohort`, `spec.hooks` | `status.conditions[type=Ready]` with reasons `Ready`, `Initializing`, `Cleaning`, `InitializationFailed`, `CleaningFailed`. Also `status.hooksStatus`. | `may/api/v1alpha1/runner_types.go` → `RunnerSpec`; `may/pkg/runner/runner.go` → `ConditionReasonReady`, `ConditionReasonInitializing`, `ConditionReasonCleaning`, `ConditionReasonInitializationFailed`, `ConditionReasonCleaningFailed` |
| `StaticHost` | Host serving a fixed number of Runners. | `HostCoreSpec` (`flavor`, `queue`, `status`, `rootKey`), `spec.runners.instances`, `spec.runners.resources`, `spec.runners.hooks` | `status.state`, `status.runners.ready`, `status.runners.stopped`, `status.pipelines` | `may/api/v1alpha1/statichost_types.go` → `StaticHostSpec`; `may/api/v1alpha1/host_types.go` → `HostCommonStatus` |
| `DynamicHost` | Host backing exactly one Runner. | `HostCoreSpec`, `spec.runner.resources`, `spec.runner.hooks` | `status.state`, `status.runners`, `status.pipeline` | `may/api/v1alpha1/dynamichost_types.go` → `DynamicHostSpec`; `may/api/v1alpha1/host_types.go` → `HostCommonStatus` |
| `DynamicHostAutoscaler` | Template that creates a DynamicHost per Pending Claim of one flavor. | `spec.flavor`, `spec.template.spec` | `status.conditions` | `may/api/v1alpha1/dynamichostautoscaler_types.go` → `DynamicHostAutoscalerSpec` |

All five CRDs are namespaced. — `may/config/crd/bases/*` → `scope: Namespaced`
The Kueue `ClusterQueue` that `may` creates per Runner is cluster-scoped. — `may/internal/controller/provisioner/runner/runner_controller.go` → `ensureClusterQueue`
The Pod annotation key is `kueue.konflux-ci.dev/requests-<flavor>`. — `may/pkg/pod/annotation.go` → `ExtractFlavor`
A Runner matches when `<flavor>` is a key of `spec.resources`. — `may/pkg/scheduler/scheduler.go` → `findBestRunner`
The flavors `localhost`, `local`, and `linux-x86-64` are hardcoded exclusions. — `may/pkg/pod/excluded.go` → `excludedFlavors`
The Claimer and Pod webhook target namespaces labelled `konflux-ci.dev/type=tenant`. — `may/pkg/constants/label.go` → `TenantNamespaceLabelKey`/`TenantNamespaceLabelValue`; `may/config/webhook/manifests_patch.yaml` → `namespaceSelector`

| Constant | Value | Set by / read by | Anchor |
| --- | --- | --- | --- |
| `RunnerTypeLabel` | `may.konflux-ci.dev/runner-type` | Values `static` and `dynamic`. Gates the Provisioner's Runner predicate. | `may/pkg/constants/constants.go` → `RunnerTypeLabel`; `may/internal/controller/provisioner/runner/runner_controller.go` → `SetupWithManager` |
| `RunnerIdLabel` | `may.konflux-ci.dev/runner-id` | StaticHost reconciler writes the instance index. | `may/pkg/constants/constants.go` → `RunnerIdLabel`; `may/internal/controller/provisioner/statichost_controller.go` → `ensureHostRunnersExists` |
| `RunnerNameLabel` | `may.konflux-ci.dev/runner` | Hook Pods. | `may/pkg/constants/constants.go` → `RunnerNameLabel`; `may/internal/controller/provisioner/runner/runner_controller.go` → `runNextHookPod` |
| `RunnerHookNameLabel` | `may.konflux-ci.dev/hook` | Hook Pods. | `may/pkg/constants/constants.go` → `RunnerHookNameLabel`; `may/internal/controller/provisioner/runner/runner_controller.go` → `runNextHookPod` |
| `RunnerHookPhaseLabel` | `may.konflux-ci.dev/hook-phase` | Values `provisioning` and `cleanup`. | `may/pkg/constants/constants.go` → `RunnerHookPhaseLabel`; `may/internal/controller/provisioner/constants/constants.go` → `RunnerHookPhaseLabelProvisioningValue`, `RunnerHookPhaseLabelCleanupValue` |
| `RunnerUIDLabel` | `may.konflux-ci.dev/runner-uid` | Hook Pods. Set to the Runner UID. | `may/pkg/constants/constants.go` → `RunnerUIDLabel`; `may/internal/controller/provisioner/runner/runner_controller.go` → `runNextHookPod` |
| `RunnerUserLabel` | `may.konflux-ci.dev/runner-user` | Set on hook Pods to the Runner UID. The Binder reads it from the Runner's labels. | `may/pkg/constants/constants.go` → `RunnerUserLabel`; `may/internal/controller/provisioner/runner/runner_controller.go` → `runNextHookPod`; `may/internal/controller/binder/runnerbinder_controller.go` → `Reconcile` |
| `HostLabel` | `may.konflux-ci.dev/host` | Runner → owning host. | `may/pkg/constants/label.go` → `HostLabel`; `may/internal/controller/provisioner/statichost_controller.go` → `ensureHostRunnersExists`; `may/internal/controller/provisioner/dynamichost_controller.go` → `ensureRunnerExists` |
| `MayPodSchedulingGate` | `may.konflux-ci.dev/scheduling` | Webhook adds. Gater removes. | `may/pkg/constants/pod.go` → `MayPodSchedulingGate`; `may/internal/webhook/v1/pod_webhook.go` → `Default`; `may/internal/controller/gater/gater_controller.go` → `ensurePodCanBeScheduled` |
| `ClaimControllerFinalizer` | `may.konflux-ci.dev/reservation-controller` | Claim and reserved Runner. | `may/pkg/constants/constants.go` → `ClaimControllerFinalizer`; `may/internal/controller/scheduler/claim_controller.go` → `Reconcile`; `may/pkg/scheduler/scheduler.go` → `setInUseBy` |
| `HostControllerFinalizer` | `may.konflux-ci.dev/host-controller` | Core host finalizer. | `may/pkg/constants/label.go` → `HostControllerFinalizer`; `may/internal/controller/provisioner/statichost_controller.go` → `Reconcile`; `may/internal/controller/provisioner/dynamichost_controller.go` → `Reconcile` |
| `RunnerControllerFinalizer` | `may.konflux-ci.dev/runner-controller` | Provisioner Runner finalizer. | `may/internal/controller/provisioner/constants/constants.go` → `RunnerControllerFinalizer`; `may/internal/controller/provisioner/runner/runner_controller.go` → `Reconcile`, `finalize` |
| `InClusterDriverFinalizer` | `drivers.may.konflux-ci.dev/incluster` | Incluster driver's own finalizer. | `drivers/incluster/internal/controller/statichost_controller.go` → `InClusterDriverFinalizer` |
| Pipeline label | `tekton.dev/pipeline` | Claimer copies Pod → Claim. Host controllers mirror it into `status.pipelines` / `status.pipeline`. | `may/internal/controller/claimer/claimer_controller.go` → `pipelineLabelKey`; `may/internal/controller/provisioner/statichost_controller.go` → `ensureRunnersInfoIsInStatus`; `may/internal/controller/provisioner/dynamichost_controller.go` → `ensureRunnersPipelinesAreInStatus` |
| `FieldOwnerOperator` | `may.konflux-ci.dev` | Declared in `may/pkg/constants/constants.go`. No caller today. | `may/pkg/constants/constants.go` → `FieldOwnerOperator` |

`spec.status` is the requested `HostRequestedStatus`. The CRD enum is `Pending;Ready;Drain;Halted`. — `may/api/v1alpha1/host_types.go` → `HostRequestedStatus`
`status.state` is the observed `HostActualState`. Values are `Pending`, `Ready`, `Draining`, and `Drained`. — `may/api/v1alpha1/host_types.go` → `HostActualState`
`Drain` and `Halted` are accepted by the API but implemented by no current driver. — `may/api/v1alpha1/host_types.go` → `HostCoreSpec`; `drivers/incluster/internal/controller/statichost_controller.go` → `Reconcile`
The Runner reason `Stopped` is only read. `SetNotReady` has no caller, so `status.runners.stopped` stays `0`. — `may/pkg/runner/runner.go` → `ConditionReasonStopped`, `SetNotReady`

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

Registration order is the scheduler setup (`claim` controller), provisioner, binder, claimer, gater, and the Pod webhook when `ENABLE_WEBHOOKS != "false"`. Field indexers are registered after that. — `may/cmd/main.go` → `main`
Field indexers are registered by `may/pkg/indexer/indexers.go` → `SetupFieldIndexers`.
The provisioner's seven reconcilers are registered from a map in `may/internal/controller/provisioner/provisioner_controllers.go` → `SetupWithManager`. Their order relative to each other is not deterministic.

On `Runner`, `status.ready` is registered but has no caller today. `status.is-ready` drives `listFreeRunners`. `spec.inUseBy` drives `listFreeRunners` with value `not-in-use` and `findAlreadyReserved` with value `in-use`. — `may/pkg/indexer/indexers.go` → `SetupFieldIndexers`, `may/pkg/runner/runner.go` → `FieldStatusReady`, `FieldIsStatusReady`, `FieldSpecInUseBy`
On `Runner`, `spec.inUseBy.name` and `spec.inUseBy.namespace` drive `ensureRunnersAreDeleted` in the Claim finalizer. — `may/internal/controller/scheduler/claim_controller.go` → `ensureRunnersAreDeleted`, `may/pkg/runner/runner.go` → `FieldSpecInUseByName`, `FieldSpecInUseByNamespace`
On `Claim`, `status.conditions[claimed]` drives `retrieveData` in the DynamicHost provisioner. — `may/internal/controller/provisioner/dynamichost_provisioner.go` → `retrieveData`, `may/pkg/claim/claim.go` → `FieldStatusConditionClaimed`
The Provisioner ignores any Runner whose `may.konflux-ci.dev/runner-type` label is missing or outside `{static, dynamic}`. — `may/internal/controller/provisioner/runner/runner_controller.go` → `SetupWithManager` predicate

## 5. End-to-End Trace

1. A tenant namespace labelled `konflux-ci.dev/type=tenant` hosts a Pod carrying `kueue.konflux-ci.dev/requests-<flavor>`. Demo source: `demo/dependencies/tekton-kueue/config/config.yaml`.
2. Pod CREATE runs `mpod-v1.kb.io`. It extracts the flavor, skips excluded flavors, and appends the gate `may.konflux-ci.dev/scheduling`. — `may/internal/webhook/v1/pod_webhook.go` → `Default`
3. Claimer creates a Claim named after the Pod. It copies the `tekton.dev/pipeline` label and sets the controller reference. — `may/internal/controller/claimer/claimer_controller.go` → `ensureClaimExists`
4. ClaimReconciler adds `may.konflux-ci.dev/reservation-controller`. It sets `Claimed=False` with reason `Pending`. — `may/internal/controller/scheduler/claim_controller.go` → `Reconcile`; `may/pkg/claim/claim.go` → `SetToSchedule`
5. Scheduler lists Runners that are `status.is-ready=true` and `spec.inUseBy=not-in-use` inside `WATCH_NAMESPACE`. It picks the first whose `spec.resources` contains the flavor key. It writes `spec.inUseBy` and the finalizer. — `may/pkg/scheduler/scheduler.go` → `Schedule`, `findBestRunner`
6. ClaimReconciler sets `Claimed=True` with reason `Scheduled`. — `may/internal/controller/scheduler/claim_controller.go` → `setClaimed`
7. Binder POSTs the Runner Secret's `id_rsa` to `https://multi-platform-otp-server.<namespace>.svc.cluster.local/store-key`. It creates the workload Secret named after the Claim with keys `otp-ca`, `otp`, `otp-server`, `host`, and `user-dir`. — `may/internal/controller/binder/runnerbinder_controller.go` → `Reconcile`
8. Gater removes the scheduling gate once the Claim is `Claimed`. Kueue can then admit the workload and the kubelet can schedule the Pod. — `may/internal/controller/gater/gater_controller.go` → `ensurePodCanBeScheduled`
9. Provisioner drives the Runner through `Initializing`, provisioning hooks, and `Ready`. When `spec.queue` is set, it creates a `ClusterQueue` named after the Runner. — `may/internal/controller/provisioner/runner/runner_controller.go` → `Reconcile`, `ensureClusterQueueExists`
10. Pod reaches `Succeeded` or `Failed`. The Claim is deleted, and its finalizer deletes linked Runners. The Runner finalizer runs cleanup hooks, deletes the ClusterQueue, and releases the Runner. — `may/internal/controller/scheduler/claim_controller.go` → `deleteIfNeeded`, `ensureRunnersAreDeleted`; `may/internal/controller/provisioner/runner/runner_controller.go` → `finalize`
11. Dynamic variant: a host is provisioned for the pending Claim instead of using an existing Runner. — `may/internal/controller/provisioner/dynamichost_provisioner.go` → `processClaims`

DynamicHostProvisioner lists Pending Claims through the `status.conditions[claimed]` index. It matches a `DynamicHostAutoscaler` on `spec.flavor` and creates one `DynamicHost` named after the Claim in the controller namespace. — `may/internal/controller/provisioner/dynamichost_provisioner.go` → `processClaims`, `createDynamicHost`
The driver moves `status.state` to `Ready`. — `drivers/incluster/internal/controller/dynamichost_controller.go` → `ensureReady`
The core then creates exactly one Runner carrying the host's name. It adds the labels `may.konflux-ci.dev/host` and `may.konflux-ci.dev/runner-type=dynamic`. — `may/internal/controller/provisioner/dynamichost_controller.go` → `ensureRunnerExists`
`dynamichost-gc` deletes the host once `status.state` is `Drained`. — `may/internal/controller/provisioner/dynamichost_gc.go` → `Reconcile`
Executable anchors: `may/test/e2e/full_flow_test.go` → `FullFlowContexts` (labelled `smoke`), `may/test/e2e/dynamichost_test.go`, and `may/test/e2e/dynamichostautoscaler_test.go`. Demo manifests: `demo/config/static/hosts/arm64/aws-host-arm64.yaml` and `demo/config/dynamic/hostautoscaler/amd64/aws-host-amd64.yaml`. Apply them with the commands in `README.md`.

## 6. Driver Contract and Adding a Driver

A driver owns `StaticHost` and `DynamicHost` reconciliation and writes `status.state`. The core creates and deletes Runners. — `may/internal/controller/provisioner/statichost_controller.go` → `Reconcile`; `drivers/incluster/internal/controller/statichost_controller.go` → `Reconcile`; `drivers/incluster/internal/controller/dynamichost_controller.go` → `Reconcile`
The core waits while `status.state` is nil or `Pending`. — `may/internal/controller/provisioner/statichost_controller.go` → `Reconcile`; `may/internal/controller/provisioner/dynamichost_controller.go` → `Reconcile`
A driver sets `status.state=Ready` once its host resources exist. The incluster driver treats resource creation as acceptance and does not probe runner availability. A production driver should probe acceptance before setting Ready. — `drivers/incluster/internal/controller/statichost_controller.go` → `ensureReady`; `drivers/incluster/internal/controller/dynamichost_controller.go` → `ensureReady`
A driver adds its own finalizer and removes it after its resources are gone. The core's `may.konflux-ci.dev/host-controller` finalizer is separate and independently managed. — `drivers/incluster/internal/controller/statichost_controller.go` → `InClusterDriverFinalizer`, `finalize`; `may/pkg/constants/label.go` → `HostControllerFinalizer`
Requested state is `spec.status`. Observed state is `status.state`. The core reacts to observed `Draining` and `Drained`. — `may/internal/controller/provisioner/statichost_controller.go` → `ensureHostIsDraining`, `ensureHostIsDrained`
`drivers/incluster` is the reference implementation. It is for demo and development only. It creates one Secret, Pod, and Service per host and reaches runners over SSH. — `drivers/incluster/internal/controller/statichost_controller.go` → `ensureResourcesExists`, `ensureKeySecretExists`, `ensurePodExists`, `ensureServiceExists`
AWS has a working EC2 client: `drivers/aws/internal/ec2/ec2.go` → `LaunchInstance`, `DescribeInstance`, `SSHReady`, `TerminateInstance`. `drivers/aws/internal/client/client.go` → `NewStaticEC2Client`, `NewDynamicEC2Client`. The Host reconcilers in `drivers/aws/internal/controller/` are kubebuilder scaffolds. They import neither package, so no EC2 call happens during reconciliation.
IBM's `drivers/ibm/internal/controller/*` are scaffolds only. There is no provider client in the module.
End-to-end drain is not wired. Do not document it as working. — `drivers/incluster/internal/controller/statichost_controller.go` → `Reconcile`; `drivers/aws/internal/controller/statichost_controller.go` → `Reconcile`; `drivers/ibm/internal/controller/statichost_controller.go` → `Reconcile`
The reason: the CRD enum accepts `Drain`, but the incluster `Reconcile` switches on `h.Spec.Status` with cases only for `HostStatusPending`, `HostStatusReady`, and `HostStatusDrained`. `HostStatusDrained` is the string `"Drained"`, which the enum rejects. Its branch is `panic("to be implemented")`. A requested `Drain` therefore falls through to a no-op. — `drivers/incluster/internal/controller/statichost_controller.go` → `Reconcile`

Copy `drivers/incluster` as the starting point, then:
1. Create a new module under `drivers/<name>/` with its own `go.mod`, `Makefile`, `Dockerfile`, and `AGENTS.md`.
2. Add an entry point `cmd/main.go` under the module root. Register the manager and reconcilers.
3. Add `StaticHostReconciler` and `DynamicHostReconciler` in the module's `internal/controller/`.
4. Implement the status contract: initialize `status.state=Pending`. Set `Ready` once host resources exist. A production driver should probe acceptance first.
5. Add your own finalizer `drivers.may.konflux-ci.dev/<name>`. Remove it after provider resources are gone.
6. Add RBAC through `+kubebuilder:rbac` markers. Run `make -C drivers/<name> manifests`.
7. Add an envtest suite under `internal/controller/` and a Kind e2e suite under `test/e2e/`.
8. Document the module in its `AGENTS.md`. Add it to the module table in this guide.

## 7. Development and Test Topology

| Module | test | e2e | other |
| --- | --- | --- | --- |
| `may` | `make -C may test` | `make -C may test-e2e`, `make -C may test-e2e-smoke` | `lint`, `lint-fix`, `manifests`, `generate`, `run`, `cleanup-test-e2e` |
| `drivers/incluster` | `make -C drivers/incluster test` | `make -C drivers/incluster test-e2e` | `lint`, `lint-fix`, `manifests`, `generate`, `run` |
| `drivers/ibm` | `make -C drivers/ibm test` | `make -C drivers/ibm test-e2e` | `lint`, `lint-fix`, `manifests`, `generate`, `run` |
| `drivers/aws` | `make -C drivers/aws test` | `make -C drivers/aws test-e2e-aws` (needs AWS credentials) | `lint`, `lint-fix`, `manifests`, `generate`, `run` |

`test-e2e-smoke` exists only in `may/Makefile`.
`make test` depends on `manifests generate fmt vet setup-envtest`. It then runs `go test ./... -coverprofile cover.out -covermode atomic`. Regenerated CRD, RBAC, and deepcopy diffs after a run are expected, not a failure. — `may/Makefile` → `test`
Unit tests use envtest with Ginkgo in the controller packages' `suite_test.go`. `may/internal/webhook/v1/webhook_suite_test.go` and `may/pkg/pod/excluded_test.go` bootstrap Ginkgo without envtest.
The `may`, `drivers/incluster`, and `drivers/ibm` e2e suites are behind the `e2e` build tag. The `drivers/aws` suite uses `e2e_aws`. — `drivers/aws/Makefile` → `test-e2e-aws`
The `may` suite runs against an isolated Kind cluster. — `may/Makefile` → `KIND_CLUSTER ?= may-test-e2e`, `E2E_TEST_TIMEOUT ?= 30m`
Tear the cluster down with `make -C may cleanup-test-e2e`. Select the smoke subset with `-ginkgo.label-filter "smoke"`.
`./demo/hack/setup-cluster.sh` builds the Kind cluster `mpc-v2-poc` with cert-manager, Tekton, Kueue, tekton-kueue, MAY, and the incluster driver.
Unit and e2e coverage is uneven across packages. This guide makes no claim that coverage is sufficient.
As of 2026-11-25, no performance test layer exists. Performance test design is backlogged and scheduled to start soon. This is a current gap, not a permanent non-goal. — `may/Makefile`, `drivers/*/Makefile` (no performance target)

## 8. Load-Bearing Decisions

| Decision | Enforced at | Rationale |
| --- | --- | --- |
| Claim, Runner, and Host are separate kinds. | `may/api/v1alpha1/claim_types.go`, `may/api/v1alpha1/runner_types.go`, `may/api/v1alpha1/host_types.go` | No stated rationale. |
| Core owns Runners. Drivers own Hosts. | `may/internal/controller/provisioner/statichost_controller.go` → `Reconcile` waits on `status.state` | `may/AGENTS.md`: "Host *implementation* is in `drivers/*`" |
| Flavor matches a Runner resource key, not a field. | `may/pkg/scheduler/scheduler.go` → `findBestRunner` | No stated rationale. |
| Gate is added by the webhook and removed by a separate controller. | `may/internal/webhook/v1/pod_webhook.go` → `Default`; `may/internal/controller/gater/gater_controller.go` → `ensurePodCanBeScheduled` | No stated rationale. |
| One ClusterQueue per Runner. Borrowing is disabled. | `may/internal/controller/provisioner/runner/runner_controller.go` → `ensureClusterQueue` sets `BorrowingLimit` to zero | Code comment: "disable borrowing between sibling ClusterQueues" |
| Hooks run as Pods. `CleaningFailed` Runners are retained. | `may/internal/controller/provisioner/runner/runner_controller.go` → `runNextHookPod`, `finalize` returns early on `CleaningFailed` | `README.md`: "so the failure is not lost" |
| Credentials are handed over as an OTP, not a mounted key. | `may/internal/controller/binder/runnerbinder_controller.go` → `Reconcile` POSTs to `/store-key` | No stated rationale. |
| Each component is its own Go module. | `may/go.mod`, `drivers/aws/go.mod`, `drivers/ibm/go.mod`, `drivers/incluster/go.mod` | No stated rationale. |

Rows marked `No stated rationale` mean the repository carries no comment, commit message, or PR text explaining the choice. They do not mean the choice is arbitrary.

## 9. Troubleshooting

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
| Pod schedules with no gate and no Claim. | The flavor is excluded (`localhost`, `local`, `linux-x86-64`), or the namespace is not a tenant. | `may/pkg/pod/excluded.go`; `may/config/webhook/manifests_patch.yaml` |
| Controller lists cluster-wide or finds nothing. | `WATCH_NAMESPACE` is unset. Startup logs the error and continues with an empty namespace. | `may/cmd/main.go` → `main` |
| `git status` is dirty after `make test`. | `manifests generate` regenerated CRDs, RBAC, and deepcopy files. | `may/Makefile` → `test`; commit only the intended regenerated files |
| `controller-gen` fails with `directory prefix . does not contain modules listed in go.work`. | A parent workspace `go.work` does not include this module. | Prefix the command with `GOWORK=off` |

For CI-only failures, use `skills/ci-troubleshooting.md` instead of this table.

## 10. Where Truth Lives

- CRD schema: `may/api/v1alpha1/*`. Generated output is never hand-edited: `may/config/crd/bases/*`, `**/zz_generated.*`, `may/config/rbac/role.yaml`, and `PROJECT`. Regenerate with `make manifests` / `make generate`. — `may/AGENTS.md` → "Key Conventions"
- Behavior: `may/internal/controller/*`, `may/internal/webhook/v1/`, and `drivers/*/internal/controller/*`. Executable examples live in `may/test/e2e/*`.
- Commands and per-module conventions: the module's own `AGENTS.md`. Process: `skills/pr-workflow.md` and `skills/ci-troubleshooting.md`.
- Product framing: `README.md` → "Project Status" states MAY is a proof of concept. APIs and behavior may break.
- Every status statement in this guide is a 2026-11-25 snapshot of `main`. Before relying on one, re-read the cited symbol. Code wins over this guide.

Follow-ups, out of scope for this change:

- `README.md` → "Prerequisites" says `Go 1.24+`. `drivers/aws/README.md` and `drivers/ibm/README.md` say `go version v1.24.6+`. All four `go.mod` files require `1.26.0`. The driver READMEs are kubebuilder boilerplate with TODO placeholders.
- `drivers/aws/AGENTS.md` describes EC2 launch, describe, SSH probe, and terminate operations that the module's Host reconcilers do not perform, because they are still scaffolds.
- `drivers/ibm/AGENTS.md` describes instance management that the module does not implement.
