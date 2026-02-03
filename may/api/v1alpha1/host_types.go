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
)

// HostCoreSpec is the common spec shared by StaticHost and DynamicHost.
// It defines flavor, desired status, queue, and root key for runner access.
type HostCoreSpec struct {
	// +kubebuilder:validation:MaxLength=253
	// +kubebuilder:validation:Pattern="^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$"
	// +kubebuilder:validation:required
	Flavor string `json:"flavor"`

	// +optional
	Queue *RunnerQueue `json:"queue,omitempty"`

	// +kubebuilder:default:=Pending
	// +kubebuilder:validation:Enum:=Pending;Ready;Drain;Halted
	Status HostRequestedStatus `json:"status"`

	RootKey corev1.LocalObjectReference `json:"rootKey"`
}

type HostSpecRunners struct {
	Hooks *RunnerHooks `json:"hooks,omitempty"`

	// +kubebuilder:validation:Min:=0
	Instances int64 `json:"instances"`

	// +kubebuilder:validation:required
	Resources corev1.ResourceList `json:"resources"`
}

type HostHooks struct {
	Provisioning []corev1.PodTemplateSpec `json:"provisioning,omitempty"`
	Cleanup      []corev1.PodTemplateSpec `json:"cleanup,omitempty"`
}

type HostRequestedStatus string

const (
	HostStatusPending HostRequestedStatus = "Pending"
	HostStatusReady   HostRequestedStatus = "Ready"
	HostStatusDrained HostRequestedStatus = "Drained"
	// HostStatusHalted  HostRequestedStatus = "Halted"
)

type HostActualState string

const (
	HostActualStatePending  HostActualState = "Pending"
	HostActualStateReady    HostActualState = "Ready"
	HostActualStateDraining HostActualState = "Draining"
	HostActualStateDrained  HostActualState = "Drained"
	// HostActualStateHalting  HostActualState = "Halting"
	// HostActualStateHalted   HostActualState = "Halted"
)

type HostCommonStatus struct {
	State *HostActualState `json:"state,omitempty"`

	Runners HostStatusRunners `json:"runners,omitempty"`
}

type HostStatusRunners struct {
	Ready int `json:"ready"`

	Stopped int `json:"stopped"`
}
