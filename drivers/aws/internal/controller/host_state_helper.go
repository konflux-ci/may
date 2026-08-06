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
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

type hostEC2Client interface {
	LaunchInstance(ctx context.Context, cfg internalconfig.AWSConfiguration) (string, error)
	DescribeInstance(ctx context.Context, instanceID string) (internalec2.InstanceDetails, error)
	SSHReadyOnPublicIP(ctx context.Context, instanceID string) (publicIP string, ready bool, err error)
	TerminateInstance(ctx context.Context, instanceID string) error
}

// HostStateHelper implements shared EC2 lifecycle steps for AWS driver hosts.
type HostStateHelper struct {
	client.Client
}

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

func (h *HostStateHelper) EnsureReady(
	ctx context.Context,
	ec2 hostEC2Client,
	host client.Object,
	actualState maykonfluxcidevv1alpha1.HostActualState,
	awsConfig func(context.Context) (internalconfig.AWSConfiguration, error),
	statusState **maykonfluxcidevv1alpha1.HostActualState,
) (ctrl.Result, error) {
	l := logf.FromContext(ctx).WithValues("actualState", actualState)

	switch actualState {
	case maykonfluxcidevv1alpha1.HostActualStatePending:
		return h.EnsureInstanceReady(ctx, ec2, host, awsConfig, statusState)
	case maykonfluxcidevv1alpha1.HostActualStateReady:
		return h.EnsureInstanceStillRunning(ctx, ec2, host)
	default:
		l.Info("actual state not implemented")
		return ctrl.Result{}, nil
	}
}

func (h *HostStateHelper) EnsureInstanceReady(
	ctx context.Context,
	ec2 hostEC2Client,
	host client.Object,
	awsConfig func(context.Context) (internalconfig.AWSConfiguration, error),
	statusState **maykonfluxcidevv1alpha1.HostActualState,
) (ctrl.Result, error) {
	l := logf.FromContext(ctx)

	instanceID := host.GetAnnotations()[internalconfig.AnnotationInstanceID]
	if instanceID == "" {
		cfg, err := awsConfig(ctx)
		if err != nil {
			return ctrl.Result{}, err
		}

		instanceID, err = ec2.LaunchInstance(ctx, cfg)
		if err != nil {
			return ctrl.Result{}, err
		}

		l.Info("EC2 instance launched", "instanceID", instanceID)
		if err := h.SetInstanceID(ctx, host, instanceID); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: instancePollInterval}, nil
	}

	publicIP, ready, err := ec2.SSHReadyOnPublicIP(ctx, instanceID)
	if err != nil {
		return ctrl.Result{}, err
	}
	if !ready {
		if publicIP == "" {
			l.Info("waiting for EC2 instance public IP", "instanceID", instanceID)
		} else {
			l.Info("waiting for SSH on public IP", "instanceID", instanceID, "publicIP", publicIP)
		}
		return ctrl.Result{RequeueAfter: instancePollInterval}, nil
	}

	l.Info("EC2 instance accepts SSH on public IP", "instanceID", instanceID, "publicIP", publicIP)
	if err := h.SetInstanceMetadata(ctx, host, instanceID, publicIP); err != nil {
		return ctrl.Result{}, err
	}
	readyState := maykonfluxcidevv1alpha1.HostActualStateReady
	*statusState = &readyState
	return ctrl.Result{}, h.Status().Update(ctx, host)
}

func (h *HostStateHelper) EnsureInstanceStillRunning(ctx context.Context, ec2 hostEC2Client, host client.Object) (ctrl.Result, error) {
	instanceID := host.GetAnnotations()[internalconfig.AnnotationInstanceID]
	if instanceID == "" {
		return ctrl.Result{}, fmt.Errorf("host is Ready but annotation %q is missing", internalconfig.AnnotationInstanceID)
	}

	instanceDetails, err := ec2.DescribeInstance(ctx, instanceID)
	if err != nil {
		return ctrl.Result{}, err
	}

	if instanceDetails.State == types.InstanceStateNameRunning {
		return ctrl.Result{}, nil
	}

	logf.FromContext(ctx).Info("EC2 instance is not running", "instanceID", instanceID, "state", instanceDetails.State)
	return ctrl.Result{RequeueAfter: instancePollInterval}, nil
}

// EnsureInstanceTerminated drives EC2 termination during host deletion.
// The returned bool is true when the controller may remove its finalizer.
func (h *HostStateHelper) EnsureInstanceTerminated(ctx context.Context, ec2 hostEC2Client, host client.Object) (ctrl.Result, bool, error) {
	instanceID := host.GetAnnotations()[internalconfig.AnnotationInstanceID]
	if instanceID == "" {
		return ctrl.Result{}, true, nil
	}

	instanceDetails, err := ec2.DescribeInstance(ctx, instanceID)
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

func (h *HostStateHelper) SetInstanceMetadata(ctx context.Context, host client.Object, instanceID, publicIP string) error {
	base := host.DeepCopyObject().(client.Object)
	patch := client.MergeFrom(base)
	annotations := host.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}
	annotations[internalconfig.AnnotationInstanceID] = instanceID
	annotations[internalconfig.AnnotationPublicIPAddress] = publicIP
	host.SetAnnotations(annotations)
	return h.Patch(ctx, host, patch)
}

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
