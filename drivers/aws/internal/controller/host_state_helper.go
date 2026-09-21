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

package controller

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	maykonfluxcidevv1alpha1 "github.com/konflux-ci/may/api/v1alpha1"
	internalconfig "github.com/konflux-ci/may/drivers/aws/internal/config"
	internalec2 "github.com/konflux-ci/may/drivers/aws/internal/ec2"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

type hostEC2Client interface {
	LaunchInstance(ctx context.Context, cfg internalconfig.AWSConfiguration, clientToken string) (string, error)
	DescribeInstance(ctx context.Context, instanceID string, strictPublicAddress bool) (internalec2.InstanceDetails, error)
	SSHReady(ctx context.Context, instanceID string, strictPublicAddress bool) (string, bool, error)
	TerminateInstance(ctx context.Context, instanceID string) error
}

// HostStateHelper implements shared EC2 lifecycle steps for AWS driver hosts.
type HostStateHelper struct {
	client.Client
}

// EnsurePending is a no-op when the host is already Pending.
func (h *HostStateHelper) EnsurePending(ctx context.Context, actualState maykonfluxcidevv1alpha1.HostActualState) (ctrl.Result, error) {
	l := logf.FromContext(ctx)
	switch actualState {
	case maykonfluxcidevv1alpha1.HostActualStatePending:
		l.Info("host is already Pending")
		return ctrl.Result{}, nil
	default:
		l.Info("host cannot move back to Pending", "actualState", actualState)
		return ctrl.Result{}, nil
	}
}

// EnsureReady launches or verifies the EC2 instance when Ready is requested.
// A non-nil HostActualState is a status transition the caller must persist
// with Status().Update.
func (h *HostStateHelper) EnsureReady(
	ctx context.Context,
	ec2 hostEC2Client,
	host client.Object,
	actualState maykonfluxcidevv1alpha1.HostActualState,
	awsConfig func(context.Context) (internalconfig.AWSConfiguration, error),
) (ctrl.Result, *maykonfluxcidevv1alpha1.HostActualState, error) {
	switch actualState {
	case maykonfluxcidevv1alpha1.HostActualStatePending:
		return h.EnsureInstanceReady(ctx, ec2, host, awsConfig)
	case maykonfluxcidevv1alpha1.HostActualStateDraining, maykonfluxcidevv1alpha1.HostActualStateDrained:
		// Spec Ready after drain: reset to Pending so the next reconcile can provision.
		return ctrl.Result{}, ptr.To(maykonfluxcidevv1alpha1.HostActualStatePending), nil
	case maykonfluxcidevv1alpha1.HostActualStateReady:
		cfg, err := awsConfig(ctx)
		if err != nil {
			return ctrl.Result{}, nil, err
		}
		result, err := h.EnsureInstanceStillRunning(ctx, ec2, host, cfg.StrictPublicAddress)
		return result, nil, err
	default:
		return ctrl.Result{}, nil, fmt.Errorf("unsupported host actual state %q", actualState)
	}
}

// EnsureInstanceReady launches an instance if needed and waits until SSH is reachable.
// When SSH is reachable it returns HostActualStateReady for the caller to persist.
func (h *HostStateHelper) EnsureInstanceReady(
	ctx context.Context,
	ec2 hostEC2Client,
	host client.Object,
	awsConfig func(context.Context) (internalconfig.AWSConfiguration, error),
) (ctrl.Result, *maykonfluxcidevv1alpha1.HostActualState, error) {
	l := logf.FromContext(ctx)

	cfg, err := awsConfig(ctx)
	if err != nil {
		return ctrl.Result{}, nil, err
	}

	instanceID := host.GetAnnotations()[internalconfig.AnnotationInstanceID]
	if instanceID == "" {
		instanceID, err = ec2.LaunchInstance(ctx, cfg, string(host.GetUID()))
		if err != nil {
			return ctrl.Result{}, nil, err
		}

		l.Info("EC2 instance launched", "instanceID", instanceID)
		if err := h.SetInstanceID(ctx, host, instanceID); err != nil {
			return ctrl.Result{}, nil, err
		}
		return ctrl.Result{RequeueAfter: instancePollInterval}, nil, nil
	}

	address, ready, err := ec2.SSHReady(ctx, instanceID, cfg.StrictPublicAddress)
	if err != nil {
		if ctx.Err() != nil {
			return ctrl.Result{}, nil, err
		}
		if internalec2.IsSSHProbeError(err) {
			l.Info("waiting for SSH on address", "instanceID", instanceID, "error", err)
			return ctrl.Result{RequeueAfter: instancePollInterval}, nil, nil
		}
		return ctrl.Result{}, nil, err
	}
	if !ready {
		if address == "" {
			l.Info("waiting for EC2 instance SSH address", "instanceID", instanceID)
		} else {
			l.Info("waiting for SSH on address", "instanceID", instanceID, "address", address)
		}
		return ctrl.Result{RequeueAfter: instancePollInterval}, nil, nil
	}

	l.Info("EC2 instance accepts SSH", "instanceID", instanceID, "address", address)
	if err := h.SetInstanceMetadata(ctx, host, instanceID, address); err != nil {
		return ctrl.Result{}, nil, err
	}
	return ctrl.Result{}, ptr.To(maykonfluxcidevv1alpha1.HostActualStateReady), nil
}

