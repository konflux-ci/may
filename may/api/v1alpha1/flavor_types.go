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
)

// --- Option 3: Flavor CRD ---

// FlavorSpec defines the desired state of Flavor.
type FlavorSpec struct {
	// +kubebuilder:default:=false
	Local bool `json:"local"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="Local",type="boolean",JSONPath=`.spec.local`,description="Local"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// Flavor declares per-flavor behavior for MAY.
// The metadata.name must match the flavor string used in pod annotations.
type Flavor struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec FlavorSpec `json:"spec,omitempty"`
}

// +kubebuilder:object:root=true

// FlavorList contains a list of Flavor.
type FlavorList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Flavor `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Flavor{}, &FlavorList{})
}
