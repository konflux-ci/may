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

package scheduler

import (
	"context"
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	maykonfluxcidevv1alpha1 "github.com/konflux-ci/may/api/v1alpha1"
	"github.com/konflux-ci/may/pkg/claim"
	"github.com/konflux-ci/may/pkg/constants"
	"github.com/konflux-ci/may/pkg/runner"
	"github.com/konflux-ci/may/pkg/scheduler"
)

// ClaimReconciler reconciles a Claim object
type ClaimReconciler struct {
	client.Client
	scheduler.Scheduler
	Scheme    *runtime.Scheme
	Namespace string
}

// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups=may.konflux-ci.dev,resources=runners,verbs=get;list;watch;update;delete
// +kubebuilder:rbac:groups=may.konflux-ci.dev,resources=claims,verbs=get;list;watch;update;patch;delete
// +kubebuilder:rbac:groups=may.konflux-ci.dev,resources=claims/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=may.konflux-ci.dev,resources=claims/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Claim object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.19.4/pkg/reconcile
func (r *ClaimReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := ctrl.LoggerFrom(ctx).WithValues("claim", req)

	l.Info("reconciling")
	c := maykonfluxcidevv1alpha1.Claim{}
	if err := r.Get(ctx, req.NamespacedName, &c, &client.GetOptions{}); err != nil {
		if kerrors.IsNotFound(err) {
			l.Info("Claim deleted")
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// resource is getting deleted, finalize it
	if !c.DeletionTimestamp.IsZero() {
		l.Info("finalizing")
		return ctrl.Result{}, r.finalize(ctx, c)
	}

	// ensure finalizer is set
	if controllerutil.AddFinalizer(&c, constants.ClaimControllerFinalizer) {
		l.Info("adding finalizer")
		return ctrl.Result{}, r.Update(ctx, &c)
	}

	// check owner reference
	if updated, err := r.ensureOwnerReference(ctx, c); err != nil || updated {
		return ctrl.Result{}, err
	}

	switch {
	// initialize
	case len(c.Status.Conditions) == 0:
		if claim.SetToSchedule(&c) {
			return ctrl.Result{}, r.Status().Update(ctx, &c)
		}
		return ctrl.Result{}, nil

	// already claimed, nothing to do
	case claim.IsClaimed(c):
		l.Info("already claimed, checking if we can delete the claim")
		return ctrl.Result{}, r.deleteIfNeeded(ctx, &c)

	// to schedule, running scheduler
	case claim.IsPending(c):
		l.Info("reserving Runner for Claim")
		return r.ensureReserved(ctx, c)

		// invalid
	case claim.IsUnclaimable(c):
		l.Info("unclaimable")
		return ctrl.Result{}, nil

	default:
		l.Info("invalid status conditions")
		return ctrl.Result{}, nil
	}
}

func (r *ClaimReconciler) ensureOwnerReference(ctx context.Context, c maykonfluxcidevv1alpha1.Claim) (bool, error) {
	oo := unstructured.Unstructured{}
	oo.SetKind(c.Spec.For.Kind)
	oo.SetAPIVersion(c.Spec.For.APIVersion)
	oo.SetUID(c.Spec.For.UID)
	oo.SetName(c.Spec.For.Name)
	ok, err := controllerutil.HasOwnerReference(c.OwnerReferences, &oo, r.Scheme)
	if err != nil {
		return false, err
	}
	if !ok {
		if err := controllerutil.SetOwnerReference(&oo, &c, r.Scheme); err != nil {
			return false, err
		}
		return true, r.Update(ctx, &c)
	}

	return false, nil
}

func (r *ClaimReconciler) finalize(ctx context.Context, c maykonfluxcidevv1alpha1.Claim) error {
	uu := maykonfluxcidevv1alpha1.RunnerList{}
	if err := r.List(ctx, &uu, client.MatchingFields{
		runner.FieldSpecInUseByName:      c.Name,
		runner.FieldSpecInUseByNamespace: c.Namespace,
	}, client.InNamespace(r.Namespace)); err != nil {
		return err
	}

	errs := []error{}
	for _, u := range uu.Items {
		if err := r.Delete(ctx, &u); err != nil {
			errs = append(errs, err)
			continue
		}

		if controllerutil.RemoveFinalizer(&u, constants.ClaimControllerFinalizer) {
			errs = append(errs, r.Update(ctx, &u))
		}
	}
	if err := errors.Join(errs...); err != nil {
		return err
	}

	if controllerutil.RemoveFinalizer(&c, constants.ClaimControllerFinalizer) {
		return r.Update(ctx, &c)
	}
	return nil
}

func (r *ClaimReconciler) ensureReserved(ctx context.Context, c maykonfluxcidevv1alpha1.Claim) (ctrl.Result, error) {
	l := ctrl.LoggerFrom(ctx).
		WithValues("claim", client.ObjectKeyFromObject(&c)).
		WithValues("op", "schedule")

	rr := maykonfluxcidevv1alpha1.RunnerList{}
	if err := r.List(ctx, &rr, client.MatchingFields{
		runner.FieldSpecInUseByName:      c.Name,
		runner.FieldSpecInUseByNamespace: c.Namespace,
	}, client.InNamespace(r.Namespace)); err != nil {
		return ctrl.Result{}, err
	}

	switch len(rr.Items) {
	// still to be scheduled
	case 0:
		return r.schedule(ctx, &c)

	// already scheduled
	case 1:
		return ctrl.Result{}, r.ensureScheduled(ctx, &c)

	// something went wrong
	default:
		rrn := make([]string, len(rr.Items))
		for i, r := range rr.Items {
			rrn[i] = client.ObjectKeyFromObject(&r).String()
		}
		l.Error(fmt.Errorf("more than one runner found: not recoverable"), "runners", rrn)
		return ctrl.Result{}, nil
	}
}

func (r *ClaimReconciler) ensureScheduled(ctx context.Context, c *maykonfluxcidevv1alpha1.Claim) error {
	l := ctrl.LoggerFrom(ctx).
		WithValues("claim", client.ObjectKeyFromObject(c)).
		WithValues("op", "schedule")

	l.Info("already reserved, ensuring state is up to date")
	if err := r.setClaimed(ctx, c); err != nil {
		return err
	}
	return r.deleteIfNeeded(ctx, c)
}

func (r *ClaimReconciler) deleteIfNeeded(ctx context.Context, c *maykonfluxcidevv1alpha1.Claim) error {
	p := corev1.Pod{}
	key := types.NamespacedName{Name: c.Spec.For.Name, Namespace: c.Namespace}
	if err := r.Get(ctx, key, &p); err != nil {
		if !kerrors.IsNotFound(err) {
			return err
		}
		return nil
	}
	if p.Status.Phase == corev1.PodFailed || p.Status.Phase == corev1.PodSucceeded {
		return r.Delete(ctx, c)
	}
	return nil
}

func (r *ClaimReconciler) schedule(ctx context.Context, c *maykonfluxcidevv1alpha1.Claim) (ctrl.Result, error) {
	l := ctrl.LoggerFrom(ctx).
		WithValues("claim", client.ObjectKeyFromObject(c)).
		WithValues("op", "schedule")

	u, err := r.Schedule(ctx, *c)
	switch {
	case err == nil:
		l.Info("claim scheduled", "runner", client.ObjectKeyFromObject(u))
		return ctrl.Result{}, r.setClaimed(ctx, c)

	case scheduler.IsNoAvailableRunner(err):
		l.Info("no runner available, can not schedule", "error", err)
		if claim.SetNotClaimed(c, claim.ConditionReasonPending, "no available runner") {
			return ctrl.Result{}, r.Status().Update(ctx, c)
		}
		return ctrl.Result{}, nil

	case scheduler.IsClaimNotSchedulable(err):
		l.Info("claim not schedulable", "error", err)
		return ctrl.Result{}, nil

	default:
		l.Error(err, "error scheduling claim")
		return ctrl.Result{}, err
	}
}

func (r *ClaimReconciler) setClaimed(ctx context.Context, c *maykonfluxcidevv1alpha1.Claim) error {
	if !claim.SetClaimed(c) {
		return nil
	}
	return r.Status().Update(ctx, c)
}

// SetupWithManager sets up the controller with the Manager.
func (r *ClaimReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&maykonfluxcidevv1alpha1.Claim{}).
		Named("claim").
		Watches(&corev1.Pod{}, handler.Funcs{
			UpdateFunc: func(ctx context.Context, e event.TypedUpdateEvent[client.Object], w workqueue.TypedRateLimitingInterface[reconcile.Request]) {
				l := log.FromContext(ctx).WithValues("pod", e.ObjectNew.GetName(), "namespace", e.ObjectNew.GetNamespace())

				l.Info("looking for claims for pod")
				// TODO(@konflux-ci): add a cache index
				cc := maykonfluxcidevv1alpha1.ClaimList{}
				if err := r.List(ctx, &cc, client.InNamespace(e.ObjectNew.GetNamespace())); err != nil {
					return
				}

				for _, c := range cc.Items {
					if ok, err := controllerutil.HasOwnerReference(c.OwnerReferences, e.ObjectNew, r.Scheme); err == nil && ok {
						k := client.ObjectKeyFromObject(&c)
						l.Info("claim found for pod", "claim", k)
						w.Add(ctrl.Request{NamespacedName: k})
						return
					}
				}
			},
		}, builder.WithPredicates(predicate.NewPredicateFuncs(func(object client.Object) bool {
			p, ok := object.(*corev1.Pod)
			return ok && (p.Status.Phase == corev1.PodFailed || p.Status.Phase == corev1.PodSucceeded)
		}))).
		Watches(&maykonfluxcidevv1alpha1.Runner{}, handler.Funcs{
			UpdateFunc: func(ctx context.Context, e event.TypedUpdateEvent[client.Object], w workqueue.TypedRateLimitingInterface[reconcile.Request]) {
				l := log.FromContext(ctx, "event", "update", "runner", e.ObjectNew)
				nr, ok := e.ObjectNew.(*maykonfluxcidevv1alpha1.Runner)
				if !ok {
					return
				}

				// reconcile the claim if something append to the bound runner
				if runner.IsReserved(*nr) {
					rq := reconcile.Request{
						NamespacedName: types.NamespacedName{
							Name:      nr.Spec.InUseBy.Name,
							Namespace: nr.Spec.InUseBy.Namespace,
						},
					}
					w.Add(rq)
					return
				}

				// nothing to do if the runner is not ready
				if !runner.IsReady(*nr) {
					return
				}

				// reconcile pending claims if runner is ready
				cc := maykonfluxcidevv1alpha1.ClaimList{}
				if err := r.List(ctx, &cc,
					&client.ListOptions{
						FieldSelector: fields.OneTermEqualSelector(claim.FieldStatusConditionClaimed, claim.ConditionReasonPending),
					}); err != nil {
					l.Error(err, "error retrieving list of Pending Claims")
					return
				}

				for _, c := range cc.Items {
					w.Add(reconcile.Request{
						NamespacedName: types.NamespacedName{
							Name:      c.Name,
							Namespace: c.Namespace,
						},
					})
				}
			},
		}).
		Complete(r)
}
