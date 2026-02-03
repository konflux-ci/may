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

package claimer

import (
	"context"

	"github.com/konflux-ci/may/api/v1alpha1"
	"github.com/konflux-ci/may/pkg/constants"
	"github.com/konflux-ci/may/pkg/pod"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// ClaimerController reconciles a Claim object
type ClaimerController struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;patch;update
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch
// +kubebuilder:rbac:groups=may.konflux-ci.dev,resources=claims,verbs=get;list;watch;create

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *ClaimerController) Reconcile(
	ctx context.Context,
	req ctrl.Request,
) (ctrl.Result, error) {
	l := logf.FromContext(ctx).WithValues("name", req.Name, "namespace", req.Namespace)
	l.Info("reconciling")

	p := corev1.Pod{}
	if err := r.Get(ctx, req.NamespacedName, &p); err != nil {
		return ctrl.Result{}, err
	}

	// Only create Claims for Pods in tenant namespaces
	ns := corev1.Namespace{}
	if err := r.Get(ctx, client.ObjectKey{Name: p.Namespace}, &ns); err != nil {
		return ctrl.Result{}, err
	}
	if ns.Labels[constants.TenantNamespaceLabelKey] != constants.TenantNamespaceLabelValue {
		l.Info("skipping non-tenant namespace")
		return ctrl.Result{}, nil
	}

	return ctrl.Result{}, r.ensureClaimExists(ctx, p)
}

const pipelineLabelKey = "tekton.dev/pipeline"

func (r *ClaimerController) ensureClaimExists(ctx context.Context, p corev1.Pod) error {
	f, _ := pod.ExtractFlavor(p.Annotations)
	c := v1alpha1.Claim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      p.Name,
			Namespace: p.Namespace,
		},
		Spec: v1alpha1.ClaimSpec{
			For: v1alpha1.ForReference{
				Name:       p.Name,
				Kind:       p.Kind,
				APIVersion: p.APIVersion,
				UID:        p.UID,
			},
			Flavor: f,
		},
	}
	if v, ok := p.Labels[pipelineLabelKey]; ok {
		if c.Labels == nil {
			c.Labels = make(map[string]string)
		}
		c.Labels[pipelineLabelKey] = v
	}
	if err := controllerutil.SetControllerReference(&p, &c, r.Scheme); err != nil {
		return err
	}

	return client.IgnoreAlreadyExists(r.Create(ctx, &c))
}

// SetupWithManager sets up the controller with the Manager.
func (r *ClaimerController) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Pod{}, builder.WithPredicates(predicate.Funcs{
			CreateFunc: func(tce event.TypedCreateEvent[client.Object]) bool {
				_, f := pod.ExtractFlavor(tce.Object.GetAnnotations())
				return f
			},
			UpdateFunc: func(tue event.TypedUpdateEvent[client.Object]) bool { return false },
			DeleteFunc: func(tde event.TypedDeleteEvent[client.Object]) bool { return false },
		})).
		Named("claimer").
		Complete(r)
}
