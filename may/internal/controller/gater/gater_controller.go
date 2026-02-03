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

package gater

import (
	"context"
	"slices"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	maykonfluxcidevv1alpha1 "github.com/konflux-ci/may/api/v1alpha1"
	"github.com/konflux-ci/may/pkg/claim"
	"github.com/konflux-ci/may/pkg/constants"
)

const (
	KueueFlavorLabelPrefix string = "kueue.konflux-ci.dev/requests-"
)

// GaterController reconciles a Claim object
type GaterController struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch
// +kubebuilder:rbac:groups=may.konflux-ci.dev,resources=claims,verbs=get;list;watch;create

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *GaterController) Reconcile(
	ctx context.Context,
	req ctrl.Request,
) (ctrl.Result, error) {
	l := logf.FromContext(ctx).WithValues("name", req.Name, "namespace", req.Namespace)
	l.Info("reconciling")

	c := maykonfluxcidevv1alpha1.Claim{}
	if err := r.Get(ctx, req.NamespacedName, &c); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !claim.IsClaimed(c) {
		// wait for the Claim to be Claimed
		return ctrl.Result{}, nil
	}
	return ctrl.Result{}, r.ensurePodCanBeScheduled(ctx, c)
}

func (r *GaterController) ensurePodCanBeScheduled(ctx context.Context, c maykonfluxcidevv1alpha1.Claim) error {
	// ensure the pod can be scheduled
	p := corev1.Pod{}
	k := types.NamespacedName{Namespace: c.Namespace, Name: c.Name}
	if err := r.Get(ctx, k, &p); err != nil {
		return err
	}

	sg := corev1.PodSchedulingGate{Name: constants.MayPodSchedulingGate}
	i := slices.Index(p.Spec.SchedulingGates, sg)
	if i == -1 {
		// schedulingGate not present
		return nil
	}

	p.Spec.SchedulingGates = slices.Delete(p.Spec.SchedulingGates, i, i+1)
	return r.Update(ctx, &p)
}

// SetupWithManager sets up the controller with the Manager.
func (r *GaterController) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&maykonfluxcidevv1alpha1.Claim{}, builder.WithPredicates(predicate.Funcs{
			CreateFunc: func(tce event.TypedCreateEvent[client.Object]) bool { return false },
			UpdateFunc: func(tue event.TypedUpdateEvent[client.Object]) bool { return true },
			DeleteFunc: func(tde event.TypedDeleteEvent[client.Object]) bool { return false },
		})).
		Named("gater").
		Complete(r)
}
