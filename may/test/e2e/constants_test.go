//go:build e2e
// +build e2e

/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package e2e

// Manager / infrastructure (used in e2e_test.go)
const (
	// namespace is where the may controller is deployed
	namespace = "may-system"
	// serviceAccountName is the controller's service account
	serviceAccountName = "may-controller-manager"
	// metricsServiceName is the metrics service of the controller
	metricsServiceName = "may-controller-manager-metrics-service"
	// metricsRoleBindingName is the RBAC for metrics access
	metricsRoleBindingName = "may-metrics-binding"
)

// Scheduler test context
const (
	schedulerTestNamespace = "e2e-scheduler"
	schedulerFlavor        = "e2e-scheduler-flavor"
	noRunnerFlavor         = "e2e-no-runner-flavor"
)

// Gater test context
const (
	gaterTestNamespace  = "e2e-gater"
	gaterFlavor         = "e2e-gater-flavor"
	gaterNoRunnerFlavor = "e2e-gater-no-runner"
)

// Runner Lifecycle test context
// Runner type label values are not in pkg/constants (they live in internal/provisioner).
const (
	runnerLifecycleNamespace = "e2e-runner-lifecycle"
	runnerLifecycleFlavor    = "e2e-runner-lifecycle-flavor"
	runnerTypeStatic         = "static"
)

// Claimer test context
const (
	claimerTestNamespace = "e2e-claimer"
)

// Pod Webhook test context
const (
	webhookTestNamespace = "e2e-pod-webhook"
)

// Binder test context
const (
	binderTestNamespace = "e2e-binder"
	binderFlavor        = "e2e-binder-flavor"
	binderRunnerUser    = "builder"
)

// Full Flow test context
const (
	fullFlowTestNamespace = "e2e-full-flow"
	fullFlowFlavor        = "e2e-full-flow-flavor"
	fullFlowRunnerUser    = "builder"
)

// StaticHost test context
const (
	statichostTestNamespace = "e2e-statichost"
	statichostFlavor        = "e2e-statichost-flavor"
	statichostRootKeyName   = "e2e-statichost-root-key"
	// Pipeline label values for "status reflects pipelines" (tekton.dev/pipeline)
	statichostPipelineA = "e2e-statichost-pipeline-a"
	statichostPipelineB = "e2e-statichost-pipeline-b"
)

// DynamicHost test context (one Runner per host, same name as DynamicHost)
const (
	dynamichostTestNamespace = "e2e-dynamichost"
	dynamichostFlavor        = "e2e-dynamichost-flavor"
	dynamichostRootKeyName   = "e2e-dynamichost-root-key"
	dynamichostPipeline      = "e2e-dynamichost-pipeline"
	// e2eTestFinalizer is added to DynamicHost in "Runner is deleted" tests so the host
	// is not deleted before we assert Draining/Drained; removed in AfterAll or in "removes finalizer" test.
	e2eTestFinalizer = "e2e-test-finalizer"
)

// DynamicHostAutoscaler test context (provisioner creates DynamicHost per Pending Claim when Autoscaler matches flavor)
const (
	autoscalerTestNamespace = "e2e-autoscaler"
	autoscalerFlavor        = "e2e-autoscaler-flavor"
	autoscalerRootKeyName   = "e2e-autoscaler-root-key"
	autoscalerName          = "e2e-autoscaler"
	// autoscalerNoMatchFlavor is a flavor with no DynamicHostAutoscaler; used to assert no DynamicHost is created.
	autoscalerNoMatchFlavor = "e2e-autoscaler-no-match"
)
