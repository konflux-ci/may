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
	"errors"
	"fmt"
	"maps"
	"slices"
	"sort"

	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/selection"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	maykonfluxcidevv1alpha1 "github.com/konflux-ci/may/api/v1alpha1"
	"github.com/konflux-ci/may/pkg/constants"
	"github.com/konflux-ci/may/pkg/runner"
)

var ErrCleanup error = fmt.Errorf("error cleaning up exceeding Runners")

// StaticHostReconciler reconciles a Host object
type StaticHostReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=may.konflux-ci.dev,resources=runners,verbs=get;list;watch;create;update;patch;delete;deletecollection
// +kubebuilder:rbac:groups=may.konflux-ci.dev,resources=statichosts,verbs=get;list;watch;create;update;patch;delete;deletecollection
// +kubebuilder:rbac:groups=may.konflux-ci.dev,resources=statichosts/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=may.konflux-ci.dev,resources=statichosts/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *StaticHostReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := logf.FromContext(ctx).WithValues("host", req)
	l.Info("reconciling")

	h := maykonfluxcidevv1alpha1.StaticHost{}
	if err := r.Get(ctx, req.NamespacedName, &h); err != nil {
		if kerrors.IsNotFound(err) {
			l.Info("host not found")
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// if resource is getting deleted, finalize it
	if !h.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, r.finalize(ctx, h)
	}
	// add finalizer
	if controllerutil.AddFinalizer(&h, constants.HostControllerFinalizer) {
		return ctrl.Result{}, r.Update(ctx, &h)
	}

	// retrieve statichosts' runners
	rr := maykonfluxcidevv1alpha1.RunnerList{}
	if err := r.List(ctx, &rr,
		client.InNamespace(h.Namespace),
		client.MatchingLabels{constants.HostLabel: h.Name},
	); err != nil {
		return ctrl.Result{}, err
	}

	// ensure runners' info are in status
	if updated, err := r.ensureRunnersInfoIsInStatus(ctx, h, rr); err != nil || updated {
		return ctrl.Result{}, err
	}

	// if host has not been initialized by the driver,
	// the controller has to wait
	if h.Status.State == nil {
		return ctrl.Result{}, nil
	}

	switch *h.Status.State {
	case maykonfluxcidevv1alpha1.HostActualStatePending:
		// wait for the Host to be ready
		return ctrl.Result{}, nil

	case maykonfluxcidevv1alpha1.HostActualStateReady:
		return ctrl.Result{}, r.ensureHostIsReady(ctx, h)

	case maykonfluxcidevv1alpha1.HostActualStateDraining:
		// TODO(@konflux-ci): if not already claimed, then delete the runners;
		// if running, then wait for the runners to be deleted
		panic("not implemented")

	case maykonfluxcidevv1alpha1.HostActualStateDrained:
		// TODO(@konflux-ci): ensure all runners were deleted
		panic("not implemented")

	default:
		l.Info("invalid status: skipping reconciliation", "status", *h.Status.State)
		return ctrl.Result{}, nil
	}
}

func (r *StaticHostReconciler) ensureHostIsReady(ctx context.Context, h maykonfluxcidevv1alpha1.StaticHost) error {
	// ensure the appropriate number of runners exists
	errs := []error{
		r.ensureHostRunnersExists(ctx, &h),
		r.ensureHostRunnersAreDeleted(ctx, &h),
	}
	return errors.Join(errs...)
}

func (r *StaticHostReconciler) ensureRunnersInfoIsInStatus(ctx context.Context, h maykonfluxcidevv1alpha1.StaticHost, rr maykonfluxcidevv1alpha1.RunnerList) (bool, error) {
	uR, errR := r.ensureReadyRunnersAreInStatus(ctx, h, rr)
	uP, errP := r.ensureRunnersPipelinesAreInStatus(ctx, h, rr)
	return uP || uR, errors.Join(errP, errR)
}

func (r *StaticHostReconciler) ensureReadyRunnersAreInStatus(ctx context.Context, h maykonfluxcidevv1alpha1.StaticHost, rr maykonfluxcidevv1alpha1.RunnerList) (bool, error) {
	ready := 0
	stopped := 0
	for _, u := range rr.Items {
		switch {
		case runner.IsReady(u):
			ready++
		case runner.IsStopped(u):
			stopped++
		}
	}
	// check if already up to date
	if ready == h.Status.Runners.Ready && stopped == h.Status.Runners.Stopped {
		return false, nil
	}

	h.Status.Runners.Ready = ready
	h.Status.Runners.Stopped = stopped
	return true, r.Status().Update(ctx, &h)
}

