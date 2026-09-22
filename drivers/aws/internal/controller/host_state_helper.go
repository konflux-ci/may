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
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// HostStateHelper implements shared EC2 lifecycle steps for AWS driver hosts.
type HostStateHelper struct {
	client.Client
}

// EnsurePending is a no-op. The AWS driver does not move a host back to Pending
// once it has left that state.
func (h *HostStateHelper) EnsurePending(ctx context.Context, actualState maykonfluxcidevv1alpha1.HostActualState) (ctrl.Result, error) {
	if actualState != maykonfluxcidevv1alpha1.HostActualStatePending {
		logf.FromContext(ctx).Info("host cannot move back to Pending", "actualState", actualState)
	}
	return ctrl.Result{}, nil
}

// EnsureReady launches or verifies the EC2 instance when Ready is requested.
// newEC2 is called only when an instance must be launched or health-checked,
// not for Draining or Drained. A non-nil HostActualState is a status
// transition the caller must persist with Status().Update.
func (h *HostStateHelper) EnsureReady(
	ctx context.Context,
	host client.Object,
	actualState maykonfluxcidevv1alpha1.HostActualState,
	newEC2 func(context.Context) (hostEC2Client, error),
	awsConfig func(context.Context) (internalconfig.AWSConfiguration, error),
) (ctrl.Result, *maykonfluxcidevv1alpha1.HostActualState, error) {
	switch actualState {
	case maykonfluxcidevv1alpha1.HostActualStatePending:
		ec2, err := newEC2(ctx)
		if err != nil {
			return ctrl.Result{}, nil, err
		}
		return h.EnsureInstanceReady(ctx, ec2, host, awsConfig)
	case maykonfluxcidevv1alpha1.HostActualStateDraining, maykonfluxcidevv1alpha1.HostActualStateDrained:
		// Provisioner owns drain. Spec often stays Ready (DynamicHost), so do
		// not reset actual state or the host never reaches Drained for GC.
		return ctrl.Result{}, nil, nil
	case maykonfluxcidevv1alpha1.HostActualStateReady:
		ec2, err := newEC2(ctx)
		if err != nil {
			return ctrl.Result{}, nil, err
		}
		strictPublicAddress, err := strictPublicAddressFromHost(host)
		if err != nil {
			return ctrl.Result{}, nil, err
		}
		return h.EnsureInstanceStillRunning(ctx, ec2, host, strictPublicAddress)
	default:
		return ctrl.Result{}, nil, fmt.Errorf("unsupported host actual state %q", actualState)
	}
}

// EnsureInstanceReady launches an instance if needed and waits until SSH is reachable.
// When SSH is reachable it returns HostActualStateReady for the caller to persist.
// When the instance is gone it returns HostActualStateDraining, matching EnsureInstanceStillRunning.
func (h *HostStateHelper) EnsureInstanceReady(
	ctx context.Context,
	ec2 hostEC2Client,
	host client.Object,
	awsConfig func(context.Context) (internalconfig.AWSConfiguration, error),
) (ctrl.Result, *maykonfluxcidevv1alpha1.HostActualState, error) {
	log := logf.FromContext(ctx)

	instanceID := host.GetAnnotations()[internalconfig.AnnotationInstanceID]
	if instanceID == "" {
		cfg, err := awsConfig(ctx)
		if err != nil {
			return ctrl.Result{}, nil, err
		}
		instanceID, err = ec2.LaunchInstance(ctx, cfg, string(host.GetUID()))
		if err != nil {
			return ctrl.Result{}, nil, err
		}

		log.Info("EC2 instance launched", "instanceID", instanceID)
		if err := h.SetInstanceID(ctx, host, instanceID); err != nil {
			return ctrl.Result{}, nil, err
		}
		return ctrl.Result{RequeueAfter: instancePollInterval}, nil, nil
	}

	strictPublicAddress, err := strictPublicAddressFromHost(host)
	if err != nil {
		return ctrl.Result{}, nil, err
	}
	address, ready, err := ec2.SSHReady(ctx, instanceID, strictPublicAddress)
	if err != nil {
		if ctx.Err() != nil {
			return ctrl.Result{}, nil, err
		}
		if internalec2.IsSSHProbeError(err) {
			log.Info("waiting for SSH on address", "instanceID", instanceID, "error", err)
			return ctrl.Result{RequeueAfter: instancePollInterval}, nil, nil
		}
		if internalec2.IsInstanceNotRunningError(err) {
			return instanceLost(err)
		}
		return ctrl.Result{}, nil, err
	}
	if !ready {
		if address == "" {
			log.Info("waiting for EC2 instance SSH address", "instanceID", instanceID)
		} else {
			log.Info("waiting for SSH on address", "instanceID", instanceID, "address", address)
		}
		return ctrl.Result{RequeueAfter: instancePollInterval}, nil, nil
	}

	log.Info("EC2 instance accepts SSH", "instanceID", instanceID, "address", address)
	if err := h.SetInstanceMetadata(ctx, host, instanceID, address); err != nil {
		return ctrl.Result{}, nil, err
	}
	return ctrl.Result{}, ptr.To(maykonfluxcidevv1alpha1.HostActualStateReady), nil
}

