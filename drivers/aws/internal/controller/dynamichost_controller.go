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
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// DynamicHostReconciler reconciles a DynamicHost object
type DynamicHostReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// NewDynamicHostReconciler constructs a DynamicHost reconciler.
func NewDynamicHostReconciler(cl client.Client, scheme *runtime.Scheme) *DynamicHostReconciler {
	return &DynamicHostReconciler{
		Client: cl,
		Scheme: scheme,
	}
}

// +kubebuilder:rbac:groups=may.konflux-ci.dev,resources=dynamichosts,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=may.konflux-ci.dev,resources=dynamichosts/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=may.konflux-ci.dev,resources=dynamichosts/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *DynamicHostReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	host := &maykonfluxcidevv1alpha1.DynamicHost{}
	if err := r.Get(ctx, req.NamespacedName, host); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !host.GetDeletionTimestamp().IsZero() {
		if controllerutil.RemoveFinalizer(host, AWSDriverFinalizer) {
			return ctrl.Result{}, r.Update(ctx, host)
		}
		return ctrl.Result{}, nil
	}

	if controllerutil.AddFinalizer(host, AWSDriverFinalizer) {
		return ctrl.Result{}, r.Update(ctx, host)
	}

	if host.Status.State == nil {
		logf.FromContext(ctx).Info("initializing host state to Pending", "host", req.NamespacedName)
		pending := maykonfluxcidevv1alpha1.HostActualStatePending
		host.Status.State = &pending
		return ctrl.Result{}, r.Status().Update(ctx, host)
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *DynamicHostReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&maykonfluxcidevv1alpha1.DynamicHost{}, builder.WithPredicates(predicate.NewPredicateFuncs(isAWSDriverHost))).
		Named("dynamichost-aws").
		Complete(r)
}
