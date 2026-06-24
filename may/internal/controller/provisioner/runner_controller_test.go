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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	maykonfluxcidevv1alpha1 "github.com/konflux-ci/may/api/v1alpha1"
	"github.com/konflux-ci/may/pkg/runner"
)

var image = "registry.access.redhat.com/ubi10/ubi-micro@sha256:f86852f349dcd2b9ebccef4c8a46fdb75ff2fef9fde8581cef1feddb706be7ba"

var _ = Describe("Runner Controller", Ordered, Serial, func() {
	Context("When reconciling a resource", func() {
		const (
			runnerName = "test-resource"
			namespace  = "default"
		)

		var controllerReconciler *RunnerReconciler

		typeNamespacedName := types.NamespacedName{
			Name:      runnerName,
			Namespace: namespace,
		}

		BeforeAll(func(ctx context.Context) {
			By("Create RunnerReconciler")
			controllerReconciler = &RunnerReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			By("creating a Runner")
			runner := &maykonfluxcidevv1alpha1.Runner{
				ObjectMeta: metav1.ObjectMeta{
					Name:      runnerName,
					Namespace: namespace,
				},
				Spec: maykonfluxcidevv1alpha1.RunnerSpec{
					Queue: &maykonfluxcidevv1alpha1.RunnerQueue{
						Cohort: "my-cohort",
					},
					Flavor: "my-flavor",
					Resources: v1.ResourceList{
						v1.ResourceCPU: resource.MustParse("1"),
					},
					Hooks: &maykonfluxcidevv1alpha1.RunnerHooks{
						Provisioning: []maykonfluxcidevv1alpha1.RunnerHookPodTemplateSpec{
							{
								Name: "provisioning-pod",
								Template: v1.PodTemplateSpec{
									Spec: v1.PodSpec{
										RestartPolicy: v1.RestartPolicyNever,
										Containers: []v1.Container{
											{
												Name:          "provisioning-container",
												RestartPolicy: ptr.To(v1.ContainerRestartPolicyNever),
												Image:         image,
												Command:       []string{"exit"},
												Args:          []string{"0"},
											},
										},
									},
								},
							},
						},
						Cleanup: []maykonfluxcidevv1alpha1.RunnerHookPodTemplateSpec{},
					},
				},
			}
			Expect(k8sClient.Create(ctx, runner)).To(Succeed())
		})

		AfterAll(func(ctx context.Context) {
			// TODO(user): Cleanup logic after each test, like removing the runner instance.
			runner := &maykonfluxcidevv1alpha1.Runner{}
			err := k8sClient.Get(ctx, typeNamespacedName, runner)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance Runner")
			Expect(k8sClient.Delete(ctx, runner)).To(Succeed())
		})

		It("initializes the runner", func(ctx context.Context) {
			By("Reconciling the created resource")
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			r := maykonfluxcidevv1alpha1.Runner{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, &r)).To(Succeed())
			Expect(r.Finalizers).To(Equal([]string{RunnerControllerFinalizer}))
			Expect(r.Status).ToNot(BeNil())
			Expect(runner.IsInitializing(r)).To(BeTrue())
		})

		It("creates the first provisioning pod", func(ctx context.Context) {
			By("Reconciling the created resource")
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			r := maykonfluxcidevv1alpha1.Runner{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, &r)).To(Succeed())

			p := corev1.Pod{}
			pk := types.NamespacedName{
				Name: fmt.Sprintf("%s-%s-%s",
					"p",
					r.Name,
					r.Spec.Hooks.Provisioning[0].Name),
				Namespace: r.Namespace,
			}

			Expect(k8sClient.Get(ctx, pk, &p)).To(Succeed())
			Expect(p.Spec).ToNot(Equal(r.Spec.Hooks.Provisioning[0].Template))
			Expect(controllerutil.HasControllerReference(&p)).To(BeTrue())
			Expect(r.Status.HooksStatus.Provisioning).NotTo(BeEmpty())
		})

		It("waits for the provisioning pod to complete", func(ctx context.Context) {
			By("Reconciling the created resource")
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			r := maykonfluxcidevv1alpha1.Runner{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, &r)).To(Succeed())
			Expect(r.Finalizers).To(Equal([]string{RunnerControllerFinalizer}))
			Expect(r.Status).ToNot(BeNil())
			Expect(r.Status.HooksStatus).ToNot(BeNil())
			Expect(r.Status.HooksStatus.Provisioning).To(HaveLen(1))

			for _, h := range r.Status.HooksStatus.Provisioning {
				p := corev1.Pod{}
				pk := types.NamespacedName{
					Name:      h.Pod,
					Namespace: r.Namespace,
				}
				Expect(k8sClient.Get(ctx, pk, &p)).To(Succeed())

				Expect(h.Phase).To(Equal(p.Status.Phase))
				Expect(h.PodMessage).To(Equal(p.Status.Message))
				Expect(h.DeletionTimestamp).To(Equal(p.DeletionTimestamp))
				Expect(h.Pod).To(Equal(p.Name))
			}
		})
	})
})
