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

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	maykonfluxcidevv1alpha1 "github.com/konflux-ci/may/api/v1alpha1"
	"github.com/konflux-ci/may/pkg/constants"
	"github.com/konflux-ci/may/pkg/runner"
)

// RunnerHookReconciler reconciles a RunnerHook object
type RunnerHookReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=pods/status,verbs=get

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *RunnerHookReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := logf.FromContext(ctx)

	l.Info("reconciling")
	// retrieve Runner
	u := maykonfluxcidevv1alpha1.Runner{}
	if err := r.Get(ctx, req.NamespacedName, &u); err != nil {
		l.Info("not able to retrieve Runner")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// retrieve all pods with label
	switch {
	case runner.IsInitializing(u):
		l.Info("runner is initializing")
		pp := corev1.PodList{}
		if err := r.List(ctx, &pp,
			client.InNamespace(req.Namespace),
			client.MatchingLabels{
				constants.RunnerNameLabel:      u.Name,
				constants.RunnerHookPhaseLabel: RunnerHookPhaseLabelProvisioningValue,
			}); err != nil {
			return ctrl.Result{}, err
		}
		l.Info("retrieved provisioning pods", "num-provisioning-pods", len(pp.Items))
		u.Status.HooksStatus.Provisioning = r.calculateStatus(pp, u.Status.HooksStatus.Provisioning)
		l.Info("updating runner provisioning hooks status", "num-provisioning-hooks-status", len(u.Status.HooksStatus.Provisioning))
		return ctrl.Result{}, r.Status().Update(ctx, &u)

	case runner.IsCleaning(u):
		l.Info("runner is cleaning up")
		pp := corev1.PodList{}
		if err := r.List(ctx, &pp,
			client.InNamespace(req.Namespace),
			client.MatchingLabels{
				constants.RunnerNameLabel:      u.Name,
				constants.RunnerHookPhaseLabel: RunnerHookPhaseLabelCleanupValue,
			}); err != nil {
			return ctrl.Result{}, err
		}
		l.Info("retrieved cleanup pods", "num-cleanup-pods", len(pp.Items))
		u.Status.HooksStatus.Cleanup = r.calculateStatus(pp, u.Status.HooksStatus.Cleanup)
		l.Info("updating runner cleanup hooks status", "num-cleanup-hooks-status", len(u.Status.HooksStatus.Cleanup))
		return ctrl.Result{}, r.Status().Update(ctx, &u)

	default:
		return ctrl.Result{}, nil
	}
}

func (r *RunnerHookReconciler) calculateStatus(pp corev1.PodList, us []maykonfluxcidevv1alpha1.RunnerHookStatus) []maykonfluxcidevv1alpha1.RunnerHookStatus {
	for _, p := range pp.Items {
		hn, ok := p.GetLabels()[constants.RunnerHookNameLabel]
		if !ok {
			continue
		}

		rhs := maykonfluxcidevv1alpha1.RunnerHookStatus{
			PodMessage:        p.Status.Message,
			Pod:               p.Name,
			Hook:              hn,
			Phase:             p.Status.Phase,
			DeletionTimestamp: p.DeletionTimestamp,
		}
		if i := runner.IndexHookStatus(us, hn); i != -1 {
			us[i] = rhs
			continue
		}
		us = append(us, rhs)
	}
	return us
}

// SetupWithManager sets up the controller with the Manager.
func (r *RunnerHookReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&maykonfluxcidevv1alpha1.Runner{}, builder.WithPredicates(predicate.NewPredicateFuncs(func(obj client.Object) bool {
			return obj.GetLabels()[constants.RunnerTypeLabel] == RunnerTypeStatic
		}))).
		Owns(&corev1.Pod{}).
		Named("runnerhooks").
		Complete(r)
}
