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
	"fmt"
	"maps"
	"slices"

	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	kueuev1beta1 "sigs.k8s.io/kueue/apis/kueue/v1beta1"

	maykonfluxcidevv1alpha1 "github.com/konflux-ci/may/api/v1alpha1"
	"github.com/konflux-ci/may/pkg/constants"
	"github.com/konflux-ci/may/pkg/runner"
)

// RunnerReconciler reconciles a Runner object
type RunnerReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups="",resources=secrets,verbs=create;get;list;watch
// +kubebuilder:rbac:groups="",resources=pods,verbs=create;delete;deletecollection
// +kubebuilder:rbac:groups=kueue.x-k8s.io,resources=clusterqueues,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=may.konflux-ci.dev,resources=claims,verbs=get;list;watch
// +kubebuilder:rbac:groups=may.konflux-ci.dev,resources=runners,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=may.konflux-ci.dev,resources=runners/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=may.konflux-ci.dev,resources=runners/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Runner object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.19.4/pkg/reconcile
func (r *RunnerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := ctrl.LoggerFrom(ctx).WithValues("runner", req)

	u := maykonfluxcidevv1alpha1.Runner{}
	if err := r.Get(ctx, req.NamespacedName, &u); err != nil {
		if kerrors.IsNotFound(err) {
			l.Info("runner not found")
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// resource is getting deleted, finalize it
	if !u.DeletionTimestamp.IsZero() {
		l.Info("finalizing")
		return ctrl.Result{}, r.finalize(ctx, u)
	}

	// ensure finalizer is set
	if controllerutil.AddFinalizer(&u, RunnerControllerFinalizer) {
		l.Info("adding finalizer")
		if err := r.Update(ctx, &u); err != nil {
			return ctrl.Result{}, err
		}
	}

	switch {
	case !runner.IsReadySet(u):
		// runner is new, let's start the Initialization procedure
		runner.SetNotReadyInitializing(&u)
		l.Info("initializing runner", "status", u.Status)
		return ctrl.Result{}, r.Status().Update(ctx, &u)

	case runner.IsInitializing(u):
		if ok, err := r.ensureRunnerIsProvisioned(ctx, u); err != nil || !ok {
			return ctrl.Result{}, err
		}

		runner.SetReady(&u)
		l.Info("setting ready", "status", u.Status)
		return ctrl.Result{}, r.Status().Update(ctx, &u)

	case runner.IsReady(u):
		// create ClusterQueue
		// l.Info("ensuring queue exists", "status", u.Status)
		// if err := r.ensureClusterQueueExists(ctx, u); err != nil {
		// 	return ctrl.Result{}, err
		// }

		if runner.IsReserved(u) {
			l.Info("runner is reserved")
			// do nothing: RunnerBinder controller will do
			return ctrl.Result{}, nil
		}

		// if requested, create the Queue
		if u.Spec.Queue != nil {
			l.Info("ensuring queue exists")
			return ctrl.Result{}, r.ensureClusterQueueExists(ctx, u)
		}

		// Nothing to do, just wait to be reserved
		return ctrl.Result{}, nil

	case runner.IsStopped(u):
		// if Queue was requested, ensure the ClusterQueue is stopped
		if u.Spec.Queue != nil {
			return ctrl.Result{}, r.ensureClusterQueueIsStopped(ctx, u)
		}
		return ctrl.Result{}, nil

	default:
		l.Info("unexpected status", "status", u.Status)
		return ctrl.Result{}, nil
	}
}

func (r *RunnerReconciler) ensureClusterQueueExists(ctx context.Context, u maykonfluxcidevv1alpha1.Runner) error {
	return r.ensureClusterQueue(ctx, u, func(cq *kueuev1beta1.ClusterQueue) {
		p := kueuev1beta1.None
		cq.Spec.StopPolicy = &p
	})
}

func (r *RunnerReconciler) ensureClusterQueueIsStopped(ctx context.Context, u maykonfluxcidevv1alpha1.Runner) error {
	return r.ensureClusterQueue(ctx, u, func(cq *kueuev1beta1.ClusterQueue) {
		p := kueuev1beta1.HoldAndDrain
		cq.Spec.StopPolicy = &p
	})
}

// TODO(@konflux-ci): refactor this function, it's too complex and not testable
func (r *RunnerReconciler) ensureRunnerIsProvisioned(ctx context.Context, u maykonfluxcidevv1alpha1.Runner) (bool, error) {
	if u.Spec.Hooks == nil {
		return true, nil
	}

	// cycle through all hooks and put them in execution or async wait for their completion
	for _, h := range u.Spec.Hooks.Provisioning {
		if s := runner.FindHookStatus(u.Status.HooksStatus.Provisioning, h.Name); s != nil {
			switch s.Phase {
			// provisioning failed, let's propagate the error
			case corev1.PodFailed:
				if runner.SetNotReadyFailed(&u, fmt.Sprintf("provisioning hook's pod '%s' failed with message: %s", s.Pod, s.PodMessage)) {
					return false, r.Status().Update(ctx, &u)
				}

			case corev1.PodSucceeded:
				// pod succeeded, we can proceed with next hook
				continue

			default:
				// wait for pod to complete
				return false, nil
			}
		}

		return false, r.runNextHookPod(ctx, u, h, RunnerHookPhaseLabelProvisioningValue, "p")
	}
	return true, nil
}

func (r *RunnerReconciler) ensureRunnerIsCleaned(ctx context.Context, u maykonfluxcidevv1alpha1.Runner) (bool, error) {
	if u.Spec.Hooks == nil {
		return true, nil
	}

	// cycle through all hooks and put them in execution or async wait for their completion
	for _, h := range u.Spec.Hooks.Cleanup {
		if s := runner.FindHookStatus(u.Status.HooksStatus.Cleanup, h.Name); s != nil {
			switch s.Phase {
			// provisioning failed, let's propagate the error
			case corev1.PodFailed:
				if runner.SetNotReadyCleaningFailed(&u, fmt.Sprintf("cleaning hooks's pod '%s' failed with message: %s", s.Pod, s.PodMessage)) {
					return false, r.Status().Update(ctx, &u)
				}

			case corev1.PodSucceeded:
				// pod succeeded, we can proceed with next hook
				continue

			default:
				// wait for pod to complete
				return false, nil
			}
		}

		return false, r.runNextHookPod(ctx, u, h, RunnerHookPhaseLabelCleanupValue, "c")
	}
	return true, nil
}

func (r *RunnerReconciler) runNextHookPod(
	ctx context.Context,
	u maykonfluxcidevv1alpha1.Runner,
	h maykonfluxcidevv1alpha1.RunnerHookPodTemplateSpec,
	phaseLabel, prefix string,
) error {
	ll := map[string]string{
		constants.RunnerNameLabel:      u.Name,
		constants.RunnerHookNameLabel:  h.Name,
		constants.RunnerHookPhaseLabel: phaseLabel,
		constants.RunnerUserLabel:      string(u.UID),
		constants.RunnerUIDLabel:       string(u.UID),
	}
	maps.Copy(ll, u.Labels)

	p := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%s-%s", prefix, u.Name, h.Name),
			Namespace: u.Namespace,
			Labels:    ll,
		},
		Spec: h.Template.Spec,
	}
	if err := controllerutil.SetControllerReference(&u, &p, r.Scheme); err != nil {
		return err
	}
	return client.IgnoreAlreadyExists(r.Create(ctx, &p))
}

