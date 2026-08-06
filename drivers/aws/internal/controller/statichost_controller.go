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

	maykonfluxcidevv1alpha1 "github.com/konflux-ci/may/api/v1alpha1"
	internalclient "github.com/konflux-ci/may/drivers/aws/internal/client"
	internalconfig "github.com/konflux-ci/may/drivers/aws/internal/config"
	internalec2 "github.com/konflux-ci/may/drivers/aws/internal/ec2"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// StaticHostReconciler reconciles a StaticHost object
type StaticHostReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	hostStateHelper HostStateHelper
	newEC2client    func(ctx context.Context, host *maykonfluxcidevv1alpha1.StaticHost) (hostEC2Client, error)
}

// +kubebuilder:rbac:groups=may.konflux-ci.dev,resources=statichosts,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=may.konflux-ci.dev,resources=statichosts/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=may.konflux-ci.dev,resources=statichosts/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *StaticHostReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	host := &maykonfluxcidevv1alpha1.StaticHost{}
	if err := r.Get(ctx, req.NamespacedName, host); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !host.GetDeletionTimestamp().IsZero() {
		return r.finalize(ctx, host)
	}

	if controllerutil.AddFinalizer(host, AWSDriverFinalizer) {
		return ctrl.Result{}, r.Update(ctx, host)
	}

	if host.Status.State == nil {
		logf.FromContext(ctx).Info("initializing host state to Pending", "host", req.NamespacedName)
		host.Status.State = ptr.To(maykonfluxcidevv1alpha1.HostActualStatePending)
		return ctrl.Result{}, r.Status().Update(ctx, host)
	}

	switch host.Spec.Status {
	case maykonfluxcidevv1alpha1.HostStatusPending:
		return r.hostStateHelper.EnsurePending(ctx, *host.Status.State)
	case maykonfluxcidevv1alpha1.HostStatusReady:
		ec2, err := r.buildEC2Client(ctx, host)
		if err != nil {
			return ctrl.Result{}, err
		}
		return r.hostStateHelper.EnsureReady(
			ctx,
			ec2,
			host,
			*host.Status.State,
			func(ctx context.Context) (internalconfig.AWSConfiguration, error) {
				return internalconfig.GetStaticAWSConfiguration(ctx, host)
			},
			&host.Status.State,
		)
	default:
		logf.FromContext(ctx).Info("requested status not implemented", "requestedStatus", host.Spec.Status)
		return ctrl.Result{}, nil
	}
}

func (r *StaticHostReconciler) finalize(ctx context.Context, host *maykonfluxcidevv1alpha1.StaticHost) (ctrl.Result, error) {
	if len(host.GetFinalizers()) > 1 {
		return ctrl.Result{}, nil
	}

	var ec2 hostEC2Client
	if host.GetAnnotations()[internalconfig.AnnotationInstanceID] != "" {
		var err error
		ec2, err = r.buildEC2Client(ctx, host)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	result, done, err := r.hostStateHelper.EnsureInstanceTerminated(ctx, ec2, host)
	if err != nil || !done {
		return result, err
	}

	return ctrl.Result{}, r.removeFinalizer(ctx, host)
}

func (r *StaticHostReconciler) removeFinalizer(ctx context.Context, host *maykonfluxcidevv1alpha1.StaticHost) error {
	if controllerutil.RemoveFinalizer(host, AWSDriverFinalizer) {
		return r.Update(ctx, host)
	}
	return nil
}

func (r *StaticHostReconciler) buildEC2Client(ctx context.Context, host *maykonfluxcidevv1alpha1.StaticHost) (hostEC2Client, error) {
	if r.newEC2client != nil {
		return r.newEC2client(ctx, host)
	}
	sdk, err := internalclient.NewStaticEC2Client(ctx, host)
	if err != nil {
		return nil, err
	}
	return internalec2.NewClient(sdk), nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *StaticHostReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.hostStateHelper = HostStateHelper{Client: r.Client}

	return ctrl.NewControllerManagedBy(mgr).
		For(&maykonfluxcidevv1alpha1.StaticHost{}, builder.WithPredicates(predicate.NewPredicateFuncs(isAWSDriverHost))).
		Named("statichost-aws").
		Complete(r)
}
