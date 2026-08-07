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
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	kueuev1beta1 "sigs.k8s.io/kueue/apis/kueue/v1beta1"

	maykonfluxcidevv1alpha1 "github.com/konflux-ci/may/api/v1alpha1"
	provisionerconstants "github.com/konflux-ci/may/internal/controller/provisioner/constants"
	"github.com/konflux-ci/may/pkg/runner"
)

var _ = Describe("Runner Controller (Finalize)", func() {
	const (
		runnerName      = "finalize-test-runner"
		runnerNamespace = "default"
		runnerFlavor    = "test-flavor"
		hookName        = "teardown"
		hookPodName     = "c-finalize-test-runner-teardown"
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
		if r.Spec.Hooks == nil {
			r.Spec.Hooks = &maykonfluxcidevv1alpha1.RunnerHooks{}
		}
		r.Spec.Hooks.Cleanup = []maykonfluxcidevv1alpha1.RunnerHookPodTemplateSpec{
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
		}
	}

	withQueue := func(r *maykonfluxcidevv1alpha1.Runner) {
		r.Spec.Queue = &maykonfluxcidevv1alpha1.RunnerQueue{
			Cohort: "test-cohort",
		}
	}

	withCleaningCondition := func(r *maykonfluxcidevv1alpha1.Runner) { runner.SetNotReadyCleaning(r) }

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

	setupRunner := func(ctx context.Context, opts ...func(*maykonfluxcidevv1alpha1.Runner)) *maykonfluxcidevv1alpha1.Runner {
		GinkgoHelper()
		r := newRunner(opts...)
		s := r.Status
		Expect(k8sClient.Create(ctx, r)).Should(Succeed())
		r.Status = s
		Expect(k8sClient.Status().Update(ctx, r)).Should(Succeed())
		return r
	}

	setupDeletingRunner := func(ctx context.Context, opts ...func(*maykonfluxcidevv1alpha1.Runner)) *maykonfluxcidevv1alpha1.Runner {
		GinkgoHelper()
		r := setupRunner(ctx, opts...)
		Expect(k8sClient.Delete(ctx, r)).Should(Succeed())
		return r
	}

	setupCleaningRunner := func(ctx context.Context, phase corev1.PodPhase, msg string, opts ...func(*maykonfluxcidevv1alpha1.Runner)) {
		GinkgoHelper()
		allOpts := append([]func(*maykonfluxcidevv1alpha1.Runner){
			withCleanupHook,
			withCleaningCondition,
			withCleanupHookStatus(phase, msg),
		}, opts...)
		setupDeletingRunner(ctx, allOpts...)
	}

	AfterEach(func(ctx context.Context) {
		r := &maykonfluxcidevv1alpha1.Runner{}
		err := k8sClient.Get(ctx, typeNamespacedName, r)
		if kerrors.IsNotFound(err) {
			return
		}
		Expect(err).ShouldNot(HaveOccurred())
		if len(r.Finalizers) > 0 {
			r.Finalizers = []string{}
			Expect(k8sClient.Update(ctx, r)).Should(Succeed())
		}
		Expect(k8sClient.Delete(ctx, r)).Should(
			Or(Succeed(), MatchError(kerrors.IsNotFound, "IsNotFound")))
		Expect(k8sClient.Get(ctx, typeNamespacedName, r)).
			Should(MatchError(kerrors.IsNotFound, "IsNotFound"))
	})

	When("the runner is marked for deletion with no hooks defined", Serial, func() {
		It("should set the Cleaning condition", func(ctx context.Context) {
			By("creating a runner marked for deletion with no hooks")
			setupDeletingRunner(ctx)

			By("reconciling the runner")
			res, err := reconcileRunner(ctx)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(res).Should(Equal(reconcile.Result{}))

			By("checking the runner is cleaning")
			r := &maykonfluxcidevv1alpha1.Runner{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, r)).Should(Succeed())
			Expect(runner.IsCleaning(*r)).Should(BeTrue())
		})

		It("should remove the finalizer and delete the runner", func(ctx context.Context) {
			By("reconciling the runner")
			res, err := reconcileRunner(ctx)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(res).Should(Equal(reconcile.Result{}))

			By("verifying the runner was deleted")
			Expect(k8sClient.Get(ctx, typeNamespacedName, &maykonfluxcidevv1alpha1.Runner{})).Should(MatchError(kerrors.IsNotFound, "IsNotFound"))
		})
	})

	When("the runner is marked for deletion with hooks that succeed", func() {
		It("should remove the finalizer and delete the runner", func(ctx context.Context) {
			By("creating a cleaning runner with a succeeded hook")
			setupCleaningRunner(ctx, corev1.PodSucceeded, "")

			By("reconciling the runner")
			res, err := reconcileRunner(ctx)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(res).Should(Equal(reconcile.Result{}))
			Expect(k8sClient.Get(ctx, typeNamespacedName, &maykonfluxcidevv1alpha1.Runner{})).Should(MatchError(kerrors.IsNotFound, "IsNotFound"))
		})
	})

	When("the runner is marked for deletion with hooks still running", func() {
		DescribeTable("should wait without deleting the runner",
			func(ctx context.Context, phase corev1.PodPhase) {
				By("creating a cleaning runner with a non-terminal hook")
				setupCleaningRunner(ctx, phase, "")

				By("reconciling the runner")
				res, err := reconcileRunner(ctx)
				Expect(err).ShouldNot(HaveOccurred())
				Expect(res).Should(Equal(reconcile.Result{}))

				By("verifying the runner still exists with Cleaning condition and finalizer")
				r := &maykonfluxcidevv1alpha1.Runner{}
				Expect(k8sClient.Get(ctx, typeNamespacedName, r)).Should(Succeed())
				Expect(runner.IsCleaning(*r)).Should(BeTrue())
				Expect(r.Finalizers).Should(ConsistOf(provisionerconstants.RunnerControllerFinalizer))
			},
			Entry("pod is running", corev1.PodRunning),
			Entry("pod is pending", corev1.PodPending),
		)
	})

	When("the runner has a ClusterQueue and is marked for deletion", func() {
		It("should delete the ClusterQueue during finalize", func(ctx context.Context) {
			By("creating a cleaning runner with a succeeded hook and a queue")
			setupCleaningRunner(ctx, corev1.PodSucceeded, "", withQueue)

			By("creating a ClusterQueue matching the runner name")
			cq := &kueuev1beta1.ClusterQueue{
				ObjectMeta: metav1.ObjectMeta{Name: runnerName},
			}
			Expect(k8sClient.Create(ctx, cq)).Should(Succeed())

			By("reconciling the runner")
			res, err := reconcileRunner(ctx)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(res).Should(Equal(reconcile.Result{}))

			By("verifying the runner was deleted")
			Expect(k8sClient.Get(ctx, typeNamespacedName, &maykonfluxcidevv1alpha1.Runner{})).Should(MatchError(kerrors.IsNotFound, "IsNotFound"))

			By("verifying the ClusterQueue was deleted")
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: runnerName}, &kueuev1beta1.ClusterQueue{})).Should(MatchError(kerrors.IsNotFound, "IsNotFound"))
		})
	})

	When("the runner with Queue spec is deleted but no ClusterQueue object exists", func() {
		It("should finalize without error even though no ClusterQueue exists", func(ctx context.Context) {
			By("creating a cleaning runner with a queue but no ClusterQueue")
			setupCleaningRunner(ctx, corev1.PodSucceeded, "", withQueue)

			By("reconciling the runner")
			res, err := reconcileRunner(ctx)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(res).Should(Equal(reconcile.Result{}))

			By("verifying the runner was deleted")
			Expect(k8sClient.Get(ctx, typeNamespacedName, &maykonfluxcidevv1alpha1.Runner{})).Should(MatchError(kerrors.IsNotFound, "IsNotFound"))
		})
	})

	Context("Metrics tests", Serial, func() {
		When("cleanup hooks succeed and the runner is deleted", func() {
			It("should increment the deleted metric", func(ctx context.Context) {
				setupCleaningRunner(ctx, corev1.PodSucceeded, "")

				oldValue := testutil.ToFloat64(runnerDeleted)

				By("reconciling the runner")
				res, err := reconcileRunner(ctx)
				Expect(err).ShouldNot(HaveOccurred())
				Expect(res).Should(Equal(reconcile.Result{}))

				Expect(testutil.ToFloat64(runnerDeleted)).Should(Equal(oldValue + 1))
			})
		})

		When("the runner is not deleted", func() {
			DescribeTable("should not increment the deleted metric",
				func(ctx context.Context, phase corev1.PodPhase, msg string) {
					setupCleaningRunner(ctx, phase, msg)

					oldValue := testutil.ToFloat64(runnerDeleted)

					By("reconciling the runner")
					res, err := reconcileRunner(ctx)
					Expect(err).ShouldNot(HaveOccurred())
					Expect(res).Should(Equal(reconcile.Result{}))

					Expect(testutil.ToFloat64(runnerDeleted)).Should(Equal(oldValue))
				},
				Entry("hooks still running", corev1.PodRunning, ""),
				Entry("hooks failed", corev1.PodFailed, "exit code 1"),
			)
		})
	})
})
