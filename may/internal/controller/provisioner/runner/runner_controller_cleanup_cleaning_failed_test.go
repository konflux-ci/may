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

package runner

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus/testutil"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	maykonfluxcidevv1alpha1 "github.com/konflux-ci/may/api/v1alpha1"
	provisionerconstants "github.com/konflux-ci/may/internal/controller/provisioner/constants"
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

	typeNamespacedName := types.NamespacedName{
		Name:      runnerName,
		Namespace: runnerNamespace,
	}

	newRunner := func(opts ...func(*maykonfluxcidevv1alpha1.Runner)) *maykonfluxcidevv1alpha1.Runner {
		r := &maykonfluxcidevv1alpha1.Runner{
			ObjectMeta: metav1.ObjectMeta{
				Name:       runnerName,
				Namespace:  runnerNamespace,
				Finalizers: []string{provisionerconstants.RunnerControllerFinalizer},
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

	withCleanupHookStatus := func(phase corev1.PodPhase, msg string) func(*maykonfluxcidevv1alpha1.Runner) {
		return func(r *maykonfluxcidevv1alpha1.Runner) {
			r.Status.HooksStatus.Cleanup = []maykonfluxcidevv1alpha1.RunnerHookStatus{
				{Hook: hookName, Phase: phase, Pod: hookPodName, PodMessage: msg},
			}
		}
	}

	reconcileRunner := func(ctx context.Context) (reconcile.Result, error) {
		return (&RunnerReconciler{
			Client: k8sClient,
			Scheme: k8sClient.Scheme(),
		}).Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
	}

	getRunner := func(ctx context.Context) *maykonfluxcidevv1alpha1.Runner {
		GinkgoHelper()
		r := &maykonfluxcidevv1alpha1.Runner{}
		Expect(k8sClient.Get(ctx, typeNamespacedName, r)).Should(Succeed())
		return r
	}

	withCleaningCondition := func(r *maykonfluxcidevv1alpha1.Runner) {
		runner.SetNotReadyCleaning(r)
	}

	withCleaningFailedCondition := func(r *maykonfluxcidevv1alpha1.Runner) {
		runner.SetNotReadyCleaningFailed(r, "cleaning failed")
	}

	setupCleaningRunner := func(ctx context.Context, phase corev1.PodPhase, msg string, conditionFn func(*maykonfluxcidevv1alpha1.Runner)) {
		GinkgoHelper()
		r := newRunner(withCleanupHook)
		Expect(k8sClient.Create(ctx, r)).Should(Succeed())
		Expect(k8sClient.Delete(ctx, r)).Should(Succeed())
		Expect(k8sClient.Get(ctx, typeNamespacedName, r)).Should(Succeed())
		conditionFn(r)
		withCleanupHookStatus(phase, msg)(r)
		Expect(k8sClient.Status().Update(ctx, r)).Should(Succeed())
	}

	AfterEach(func(ctx context.Context) {
		r := &maykonfluxcidevv1alpha1.Runner{}
		err := k8sClient.Get(ctx, typeNamespacedName, r)
		if kerrors.IsNotFound(err) {
			return
		}
		Expect(err).ShouldNot(HaveOccurred())
		controllerutil.RemoveFinalizer(r, provisionerconstants.RunnerControllerFinalizer)
		Expect(k8sClient.Update(ctx, r)).Should(Succeed())
	})

	When("runner is being deleted with a failed cleanup hook", func() {
		It("should set CleaningFailed condition and keep the finalizer", func(ctx context.Context) {
			By("creating a cleaning runner with a failed hook")
			setupCleaningRunner(ctx, corev1.PodFailed, "exit code 1", withCleaningCondition)

			By("reconciling the runner")
			res, err := reconcileRunner(ctx)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(res).Should(Equal(reconcile.Result{}))

			By("verifying the runner is marked CleaningFailed")
			updated := getRunner(ctx)
			Expect(runner.IsNotReadyWithReason(*updated, runner.ConditionReasonCleaningFailed)).Should(BeTrue())

			By("verifying the finalizer is still present")
			Expect(updated.Finalizers).Should(ConsistOf(provisionerconstants.RunnerControllerFinalizer))
		})
	})

	When("runner is already CleaningFailed", func() {
		It("should not flap back to Cleaning", func(ctx context.Context) {
			By("creating a CleaningFailed runner with a failed hook")
			setupCleaningRunner(ctx, corev1.PodFailed, "exit code 1", withCleaningFailedCondition)

			By("reconciling the runner")
			res, err := reconcileRunner(ctx)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(res).Should(Equal(reconcile.Result{}))

			By("verifying the runner stays CleaningFailed")
			updated := getRunner(ctx)
			Expect(runner.IsNotReadyWithReason(*updated, runner.ConditionReasonCleaningFailed)).Should(BeTrue())

			By("verifying the finalizer is still present")
			Expect(updated.Finalizers).Should(ConsistOf(provisionerconstants.RunnerControllerFinalizer))
		})
	})

	When("runner is being deleted with a succeeded cleanup hook", func() {
		It("should remove the finalizer", func(ctx context.Context) {
			By("creating a cleaning runner with a succeeded hook")
			setupCleaningRunner(ctx, corev1.PodSucceeded, "", withCleaningCondition)

			By("reconciling the runner")
			res, err := reconcileRunner(ctx)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(res).Should(Equal(reconcile.Result{}))

			By("verifying the runner was deleted")
			Expect(k8sClient.Get(ctx, typeNamespacedName, &maykonfluxcidevv1alpha1.Runner{})).Should(MatchError(kerrors.IsNotFound, "IsNotFound"))
		})
	})

	When("runner is being deleted with a non-terminal cleanup hook", func() {
		DescribeTable("should wait without changing the runner",
			func(ctx context.Context, phase corev1.PodPhase) {
				By("creating a cleaning runner with a non-terminal hook")
				setupCleaningRunner(ctx, phase, "", withCleaningCondition)

				By("reconciling the runner")
				res, err := reconcileRunner(ctx)
				Expect(err).ShouldNot(HaveOccurred())
				Expect(res).Should(Equal(reconcile.Result{}))

				By("verifying the runner still has Cleaning condition and finalizer")
				updated := getRunner(ctx)
				Expect(runner.IsCleaning(*updated)).Should(BeTrue())
				Expect(updated.Finalizers).Should(ConsistOf(provisionerconstants.RunnerControllerFinalizer))
			},
			Entry("pod is running", corev1.PodRunning),
			Entry("pod is pending", corev1.PodPending),
		)
	})

	Context("Metrics tests", Serial, func() {
		When("cleanup hook pod fails", func() {
			It("should increment the cleaning_failed metric", func(ctx context.Context) {
				By("creating a cleaning runner with a failed hook")
				setupCleaningRunner(ctx, corev1.PodFailed, "exit code 1", withCleaningCondition)

				oldValue := testutil.ToFloat64(runnerCleaningFailed)

				By("reconciling the runner")
				res, err := reconcileRunner(ctx)
				Expect(err).ShouldNot(HaveOccurred())
				Expect(res).Should(Equal(reconcile.Result{}))

				By("verifying the metric was incremented")
				Expect(testutil.ToFloat64(runnerCleaningFailed)).Should(Equal(oldValue + 1))
			})
		})

		When("runner is already CleaningFailed", func() {
			It("should not falsely increment the cleaning_failed metric", func(ctx context.Context) {
				By("creating a CleaningFailed runner with a failed hook")
				setupCleaningRunner(ctx, corev1.PodFailed, "exit code 1", withCleaningFailedCondition)

				oldValue := testutil.ToFloat64(runnerCleaningFailed)

				By("reconciling the runner")
				res, err := reconcileRunner(ctx)
				Expect(err).ShouldNot(HaveOccurred())
				Expect(res).Should(Equal(reconcile.Result{}))

				By("verifying the metric was not incremented")
				Expect(testutil.ToFloat64(runnerCleaningFailed)).Should(Equal(oldValue))
			})
		})

		When("cleanup hook pod succeeds", func() {
			It("should not increment the cleaning_failed metric", func(ctx context.Context) {
				By("creating a cleaning runner with a succeeded hook")
				setupCleaningRunner(ctx, corev1.PodSucceeded, "", withCleaningCondition)

				oldValue := testutil.ToFloat64(runnerCleaningFailed)

				By("reconciling the runner")
				res, err := reconcileRunner(ctx)
				Expect(err).ShouldNot(HaveOccurred())
				Expect(res).Should(Equal(reconcile.Result{}))

				By("verifying the metric was not incremented")
				Expect(testutil.ToFloat64(runnerCleaningFailed)).Should(Equal(oldValue))
			})
		})
	})
})
