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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// ClaimSpec defines the desired state of Claim.
// A Claim represents a request for a Runner: it is tied to a workload (e.g. a Tekton Task's Pod)
// and specifies the flavor (e.g. amd64, arm64) required.
type ClaimSpec struct {
	// For identifies the workload this claim is for (e.g. the Pod that will use the runner).
	For ForReference `json:"for"`

	// Flavor is the runner flavor required (e.g. amd64, arm64). Must match a Runner's flavor.
	// +kubebuilder:validation:MaxLength=253
	// +kubebuilder:validation:Pattern="^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$"
	// +kubebuilder:validation:required
	Flavor string `json:"flavor"`
}

// ForReference identifies a Resource in the same namespace.
type ForReference struct {
	Name       string    `json:"name"`
	Kind       string    `json:"kind"`
	APIVersion string    `json:"apiVersion"`
	UID        types.UID `json:"uid"`
}

// ClaimStatus defines the observed state of Claim.
type ClaimStatus struct {
	// For Kubernetes API conventions, see:
	// https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#typical-status-properties

	// conditions represent the current state of the Claim resource.
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
// +kubebuilder:printcolumn:name="Flavor",type="string",JSONPath=`.spec.flavor`,description="Flavor"
// +kubebuilder:printcolumn:name="Claimed",type="string",JSONPath=`.status.conditions[?(@.type=="Claimed")].status`,description="Claimed"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// Claim represents a request for a Runner, tied to a workload (e.g. a Tekton Task's Pod).
// The Claimer creates Claims for Pods in tenant namespaces; the Scheduler assigns them to Runners.
type Claim struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ClaimSpec   `json:"spec,omitempty"`
	Status ClaimStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ClaimList contains a list of Claim.
type ClaimList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Claim `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Claim{}, &ClaimList{})
}
