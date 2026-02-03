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
	"maps"

	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	maykonfluxcidevv1alpha1 "github.com/konflux-ci/may/api/v1alpha1"
	"github.com/konflux-ci/may/pkg/constants"
	"github.com/konflux-ci/may/pkg/runner"
)

// DynamicHostReconciler reconciles a DynamicHost object
type DynamicHostReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=may.konflux-ci.dev,resources=dynamichosts,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=may.konflux-ci.dev,resources=dynamichosts/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=may.konflux-ci.dev,resources=dynamichosts/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *DynamicHostReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := logf.FromContext(ctx).WithValues("host", req)
	l.Info("reconciling")

	h := maykonfluxcidevv1alpha1.DynamicHost{}
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
		return ctrl.Result{}, r.ensureRunnerExists(ctx, &h)

	case maykonfluxcidevv1alpha1.HostActualStateDraining:
		// TODO(@konflux-ci): if not already claimed, then delete the runner;
		// if running, then wait for the runner to be deleted
		return ctrl.Result{}, r.drainHost(ctx, h)

	case maykonfluxcidevv1alpha1.HostActualStateDrained:
		// TODO(@konflux-ci): ensure the runner was deleted
		return ctrl.Result{}, nil

	default:
		l.Info("invalid status: skipping reconciliation", "status", *h.Status.State)
		return ctrl.Result{}, nil
	}
}

func (r *DynamicHostReconciler) drainHost(ctx context.Context, h maykonfluxcidevv1alpha1.DynamicHost) error {
	u := maykonfluxcidevv1alpha1.Runner{
		ObjectMeta: metav1.ObjectMeta{
			Name:      h.Name,
			Namespace: h.Namespace,
		},
	}
	if err := r.Get(ctx, client.ObjectKeyFromObject(&u), &u); err != nil {
		if kerrors.IsNotFound(err) {
			// the runner was deleted, so we can mark the host as drained
			h.Status.State = ptr.To(maykonfluxcidevv1alpha1.HostActualStateDrained)
			return r.Status().Update(ctx, &h)
		}
		return err
	}

	// ensure runners' info are in status
	if updated, err := r.ensureRunnersInfoIsInStatus(ctx, h, u); err != nil || updated {
		return err
	}

	// the runner is still alive, let's remove the finalizer
	if controllerutil.RemoveFinalizer(&u, constants.HostControllerFinalizer) {
		return r.Update(ctx, &u)
	}
	return nil
}

func (r *DynamicHostReconciler) ensureRunnersInfoIsInStatus(ctx context.Context, h maykonfluxcidevv1alpha1.DynamicHost, rr maykonfluxcidevv1alpha1.Runner) (bool, error) {
	uR, errR := r.ensureReadyRunnersAreInStatus(ctx, h, rr)
	uP, errP := r.ensureRunnersPipelinesAreInStatus(ctx, h, rr)
	return uP || uR, errors.Join(errP, errR)
}

func (r *DynamicHostReconciler) ensureReadyRunnersAreInStatus(ctx context.Context, h maykonfluxcidevv1alpha1.DynamicHost, u maykonfluxcidevv1alpha1.Runner) (bool, error) {
	ready := 0
	stopped := 0
	switch {
	case runner.IsReady(u):
		ready++
	case runner.IsStopped(u):
		stopped++
	}

	// check if already up to date
	if ready == h.Status.Runners.Ready && stopped == h.Status.Runners.Stopped {
		return false, nil
	}

	h.Status.Runners.Ready = ready
	h.Status.Runners.Stopped = stopped
	return true, r.Status().Update(ctx, &h)
}

func (r *DynamicHostReconciler) ensureRunnersPipelinesAreInStatus(ctx context.Context, h maykonfluxcidevv1alpha1.DynamicHost, u maykonfluxcidevv1alpha1.Runner) (bool, error) {
	n, ok := u.Labels["tekton.dev/pipeline"]
	if !ok {
		return false, nil
	}

	if n == h.Status.Pipeline {
		return false, nil
	}

	h.Status.Pipeline = n
	return true, r.Status().Update(ctx, &h)
}

