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

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RunnerSpec defines the desired state of Runner.
// A Runner is a build environment with a given flavor and resources; it may be reserved for a Claim (inUseBy).
type RunnerSpec struct {
	// +kubebuilder:validation:MaxLength=253
	// +kubebuilder:validation:Pattern="^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$"
	// +kubebuilder:validation:required
	Flavor string `json:"flavor"`

	// +kubebuilder:validation:required
	Resources corev1.ResourceList `json:"resources"`

	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="Value is immutable"
	InUseBy *ClaimReference `json:"inUseBy,omitempty"`

	// +optional
	Queue *RunnerQueue `json:"queue"`
	// Hooks
	Hooks *RunnerHooks `json:"hooks,omitempty"`
}

type RunnerQueue struct {
	// +kubebuilder:validation:MaxLength=253
	// +kubebuilder:validation:Pattern="^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$"
	// +kubebuilder:validation:required
	Cohort string `json:"cohort"`
}

type RunnerHooks struct {
	Provisioning []RunnerHookPodTemplateSpec `json:"provisioning,omitempty"`
	Cleanup      []RunnerHookPodTemplateSpec `json:"cleanup,omitempty"`
}

type RunnerHookPodTemplateSpec struct {
	Name     string                 `json:"name"`
	Template corev1.PodTemplateSpec `json:"template"`
}

// RunnerStatus defines the observed state of Runner.
type RunnerStatus struct {
	// For Kubernetes API conventions, see:
	// https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#typical-status-properties

	// conditions represent the current state of the Runner resource.
	// Each condition has a unique type and reflects the status of a specific aspect of the resource.
	//
	// Standard condition types include:
	// - "Available": the resource is fully functional
	// - "Progressing": the resource is being created or updated
	// - "Degraded": the resource failed to reach or maintain its desired state
	//
	// The status of each condition is one of True, False, or Unknown.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	HooksStatus RunnerHooksStatus `json:"hooksStatus,omitempty"`
}

type RunnerHooksStatus struct {
	// +listType=map
	// +listMapKey=hook
	// +optional
	Provisioning []RunnerHookStatus `json:"provisioning,omitempty"`

	// +listType=map
	// +listMapKey=hook
	// +optional
	Cleanup []RunnerHookStatus `json:"cleanup,omitempty"`
}

type RunnerHookStatus struct {
	Hook  string          `json:"hook"`
	Phase corev1.PodPhase `json:"phase"`
	Pod   string          `json:"pod"`
	// +optional
	PodMessage string `json:"podMessage"`
	// +optional
	DeletionTimestamp *metav1.Time `json:"deletionTimestamp"`
}

type ClaimReference struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Reserved For Namespace",type="string",JSONPath=`.spec.inUseBy.namespace`,description="namespace of the claim reserving the runner"
// +kubebuilder:printcolumn:name="Reserved For Name",type="string",JSONPath=`.spec.inUseBy.name`,description="name of the claim reserving the runner"
// +kubebuilder:printcolumn:name="State",type="string",JSONPath=`.status.conditions[?(@.type=="Ready")].reason`,description="Reason for Runner Ready state"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// Runner is the Schema for the runners API.
type Runner struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RunnerSpec   `json:"spec,omitempty"`
	Status RunnerStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// RunnerList contains a list of Runner.
type RunnerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Runner `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Runner{}, &RunnerList{})
}