func (r *StaticHostReconciler) ensureRunnersPipelinesAreInStatus(ctx context.Context, h maykonfluxcidevv1alpha1.StaticHost, rr maykonfluxcidevv1alpha1.RunnerList) (bool, error) {
	nn := make([]string, 0, len(rr.Items))
	for _, u := range rr.Items {
		if n, ok := u.Labels["tekton.dev/pipeline"]; ok {
			nn = append(nn, n)
		}
	}
	sort.Strings(nn)
	if slices.Equal(nn, h.Status.Pipelines) {
		return false, nil
	}

	h.Status.Pipelines = nn
	return true, r.Status().Update(ctx, &h)
}

func (r *StaticHostReconciler) finalize(ctx context.Context, h maykonfluxcidevv1alpha1.StaticHost) error {
	l := logf.FromContext(ctx).WithValues("phase", "finalize")

	l.Info("deleting all runners")
	if err := r.DeleteAllOf(ctx, &maykonfluxcidevv1alpha1.Runner{},
		client.InNamespace(h.Namespace),
		client.MatchingLabels{constants.HostLabel: h.Name},
	); err != nil {
		return errors.Join(ErrCleanup, err)
	}

	l.Info("ensuring no runners exists")
	rr := maykonfluxcidevv1alpha1.RunnerList{}
	if err := r.List(ctx, &rr,
		client.InNamespace(h.Namespace),
		client.MatchingLabels{constants.HostLabel: h.Name},
	); err != nil {
		return errors.Join(ErrCleanup, err)
	}

	if len(rr.Items) == 0 {
		if controllerutil.RemoveFinalizer(&h, constants.HostControllerFinalizer) {
			l.Info("no runners left: removing finalizer")
			return r.Update(ctx, &h)
		}
	}

	l.Info("ensuring host's finalizer is removed from its runners")
	for _, u := range rr.Items {
		if controllerutil.RemoveFinalizer(&u, constants.HostControllerFinalizer) {
			return r.Update(ctx, &u)
		}
	}

	l.Info("remaining runners to delete", "runners", len(rr.Items))
	return nil
}

func (r *StaticHostReconciler) ensureHostRunnersExists(ctx context.Context, h *maykonfluxcidevv1alpha1.StaticHost) error {
	errs := []error{}
	for i := range h.Spec.Runners.Instances {
		u := maykonfluxcidevv1alpha1.Runner{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("%s-%v", h.Name, i),
				Namespace: h.Namespace,
			},
		}
		if _, err := controllerutil.CreateOrPatch(ctx, r.Client, &u, func() error {
			if u.Labels == nil {
				u.Labels = map[string]string{}
			}
			maps.Copy(u.Labels, h.GetLabels())
			u.Labels[constants.HostLabel] = h.Name
			u.Labels[constants.RunnerIdLabel] = fmt.Sprintf("%v", i)
			u.Labels[constants.RunnerTypeLabel] = RunnerTypeStatic
			u.Spec.Resources = h.Spec.Runners.Resources
			u.Spec.Flavor = h.Spec.Flavor
			u.Spec.Queue = h.Spec.Queue
			u.Spec.Hooks = h.Spec.Runners.Hooks
			return controllerutil.SetControllerReference(h, &u, r.Scheme, controllerutil.WithBlockOwnerDeletion(true))
		}); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (r *StaticHostReconciler) ensureHostRunnersAreDeleted(ctx context.Context, h *maykonfluxcidevv1alpha1.StaticHost) error {
	ll := func() []string {
		if h.Status.State != nil && *h.Status.State == maykonfluxcidevv1alpha1.HostActualStateReady {
			ll := make([]string, h.Spec.Runners.Instances)
			for i := range ll {
				ll[i] = fmt.Sprintf("%v", i)
			}
			return ll
		}
		return nil
	}()
	// cut-short: nothing to do
	if len(ll) == 0 {
		return nil
	}

	s := labels.NewSelector()
	re, err := labels.NewRequirement(constants.RunnerIdLabel, selection.NotIn, ll)
	if err != nil {
		return errors.Join(ErrCleanup, err)
	}
	s = s.Add(*re)

	if err := r.DeleteAllOf(ctx, &maykonfluxcidevv1alpha1.Runner{},
		client.InNamespace(h.Namespace),
		client.MatchingLabels{constants.HostLabel: h.Name},
		client.MatchingLabelsSelector{Selector: s},
	); err != nil {
		return errors.Join(ErrCleanup, err)
	}
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *StaticHostReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&maykonfluxcidevv1alpha1.StaticHost{}, builder.WithPredicates(predicate.NewPredicateFuncs(func(object client.Object) bool {
			h := object.(*maykonfluxcidevv1alpha1.StaticHost)
			return h.Status.State != nil && *h.Status.State != maykonfluxcidevv1alpha1.HostActualStatePending
		}))).
		Owns(&maykonfluxcidevv1alpha1.Runner{}).
		Named("statichost").
		Complete(r)
}