func (r *DynamicHostReconciler) ensureDynamicHostIsDraining(ctx context.Context, h maykonfluxcidevv1alpha1.DynamicHost, u maykonfluxcidevv1alpha1.Runner) (bool, error) {
	if u.DeletionTimestamp.IsZero() {
		return false, nil
	}

	// the runners are all exausted, we can delete the dynamic host too
	h.Status.State = ptr.To(maykonfluxcidevv1alpha1.HostActualStateDraining)
	return true, r.Status().Update(ctx, &h)
}

func (r *DynamicHostReconciler) finalize(ctx context.Context, h maykonfluxcidevv1alpha1.DynamicHost) error {
	l := logf.FromContext(ctx).WithValues("phase", "finalize")

	l.Info("deleting the runner")
	if err := r.ensureHostRunnerIsDeleted(ctx, &h); err != nil {
		return err
	}

	if controllerutil.RemoveFinalizer(&h, constants.HostControllerFinalizer) {
		l.Info("removing the finalizer")
		return r.Update(ctx, &h)
	}

	return nil
}

func (r *DynamicHostReconciler) ensureRunnerExists(ctx context.Context, h *maykonfluxcidevv1alpha1.DynamicHost) error {
	l := logf.FromContext(ctx)
	l.Info("ensuring DynamicHost's runner exists")

	u := maykonfluxcidevv1alpha1.Runner{
		ObjectMeta: metav1.ObjectMeta{
			Name:      h.Name,
			Namespace: h.Namespace,
		},
	}
	if err := r.Get(ctx, client.ObjectKeyFromObject(&u), &u); err != nil {
		if kerrors.IsNotFound(err) {
			controllerutil.AddFinalizer(&u, constants.HostControllerFinalizer)
			if u.Labels == nil {
				u.Labels = map[string]string{}
			}
			maps.Copy(u.Labels, h.GetLabels())
			u.Labels[constants.HostLabel] = h.Name
			u.Labels[constants.RunnerTypeLabel] = RunnerTypeDynamic
			u.Spec.Resources = h.Spec.Runner.Resources
			u.Spec.Flavor = h.Spec.Flavor
			u.Spec.Queue = h.Spec.Queue
			u.Spec.Hooks = h.Spec.Runner.Hooks
			if err := controllerutil.SetControllerReference(h, &u, r.Scheme, controllerutil.WithBlockOwnerDeletion(true)); err != nil {
				return err
			}

			return r.Create(ctx, &u)
		}

		return err
	}

	// ensure runners' info are in status
	if updated, err := r.ensureRunnersInfoIsInStatus(ctx, *h, u); err != nil || updated {
		return err
	}

	// if all runners are done, ensure dynamic host is marked as draining
	updated, err := r.ensureDynamicHostIsDraining(ctx, *h, u)
	if err != nil {
		l.Error(err, "error marking DynamicHost as stopped")
		return err
	}
	if updated {
		l.Info("DynamicHost moved to stopping state")
		return nil
	}
	return nil
}

func (r *DynamicHostReconciler) ensureHostRunnerIsDeleted(ctx context.Context, h *maykonfluxcidevv1alpha1.DynamicHost) error {
	u := maykonfluxcidevv1alpha1.Runner{
		ObjectMeta: metav1.ObjectMeta{
			Name:      h.Name,
			Namespace: h.Namespace,
		},
	}
	if err := r.Get(ctx, client.ObjectKeyFromObject(&u), &u); err != nil {
		if kerrors.IsNotFound(err) {
			// make sure it was deleted on the APIServer
			return client.IgnoreNotFound(r.Delete(ctx, &u))
		}
		return err
	}

	// if not already marked for deletion
	if u.DeletionTimestamp.IsZero() {
		return r.Delete(ctx, &u)
	}

	if controllerutil.RemoveFinalizer(&u, constants.HostControllerFinalizer) {
		return r.Update(ctx, &u)
	}
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *DynamicHostReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&maykonfluxcidevv1alpha1.DynamicHost{}).
		Owns(&maykonfluxcidevv1alpha1.Runner{}).
		Named("dynamichost").
		Complete(r)
}
