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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	mayv1alpha1 "github.com/konflux-ci/may/api/v1alpha1"
	"github.com/konflux-ci/may/pkg/claim"
	"github.com/konflux-ci/may/pkg/constants"
	"github.com/konflux-ci/may/pkg/runner"
	"github.com/konflux-ci/may/pkg/scheduler"
	"github.com/prometheus/client_golang/prometheus/testutil"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
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
		Expect(k8sCachedClient.Create(ctx, ns)).Should(Succeed())

		reconciler = &ClaimReconciler{
			Client:    k8sCachedClient,
			Scheduler: scheduler.New(k8sCachedClient, scheme.Scheme, ns.Name),
			Scheme:    scheme.Scheme,
			Namespace: ns.Name,
		}
	})

	AfterEach(func(ctx context.Context) {
		Expect(k8sCachedClient.Delete(ctx, ns)).Should(Succeed())
	})

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
		Expect(k8sCachedClient.Create(ctx, p)).Should(Succeed())

		if phase != "" {
			p.Status.Phase = phase
			Expect(k8sCachedClient.Status().Update(ctx, p)).Should(Succeed())
		}
		return p
	}

	createRunner := func(ctx context.Context, name, flv string, statusOpts ...func(*mayv1alpha1.Runner)) *mayv1alpha1.Runner {
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

		if len(statusOpts) > 0 {
			for _, o := range statusOpts {
				o(r)
			}
		}

		// save status for later
		s := r.Status

		// create the Runner
		r.Status = mayv1alpha1.RunnerStatus{}
		Expect(k8sCachedClient.Create(ctx, r)).Should(Succeed())

		// retrieve the Runner from the API Server
		rr := mayv1alpha1.Runner{}
		Expect(k8sReader.Get(ctx, client.ObjectKeyFromObject(r), &rr)).Should(Succeed())

		// update the status
		rr.Status = s
		Expect(k8sCachedClient.Status().Update(ctx, &rr)).Should(Succeed())

		// ensure the cached client is up to date
		EventuallyWithOffset(1, func(g Gomega) {
			g.Expect(k8sCachedClient.Get(ctx, client.ObjectKeyFromObject(r), r)).Should(Succeed())
			if len(statusOpts) > 0 {
				g.Expect(runner.IsReady(*r)).Should(BeTrue())
			}
		}).WithTimeout(10 * time.Second).Should(Succeed())

		return &rr
	}

	createReadyRunner := func(ctx context.Context, name, flv string) *mayv1alpha1.Runner {
		return createRunner(ctx, name, flv, func(r *mayv1alpha1.Runner) { runner.SetReady(r) })
	}

	createClaim := func(ctx context.Context, p *corev1.Pod, cacheCheck func(mayv1alpha1.Claim) bool, opts ...func(*mayv1alpha1.Claim)) *mayv1alpha1.Claim {
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

		if len(opts) > 0 {
			for _, o := range opts {
				o(c)
			}
		}

		// save status for later
		s := c.Status

		// create the Claim
		c.Status = mayv1alpha1.ClaimStatus{}
		Expect(k8sCachedClient.Create(ctx, c)).Should(Succeed())

		// retrieve the Claim from the API Server
		cc := mayv1alpha1.Claim{}
		Expect(k8sReader.Get(ctx, client.ObjectKeyFromObject(c), &cc)).Should(Succeed())

		// update the status
		cc.Status = s
		Expect(k8sCachedClient.Status().Update(ctx, &cc)).Should(Succeed())

		// ensure the cached client is up to date
		EventuallyWithOffset(1, func(g Gomega) {
			g.Expect(k8sCachedClient.Get(ctx, client.ObjectKeyFromObject(c), c)).Should(Succeed())
			g.Expect(cacheCheck(*c)).Should(BeTrue())
		}).WithTimeout(10 * time.Second).Should(Succeed())

		return &cc
	}

	createPendingClaim := func(ctx context.Context, p *corev1.Pod, opts ...func(*mayv1alpha1.Claim)) *mayv1alpha1.Claim {
		return createClaim(ctx, p, claim.IsPending,
			append(opts, func(c *mayv1alpha1.Claim) {
				c.Finalizers = append(c.Finalizers, constants.ClaimControllerFinalizer)
				Expect(controllerutil.SetControllerReference(p, c, k8sCachedClient.Scheme())).Should(Succeed())
				claim.SetNotClaimed(c, claim.ConditionReasonPending, "no available runner")
			})...,
		)
	}

	createClaimedClaim := func(ctx context.Context, p *corev1.Pod, opts ...func(*mayv1alpha1.Claim)) *mayv1alpha1.Claim {
		return createClaim(ctx, p, claim.IsClaimed,
			append(opts, func(c *mayv1alpha1.Claim) {
				c.Finalizers = append(c.Finalizers, constants.ClaimControllerFinalizer)
				Expect(controllerutil.SetControllerReference(p, c, k8sCachedClient.Scheme())).Should(Succeed())
				claim.SetToSchedule(c)
				claim.SetClaimed(c)
			})...,
		)
	}

	reconcileClaim := func(ctx context.Context, c *mayv1alpha1.Claim) (reconcile.Result, error) {
		return reconciler.Reconcile(ctx, reconcile.Request{
			NamespacedName: client.ObjectKeyFromObject(c),
		})
	}

	When("cached client sync", func() {
		It("should reflect Runner Spec and Status from the API Server", func(ctx context.Context) {
			By("creating a Ready Runner")
			rr := createReadyRunner(ctx, "sync-runner", flavor)

			By("verifying the cached client returns the same Spec and Status")
			cached := &mayv1alpha1.Runner{}
			Expect(k8sCachedClient.Get(ctx, client.ObjectKeyFromObject(rr), cached)).Should(Succeed())
			Expect(cached.Spec).Should(BeEquivalentTo(rr.Spec))
			Expect(cached.Status.Conditions).ShouldNot(BeEmpty())
			Expect(runner.IsReady(*cached)).Should(BeTrue())
		})

		It("should reflect Claim Spec and Status from the API Server", func(ctx context.Context) {
			By("creating a pod and a Pending Claim")
			p := createPod(ctx, "")
			cc := createPendingClaim(ctx, p)

			By("verifying the cached client returns the same Spec and Status")
			cached := &mayv1alpha1.Claim{}
			Expect(k8sCachedClient.Get(ctx, client.ObjectKeyFromObject(cc), cached)).Should(Succeed())
			Expect(cached.Spec).Should(BeEquivalentTo(cc.Spec))
			Expect(cached.Status.Conditions).ShouldNot(BeEmpty())
			Expect(claim.IsPending(*cached)).Should(BeTrue())
		})
	})

	When("claim is Claimed and pod is Succeeded", func() {
		It("should delete the Claim", func(ctx context.Context) {
			By("creating a Succeeded pod and a Claimed claim")
			p := createPod(ctx, corev1.PodSucceeded)
			c := createClaimedClaim(ctx, p)

			By("reconciling the Claim")
			Expect(reconcileClaim(ctx, c)).Should(Equal(reconcile.Result{}))

			By("verifying the Claim has a deletion timestamp")
			Eventually(func(g Gomega) {
				g.Expect(k8sCachedClient.Get(ctx, client.ObjectKeyFromObject(c), c)).Should(Succeed())
				g.Expect(c.DeletionTimestamp.IsZero()).Should(BeFalse())
			}).WithTimeout(10 * time.Second).Should(Succeed())
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
			Eventually(func(g Gomega) {
				g.Expect(k8sCachedClient.Get(ctx, client.ObjectKeyFromObject(c), c)).Should(Succeed())
				g.Expect(c.DeletionTimestamp.IsZero()).Should(BeFalse())
			}).WithTimeout(10 * time.Second).Should(Succeed())
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
			Expect(k8sCachedClient.Get(ctx, client.ObjectKeyFromObject(c), &mayv1alpha1.Claim{})).Should(Succeed())
		})
	})

	When("claim is Claimed and pod does not exist", func() {
		It("should not delete the Claim", func(ctx context.Context) {
			By("creating a pod, creating a Claimed claim, then deleting the pod")
			p := createPod(ctx, "")
			c := createClaimedClaim(ctx, p)
			Expect(k8sCachedClient.Delete(ctx, p)).Should(Succeed())

			By("reconciling the Claim")
			Expect(reconcileClaim(ctx, c)).Should(Equal(reconcile.Result{}))

			By("verifying the Claim still exists")
			Expect(k8sCachedClient.Get(ctx, client.ObjectKeyFromObject(c), &mayv1alpha1.Claim{})).Should(Succeed())
		})
	})

	When("claim is Pending and a matching Ready Runner exists", func() {
		It("should match the Claim to the Runner", func(ctx context.Context) {
			By("creating a pod, a Ready Runner and a Pending Claim")
			p := createPod(ctx, "")
			createReadyRunner(ctx, "ready-runner", flavor)
			c := createPendingClaim(ctx, p)

			By("reconciling the Claim to trigger scheduling")
			Expect(reconcileClaim(ctx, c)).Should(Equal(reconcile.Result{}))

			By("verifying the Claim is now Claimed")
			Eventually(func(g Gomega) {
				g.Expect(k8sCachedClient.Get(ctx, client.ObjectKeyFromObject(c), c)).Should(Succeed())
				g.Expect(claim.IsClaimed(*c)).Should(BeTrue())
			}).WithTimeout(10 * time.Second).Should(Succeed())

			By("verifying the Runner has InUseBy set to the Claim and flavor matches")
			r := &mayv1alpha1.Runner{}
			Eventually(func(g Gomega) {
				g.Expect(k8sCachedClient.Get(ctx, client.ObjectKey{Name: "ready-runner", Namespace: ns.Name}, r)).Should(Succeed())
				g.Expect(r.Spec.Flavor).Should(Equal(flavor))
				g.Expect(r.Spec.InUseBy).ShouldNot(BeNil())
				g.Expect(r.Spec.InUseBy.Name).Should(Equal(claimName))
				g.Expect(r.Spec.InUseBy.Namespace).Should(Equal(ns.Name))
			}).WithTimeout(10 * time.Second).Should(Succeed())
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
			Expect(k8sCachedClient.Get(ctx, client.ObjectKeyFromObject(c), c)).Should(Succeed())
			Expect(claim.IsPending(*c)).Should(BeTrue())
		})
	})

	When("claim is Pending and Runner exists but with non-matching flavor", func() {
		It("should keep the Claim Pending", func(ctx context.Context) {
			By("creating a Pod, a Ready Runner and a Pending Claim with non-matching flavors")
			p := createPod(ctx, "")
			createReadyRunner(ctx, "wrong-flavor-runner", "arm64")
			c := createPendingClaim(ctx, p)

			By("reconciling the Claim")
			_, err := reconcileClaim(ctx, c)
			Expect(err).Should(HaveOccurred())

			By("verifying the Claim stays Pending")
			Expect(k8sCachedClient.Get(ctx, client.ObjectKeyFromObject(c), c)).Should(Succeed())
			Expect(claim.IsPending(*c)).Should(BeTrue())

			By("verifying the Runner was not reserved")
			r := &mayv1alpha1.Runner{}
			Expect(k8sCachedClient.Get(ctx, client.ObjectKey{Name: "wrong-flavor-runner", Namespace: ns.Name}, r)).Should(Succeed())
			Expect(r.Spec.InUseBy).Should(BeNil())
			Expect(runner.IsInUseBy(*r, *c)).Should(BeFalse())
		})
	})

	When("claim is Pending and Runner exists but is not Ready", func() {
		It("should keep the Claim Pending", func(ctx context.Context) {
			By("creating a pod, a non-Ready Runner and a Pending Claim")
			p := createPod(ctx, "")
			createRunner(ctx, "not-ready-runner", flavor)
			c := createPendingClaim(ctx, p)

			By("reconciling the Claim")
			Expect(reconcileClaim(ctx, c)).Should(Equal(reconcile.Result{}))

			By("verifying the Claim stays Pending")
			Expect(k8sCachedClient.Get(ctx, client.ObjectKeyFromObject(c), c)).Should(Succeed())
			Expect(claim.IsPending(*c)).Should(BeTrue())
		})
	})

	Context("Metrics tests", Serial, func() {
		It("should increment may_claim_matched when a Claim is matched", func(ctx context.Context) {
			By("recording the metric value before reconciling")
			before := testutil.ToFloat64(claimMatched)

			By("creating a pod, a Ready Runner and a Pending Claim")
			p := createPod(ctx, "")
			createReadyRunner(ctx, "metrics-runner", flavor)
			c := createPendingClaim(ctx, p)

			By("reconciling the Claim to trigger scheduling")
			Expect(reconcileClaim(ctx, c)).Should(Equal(reconcile.Result{}))

			By("verifying the metric was incremented by 1")
			Expect(testutil.ToFloat64(claimMatched)).Should(Equal(before + 1))
		})

		It("should not increment may_claim_matched when no Runner is available", func(ctx context.Context) {
			By("recording the metric value before reconciling")
			before := testutil.ToFloat64(claimMatched)

			By("creating a pod and a Pending Claim without any Runners")
			p := createPod(ctx, "")
			c := createPendingClaim(ctx, p)

			By("reconciling the Claim")
			Expect(reconcileClaim(ctx, c)).Should(Equal(reconcile.Result{}))

			By("verifying the metric was not incremented")
			Expect(testutil.ToFloat64(claimMatched)).Should(Equal(before))
		})
	})

})
