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
	"github.com/konflux-ci/may/pkg/scheduler"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ = Describe("ClaimReconciler", func() {
	const (
		claimName = "test-claim"
		podName   = "test-pod"
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
			Client:    k8sClient,
			Scheduler: scheduler.New(k8sClient, scheme.Scheme, ns.Name),
			Scheme:    scheme.Scheme,
			Namespace: ns.Name,
		}
	})

	AfterEach(func(ctx context.Context) {
		Expect(k8sClient.Delete(ctx, ns)).Should(Succeed())
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
		Expect(k8sClient.Create(ctx, p)).Should(Succeed())

		if phase != "" {
			p.Status.Phase = phase
			Expect(k8sClient.Status().Update(ctx, p)).Should(Succeed())
		}
		return p
	}

	createClaimedClaim := func(ctx context.Context, p *corev1.Pod) *mayv1alpha1.Claim {
		c := &mayv1alpha1.Claim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      claimName,
				Namespace: ns.Name,
			},
			Spec: mayv1alpha1.ClaimSpec{
				Flavor: "amd64",
				For: mayv1alpha1.ForReference{
					Name:       p.Name,
					Kind:       "Pod",
					APIVersion: "v1",
					UID:        p.UID,
				},
			},
		}
		Expect(k8sClient.Create(ctx, c)).Should(Succeed())

		By("reconciling until finalizer and owner ref are set")
		// Reconcile 1: adds finalizer (returns early)
		// Reconcile 2: sets owner reference (returns early)
		// Reconcile 3: initializes status conditions
		for i := 0; i < 3; i++ {
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: client.ObjectKeyFromObject(c),
			})
			Expect(err).ShouldNot(HaveOccurred())
		}

		By("setting Claim status to Claimed")
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(c), c)).Should(Succeed())
		claim.SetClaimed(c)
		Expect(k8sClient.Status().Update(ctx, c)).Should(Succeed())

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
})
