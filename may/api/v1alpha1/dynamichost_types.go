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

// DynamicHostSpec defines the desired state of DynamicHost
type DynamicHostSpec struct {
	HostCoreSpec `json:",inline"`

	Runner HostSpecRunner `json:"runner"`
}

type HostSpecRunner struct {
	Hooks *RunnerHooks `json:"hooks,omitempty"`

	// +kubebuilder:validation:required
	Resources corev1.ResourceList `json:"resources"`
}

// DynamicHostStatus defines the observed state of DynamicHost.
type DynamicHostStatus struct {
	HostCommonStatus `json:",inline"`

	Pipeline string `json:"pipeline,omitempty"`

	// conditions represent the current state of the DynamicHost resource.
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
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// DynamicHost is the Schema for the dynamichosts API
type DynamicHost struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty,omitzero"`

	// spec defines the desired state of DynamicHost
	// +required
	Spec DynamicHostSpec `json:"spec"`

	// status defines the observed state of DynamicHost
	// +optional
	Status DynamicHostStatus `json:"status,omitempty,omitzero"`
}

// +kubebuilder:object:root=true

// DynamicHostList contains a list of DynamicHost
type DynamicHostList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DynamicHost `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DynamicHost{}, &DynamicHostList{})
}
