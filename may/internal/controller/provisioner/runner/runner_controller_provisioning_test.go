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

package runner

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	maykonfluxcidevv1alpha1 "github.com/konflux-ci/may/api/v1alpha1"
	"github.com/konflux-ci/may/internal/controller/provisioner/constants"
	"github.com/konflux-ci/may/pkg/runner"
)

var _ = Describe("Runner Controller (Provisioning)", Ordered, Serial, func() {
	Context("Reconciles a non existing Runner", func() {
		It("doesn't propagate the error", func(ctx context.Context) {
			controllerReconciler := &RunnerReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "non-existing-runner",
					Namespace: "default",
				},
			})
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("Initializes a Runner", func() {
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
			r := &maykonfluxcidevv1alpha1.Runner{
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
								Name: "provisioning-pod-1",
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
							{
								Name: "provisioning-pod-2",
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
			Expect(k8sClient.Create(ctx, r)).To(Succeed())
		})

		AfterAll(func(ctx context.Context) {
			// TODO(user): Cleanup logic after each test, like removing the r instance.
			r := &maykonfluxcidevv1alpha1.Runner{}
			err := k8sClient.Get(ctx, typeNamespacedName, r)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance Runner")
			Expect(k8sClient.Delete(ctx, r)).To(Succeed())
		})

		Describe("Initialization", Serial, Ordered, func() {
			It("initializes the runner", func(ctx context.Context) {
				By("Reconciling the created resource")
				_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())

				r := maykonfluxcidevv1alpha1.Runner{}
				Expect(k8sClient.Get(ctx, typeNamespacedName, &r)).To(Succeed())
				Expect(r.Finalizers).To(Equal([]string{constants.RunnerControllerFinalizer}))
				Expect(r.Status).ToNot(BeNil())
				Expect(runner.IsInitializing(r)).To(BeTrue())
			})

			It("creates the first provisioning pod", func(ctx context.Context) {
				By("Reconciling the runner")
				_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())

				By("Checking the first provisioning pod is created")
				r := maykonfluxcidevv1alpha1.Runner{}
				Expect(k8sClient.Get(ctx, typeNamespacedName, &r)).To(Succeed())
				{
					p := corev1.Pod{}
					pk := types.NamespacedName{
						Name: fmt.Sprintf("p-%s-%s",
							r.Name,
							r.Spec.Hooks.Provisioning[0].Name),
						Namespace: r.Namespace,
					}
					Expect(k8sClient.Get(ctx, pk, &p)).To(Succeed())
					Expect(p.Spec).ToNot(Equal(r.Spec.Hooks.Provisioning[0].Template))
					Expect(controllerutil.HasControllerReference(&p)).To(BeTrue())
				}

				By("Checking the second provisioning pod doesn't exist yet")
				{
					p := corev1.Pod{}
					pk := types.NamespacedName{
						Name: fmt.Sprintf("p-%s-%s",
							r.Name,
							r.Spec.Hooks.Provisioning[1].Name),
						Namespace: r.Namespace,
					}
					Expect(k8sClient.Get(ctx, pk, &p)).To(MatchError(errors.IsNotFound, "NotFound"))
				}
			})

			It("creates the second provisioning pod when the first is done", func(ctx context.Context) {
				By("The pod succeed and Runner status is updated by the RunnerHookController")
				r := maykonfluxcidevv1alpha1.Runner{}
				Expect(k8sClient.Get(ctx, typeNamespacedName, &r)).To(Succeed())

				r.Status.HooksStatus.Provisioning = []maykonfluxcidevv1alpha1.RunnerHookStatus{
					{
						Hook:              r.Spec.Hooks.Provisioning[0].Name,
						Phase:             corev1.PodSucceeded,
						PodMessage:        "pod succeeded",
						DeletionTimestamp: nil,
					},
				}
				Expect(k8sClient.Status().Update(ctx, &r)).To(Succeed())

				By("Reconciling the Runner")
				_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())

				By("Checking the second provisioning pod is created")
				p := corev1.Pod{}
				pk := types.NamespacedName{
					Name: fmt.Sprintf("p-%s-%s",
						r.Name,
						r.Spec.Hooks.Provisioning[1].Name),
					Namespace: r.Namespace,
				}

				Expect(k8sClient.Get(ctx, pk, &p)).To(Succeed())
				Expect(p.Spec).ToNot(Equal(r.Spec.Hooks.Provisioning[1].Template))
				Expect(controllerutil.HasControllerReference(&p)).To(BeTrue())
			})

			It("sets the runner as ready when provisioning pods are done", func(ctx context.Context) {
				By("The pod succeed and Runner status is updated by the RunnerHookController")
				r := maykonfluxcidevv1alpha1.Runner{}
				Expect(k8sClient.Get(ctx, typeNamespacedName, &r)).To(Succeed())

				r.Status.HooksStatus.Provisioning = []maykonfluxcidevv1alpha1.RunnerHookStatus{
					{
						Hook:              r.Spec.Hooks.Provisioning[0].Name,
						Phase:             corev1.PodSucceeded,
						PodMessage:        "pod succeeded",
						DeletionTimestamp: nil,
					},
					{
						Hook:              r.Spec.Hooks.Provisioning[1].Name,
						Phase:             corev1.PodSucceeded,
						PodMessage:        "pod succeeded",
						DeletionTimestamp: nil,
					},
				}
				Expect(k8sClient.Status().Update(ctx, &r)).To(Succeed())

				By("Reconciling the Runner")
				_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())

				By("Checking the Runner is Ready")
				Expect(k8sClient.Get(ctx, typeNamespacedName, &r)).To(Succeed())
				Expect(runner.IsReady(r)).To(BeTrue())
			})
		})
	})
})
