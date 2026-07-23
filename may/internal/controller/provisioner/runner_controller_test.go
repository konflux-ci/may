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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus/testutil"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	maykonfluxcidevv1alpha1 "github.com/konflux-ci/may/api/v1alpha1"
	"github.com/konflux-ci/may/pkg/runner"
)

var _ = Describe("Runner Controller", func() {
	const (
		runnerName      = "test-runner"
		runnerNamespace = "default"
		runnerFlavor    = "test-flavor"
		hookName        = "teardown"
		hookPodName     = "c-test-runner-teardown"
	)

	var (
		scheme             *runtime.Scheme
		typeNamespacedName types.NamespacedName
	)

	scheme = runtime.NewScheme()
	utilruntime.Must(maykonfluxcidevv1alpha1.AddToScheme(scheme))
	utilruntime.Must(corev1.AddToScheme(scheme))

	typeNamespacedName = types.NamespacedName{
		Name:      runnerName,
		Namespace: runnerNamespace,
	}

	newRunner := func(opts ...func(*maykonfluxcidevv1alpha1.Runner)) *maykonfluxcidevv1alpha1.Runner {
		r := &maykonfluxcidevv1alpha1.Runner{
			ObjectMeta: metav1.ObjectMeta{
				Name:       runnerName,
				Namespace:  runnerNamespace,
				Finalizers: []string{RunnerControllerFinalizer},
				Labels: map[string]string{
					"may.konflux-ci.dev/runner-type": RunnerTypeStatic,
				},
			},
			Spec: maykonfluxcidevv1alpha1.RunnerSpec{
				Flavor: runnerFlavor,
				Resources: corev1.ResourceList{
					corev1.ResourceCPU: resource.MustParse("1"),
				},
			},
		}
		for _, o := range opts {
			o(r)
		}
		return r
	}

	withDeletionTimestamp := func(r *maykonfluxcidevv1alpha1.Runner) {
		now := metav1.NewTime(time.Now())
		r.DeletionTimestamp = &now
	}

	withCleaningCondition := func(r *maykonfluxcidevv1alpha1.Runner) {
		r.Status.Conditions = []metav1.Condition{
			{
				Type:               runner.ConditionTypeReady,
				Status:             metav1.ConditionFalse,
				Reason:             runner.ConditionReasonCleaning,
				Message:            "Cleaning",
				LastTransitionTime: metav1.Now(),
			},
		}
	}

	withCleanupHook := func(r *maykonfluxcidevv1alpha1.Runner) {
		r.Spec.Hooks = &maykonfluxcidevv1alpha1.RunnerHooks{
			Cleanup: []maykonfluxcidevv1alpha1.RunnerHookPodTemplateSpec{
				{
					Name: hookName,
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							RestartPolicy: corev1.RestartPolicyNever,
							Containers: []corev1.Container{
								{Name: "main", Image: "busybox:1.36", Command: []string{"true"}},
							},
						},
					},
				},
			},
		}
	}

	withCleanupHookStatus := func(phase corev1.PodPhase, podMessage string) func(*maykonfluxcidevv1alpha1.Runner) {
		return func(r *maykonfluxcidevv1alpha1.Runner) {
			r.Status.HooksStatus.Cleanup = []maykonfluxcidevv1alpha1.RunnerHookStatus{
				{
					Hook:       hookName,
					Phase:      phase,
					Pod:        hookPodName,
					PodMessage: podMessage,
				},
			}
		}
	}

	reconcileRunner := func(ctx context.Context, k8sClient client.Client) (reconcile.Result, error) {
		return (&RunnerReconciler{
			Client: k8sClient,
			Scheme: scheme,
		}).Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
	}

	When("runner is being deleted with a failed cleanup hook", func() {
		It("should set CleaningFailed condition and keep the finalizer", func(ctx context.Context) {
			r := newRunner(withDeletionTimestamp, withCleaningCondition, withCleanupHook,
				withCleanupHookStatus(corev1.PodFailed, "exit code 1"))

			k8sClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(r).
				WithStatusSubresource(r).
				Build()

			By("reconciling the runner")
			Expect(reconcileRunner(ctx, k8sClient)).Should(Equal(reconcile.Result{}))

			By("verifying the runner is marked CleaningFailed")
			updated := &maykonfluxcidevv1alpha1.Runner{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, updated)).Should(Succeed())
			Expect(runner.IsNotReadyWithReason(*updated, runner.ConditionReasonCleaningFailed)).Should(BeTrue())

			By("verifying the finalizer is still present")
			Expect(updated.Finalizers).Should(ContainElement(RunnerControllerFinalizer))
		})
	})

	When("runner is being deleted with a succeeded cleanup hook", func() {
		It("should remove the finalizer", func(ctx context.Context) {
			r := newRunner(withDeletionTimestamp, withCleaningCondition, withCleanupHook,
				withCleanupHookStatus(corev1.PodSucceeded, ""))

			k8sClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(r).
				WithStatusSubresource(r).
				Build()

			By("reconciling the runner")
			Expect(reconcileRunner(ctx, k8sClient)).Should(Equal(reconcile.Result{}))

			By("verifying the runner was deleted (finalizer removed + DeletionTimestamp = gone)")
			updated := &maykonfluxcidevv1alpha1.Runner{}
			Expect(kerrors.IsNotFound(k8sClient.Get(ctx, typeNamespacedName, updated))).Should(BeTrue())
		})
	})

	When("runner is being deleted with a non-terminal cleanup hook", func() {
		DescribeTable("should wait without changing the runner",
			func(ctx context.Context, phase corev1.PodPhase) {
				r := newRunner(withDeletionTimestamp, withCleaningCondition, withCleanupHook,
					withCleanupHookStatus(phase, ""))

				k8sClient := fake.NewClientBuilder().
					WithScheme(scheme).
					WithObjects(r).
					WithStatusSubresource(r).
					Build()

				By("reconciling the runner")
				Expect(reconcileRunner(ctx, k8sClient)).Should(Equal(reconcile.Result{}))

				By("verifying the runner still has Cleaning condition and finalizer")
				updated := &maykonfluxcidevv1alpha1.Runner{}
				Expect(k8sClient.Get(ctx, typeNamespacedName, updated)).Should(Succeed())
				Expect(runner.IsCleaning(*updated)).Should(BeTrue())
				Expect(updated.Finalizers).Should(ContainElement(RunnerControllerFinalizer))
			},
			Entry("pod is running", corev1.PodRunning),
			Entry("pod is pending", corev1.PodPending),
		)
	})

	Context("Metrics tests", Serial, func() {
		When("cleanup hook pod fails", func() {
			It("should increment the cleaning_failed metric", func(ctx context.Context) {
				r := newRunner(withDeletionTimestamp, withCleaningCondition, withCleanupHook,
					withCleanupHookStatus(corev1.PodFailed, "exit code 1"))

				k8sClient := fake.NewClientBuilder().
					WithScheme(scheme).
					WithObjects(r).
					WithStatusSubresource(r).
					Build()

				oldValue := testutil.ToFloat64(runnerCleaningFailed)

				By("reconciling the runner")
				Expect(reconcileRunner(ctx, k8sClient)).Should(Equal(reconcile.Result{}))

				By("verifying the metric was incremented")
				Expect(testutil.ToFloat64(runnerCleaningFailed)).Should(Equal(oldValue + 1))
			})
		})

		When("cleanup hook pod succeeds", func() {
			It("should not increment the cleaning_failed metric", func(ctx context.Context) {
				r := newRunner(withDeletionTimestamp, withCleaningCondition, withCleanupHook,
					withCleanupHookStatus(corev1.PodSucceeded, ""))

				k8sClient := fake.NewClientBuilder().
					WithScheme(scheme).
					WithObjects(r).
					WithStatusSubresource(r).
					Build()

				oldValue := testutil.ToFloat64(runnerCleaningFailed)

				By("reconciling the runner")
				Expect(reconcileRunner(ctx, k8sClient)).Should(Equal(reconcile.Result{}))

				By("verifying the metric was not incremented")
				Expect(testutil.ToFloat64(runnerCleaningFailed)).Should(Equal(oldValue))
			})
		})
	})
})
