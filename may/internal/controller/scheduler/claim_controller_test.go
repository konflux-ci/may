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

package scheduler

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	mayv1alpha1 "github.com/konflux-ci/may/api/v1alpha1"
	"github.com/konflux-ci/may/pkg/claim"
	"github.com/konflux-ci/may/pkg/runner"
	"github.com/konflux-ci/may/pkg/scheduler"
	"github.com/prometheus/client_golang/prometheus/testutil"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ = Describe("ClaimReconciler", func() {
	const (
		claimName = "test-claim"
		podName   = "test-pod"
		flavor    = "amd64"
	)

	var (
		reconciler *ClaimReconciler
		ns         *corev1.Namespace
	)

	BeforeEach(func(ctx context.Context) {
		ns = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "scheduler-test-",
			},
		}
		Expect(k8sClient.Create(ctx, ns)).Should(Succeed())

		reconciler = &ClaimReconciler{
			Client:    mgrClient,
			Scheduler: scheduler.New(mgrClient, scheme.Scheme, ns.Name),
			Scheme:    scheme.Scheme,
			Namespace: ns.Name,
		}
	})

	AfterEach(func(ctx context.Context) {
		Expect(k8sClient.Delete(ctx, ns)).Should(Succeed())
	})

	waitForCache := func(ctx context.Context, obj client.Object) {
		Eventually(func() error {
			return mgrClient.Get(ctx, client.ObjectKeyFromObject(obj), obj)
		}).Should(Succeed())
	}

	createPod := func(ctx context.Context, phase corev1.PodPhase) *corev1.Pod {
		p := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      podName,
				Namespace: ns.Name,
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{Name: "build", Image: "registry.example.com/builder:latest"},
				},
			},
		}
		Expect(k8sClient.Create(ctx, p)).Should(Succeed())

		if phase != "" {
			p.Status.Phase = phase
			Expect(k8sClient.Status().Update(ctx, p)).Should(Succeed())
		}
		waitForCache(ctx, p)
		return p
	}

	createRunner := func(ctx context.Context, name, flv string, ready bool) *mayv1alpha1.Runner {
		r := &mayv1alpha1.Runner{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: ns.Name,
			},
			Spec: mayv1alpha1.RunnerSpec{
				Flavor: flv,
				Resources: corev1.ResourceList{
					corev1.ResourceName(flv): resource.MustParse("1"),
				},
			},
		}
		Expect(k8sClient.Create(ctx, r)).Should(Succeed())

		if ready {
			runner.SetReady(r)
			Expect(k8sClient.Status().Update(ctx, r)).Should(Succeed())
		}
		waitForCache(ctx, r)
		return r
	}

	createPendingClaim := func(ctx context.Context, p *corev1.Pod) *mayv1alpha1.Claim {
		c := &mayv1alpha1.Claim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      claimName,
				Namespace: ns.Name,
			},
			Spec: mayv1alpha1.ClaimSpec{
				Flavor: flavor,
				For: mayv1alpha1.ForReference{
					Name:       p.Name,
					Kind:       "Pod",
					APIVersion: "v1",
					UID:        p.UID,
				},
			},
		}
		Expect(k8sClient.Create(ctx, c)).Should(Succeed())
		waitForCache(ctx, c)

		By("reconciling until finalizer, owner ref and status conditions are set")
		// Each Reconcile handles one gate (finalizer, owner ref, status init).
		// Eventually retries through cache conflicts until the Claim reaches Pending.
		Eventually(func(g Gomega) {
			_, _ = reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: client.ObjectKeyFromObject(c),
			})
			g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(c), c)).Should(Succeed())
			g.Expect(claim.IsPending(*c)).Should(BeTrue())
		}).Should(Succeed())
		return c
	}

	createClaimedClaim := func(ctx context.Context, p *corev1.Pod) *mayv1alpha1.Claim {
		c := createPendingClaim(ctx, p)

		By("setting Claim status to Claimed")
		claim.SetClaimed(c)
		Expect(k8sClient.Status().Update(ctx, c)).Should(Succeed())

		Eventually(func(g Gomega) {
			g.Expect(mgrClient.Get(ctx, client.ObjectKeyFromObject(c), c)).Should(Succeed())
			g.Expect(claim.IsClaimed(*c)).Should(BeTrue())
		}).Should(Succeed())

		return c
	}

	reconcileClaim := func(ctx context.Context, c *mayv1alpha1.Claim) (reconcile.Result, error) {
		return reconciler.Reconcile(ctx, reconcile.Request{
			NamespacedName: client.ObjectKeyFromObject(c),
		})
	}

	When("claim is Claimed and pod is Succeeded", func() {
		It("should delete the Claim", func(ctx context.Context) {
			By("creating a Succeeded pod and a Claimed claim")
			p := createPod(ctx, corev1.PodSucceeded)
			c := createClaimedClaim(ctx, p)

			By("reconciling the Claim")
			Expect(reconcileClaim(ctx, c)).Should(Equal(reconcile.Result{}))

			By("verifying the Claim has a deletion timestamp")
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(c), c)).Should(Succeed())
			Expect(c.DeletionTimestamp.IsZero()).Should(BeFalse())
		})
	})

	When("claim is Claimed and pod is Failed", func() {
		It("should delete the Claim", func(ctx context.Context) {
			By("creating a Failed pod and a Claimed claim")
			p := createPod(ctx, corev1.PodFailed)
			c := createClaimedClaim(ctx, p)

			By("reconciling the Claim")
			Expect(reconcileClaim(ctx, c)).Should(Equal(reconcile.Result{}))

			By("verifying the Claim has a deletion timestamp")
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(c), c)).Should(Succeed())
			Expect(c.DeletionTimestamp.IsZero()).Should(BeFalse())
		})
	})

	When("claim is Claimed and pod is Running", func() {
		It("should not delete the Claim", func(ctx context.Context) {
			By("creating a Running pod and a Claimed claim")
			p := createPod(ctx, corev1.PodRunning)
			c := createClaimedClaim(ctx, p)

			By("reconciling the Claim")
			Expect(reconcileClaim(ctx, c)).Should(Equal(reconcile.Result{}))

			By("verifying the Claim still exists")
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(c), &mayv1alpha1.Claim{})).Should(Succeed())
		})
	})

	When("claim is Claimed and pod does not exist", func() {
		It("should not delete the Claim", func(ctx context.Context) {
			By("creating a pod, creating a Claimed claim, then deleting the pod")
			p := createPod(ctx, "")
			c := createClaimedClaim(ctx, p)
			Expect(k8sClient.Delete(ctx, p)).Should(Succeed())

			By("reconciling the Claim")
			Expect(reconcileClaim(ctx, c)).Should(Equal(reconcile.Result{}))

			By("verifying the Claim still exists")
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(c), &mayv1alpha1.Claim{})).Should(Succeed())
		})
	})

	When("claim is Pending and a matching Ready Runner exists", func() {
		It("should match the Claim to the Runner", func(ctx context.Context) {
			By("creating a pod, a Ready Runner and a Pending Claim")
			p := createPod(ctx, "")
			createRunner(ctx, "ready-runner", flavor, true)
			c := createPendingClaim(ctx, p)

			By("reconciling the Claim to trigger scheduling")
			Expect(reconcileClaim(ctx, c)).Should(Equal(reconcile.Result{}))

			By("verifying the Claim is now Claimed")
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(c), c)).Should(Succeed())
			Expect(claim.IsClaimed(*c)).Should(BeTrue())

			By("verifying the Runner has InUseBy set to the Claim and flavor matches")
			r := &mayv1alpha1.Runner{}
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "ready-runner", Namespace: ns.Name}, r)).Should(Succeed())
			Expect(r.Spec.Flavor).Should(Equal(flavor))
			Expect(r.Spec.InUseBy).ShouldNot(BeNil())
			Expect(r.Spec.InUseBy.Name).Should(Equal(claimName))
			Expect(r.Spec.InUseBy.Namespace).Should(Equal(ns.Name))
		})
	})

	When("claim is Pending and no Runner exists", func() {
		It("should keep the Claim Pending", func(ctx context.Context) {
			By("creating a pod and a Pending Claim without any Runners")
			p := createPod(ctx, "")
			c := createPendingClaim(ctx, p)

			By("reconciling the Claim")
			Expect(reconcileClaim(ctx, c)).Should(Equal(reconcile.Result{}))

			By("verifying the Claim stays Pending")
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(c), c)).Should(Succeed())
			Expect(claim.IsPending(*c)).Should(BeTrue())
		})
	})

	When("claim is Pending and Runner exists but with wrong flavor", func() {
		It("should keep the Claim Pending", func(ctx context.Context) {
			By("creating a pod, a Ready Runner with a different flavor and a Pending Claim")
			p := createPod(ctx, "")
			createRunner(ctx, "wrong-flavor-runner", "arm64", true)
			c := createPendingClaim(ctx, p)

			By("reconciling the Claim")
			_, err := reconcileClaim(ctx, c)
			Expect(err).Should(HaveOccurred())

			By("verifying the Claim stays Pending")
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(c), c)).Should(Succeed())
			Expect(claim.IsPending(*c)).Should(BeTrue())

			By("verifying the Runner was not reserved")
			r := &mayv1alpha1.Runner{}
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "wrong-flavor-runner", Namespace: ns.Name}, r)).Should(Succeed())
			Expect(r.Spec.InUseBy).Should(BeNil())
		})
	})

	When("claim is Pending and Runner exists but is not Ready", func() {
		It("should keep the Claim Pending", func(ctx context.Context) {
			By("creating a pod, a non-Ready Runner and a Pending Claim")
			p := createPod(ctx, "")
			createRunner(ctx, "not-ready-runner", flavor, false)
			c := createPendingClaim(ctx, p)

			By("reconciling the Claim")
			Expect(reconcileClaim(ctx, c)).Should(Equal(reconcile.Result{}))

			By("verifying the Claim stays Pending")
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(c), c)).Should(Succeed())
			Expect(claim.IsPending(*c)).Should(BeTrue())
		})
	})

	Context("Metrics tests", Serial, func() {
		It("should increment may_claims_matched when a Claim is matched", func(ctx context.Context) {
			By("recording the metric value before reconciling")
			before := testutil.ToFloat64(claimsMatched)

			By("creating a pod, a Ready Runner and a Pending Claim")
			p := createPod(ctx, "")
			createRunner(ctx, "metrics-runner", flavor, true)
			c := createPendingClaim(ctx, p)

			By("reconciling the Claim to trigger scheduling")
			Expect(reconcileClaim(ctx, c)).Should(Equal(reconcile.Result{}))

			By("verifying the metric was incremented by 1")
			Expect(testutil.ToFloat64(claimsMatched)).Should(Equal(before + 1))
		})

		It("should not increment may_claims_matched when no Runner is available", func(ctx context.Context) {
			By("recording the metric value before reconciling")
			before := testutil.ToFloat64(claimsMatched)

			By("creating a pod and a Pending Claim without any Runners")
			p := createPod(ctx, "")
			c := createPendingClaim(ctx, p)

			By("reconciling the Claim")
			Expect(reconcileClaim(ctx, c)).Should(Equal(reconcile.Result{}))

			By("verifying the metric was not incremented")
			Expect(testutil.ToFloat64(claimsMatched)).Should(Equal(before))
		})
	})
})
