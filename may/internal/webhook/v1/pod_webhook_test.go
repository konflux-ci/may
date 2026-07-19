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

package v1

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/konflux-ci/may/api/v1alpha1"
	"github.com/konflux-ci/may/pkg/constants"
	"github.com/konflux-ci/may/pkg/pod"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func newPod(name string, annotations map[string]string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Annotations: annotations,
		},
	}
}

func newScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(s))
	utilruntime.Must(v1alpha1.AddToScheme(s))
	return s
}

func newDHA(name, flavor string) *v1alpha1.DynamicHostAutoscaler {
	return &v1alpha1.DynamicHostAutoscaler{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: v1alpha1.DynamicHostAutoscalerSpec{
			Flavor:   flavor,
			Template: v1alpha1.DynamicHostTemplate{Spec: v1alpha1.DynamicHostSpec{}},
		},
	}
}

func defaulterWithObjects(objs ...runtime.Object) PodCustomDefaulter {
	return PodCustomDefaulter{
		reader: fake.NewClientBuilder().
			WithScheme(newScheme()).
			WithRuntimeObjects(objs...).
			Build(),
	}
}

var _ = Describe("Pod Webhook", func() {
	var defaulter PodCustomDefaulter

	BeforeEach(func() {
		defaulter = PodCustomDefaulter{
			reader: fake.NewClientBuilder().WithScheme(newScheme()).Build(),
		}
	})

	When("pod has a flavor annotation", func() {
		It("should gate the pod", func(ctx context.Context) {
			By("setting up a backend so option 2 doesn't skip gating")
			defaulter = defaulterWithObjects(newDHA("amd64-autoscaler", "amd64"))

			By("defaulting a pod with a flavor annotation")
			p := newPod("test-pod", map[string]string{pod.KueueFlavorLabelPrefix + "amd64": ""})
			Expect(defaulter.Default(ctx, p)).Should(Succeed())

			By("verifying the scheduling gate was added")
			Expect(p.Spec.SchedulingGates).Should(ContainElement(
				corev1.PodSchedulingGate{Name: constants.MayPodSchedulingGate},
			))
		})
	})

	When("pod already has the scheduling gate", func() {
		It("should not add a duplicate gate", func(ctx context.Context) {
			By("setting up a backend")
			defaulter = defaulterWithObjects(newDHA("amd64-autoscaler", "amd64"))

			By("defaulting a pod that already has the scheduling gate")
			p := newPod("already-gated-pod", map[string]string{pod.KueueFlavorLabelPrefix + "amd64": ""})
			p.Spec.SchedulingGates = []corev1.PodSchedulingGate{
				{Name: constants.MayPodSchedulingGate},
			}
			Expect(defaulter.Default(ctx, p)).Should(Succeed())

			By("verifying only one scheduling gate exists")
			Expect(p.Spec.SchedulingGates).Should(HaveLen(1))
		})
	})

	DescribeTable("should not gate the pod",
		func(ctx context.Context, annotations map[string]string) {
			By("defaulting the pod")
			p := newPod("test-pod", annotations)
			Expect(defaulter.Default(ctx, p)).Should(Succeed())

			By("verifying no scheduling gate was added")
			Expect(p.Spec.SchedulingGates).Should(BeEmpty())
		},
		Entry("when pod has no flavor annotation",
			map[string]string{"some-other": "annotation"}),
		Entry("when pod has nil annotations",
			nil),
	)

	// --- Option 1: Static exclusion list ---
	When("flavor is a well-known local flavor", func() {
		DescribeTable("should skip gating",
			func(ctx SpecContext, flavor string) {
				By("defaulting a pod with a local flavor")
				p := newPod("local-pod", map[string]string{pod.KueueFlavorLabelPrefix + flavor: ""})
				Expect(defaulter.Default(ctx, p)).Should(Succeed())

				By("verifying no scheduling gate was added")
				Expect(p.Spec.SchedulingGates).Should(BeEmpty())
			},
			Entry("localhost", "localhost"),
			Entry("local", "local"),
		)
	})

	// --- Option 2: Absence-of-backend detection ---
	When("no DynamicHostAutoscaler or StaticHost exists for the flavor", func() {
		It("should skip gating", func(ctx SpecContext) {
			By("defaulting a pod with an unknown flavor")
			p := newPod("no-backend-pod", map[string]string{pod.KueueFlavorLabelPrefix + "unknown-flavor": ""})
			Expect(defaulter.Default(ctx, p)).Should(Succeed())

			By("verifying no scheduling gate was added")
			Expect(p.Spec.SchedulingGates).Should(BeEmpty())
		})
	})

	When("a DynamicHostAutoscaler exists for the flavor", func() {
		It("should gate the pod", func(ctx SpecContext) {
			By("setting up a DynamicHostAutoscaler")
			defaulter = defaulterWithObjects(newDHA("aws-amd64", "aws-linux-amd64"))

			By("defaulting the pod")
			p := newPod("remote-pod", map[string]string{pod.KueueFlavorLabelPrefix + "aws-linux-amd64": ""})
			Expect(defaulter.Default(ctx, p)).Should(Succeed())

			By("verifying the scheduling gate was added")
			Expect(p.Spec.SchedulingGates).Should(ContainElement(
				corev1.PodSchedulingGate{Name: constants.MayPodSchedulingGate},
			))
		})
	})

	When("a StaticHost exists for the flavor", func() {
		It("should gate the pod", func(ctx SpecContext) {
			By("setting up a StaticHost")
			defaulter = defaulterWithObjects(&v1alpha1.StaticHost{
				ObjectMeta: metav1.ObjectMeta{Name: "static-arm64"},
				Spec: v1alpha1.StaticHostSpec{
					HostCoreSpec: v1alpha1.HostCoreSpec{
						Flavor:  "static-linux-arm64",
						Status:  v1alpha1.HostStatusReady,
						RootKey: corev1.LocalObjectReference{Name: "key"},
					},
					Runners: v1alpha1.HostSpecRunners{
						Instances: 1,
						Resources: corev1.ResourceList{},
					},
				},
			})

			By("defaulting the pod")
			p := newPod("static-pod", map[string]string{pod.KueueFlavorLabelPrefix + "static-linux-arm64": ""})
			Expect(defaulter.Default(ctx, p)).Should(Succeed())

			By("verifying the scheduling gate was added")
			Expect(p.Spec.SchedulingGates).Should(ContainElement(
				corev1.PodSchedulingGate{Name: constants.MayPodSchedulingGate},
			))
		})
	})

	// Serialize metric tests to keep counters consistent
	Context("Metrics tests", Serial, func() {
		It("should increment the metric when a pod is gated", func(ctx context.Context) {
			By("setting up a backend")
			defaulter = defaulterWithObjects(newDHA("amd64-autoscaler", "amd64"))

			By("recording the metric value before defaulting")
			before := testutil.ToFloat64(podsGated)
			Expect(defaulter.Default(ctx, newPod("test-pod", map[string]string{pod.KueueFlavorLabelPrefix + "amd64": ""}))).Should(Succeed())

			By("verifying the metric was incremented by 1")
			Expect(testutil.ToFloat64(podsGated)).Should(Equal(before + 1))
		})

		It("should not increment the metric when the pod already has the gate", func(ctx context.Context) {
			By("setting up a backend")
			defaulter = defaulterWithObjects(newDHA("amd64-autoscaler", "amd64"))

			By("recording the metric value before defaulting")
			before := testutil.ToFloat64(podsGated)
			p := newPod("already-gated-pod", map[string]string{pod.KueueFlavorLabelPrefix + "amd64": ""})
			p.Spec.SchedulingGates = []corev1.PodSchedulingGate{
				{Name: constants.MayPodSchedulingGate},
			}
			Expect(defaulter.Default(ctx, p)).Should(Succeed())

			By("verifying the metric was not incremented")
			Expect(testutil.ToFloat64(podsGated)).Should(Equal(before))
		})

		It("should not increment the metric when the pod is not gated", func(ctx context.Context) {
			By("recording the metric value before defaulting")
			before := testutil.ToFloat64(podsGated)
			Expect(defaulter.Default(ctx, newPod("test-pod", map[string]string{"some-other": "annotation"}))).Should(Succeed())

			By("verifying the metric was not incremented")
			Expect(testutil.ToFloat64(podsGated)).Should(Equal(before))
		})
	})
})
