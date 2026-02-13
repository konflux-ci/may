/*
Copyright 2025.

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

package provisioner

import (
	"context"

	maykonfluxcidevv1alpha1 "github.com/konflux-ci/may/api/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// DynamicHostGarbageCollector reconciles a Claim object
type DynamicHostGarbageCollector struct {
	client.Client
	Scheme *runtime.Scheme

	Namespace string
}

// +kubebuilder:rbac:groups=may.konflux-ci.dev,resources=claims,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *DynamicHostGarbageCollector) Reconcile(
	ctx context.Context,
	req ctrl.Request,
) (ctrl.Result, error) {
	l := logf.FromContext(ctx)
	l.Info("reconciling")

	dh := maykonfluxcidevv1alpha1.DynamicHost{}
	if err := r.Get(ctx, req.NamespacedName, &dh); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, client.IgnoreNotFound(r.Delete(ctx, &dh))
}

// SetupWithManager sets up the controller with the Manager.
func (r *DynamicHostGarbageCollector) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&maykonfluxcidevv1alpha1.DynamicHost{}, builder.WithPredicates(predicate.NewPredicateFuncs(
			func(object client.Object) bool {
				dh, ok := object.(*maykonfluxcidevv1alpha1.DynamicHost)
				return ok && dh.Status.State != nil && *dh.Status.State == maykonfluxcidevv1alpha1.HostActualStateDrained
			},
		))).
		Named("dynamichost-gc").
		Complete(r)
}
