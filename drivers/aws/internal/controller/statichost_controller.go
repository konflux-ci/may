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
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// StaticHostReconciler reconciles a StaticHost object
type StaticHostReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	hostReconciler HostReconciler
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

	return r.hostReconciler.Reconcile(ctx, staticHostResource{StaticHost: host})
}

// SetupWithManager sets up the controller with the Manager.
func (r *StaticHostReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.hostReconciler = HostReconciler{Client: r.Client, Scheme: r.Scheme}

	return ctrl.NewControllerManagedBy(mgr).
		For(&maykonfluxcidevv1alpha1.StaticHost{}, builder.WithPredicates(predicate.NewPredicateFuncs(isAWSDriverHost))).
		Named("statichost-aws").
		Complete(r)
}