// EnsureInstanceStillRunning reports an error if a Ready host's instance is not running.
// A running instance is requeued after instanceHealthInterval so out-of-band EC2 changes are noticed.
// When the instance is gone, the returned HostActualState is Draining so callers can persist
// that transition even while returning the error. There is no Failed actual state;
// Pending would relaunch and leak, so drain lets the provisioner stop using the host.
func (h *HostStateHelper) EnsureInstanceStillRunning(ctx context.Context, ec2 hostEC2Client, host client.Object, strictPublicAddress bool) (ctrl.Result, *maykonfluxcidevv1alpha1.HostActualState, error) {
	instanceID := host.GetAnnotations()[internalconfig.AnnotationInstanceID]
	if instanceID == "" {
		return instanceLost(fmt.Errorf("host is ready but annotation %q is missing", internalconfig.AnnotationInstanceID))
	}

	instanceDetails, err := ec2.DescribeInstance(ctx, instanceID, strictPublicAddress)
	if err != nil {
		return ctrl.Result{}, nil, err
	}

	switch instanceDetails.State {
	case types.InstanceStateNameRunning:
		return ctrl.Result{RequeueAfter: instanceHealthInterval}, nil, nil
	default:
		return instanceLost(&internalec2.InstanceNotRunningError{InstanceID: instanceID, State: instanceDetails.State})
	}
}

func instanceLost(err error) (ctrl.Result, *maykonfluxcidevv1alpha1.HostActualState, error) {
	return ctrl.Result{}, ptr.To(maykonfluxcidevv1alpha1.HostActualStateDraining), err
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

	// Address is unused here; skip annotation parsing so a bad
	// strict-public-address value cannot block termination.
	instanceDetails, err := ec2.DescribeInstance(ctx, instanceID, false)
	if err != nil {
		return ctrl.Result{}, false, err
	}

	log := logf.FromContext(ctx)
	switch instanceDetails.State {
	case types.InstanceStateNameTerminated:
		return ctrl.Result{}, true, nil
	case types.InstanceStateNameShuttingDown:
		log.Info("waiting for EC2 instance termination", "instanceID", instanceID)
		return ctrl.Result{RequeueAfter: instancePollInterval}, false, nil
	default:
		if err := ec2.TerminateInstance(ctx, instanceID); err != nil {
			return ctrl.Result{}, false, err
		}
		log.Info("EC2 instance termination requested", "instanceID", instanceID)
		return ctrl.Result{RequeueAfter: instancePollInterval}, false, nil
	}
}

// Finalize terminates the instance if needed and removes the AWS driver finalizer.
// Other controllers may still need the instance (drain, unregister). Stay last:
// wait until only this driver's finalizer remains.
func (h *HostStateHelper) Finalize(ctx context.Context, host client.Object, newEC2 func(context.Context) (hostEC2Client, error)) (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(host, AWSDriverFinalizer) {
		return ctrl.Result{}, nil
	}
	if len(host.GetFinalizers()) > 1 {
		return ctrl.Result{}, nil
	}

	if host.GetAnnotations()[internalconfig.AnnotationInstanceID] == "" {
		return ctrl.Result{}, h.RemoveFinalizer(ctx, host)
	}

	ec2, err := newEC2(ctx)
	if err != nil {
		return ctrl.Result{}, err
	}

	result, done, err := h.EnsureInstanceTerminated(ctx, ec2, host)
	if err != nil || !done {
		return result, err
	}

	return ctrl.Result{}, h.RemoveFinalizer(ctx, host)
}

// RemoveFinalizer drops the AWS driver finalizer from the host.
func (h *HostStateHelper) RemoveFinalizer(ctx context.Context, host client.Object) error {
	if controllerutil.RemoveFinalizer(host, AWSDriverFinalizer) {
		return h.Update(ctx, host)
	}
	return nil
}

// SetInstanceMetadata patches the instance ID and SSH address onto the host.
func (h *HostStateHelper) SetInstanceMetadata(ctx context.Context, host client.Object, instanceID, address string) error {
	return h.patchAnnotations(ctx, host, map[string]string{
		internalconfig.AnnotationInstanceID: instanceID,
		internalconfig.AnnotationSSHAddress: address,
	})
}

// SetInstanceID patches the instance ID onto the host.
func (h *HostStateHelper) SetInstanceID(ctx context.Context, host client.Object, instanceID string) error {
	return h.patchAnnotations(ctx, host, map[string]string{
		internalconfig.AnnotationInstanceID: instanceID,
	})
}

func (h *HostStateHelper) patchAnnotations(ctx context.Context, host client.Object, values map[string]string) error {
	base := host.DeepCopyObject().(client.Object)
	patch := client.MergeFrom(base)
	annotations := host.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}
	for k, v := range values {
		annotations[k] = v
	}
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
