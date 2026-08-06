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
	"time"

	awsec2 "github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	maykonfluxcidevv1alpha1 "github.com/konflux-ci/may/api/v1alpha1"
	internalclient "github.com/konflux-ci/may/drivers/aws/internal/client"
	internalconfig "github.com/konflux-ci/may/drivers/aws/internal/config"
	internalec2 "github.com/konflux-ci/may/drivers/aws/internal/ec2"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	AWSDriverFinalizer   = "drivers.may.konflux-ci.dev/aws"
	DriverLabel          = "may.konflux-ci.dev/driver"
	DriverLabelValueAWS  = "aws"
	instancePollInterval = 15 * time.Second
)

func isAWSDriverHost(object client.Object) bool {
	return labels.
		SelectorFromSet(labels.Set{DriverLabel: DriverLabelValueAWS}).
		Matches(labels.Set(object.GetLabels()))
}

type hostResource interface {
	client.Object
	specStatus() maykonfluxcidevv1alpha1.HostRequestedStatus
	statusState() *maykonfluxcidevv1alpha1.HostActualState
	setStatusState(*maykonfluxcidevv1alpha1.HostActualState)
	awsConfiguration(ctx context.Context) (internalconfig.AWSConfiguration, error)
	ec2Client(ctx context.Context) (*awsec2.Client, error)
}

// HostReconciler implements the shared AWS host reconciliation flow.
type HostReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

func (r *HostReconciler) Reconcile(ctx context.Context, host hostResource) (ctrl.Result, error) {
	l := logf.FromContext(ctx).WithValues(
		"host", client.ObjectKeyFromObject(host),
		"kind", host.GetObjectKind().GroupVersionKind().Kind,
	)

	if !host.GetDeletionTimestamp().IsZero() {
		return r.finalize(ctx, host)
	}

	if controllerutil.AddFinalizer(host, AWSDriverFinalizer) {
		return ctrl.Result{}, r.Update(ctx, host)
	}

	if host.statusState() == nil {
		l.Info("initializing host state to Pending")
		host.setStatusState(ptr.To(maykonfluxcidevv1alpha1.HostActualStatePending))
		return ctrl.Result{}, r.Status().Update(ctx, host)
	}

	switch host.specStatus() {
	case maykonfluxcidevv1alpha1.HostStatusPending:
		return r.ensurePending(ctx, host)
	case maykonfluxcidevv1alpha1.HostStatusReady:
		return r.ensureReady(ctx, host)
	default:
		l.Info("requested status not implemented", "requestedStatus", host.specStatus())
		return ctrl.Result{}, nil
	}
}

func (r *HostReconciler) ensurePending(ctx context.Context, host hostResource) (ctrl.Result, error) {
	l := logf.FromContext(ctx)
	switch *host.statusState() {
	case maykonfluxcidevv1alpha1.HostActualStatePending:
		l.Info("host is already Pending")
		return ctrl.Result{}, nil
	default:
		l.Info("host cannot move back to Pending", "actualState", *host.statusState())
		return ctrl.Result{}, nil
	}
}

func (r *HostReconciler) ensureReady(ctx context.Context, host hostResource) (ctrl.Result, error) {
	l := logf.FromContext(ctx).WithValues("actualState", *host.statusState())

	switch *host.statusState() {
	case maykonfluxcidevv1alpha1.HostActualStatePending:
		return r.ensureInstanceReady(ctx, host)
	case maykonfluxcidevv1alpha1.HostActualStateReady:
		return r.ensureInstanceStillRunning(ctx, host)
	default:
		l.Info("actual state not implemented")
		return ctrl.Result{}, nil
	}
}

