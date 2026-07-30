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
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	maykonfluxcidevv1alpha1 "github.com/konflux-ci/may/api/v1alpha1"
	"github.com/konflux-ci/may/internal/controller/provisioner/constants"
	"github.com/konflux-ci/may/pkg/runner"
)

var _ = Describe("Runner Controller (Cleanup)", Ordered, Serial, func() {
	Context("Cleans up a Runner", func() {
		const (
			runnerName = "test-runner-to-cleanup"
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
					Name:       runnerName,
					Namespace:  namespace,
					Finalizers: []string{constants.RunnerControllerFinalizer},
				},
				Spec: maykonfluxcidevv1alpha1.RunnerSpec{
					Flavor: "my-flavor",
					Resources: corev1.ResourceList{
						corev1.ResourceCPU: resource.MustParse("1"),
					},
					Hooks: &maykonfluxcidevv1alpha1.RunnerHooks{
						Provisioning: []maykonfluxcidevv1alpha1.RunnerHookPodTemplateSpec{},
						Cleanup: []maykonfluxcidevv1alpha1.RunnerHookPodTemplateSpec{
							{
								Name: "cleanup-pod-1",
								Template: corev1.PodTemplateSpec{
									Spec: corev1.PodSpec{
										RestartPolicy: corev1.RestartPolicyNever,
										Containers: []corev1.Container{
											{
												Name:          "cleanup-container",
												RestartPolicy: ptr.To(corev1.ContainerRestartPolicyNever),
												Image:         image,
												Command:       []string{"exit"},
												Args:          []string{"0"},
											},
										},
									},
								},
							},
							{
								Name: "cleanup-pod-2",
								Template: corev1.PodTemplateSpec{
									Spec: corev1.PodSpec{
										RestartPolicy: corev1.RestartPolicyNever,
										Containers: []corev1.Container{
											{
												Name:          "cleanup-container",
												RestartPolicy: ptr.To(corev1.ContainerRestartPolicyNever),
												Image:         image,
												Command:       []string{"exit"},
												Args:          []string{"0"},
											},
										},
									},
								},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, r)).To(Succeed())
		})

		Describe("Cleanup", func() {
			It("sets the NotReadyCleaning state", func(ctx context.Context) {
				By("Deleting the Runner")
				r := maykonfluxcidevv1alpha1.Runner{}
				Expect(k8sClient.Get(ctx, typeNamespacedName, &r)).To(Succeed())
				Expect(k8sClient.Delete(ctx, &r)).To(Succeed())
				Expect(k8sClient.Get(ctx, typeNamespacedName, &r)).To(Succeed())
				Expect(r.DeletionTimestamp).NotTo(BeNil())

				By("Reconciling the Runner")
				_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())

				By("Checking the Runner's state")
				Expect(k8sClient.Get(ctx, typeNamespacedName, &r)).To(Succeed())
				Expect(runner.IsCleaning(r)).To(BeTrue())
			})

			It("creates the first cleanup pod", func(ctx context.Context) {
				By("Calculating the expected pod built from the Runner's PodSpec")
				r := maykonfluxcidevv1alpha1.Runner{}
				Expect(k8sClient.Get(ctx, typeNamespacedName, &r)).To(Succeed())
				pk := types.NamespacedName{
					Name: fmt.Sprintf("c-%s-%s",
						r.Name,
						r.Spec.Hooks.Cleanup[0].Name),
					Namespace: r.Namespace,
				}
				ep := corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{Name: pk.Name, Namespace: pk.Namespace},
					Spec:       r.Spec.Hooks.Cleanup[0].Template.Spec,
				}
				Expect(k8sClient.Create(ctx, &ep, client.DryRunAll)).To(Succeed())

				By("Reconciling the runner")
				_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())

				By("Checking the first cleanup pod is created")
				Expect(k8sClient.Get(ctx, typeNamespacedName, &r)).To(Succeed())
				{
					ap := corev1.Pod{}
					pk := types.NamespacedName{
						Name: fmt.Sprintf("c-%s-%s",
							r.Name,
							r.Spec.Hooks.Cleanup[0].Name),
						Namespace: r.Namespace,
					}
					Expect(k8sClient.Get(ctx, pk, &ap)).To(Succeed())
					Expect(ap.Spec).To(Equal(ep.Spec))
					Expect(controllerutil.HasControllerReference(&ap)).To(BeTrue())
				}

				By("Checking the second cleanup pod doesn't exist yet")
				{
					p := corev1.Pod{}
					pk := types.NamespacedName{
						Name: fmt.Sprintf("c-%s-%s",
							r.Name,
							r.Spec.Hooks.Cleanup[1].Name),
						Namespace: r.Namespace,
					}
					Expect(k8sClient.Get(ctx, pk, &p)).To(MatchError(errors.IsNotFound, "NotFound"))
				}
			})

			It("creates the second cleanup pod when the first is done", func(ctx context.Context) {
				By("Calculating the expected pod built from the Runner's PodSpec")
				r := maykonfluxcidevv1alpha1.Runner{}
				Expect(k8sClient.Get(ctx, typeNamespacedName, &r)).To(Succeed())
				pk := types.NamespacedName{
					Name: fmt.Sprintf("c-%s-%s",
						r.Name,
						r.Spec.Hooks.Cleanup[1].Name),
					Namespace: r.Namespace,
				}
				ep := corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{Name: pk.Name, Namespace: pk.Namespace},
					Spec:       r.Spec.Hooks.Cleanup[1].Template.Spec,
				}
				Expect(k8sClient.Create(ctx, &ep, client.DryRunAll)).To(Succeed())

				By("The pod succeed and Runner status is updated by the RunnerHookController")
				Expect(k8sClient.Get(ctx, typeNamespacedName, &r)).To(Succeed())

				r.Status.HooksStatus.Cleanup = []maykonfluxcidevv1alpha1.RunnerHookStatus{
					{
						Hook:              r.Spec.Hooks.Cleanup[0].Name,
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

				By("Checking the second cleanup pod is created")
				p := corev1.Pod{}
				Expect(k8sClient.Get(ctx, pk, &p)).To(Succeed())
				Expect(p.Spec).To(Equal(ep.Spec))
				Expect(controllerutil.HasControllerReference(&p)).To(BeTrue())
			})

			It("sets the runner as ready when cleanup pods are done", func(ctx context.Context) {
				By("The pod succeed and Runner status is updated by the RunnerHookController")
				r := maykonfluxcidevv1alpha1.Runner{}
				Expect(k8sClient.Get(ctx, typeNamespacedName, &r)).To(Succeed())

				r.Status.HooksStatus.Cleanup = []maykonfluxcidevv1alpha1.RunnerHookStatus{
					{
						Hook:              r.Spec.Hooks.Cleanup[0].Name,
						Phase:             corev1.PodSucceeded,
						PodMessage:        "pod succeeded",
						DeletionTimestamp: nil,
					},
					{
						Hook:              r.Spec.Hooks.Cleanup[1].Name,
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

				By("Checking the runner is deleted")
				Expect(k8sClient.Get(ctx, typeNamespacedName, &r)).To(MatchError(errors.IsNotFound, "NotFound"))
			})
		})
	})
})
