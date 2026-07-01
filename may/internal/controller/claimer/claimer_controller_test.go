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

package claimer

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/konflux-ci/may/api/v1alpha1"
	"github.com/konflux-ci/may/pkg/constants"
	"github.com/konflux-ci/may/pkg/pod"
	"github.com/prometheus/client_golang/prometheus/testutil"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ = Describe("ClaimerController", func() {
	const (
		podName  = "test-pod"
		flavor   = "amd64"
		pipeline = "my-pipeline"
	)

	var (
		reconciler *ClaimerController
		tenantNs   *corev1.Namespace
		regularNs  *corev1.Namespace
	)

	BeforeEach(func(ctx context.Context) {
		reconciler = &ClaimerController{
			Client: k8sClient,
			Scheme: scheme.Scheme,
		}

		tenantNs = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "tenant-",
				Labels: map[string]string{
					constants.TenantNamespaceLabelKey: constants.TenantNamespaceLabelValue,
				},
			},
		}
		Expect(k8sClient.Create(ctx, tenantNs)).Should(Succeed())

		regularNs = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "regular-",
			},
		}
		Expect(k8sClient.Create(ctx, regularNs)).Should(Succeed())
	})

	AfterEach(func(ctx context.Context) {
		Expect(k8sClient.Delete(ctx, tenantNs)).Should(Succeed())
		Expect(k8sClient.Delete(ctx, regularNs)).Should(Succeed())
	})

	createPod := func(ctx context.Context, namespace string, annotations, labels map[string]string) *corev1.Pod {
		p := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:        podName,
				Namespace:   namespace,
				Annotations: annotations,
				Labels:      labels,
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{Name: "build", Image: "registry.example.com/builder:latest"},
				},
			},
		}
		Expect(k8sClient.Create(ctx, p)).Should(Succeed())
		return p
	}

	reconcilePod := func(ctx context.Context, p *corev1.Pod) (reconcile.Result, error) {
		return reconciler.Reconcile(ctx, reconcile.Request{
			NamespacedName: client.ObjectKeyFromObject(p),
		})
	}

	When("a matching pod exists in a tenant namespace", func() {
		It("should create a Claim with the correct flavor and owner reference", func(ctx context.Context) {
			By("creating a pod with a flavor annotation and reconciling")
			p := createPod(ctx, tenantNs.Name,
				map[string]string{pod.KueueFlavorLabelPrefix + flavor: ""},
				nil,
			)
			Expect(reconcilePod(ctx, p)).Should(Equal(reconcile.Result{}))

			By("verifying the Claim was created with correct spec and owner reference")
			claim := &v1alpha1.Claim{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(p), claim)).Should(Succeed())
			Expect(claim.Spec.Flavor).Should(Equal(flavor))
			Expect(claim.Spec.For.Name).Should(Equal(p.Name))
			Expect(claim.Spec.For.UID).Should(Equal(p.UID))
			Expect(controllerutil.HasOwnerReference(claim.OwnerReferences, p, k8sClient.Scheme())).To(BeTrue())
			Expect(controllerutil.HasControllerReference(claim)).To(BeTrue())
		})
	})

	When("the pod has a pipeline label", func() {
		It("should copy the pipeline label to the Claim", func(ctx context.Context) {
			By("creating a pod with a pipeline label and reconciling")
			p := createPod(ctx, tenantNs.Name,
				map[string]string{pod.KueueFlavorLabelPrefix + flavor: ""},
				map[string]string{pipelineLabelKey: pipeline},
			)
			Expect(reconcilePod(ctx, p)).Error().ShouldNot(HaveOccurred())

			By("verifying the pipeline label was copied to the Claim")
			claim := &v1alpha1.Claim{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(p), claim)).Should(Succeed())
			Expect(claim.Labels).Should(HaveKeyWithValue(pipelineLabelKey, pipeline))
		})
	})

	When("the pod has no pipeline label", func() {
		It("should create a Claim without the pipeline label", func(ctx context.Context) {
			By("creating a pod without a pipeline label and reconciling")
			p := createPod(ctx, tenantNs.Name,
				map[string]string{pod.KueueFlavorLabelPrefix + flavor: ""},
				nil,
			)
			Expect(reconcilePod(ctx, p)).Error().ShouldNot(HaveOccurred())

			By("verifying the Claim has no pipeline label")
			claim := &v1alpha1.Claim{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(p), claim)).Should(Succeed())
			Expect(claim.Labels).ShouldNot(HaveKey(pipelineLabelKey))
		})
	})

	When("the pod is in a non-tenant namespace", func() {
		It("should not create a Claim", func(ctx context.Context) {
			By("creating a pod in a non-tenant namespace and reconciling")
			p := createPod(ctx, regularNs.Name,
				map[string]string{pod.KueueFlavorLabelPrefix + flavor: ""},
				nil,
			)
			Expect(reconcilePod(ctx, p)).Should(Equal(reconcile.Result{}))

			By("verifying no Claim was created")
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(p), &v1alpha1.Claim{})).Should(MatchError(apierrors.IsNotFound, "IsNotFound"))
		})
	})

	When("a Claim already exists for the pod", func() {
		It("should not return an error and should not modify the pod or the Claim", func(ctx context.Context) {
			By("creating a pod and reconciling it to create the Claim")
			p := createPod(ctx, tenantNs.Name,
				map[string]string{pod.KueueFlavorLabelPrefix + flavor: ""},
				nil,
			)
			Expect(reconcilePod(ctx, p)).Error().ShouldNot(HaveOccurred())

			By("recording resource versions before re-reconciling")
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(p), p)).Should(Succeed())
			podResourceVersion := p.ResourceVersion
			claim := &v1alpha1.Claim{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(p), claim)).Should(Succeed())
			claimResourceVersion := claim.ResourceVersion

			By("reconciling the same pod again")
			Expect(reconcilePod(ctx, p)).Error().ShouldNot(HaveOccurred())

			By("verifying the pod was not modified")
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(p), p)).Should(Succeed())
			Expect(p.ResourceVersion).Should(Equal(podResourceVersion))

			By("verifying the Claim was not modified")
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(p), claim)).Should(Succeed())
			Expect(claim.ResourceVersion).Should(Equal(claimResourceVersion))
		})
	})

	When("a Claim is created for a pod", func() {
		It("should set a controller owner reference with blockOwnerDeletion", func(ctx context.Context) {
			By("creating a pod and reconciling to create the Claim")
			p := createPod(ctx, tenantNs.Name,
				map[string]string{pod.KueueFlavorLabelPrefix + flavor: ""},
				nil,
			)
			Expect(reconcilePod(ctx, p)).Error().ShouldNot(HaveOccurred())

			By("verifying the Claim has a controller owner reference with blockOwnerDeletion")
			claim := &v1alpha1.Claim{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(p), claim)).Should(Succeed())
			Expect(claim.OwnerReferences).Should(HaveLen(1))
			Expect(claim.OwnerReferences[0].Name).Should(Equal(p.Name))
			Expect(claim.OwnerReferences[0].UID).Should(Equal(p.UID))
			Expect(claim.OwnerReferences[0].Controller).ShouldNot(BeNil())
			Expect(*claim.OwnerReferences[0].Controller).Should(BeTrue())
			Expect(claim.OwnerReferences[0].BlockOwnerDeletion).ShouldNot(BeNil())
			Expect(*claim.OwnerReferences[0].BlockOwnerDeletion).Should(BeTrue())
		})
	})

	// Serialize metric tests to keep counters consistent
	Context("Metrics tests", Serial, func() {
		It("should increment may_claim_created when a Claim is created", func(ctx context.Context) {
			By("recording the metric value before reconciling")
			before := testutil.ToFloat64(claimCreated)

			By("creating a pod with a flavor annotation and reconciling")
			Expect(reconcilePod(ctx, createPod(ctx, tenantNs.Name,
				map[string]string{pod.KueueFlavorLabelPrefix + flavor: ""},
				nil,
			))).Should(Equal(reconcile.Result{}))

			By("verifying the metric was incremented by 1")
			Expect(testutil.ToFloat64(claimCreated)).Should(Equal(before + 1))
		})

		It("should not increment may_claim_created when a Claim already exists", func(ctx context.Context) {
			By("creating a pod and reconciling to create the initial Claim")
			p := createPod(ctx, tenantNs.Name,
				map[string]string{pod.KueueFlavorLabelPrefix + flavor: ""},
				nil,
			)
			Expect(reconcilePod(ctx, p)).Error().ShouldNot(HaveOccurred())

			By("recording the metric value before re-reconciling")
			before := testutil.ToFloat64(claimCreated)

			By("reconciling the same pod again")
			Expect(reconcilePod(ctx, p)).Error().ShouldNot(HaveOccurred())

			By("verifying the metric was not incremented")
			Expect(testutil.ToFloat64(claimCreated)).Should(Equal(before))
		})

		It("should not increment may_claim_created for a non-tenant namespace", func(ctx context.Context) {
			By("recording the metric value before reconciling")
			before := testutil.ToFloat64(claimCreated)

			By("creating a pod in a non-tenant namespace and reconciling")
			Expect(reconcilePod(ctx, createPod(ctx, regularNs.Name,
				map[string]string{pod.KueueFlavorLabelPrefix + flavor: ""},
				nil,
			))).Should(Equal(reconcile.Result{}))

			By("verifying the metric was not incremented")
			Expect(testutil.ToFloat64(claimCreated)).Should(Equal(before))
		})
	})
})
