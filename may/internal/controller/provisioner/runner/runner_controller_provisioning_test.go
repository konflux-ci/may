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
	"errors"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	kueuev1beta1 "sigs.k8s.io/kueue/apis/kueue/v1beta1"

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

		Describe("Error Handling", func() {
			It("returns the error if it can not retrieve the Runner", func(ctx context.Context) {
				By("Create RunnerReconciler with a failing client")
				expectedErr := errors.New("get error")
				reconciler := &RunnerReconciler{
					Client: interceptor.NewClient(k8sClient, interceptor.Funcs{
						Get: func(ctx context.Context, client client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
							return expectedErr
						},
					}),
					Scheme: k8sClient.Scheme(),
				}

				By("Reconciling the created resource")
				_, err := reconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).To(MatchError(expectedErr))
			})

			It("returns the error if the update to add the finalizer fails", func(ctx context.Context) {
				By("Create RunnerReconciler with a failing client")
				expectedErr := errors.New("update error")
				reconciler := &RunnerReconciler{
					Client: interceptor.NewClient(k8sClient, interceptor.Funcs{
						Update: func(ctx context.Context, client client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
							return expectedErr
						},
					}),
					Scheme: k8sClient.Scheme(),
				}

				By("Reconciling the created resource")
				_, err := reconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).To(MatchError(expectedErr))
			})
		})

		Describe("Reconciliation", Serial, Ordered, func() {
			When("the Runner has no status", func() {
				It("initializes the runner", func(ctx context.Context) {
					By("Reconciling the created resource")
					_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
						NamespacedName: typeNamespacedName,
					})
					Expect(err).NotTo(HaveOccurred())

					By("Checking the Runner has a finalizer and its status is set to Initializing")
					r := maykonfluxcidevv1alpha1.Runner{}
					Expect(k8sClient.Get(ctx, typeNamespacedName, &r)).To(Succeed())
					Expect(r.Finalizers).To(ConsistOf(constants.RunnerControllerFinalizer))
					Expect(r.Status).ToNot(BeNil())
					Expect(runner.IsInitializing(r)).To(BeTrue())
				})
			})

			When("the Runner is Initializing", func() {
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
						Expect(k8sClient.Get(ctx, pk, &p)).To(MatchError(kerrors.IsNotFound, "NotFound"))
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

			When("Runner is Ready", func() {
				It("returns any error produced when creating the ClusterQueue", func(ctx context.Context) {
					By("Creating a reconciler with a failing client")
					expectedErr := errors.New("error creating the ClusterQueue")
					reconciler := &RunnerReconciler{
						Client: interceptor.NewClient(k8sClient, interceptor.Funcs{
							Create: func(ctx context.Context, client client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
								Expect(obj).To(BeAssignableToTypeOf(&kueuev1beta1.ClusterQueue{}))
								return expectedErr
							},
						}),
						Scheme: k8sClient.Scheme(),
					}

					By("Reconciling the Runner")
					_, err := reconciler.Reconcile(ctx, reconcile.Request{
						NamespacedName: typeNamespacedName,
					})
					Expect(err).To(MatchError(expectedErr))

					By("Checking the Runner is Ready")
					r := maykonfluxcidevv1alpha1.Runner{}
					Expect(k8sClient.Get(ctx, typeNamespacedName, &r)).To(Succeed())
					Expect(runner.IsReady(r)).To(BeTrue())

					By("Checking the ClusterQueue doesn't exists")
					cq := kueuev1beta1.ClusterQueue{}
					Expect(k8sClient.Get(ctx, types.NamespacedName{Name: r.Name}, &cq)).To(MatchError(kerrors.IsNotFound, "IsNotFound"))
				})

				It("creates the ClusterQueue", func(ctx context.Context) {
					By("Reconciling the Runner")
					_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
						NamespacedName: typeNamespacedName,
					})
					Expect(err).NotTo(HaveOccurred())

					By("Checking the Runner is Ready")
					r := maykonfluxcidevv1alpha1.Runner{}
					Expect(k8sClient.Get(ctx, typeNamespacedName, &r)).To(Succeed())
					Expect(runner.IsReady(r)).To(BeTrue())

					By("Checking the ClusterQueue exists")
					cq := kueuev1beta1.ClusterQueue{}
					Expect(k8sClient.Get(ctx, types.NamespacedName{Name: r.Name}, &cq)).To(Succeed())
					Expect(cq.Spec.StopPolicy).To(And(
						Not(BeNil()),
						HaveValue(Equal(kueuev1beta1.None))))
				})

				It("recreates the ClusterQueue if deleted", func(ctx context.Context) {
					By("Deleting the ClusterQueue")
					cq := kueuev1beta1.ClusterQueue{}
					Expect(k8sClient.Get(ctx, types.NamespacedName{Name: typeNamespacedName.Name}, &cq)).To(Succeed())
					Expect(k8sClient.Delete(ctx, &cq)).To(Succeed())

					By("Reconciling the Runner")
					_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
						NamespacedName: typeNamespacedName,
					})
					Expect(err).NotTo(HaveOccurred())

					By("Checking the Runner is Ready")
					r := maykonfluxcidevv1alpha1.Runner{}
					Expect(k8sClient.Get(ctx, typeNamespacedName, &r)).To(Succeed())
					Expect(runner.IsReady(r)).To(BeTrue())

					By("Checking the ClusterQueue exists")
					cq = kueuev1beta1.ClusterQueue{}
					Expect(k8sClient.Get(ctx, types.NamespacedName{Name: r.Name}, &cq)).To(Succeed())
					Expect(cq.Spec.StopPolicy).To(And(
						Not(BeNil()),
						HaveValue(Equal(kueuev1beta1.None))))
				})
			})

			When("Runner is Reserved", func() {
				It("returns no error", func(ctx context.Context) {
					By("Reserving the Runner")
					r := maykonfluxcidevv1alpha1.Runner{}
					Expect(k8sClient.Get(ctx, typeNamespacedName, &r)).To(Succeed())
					Expect(runner.IsReady(r)).To(BeTrue())
					r.Spec.InUseBy = &maykonfluxcidevv1alpha1.ClaimReference{
						Name:      "dummy-claim",
						Namespace: "dummy-namespace",
					}
					Expect(k8sClient.Update(ctx, &r)).To(Succeed())

					By("Reconciling the Runner")
					_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
						NamespacedName: typeNamespacedName,
					})
					Expect(err).NotTo(HaveOccurred())

					By("Checking the Runner is Reserved")
					r = maykonfluxcidevv1alpha1.Runner{}
					Expect(k8sClient.Get(ctx, typeNamespacedName, &r)).To(Succeed())
					Expect(runner.IsReady(r)).To(BeTrue())
					Expect(runner.IsReserved(r)).To(BeTrue())

					By("Checking the ClusterQueue exists")
					cq := kueuev1beta1.ClusterQueue{}
					Expect(k8sClient.Get(ctx, types.NamespacedName{Name: r.Name}, &cq)).To(Succeed())
					Expect(cq.Spec.StopPolicy).To(And(
						Not(BeNil()),
						HaveValue(Equal(kueuev1beta1.None))))
				})

				It("returns any error produced when creating the ClusterQueue", func(ctx context.Context) {
					By("Creating a reconciler with a failing client")
					expectedErr := errors.New("error creating the ClusterQueue")
					reconciler := &RunnerReconciler{
						Client: interceptor.NewClient(k8sClient, interceptor.Funcs{
							Create: func(ctx context.Context, client client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
								Expect(obj).To(BeAssignableToTypeOf(&kueuev1beta1.ClusterQueue{}))
								return expectedErr
							},
						}),
						Scheme: k8sClient.Scheme(),
					}

					By("Deleting the ClusterQueue")
					cq := kueuev1beta1.ClusterQueue{}
					Expect(k8sClient.Get(ctx, types.NamespacedName{Name: typeNamespacedName.Name}, &cq)).To(Succeed())
					Expect(k8sClient.Delete(ctx, &cq)).To(Succeed())

					By("Reconciling the Runner")
					_, err := reconciler.Reconcile(ctx, reconcile.Request{
						NamespacedName: typeNamespacedName,
					})
					Expect(err).To(MatchError(expectedErr))

					By("Checking the Runner is Ready")
					r := maykonfluxcidevv1alpha1.Runner{}
					Expect(k8sClient.Get(ctx, typeNamespacedName, &r)).To(Succeed())
					Expect(runner.IsReady(r)).To(BeTrue())

					By("Checking the ClusterQueue doesn't exists")
					Expect(k8sClient.Get(ctx, types.NamespacedName{Name: r.Name}, &cq)).To(MatchError(kerrors.IsNotFound, "IsNotFound"))
				})

				It("recreates the ClusterQueue if not present", func(ctx context.Context) {
					By("Deleting the ClusterQueue")
					cq := kueuev1beta1.ClusterQueue{}
					Expect(k8sClient.Get(ctx, types.NamespacedName{Name: typeNamespacedName.Name}, &cq)).
						To(MatchError(kerrors.IsNotFound, "IsNotFound"))

					By("Reconciling the Runner")
					_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
						NamespacedName: typeNamespacedName,
					})
					Expect(err).NotTo(HaveOccurred())

					By("Checking the Runner is Ready")
					r := maykonfluxcidevv1alpha1.Runner{}
					Expect(k8sClient.Get(ctx, typeNamespacedName, &r)).To(Succeed())
					Expect(runner.IsReady(r)).To(BeTrue())

					By("Checking the ClusterQueue exists")
					cq = kueuev1beta1.ClusterQueue{}
					Expect(k8sClient.Get(ctx, types.NamespacedName{Name: r.Name}, &cq)).To(Succeed())
					Expect(cq.Spec.StopPolicy).To(And(
						Not(BeNil()),
						HaveValue(Equal(kueuev1beta1.None))))
				})
			})
		})
	})
})