// EnsureInstanceStillRunning reports an error if a Ready host's instance is not running.
// A running instance is requeued after instanceHealthInterval so out-of-band EC2 changes are noticed.
func (h *HostStateHelper) EnsureInstanceStillRunning(ctx context.Context, ec2 hostEC2Client, host client.Object, strictPublicAddress bool) (ctrl.Result, error) {
	instanceID := host.GetAnnotations()[internalconfig.AnnotationInstanceID]
	if instanceID == "" {
		return ctrl.Result{}, fmt.Errorf("host is Ready but annotation %q is missing", internalconfig.AnnotationInstanceID)
	}

	instanceDetails, err := ec2.DescribeInstance(ctx, instanceID, strictPublicAddress)
	if err != nil {
		return ctrl.Result{}, err
	}

	switch instanceDetails.State {
	case types.InstanceStateNameRunning:
		return ctrl.Result{RequeueAfter: instanceHealthInterval}, nil
	case types.InstanceStateNameShuttingDown, types.InstanceStateNameTerminated:
		return ctrl.Result{}, fmt.Errorf("EC2 instance %s is %s", instanceID, instanceDetails.State)
	default:
		return ctrl.Result{}, fmt.Errorf("EC2 instance %s is %s and is not running", instanceID, instanceDetails.State)
	}
}

// EnsureInstanceTerminated drives EC2 termination during host deletion.
// The returned bool is true when the controller may remove its finalizer.
func (h *HostStateHelper) EnsureInstanceTerminated(ctx context.Context, ec2 hostEC2Client, host client.Object) (ctrl.Result, bool, error) {
	instanceID := host.GetAnnotations()[internalconfig.AnnotationInstanceID]
	if instanceID == "" {
		return ctrl.Result{}, true, nil
	}
	if ec2 == nil {
		return ctrl.Result{}, false, fmt.Errorf("EC2 client is required to terminate instance %s", instanceID)
	}

	strictPublicAddress, err := strictPublicAddressFromHost(host)
	if err != nil {
		return ctrl.Result{}, false, err
	}
	instanceDetails, err := ec2.DescribeInstance(ctx, instanceID, strictPublicAddress)
	if err != nil {
		return ctrl.Result{}, false, err
	}

	switch instanceDetails.State {
	case types.InstanceStateNameTerminated:
		return ctrl.Result{}, true, nil
	case types.InstanceStateNameShuttingDown:
		logf.FromContext(ctx).Info("waiting for EC2 instance termination", "instanceID", instanceID)
		return ctrl.Result{RequeueAfter: instancePollInterval}, false, nil
	default:
		if err := ec2.TerminateInstance(ctx, instanceID); err != nil {
			return ctrl.Result{}, false, err
		}
		logf.FromContext(ctx).Info("terminating EC2 instance", "instanceID", instanceID)
		return ctrl.Result{RequeueAfter: instancePollInterval}, false, nil
	}
}

// SetInstanceMetadata patches the instance ID and SSH address onto the host.
func (h *HostStateHelper) SetInstanceMetadata(ctx context.Context, host client.Object, instanceID, address string) error {
	base := host.DeepCopyObject().(client.Object)
	patch := client.MergeFrom(base)
	annotations := host.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}
	annotations[internalconfig.AnnotationInstanceID] = instanceID
	annotations[internalconfig.AnnotationSSHAddress] = address
	host.SetAnnotations(annotations)
	return h.Patch(ctx, host, patch)
}

// SetInstanceID patches the instance ID onto the host.
func (h *HostStateHelper) SetInstanceID(ctx context.Context, host client.Object, instanceID string) error {
	base := host.DeepCopyObject().(client.Object)
	patch := client.MergeFrom(base)
	annotations := host.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}
	annotations[internalconfig.AnnotationInstanceID] = instanceID
	host.SetAnnotations(annotations)
	return h.Patch(ctx, host, patch)
}

func strictPublicAddressFromHost(host client.Object) (bool, error) {
	v, ok := host.GetAnnotations()[internalconfig.AnnotationStrictPublicAddress]
	if !ok {
		return false, nil
	}
	return internalconfig.ParseBool(internalconfig.AnnotationStrictPublicAddress, v)
}