func (r *RunnerReconciler) ensureClusterQueue(ctx context.Context, u maykonfluxcidevv1alpha1.Runner, m func(*kueuev1beta1.ClusterQueue)) error {
	cq := kueuev1beta1.ClusterQueue{
		ObjectMeta: metav1.ObjectMeta{Name: u.Name},
	}
	_, err := controllerutil.CreateOrPatch(ctx, r.Client, &cq, func() error {
		// set cohort
		cq.Spec.Cohort = kueuev1beta1.CohortReference(u.Spec.Queue.Cohort)

		// calculate flavors
		rqq := make([]kueuev1beta1.ResourceQuota, 0, len(u.Spec.Resources))
		for k, v := range u.Spec.Resources {
			z := v.DeepCopy()
			z.Set(0)
			rqq = append(rqq, kueuev1beta1.ResourceQuota{
				Name:         k,
				NominalQuota: v,
				// disable borrowing between sibling ClusterQueues
				BorrowingLimit: &z,
			})
		}

		// set ResourceGroup
		cq.Spec.ResourceGroups = []kueuev1beta1.ResourceGroup{
			{
				CoveredResources: slices.Collect(maps.Keys(u.Spec.Resources)),
				Flavors: []kueuev1beta1.FlavorQuotas{
					{
						Name:      kueuev1beta1.ResourceFlavorReference(u.Spec.Flavor),
						Resources: rqq,
					},
				},
			},
		}
		// mutate ClusterQueue
		if m != nil {
			m(&cq)
		}
		return nil
	})
	return err
}

func (r *RunnerReconciler) finalize(ctx context.Context, u maykonfluxcidevv1alpha1.Runner) error {
	l := log.FromContext(ctx)

	if runner.SetNotReadyCleaning(&u) {
		l.Info("marking runner as cleaning")
		return r.Status().Update(ctx, &u)
	}

	l.Info("ensure cleanup")
	if ok, err := r.ensureRunnerIsCleaned(ctx, u); err != nil || !ok {
		return err
	}

	// if it was requested, remove the related ClusterQueue
	if u.Spec.Queue != nil {
		l.Info("deleting clusterqueue", "clusterqueue", u.Name)
		cq := kueuev1beta1.ClusterQueue{
			ObjectMeta: metav1.ObjectMeta{Name: u.Name},
		}
		if err := r.Delete(ctx, &cq); err != nil {
			if !kerrors.IsNotFound(err) {
				return err
			}
		}
	}

	// remove finalizer from runner
	l.Info("remove finalizer from runner")
	if controllerutil.RemoveFinalizer(&u, RunnerControllerFinalizer) {
		return r.Update(ctx, &u)
	}
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *RunnerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&maykonfluxcidevv1alpha1.Runner{},
			builder.WithPredicates(predicate.NewPredicateFuncs(func(obj client.Object) bool {
				rt, ok := obj.GetLabels()[constants.RunnerTypeLabel]
				return ok && (rt == RunnerTypeStatic || rt == RunnerTypeDynamic)
			})),
		).
		Owns(&corev1.Pod{}).
		Watches(&maykonfluxcidevv1alpha1.Claim{}, handler.Funcs{
			DeleteFunc: func(ctx context.Context, e event.TypedDeleteEvent[client.Object], w workqueue.TypedRateLimitingInterface[ctrl.Request]) {
				rr := maykonfluxcidevv1alpha1.RunnerList{}
				if err := r.List(ctx, &rr, client.MatchingLabels{constants.RunnerTypeLabel: RunnerTypeStatic}); err != nil {
					return
				}

				obj := e.Object
				for _, sr := range rr.Items {
					rf := sr.Spec.InUseBy
					if rf != nil && rf.Name == obj.GetName() && rf.Namespace == obj.GetNamespace() {
						w.Add(ctrl.Request{NamespacedName: client.ObjectKeyFromObject(&sr)})
					}
				}
			},
		}).
		Named("runner").
		Complete(r)
}