func (r *HostReconciler) ensureInstanceReady(ctx context.Context, host hostResource) (ctrl.Result, error) {
	l := logf.FromContext(ctx)

	ec2Client, err := host.ec2Client(ctx)
	if err != nil {
		return ctrl.Result{}, err
	}
	ec2 := internalec2.NewClient(ec2Client)

	instanceID := host.GetAnnotations()[internalconfig.AnnotationInstanceID]
	if instanceID == "" {
		cfg, err := host.awsConfiguration(ctx)
		if err != nil {
			return ctrl.Result{}, err
		}

		instanceID, err = ec2.LaunchInstance(ctx, cfg)
		if err != nil {
			return ctrl.Result{}, err
		}

		l.Info("EC2 instance launched", "instanceID", instanceID)
		if err := r.setInstanceID(ctx, host, instanceID); err != nil {
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
	if err := r.setInstanceMetadata(ctx, host, instanceID, publicIP); err != nil {
		return ctrl.Result{}, err
	}
	host.setStatusState(ptr.To(maykonfluxcidevv1alpha1.HostActualStateReady))
	return ctrl.Result{}, r.Status().Update(ctx, host)
}

func (r *HostReconciler) ensureInstanceStillRunning(ctx context.Context, host hostResource) (ctrl.Result, error) {
	instanceID := host.GetAnnotations()[internalconfig.AnnotationInstanceID]
	if instanceID == "" {
		return ctrl.Result{}, fmt.Errorf("host is Ready but annotation %q is missing", internalconfig.AnnotationInstanceID)
	}

	ec2Client, err := host.ec2Client(ctx)
	if err != nil {
		return ctrl.Result{}, err
	}

	details, err := internalec2.NewClient(ec2Client).DescribeInstance(ctx, instanceID)
	if err != nil {
		return ctrl.Result{}, err
	}

	if details.State == types.InstanceStateNameRunning {
		return ctrl.Result{}, nil
	}

	logf.FromContext(ctx).Info("EC2 instance is not running", "instanceID", instanceID, "state", details.State)
	return ctrl.Result{RequeueAfter: instancePollInterval}, nil
}

func (r *HostReconciler) setInstanceMetadata(ctx context.Context, host hostResource, instanceID, publicIP string) error {
	base := host.DeepCopyObject().(client.Object)
	patch := client.MergeFrom(base)
	annotations := host.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}
	annotations[internalconfig.AnnotationInstanceID] = instanceID
	annotations[internalconfig.AnnotationPublicIPAddress] = publicIP
	host.SetAnnotations(annotations)
	return r.Patch(ctx, host, patch)
}

func (r *HostReconciler) setInstanceID(ctx context.Context, host hostResource, instanceID string) error {
	base := host.DeepCopyObject().(client.Object)
	patch := client.MergeFrom(base)
	annotations := host.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}
	annotations[internalconfig.AnnotationInstanceID] = instanceID
	host.SetAnnotations(annotations)
	return r.Patch(ctx, host, patch)
}

func (r *HostReconciler) finalize(ctx context.Context, host hostResource) (ctrl.Result, error) {
	if len(host.GetFinalizers()) > 1 {
		return ctrl.Result{}, nil
	}

	instanceID := host.GetAnnotations()[internalconfig.AnnotationInstanceID]
	if instanceID == "" {
		return r.removeFinalizer(ctx, host)
	}

	ec2Client, err := host.ec2Client(ctx)
	if err != nil {
		return ctrl.Result{}, err
	}
	ec2 := internalec2.NewClient(ec2Client)

	details, err := ec2.DescribeInstance(ctx, instanceID)
	if err != nil {
		return ctrl.Result{}, err
	}
	state := details.State

	switch state {
	case types.InstanceStateNameTerminated:
		return r.removeFinalizer(ctx, host)
	case types.InstanceStateNameShuttingDown:
		logf.FromContext(ctx).Info("waiting for EC2 instance termination", "instanceID", instanceID)
		return ctrl.Result{RequeueAfter: instancePollInterval}, nil
	default:
		if err := ec2.TerminateInstance(ctx, instanceID); err != nil {
			return ctrl.Result{}, err
		}
		logf.FromContext(ctx).Info("terminating EC2 instance", "instanceID", instanceID)
		return ctrl.Result{RequeueAfter: instancePollInterval}, nil
	}
}

func (r *HostReconciler) removeFinalizer(ctx context.Context, host hostResource) (ctrl.Result, error) {
	if controllerutil.RemoveFinalizer(host, AWSDriverFinalizer) {
		if err := r.Update(ctx, host); err != nil {
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{}, nil
}

type staticHostResource struct {
	*maykonfluxcidevv1alpha1.StaticHost
}

func (h staticHostResource) specStatus() maykonfluxcidevv1alpha1.HostRequestedStatus {
	return h.Spec.Status
}

func (h staticHostResource) statusState() *maykonfluxcidevv1alpha1.HostActualState {
	return h.Status.State
}

func (h staticHostResource) setStatusState(state *maykonfluxcidevv1alpha1.HostActualState) {
	h.Status.State = state
}

func (h staticHostResource) awsConfiguration(ctx context.Context) (internalconfig.AWSConfiguration, error) {
	return internalconfig.GetStaticAWSConfiguration(ctx, h.StaticHost)
}

func (h staticHostResource) ec2Client(ctx context.Context) (*awsec2.Client, error) {
	return internalclient.NewStaticEC2Client(ctx, h.StaticHost)
}

type dynamicHostResource struct {
	*maykonfluxcidevv1alpha1.DynamicHost
}

func (h dynamicHostResource) specStatus() maykonfluxcidevv1alpha1.HostRequestedStatus {
	return h.Spec.Status
}

func (h dynamicHostResource) statusState() *maykonfluxcidevv1alpha1.HostActualState {
	return h.Status.State
}

func (h dynamicHostResource) setStatusState(state *maykonfluxcidevv1alpha1.HostActualState) {
	h.Status.State = state
}

func (h dynamicHostResource) awsConfiguration(ctx context.Context) (internalconfig.AWSConfiguration, error) {
	return internalconfig.GetDynamicAWSConfiguration(ctx, h.DynamicHost)
}

func (h dynamicHostResource) ec2Client(ctx context.Context) (*awsec2.Client, error) {
	return internalclient.NewDynamicEC2Client(ctx, h.DynamicHost)
}
