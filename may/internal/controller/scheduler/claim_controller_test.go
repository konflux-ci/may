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
	"github.com/prometheus/client_golang/prometheus/testutil"

	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	mayv1alpha1 "github.com/konflux-ci/may/api/v1alpha1"
	"github.com/konflux-ci/may/pkg/claim"
	"github.com/konflux-ci/may/pkg/constants"
	"github.com/konflux-ci/may/pkg/runner"
	"github.com/konflux-ci/may/pkg/scheduler"
)

var _ = Describe("ClaimReconciler", func() {
	const (
		claimName = "test-claim"
		podName   = "test-pod"
		flavor    = "amd64"
	)

	var (
		reconciler  *ClaimReconciler
		runnerNsObj *corev1.Namespace
	)

	BeforeEach(func(ctx context.Context) {
		runnerNsObj = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "may-system-",
			},
		}
		Expect(k8sCachedClient.Create(ctx, runnerNsObj)).Should(Succeed())

		reconciler = &ClaimReconciler{
			Client:    k8sCachedClient,
			Scheduler: scheduler.New(k8sCachedClient, scheme.Scheme, runnerNsObj.Name),
			Scheme:    scheme.Scheme,
			Namespace: runnerNsObj.Name,
		}
	})

	waitForCache := func(ctx context.Context, obj client.Object) {
		Eventually(func() error {
			return k8sClient.Get(ctx, client.ObjectKeyFromObject(obj), obj)
		}).Should(Succeed())
	}

	reconcileClaim := func(ctx context.Context, c *mayv1alpha1.Claim) (reconcile.Result, error) {
		return reconciler.Reconcile(ctx, reconcile.Request{
			NamespacedName: client.ObjectKeyFromObject(c),
		})
	}

	AfterEach(func(ctx context.Context) {
		Expect(k8sCachedClient.Delete(ctx, runnerNsObj)).Should(Succeed())
	})

	createPod := func(ctx context.Context, phase corev1.PodPhase) *corev1.Pod {
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      podName,
				Namespace: runnerNsObj.Name,
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name:  "test-container",
						Image: "test-image",
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, pod)).Should(Succeed())

		if phase != "" {
			pod.Status.Phase = phase
			Expect(k8sClient.Status().Update(ctx, pod)).Should(Succeed())
		}
		waitForCache(ctx, pod)
		return pod
	}

	createRunner := func(ctx context.Context, name, flv string, statusOpts ...func(*mayv1alpha1.Runner)) *mayv1alpha1.Runner {
		r := &mayv1alpha1.Runner{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: runnerNsObj.Name,
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

	createReservedRunner := func(ctx context.Context, name, flv, claimName, claimNamespace string) *mayv1alpha1.Runner {
		return createRunner(ctx, name, flv, func(r *mayv1alpha1.Runner) {
			runner.SetReady(r)
			r.Spec.InUseBy = &mayv1alpha1.ClaimReference{
				Name:      claimName,
				Namespace: claimNamespace,
			}
			controllerutil.AddFinalizer(r, constants.ClaimControllerFinalizer)
		})
	}

	createClaim := func(ctx context.Context, p *corev1.Pod, cacheCheck func(mayv1alpha1.Claim) bool, opts ...func(*mayv1alpha1.Claim)) *mayv1alpha1.Claim {
		c := &mayv1alpha1.Claim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      claimName,
				Namespace: runnerNsObj.Name,
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
			Expect(reconcileClaim(ctx, c)).Error().ShouldNot(HaveOccurred())

			By("verifying the Claim has a deletion timestamp")
			Eventually(func(g Gomega) {
				g.Expect(k8sCachedClient.Get(ctx, client.ObjectKeyFromObject(c), c)).Should(Succeed())
				g.Expect(c.DeletionTimestamp.IsZero()).Should(BeFalse())
			}).WithTimeout(10 * time.Second).Should(Succeed())
		})
	})

	When("finalizing a Claim with associated Runners", func() {
		It("should delete all Runners and remove the finalizer", func(ctx context.Context) {
			By("creating a pod, a reserved Runner and a Claimed Claim")
			p := createPod(ctx, "")
			createReservedRunner(ctx, "finalize-runner", flavor, claimName, runnerNsObj.Name)
			c := createClaimedClaim(ctx, p)

			By("deleting the Claim to trigger finalization")
			Expect(k8sCachedClient.Delete(ctx, c)).Should(Succeed())

			By("reconciling until finalization completes")
			Eventually(func(g Gomega) {
				_, err := reconcileClaim(ctx, c)
				g.Expect(err).ShouldNot(HaveOccurred())
				err = k8sCachedClient.Get(ctx, client.ObjectKeyFromObject(c), c)
				if kerrors.IsNotFound(err) {
					return
				}
				g.Expect(err).ShouldNot(HaveOccurred())
				g.Expect(controllerutil.ContainsFinalizer(c, constants.ClaimControllerFinalizer)).Should(BeFalse())
			}).WithTimeout(10 * time.Second).Should(Succeed())
		})
	})

	When("finalizing a Claim does not affect Runners belonging to other Claims", func() {
		It("should only delete Runners reserved by the finalized Claim", func(ctx context.Context) {
			By("creating two pods, two reserved Runners and two Claimed Claims")
			pA := createPod(ctx, "")
			pB := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod-b",
					Namespace: runnerNsObj.Name,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "test-container", Image: "test-image"},
					},
				},
			}
			Expect(k8sClient.Create(ctx, pB)).Should(Succeed())
			waitForCache(ctx, pB)

			createReservedRunner(ctx, "runner-a", flavor, claimName, runnerNsObj.Name)
			createReservedRunner(ctx, "runner-b", flavor, "test-claim-b", runnerNsObj.Name)

			cA := createClaimedClaim(ctx, pA)
			createClaim(ctx, pB, claim.IsClaimed, func(c *mayv1alpha1.Claim) {
				c.Name = "test-claim-b"
				c.Finalizers = append(c.Finalizers, constants.ClaimControllerFinalizer)
				Expect(controllerutil.SetControllerReference(pB, c, k8sCachedClient.Scheme())).Should(Succeed())
				claim.SetToSchedule(c)
				claim.SetClaimed(c)
			})

			By("deleting claim A to trigger finalization")
			Expect(k8sCachedClient.Delete(ctx, cA)).Should(Succeed())

			By("reconciling until claim A finalization completes")
			Eventually(func(g Gomega) {
				_, err := reconcileClaim(ctx, cA)
				g.Expect(err).ShouldNot(HaveOccurred())
				err = k8sCachedClient.Get(ctx, client.ObjectKeyFromObject(cA), cA)
				if kerrors.IsNotFound(err) {
					return
				}
				g.Expect(err).ShouldNot(HaveOccurred())
				g.Expect(controllerutil.ContainsFinalizer(cA, constants.ClaimControllerFinalizer)).Should(BeFalse())
			}).WithTimeout(10 * time.Second).Should(Succeed())

			By("verifying the runner assigned to claim B still exists and is reserved")
			rB := &mayv1alpha1.Runner{}
			Expect(k8sCachedClient.Get(ctx, client.ObjectKey{Name: "runner-b", Namespace: runnerNsObj.Name}, rB)).Should(Succeed())
			Expect(rB.Spec.InUseBy).ShouldNot(BeNil())
			Expect(rB.Spec.InUseBy.Name).Should(Equal("test-claim-b"))
			Expect(rB.Spec.InUseBy.Namespace).Should(Equal(runnerNsObj.Name))
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
				g.Expect(k8sCachedClient.Get(ctx, client.ObjectKey{Name: "ready-runner", Namespace: runnerNsObj.Name}, r)).Should(Succeed())
				g.Expect(r.Spec.Flavor).Should(Equal(flavor))
				g.Expect(r.Spec.InUseBy).ShouldNot(BeNil())
				g.Expect(r.Spec.InUseBy.Name).Should(Equal(claimName))
				g.Expect(r.Spec.InUseBy.Namespace).Should(Equal(runnerNsObj.Name))
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
			Expect(err).ShouldNot(HaveOccurred())

			By("verifying the Claim stays Pending with flavor-specific message")
			Eventually(func(g Gomega) {
				g.Expect(k8sCachedClient.Get(ctx, client.ObjectKeyFromObject(c), c)).Should(Succeed())
				g.Expect(claim.IsPending(*c)).Should(BeTrue())
				g.Expect(c.Status.Conditions).Should(
					ContainElement(
						WithTransform(func(cond metav1.Condition) metav1.Condition {
							cond.LastTransitionTime = metav1.Time{}
							return cond
						}, Equal(metav1.Condition{
							Status:  metav1.ConditionFalse,
							Type:    claim.ConditionTypeClaimed,
							Reason:  claim.ConditionReasonPending,
							Message: scheduler.ErrNoAvailableRunnerForFlavor.Error() + " " + flavor,
						})),
					),
				)
			}).WithTimeout(10 * time.Second).Should(Succeed())

			By("verifying the Runner was not reserved")
			r := &mayv1alpha1.Runner{}
			Expect(k8sCachedClient.Get(ctx, client.ObjectKey{Name: "wrong-flavor-runner", Namespace: runnerNsObj.Name}, r)).Should(Succeed())
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

		It("should increment may_runner_deleted when a Claim's Runner is deleted", func(ctx context.Context) {
			By("creating a pod, a Ready Runner and a Pending Claim")
			p := createPod(ctx, "")
			createReadyRunner(ctx, "metrics-delete-runner", flavor)
			c := createPendingClaim(ctx, p)

			By("reconciling to match the Claim to the Runner")
			Expect(reconcileClaim(ctx, c)).Should(Equal(reconcile.Result{}))

			By("recording the metric value before deletion")
			before := testutil.ToFloat64(runnerDeleted)

			By("deleting the Claim to trigger finalization")
			Expect(k8sCachedClient.Delete(ctx, c)).Should(Succeed())

			By("reconciling until the Runner is deleted and metric incremented")
			Eventually(func(g Gomega) {
				_, err := reconcileClaim(ctx, c)
				g.Expect(err).ShouldNot(HaveOccurred())
				g.Expect(testutil.ToFloat64(runnerDeleted)).Should(BeNumerically(">", before))
			}).WithTimeout(10 * time.Second).Should(Succeed())
		})
	})

})
