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
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus/testutil"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	mayprovkonfluxcidevv1alpha1 "github.com/konflux-ci/may/api/v1alpha1"
	"github.com/konflux-ci/may/pkg/constants"
	"github.com/konflux-ci/may/pkg/runner"
)

var _ = Describe("Host Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName string = "test-resource-"
		const namespaceName string = "statichost-"

		var (
			typeNamespacedName   types.NamespacedName
			host                 *mayprovkonfluxcidevv1alpha1.StaticHost
			ns                   *corev1.Namespace
			controllerReconciler *StaticHostReconciler
		)

		scheme := runtime.NewScheme()
		utilruntime.Must(corev1.AddToScheme(scheme))
		utilruntime.Must(mayprovkonfluxcidevv1alpha1.AddToScheme(scheme))

		BeforeEach(func(ctx context.Context) {
			ns = &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: namespaceName,
				},
			}
			Expect(k8sClient.Create(ctx, ns)).To(Or(Succeed(), MatchError(kerrors.IsAlreadyExists, "already exists")))

			host = &mayprovkonfluxcidevv1alpha1.StaticHost{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: resourceName,
					Namespace:    ns.Name,
				},
				Spec: mayprovkonfluxcidevv1alpha1.StaticHostSpec{
					HostCoreSpec: mayprovkonfluxcidevv1alpha1.HostCoreSpec{
						Flavor: "flavor",
						Status: mayprovkonfluxcidevv1alpha1.HostStatusPending,
					},
					Runners: mayprovkonfluxcidevv1alpha1.HostSpecRunners{
						Resources: corev1.ResourceList{
							corev1.ResourceCPU: resource.MustParse("1"),
						},
						Instances: 1,
					},
				},
			}

			Expect(k8sClient.Create(ctx, host)).To(Succeed())
			typeNamespacedName = client.ObjectKeyFromObject(host)

			controllerReconciler = &StaticHostReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
		})

		AfterEach(func(ctx context.Context) {
			err := k8sClient.Get(ctx, typeNamespacedName, host)
			if err == nil {
				host.Finalizers = []string{}
				Expect(k8sClient.Update(ctx, host)).To(Succeed())
				Expect(k8sClient.Delete(ctx, host)).
					To(Or(Succeed(), MatchError(kerrors.IsNotFound, "not found")))
			}
			Expect(k8sClient.Delete(ctx, ns)).
				To(Or(Succeed(), MatchError(kerrors.IsNotFound, "not found")))
		})

		It("should successfully reconcile the resource", func(ctx context.Context) {
			By("Reconciling the created resource")
			Expect(controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})).To(Equal(reconcile.Result{}))
		})

		When("Host is ready", func() {
			BeforeEach(func(ctx context.Context) {
				ready := mayprovkonfluxcidevv1alpha1.HostActualStateReady
				host.Status.State = &ready
				Expect(k8sClient.Status().Update(ctx, host)).To(Succeed())
			})

			It("should ensure a runner is created", func(ctx context.Context) {
				By("Reconciling the created resource")
				Expect(controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})).To(Equal(reconcile.Result{}))

				By("Asserting a runner was created")
				runners := fetchRunners(ctx, k8sClient, typeNamespacedName)
				Expect(runners.Items).To(HaveLen(1))
			})

			It("should ensure runners remain on further reconciles", func(ctx context.Context) {
				By("Reconciling the created resource")
				Expect(controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})).To(Equal(reconcile.Result{}))

				By("Asserting the number of runners created")
				runners := fetchRunners(ctx, k8sClient, typeNamespacedName)
				Expect(runners.Items).To(HaveLen(1))

				By("Re-reconciling the created resource")
				// when re-reconciled, assert that no new runners have been created
				Expect(controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})).To(Equal(reconcile.Result{}))

				By("Asserting no further runners have been created")
				runners = fetchRunners(ctx, k8sClient, typeNamespacedName)
				Expect(runners.Items).To(HaveLen(1))
			})

			It("should set owner references on created runners", func(ctx context.Context) {
				By("Reconciling the created resource")
				Expect(controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})).To(Equal(reconcile.Result{}))

				By("Retrieving all runners")
				runners := fetchRunners(ctx, k8sClient, typeNamespacedName)

				By("Asserting each runner has an owner reference")
				Expect(runners.Items).To(HaveEach(Satisfy(func(runner mayprovkonfluxcidevv1alpha1.Runner) bool {
					hasOwnerRef, err := controllerutil.HasOwnerReference(runner.OwnerReferences, host, k8sClient.Scheme())
					return err == nil && hasOwnerRef
				})))

				By("Asserting each runner has a controller reference")
				Expect(runners.Items).To(HaveEach(Satisfy(func(runner mayprovkonfluxcidevv1alpha1.Runner) bool {
					return controllerutil.HasControllerReference(&runner)
				})))
			})

			It("should ensure no runners have deletion timestamps set", func(ctx context.Context) {
				By("Reconciling the created resource")
				Expect(controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})).To(Equal(reconcile.Result{}))

				By("Retrieving all runners")
				runners := fetchRunners(ctx, k8sClient, typeNamespacedName)

				By("Asserting each runner has no deletion timestamp")
				Expect(runners.Items).To(HaveEach(HaveField("DeletionTimestamp", BeNil())))
			})

			// Serialize these tests to keep metrics counters consistent
			Context("Metrics tests", Serial, func() {
				It("should increment the created counter on runner creation", func(ctx context.Context) {
					By("Asserting the existence of the may_runner_created metric")
					oldValue := testutil.ToFloat64(runnersCreated)

					By("Reconciling the created resource")
					Expect(controllerReconciler.Reconcile(ctx, reconcile.Request{
						NamespacedName: typeNamespacedName,
					})).To(Equal(reconcile.Result{}))

					By("Asserting the may_runner_created metric has increased by the number of instances")
					value := testutil.ToFloat64(runnersCreated)
					Expect(value).To(Equal(float64(host.Spec.Runners.Instances) + oldValue))
				})

				It("should propagate errors when the runner fails to be created", func(ctx context.Context) {
					By("Configuring runner creation to fail")
					k8sClient := interceptor.NewClient(k8sClient, interceptor.Funcs{
						Create: func(ctx context.Context, client client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
							_, ok := obj.(*mayprovkonfluxcidevv1alpha1.Runner)
							if ok {
								return kerrors.NewBadRequest("runners not allowed")
							}
							return client.Create(ctx, obj, opts...)
						},
					})

					By("Asserting the existence of the may_runner_created metric")
					oldValue := testutil.ToFloat64(runnersCreated)

					By("Reconciling the created resource")
					// recreate the reconciler since we need to override the client
					controllerReconciler := &StaticHostReconciler{
						Client: k8sClient,
						Scheme: k8sClient.Scheme(),
					}

					Eventually(func(g Gomega) {
						g.Expect(controllerReconciler.Reconcile(ctx, reconcile.Request{
							NamespacedName: typeNamespacedName,
						})).Error().To(MatchError(kerrors.IsBadRequest, "runners not allowed"))
					}).To(Succeed())

					By("Asserting runner creation metrics were not incremented")
					value := testutil.ToFloat64(runnersCreated)
					Expect(value).To(Equal(oldValue))
				})
			})
		})

		When("Host is draining", func() {
			// we rely on the ready state to create the runners for us
			BeforeEach(func(ctx context.Context) {
				By("Moving to the ready state")
				host.Status.State = ptr.To(mayprovkonfluxcidevv1alpha1.HostActualStateReady)
				Expect(k8sClient.Status().Update(ctx, host)).To(Succeed())

				By("Reconciling the created resource")
				Expect(controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})).Error().NotTo(HaveOccurred())

				By("Asserting runners exist")
				runners := fetchRunners(ctx, k8sClient, typeNamespacedName)
				Expect(runners.Items).To(HaveLen(1))

				By("Moving to the draining state")
				Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(host), host)).To(Succeed())
				host.Status.State = ptr.To(mayprovkonfluxcidevv1alpha1.HostActualStateDraining)
				Expect(k8sClient.Status().Update(ctx, host)).To(Succeed())
				Expect(host.Status.State).To(Equal(ptr.To(mayprovkonfluxcidevv1alpha1.HostActualStateDraining)))
			})

			It("should drain all unclaimed runners", func(ctx context.Context) {
				By("Reconciling the created resource")
				Expect(controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})).Error().NotTo(HaveOccurred())

				By("Asserting all runners are deleted")
				runners := fetchRunners(ctx, k8sClient, typeNamespacedName)
				Expect(runners.Items).To(BeEmpty())
			})

			It("should not delete claimed runners", func(ctx context.Context) {
				By("Creating a claim")
				claim := mayprovkonfluxcidevv1alpha1.Claim{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-claim",
						Namespace: host.Namespace,
					},
					Spec: mayprovkonfluxcidevv1alpha1.ClaimSpec{
						Flavor: "foobar",
					},
				}
				Expect(k8sClient.Create(ctx, &claim)).To(Succeed())

				By("claiming the existing runner")
				runners := fetchRunners(ctx, k8sClient, typeNamespacedName)
				for _, r := range runners.Items {
					runner.SetInUseBy(&r, claim)
					Expect(k8sClient.Update(ctx, &r)).To(Succeed())
				}

				By("Reconciling the created resource")
				// reconcile twice due to status updates
				Expect(controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})).Error().NotTo(HaveOccurred())
				Expect(controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})).Error().NotTo(HaveOccurred())

				By("Asserting a runner still exists")
				runners = fetchRunners(ctx, k8sClient, typeNamespacedName)
				Expect(runners.Items).To(HaveLen(1))
			})

			It("should not report errors when runner deletion races with the reconciler", func(ctx context.Context) {
				k8sClient := interceptor.NewClient(k8sClient, interceptor.Funcs{
					Delete: func(ctx context.Context, client client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
						err := client.Delete(ctx, obj, opts...)
						if err != nil {
							return err
						}
						if r, ok := obj.(*mayprovkonfluxcidevv1alpha1.Runner); ok {
							return kerrors.NewNotFound(schema.GroupResource{Group: mayprovkonfluxcidevv1alpha1.GroupVersion.Group, Resource: "runners"}, r.Name)
						}
						return nil
					},
				})

				By("Reconciling the created resource")
				controllerReconciler := &StaticHostReconciler{
					Client: k8sClient,
					Scheme: k8sClient.Scheme(),
				}
				Expect(controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})).Error().NotTo(HaveOccurred())

				By("Asserting all runners are deleted")
				runners := fetchRunners(ctx, k8sClient, typeNamespacedName)
				Expect(runners.Items).To(BeEmpty())
			})

			It("should collect errors on runner deletion failure", func(ctx context.Context) {
				errorString := "unable to delete: insufficient permissions"
				k8sClient := interceptor.NewClient(k8sClient, interceptor.Funcs{
					Delete: func(ctx context.Context, client client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
						if r, ok := obj.(*mayprovkonfluxcidevv1alpha1.Runner); ok {
							return kerrors.NewForbidden(
								schema.GroupResource{
									Group:    mayprovkonfluxcidevv1alpha1.GroupVersion.Group,
									Resource: "runners",
								},
								r.Name,
								errors.New(errorString))
						}
						return client.Delete(ctx, obj, opts...)
					},
				})

				By("Reconciling the created resource")
				controllerReconciler := &StaticHostReconciler{
					Client: k8sClient,
					Scheme: k8sClient.Scheme(),
				}
				_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).Error().To(MatchError(ContainSubstring(errorString)))
			})

			Context("Metrics tests", Serial, func() {
				It("should drain all unclaimed runners", func(ctx context.Context) {
					By("Asserting the existence of the may_runner_deleted metric")
					oldValue := testutil.ToFloat64(runnersDeleted)

					By("Reconciling the created resource")
					Expect(controllerReconciler.Reconcile(ctx, reconcile.Request{
						NamespacedName: typeNamespacedName,
					})).Error().NotTo(HaveOccurred())

					By("Asserting all runners are deleted")
					runners := fetchRunners(ctx, k8sClient, typeNamespacedName)
					Expect(runners.Items).To(BeEmpty())

					By("Asserting runner deletion metrics were incremented")
					value := testutil.ToFloat64(runnersDeleted)
					Expect(value).To(Equal(oldValue + float64(host.Spec.Runners.Instances)))
				})

				It("should not delete claimed runners", func(ctx context.Context) {
					By("Asserting the existence of the may_runner_deleted metric")
					oldValue := testutil.ToFloat64(runnersDeleted)
					By("Creating a claim")
					claim := mayprovkonfluxcidevv1alpha1.Claim{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-claim",
							Namespace: host.Namespace,
						},
						Spec: mayprovkonfluxcidevv1alpha1.ClaimSpec{
							Flavor: "foobar",
						},
					}
					Expect(k8sClient.Create(ctx, &claim)).To(Succeed())

					By("claiming the existing runner")
					runners := fetchRunners(ctx, k8sClient, typeNamespacedName)
					for _, r := range runners.Items {
						runner.SetInUseBy(&r, claim)
						Expect(k8sClient.Update(ctx, &r)).To(Succeed())
					}

					By("Reconciling the created resource")
					Expect(controllerReconciler.Reconcile(ctx, reconcile.Request{
						NamespacedName: typeNamespacedName,
					})).Error().NotTo(HaveOccurred())

					By("Asserting a runner still exists")
					runners = fetchRunners(ctx, k8sClient, typeNamespacedName)
					Expect(runners.Items).To(HaveLen(1))

					By("Asserting runner deletion metrics were not incremented")
					value := testutil.ToFloat64(runnersDeleted)
					Expect(value).To(Equal(oldValue))
				})
			})
		})
	})
})

func fetchRunners(ctx context.Context, cli client.Client, host types.NamespacedName) mayprovkonfluxcidevv1alpha1.RunnerList {
	GinkgoHelper()

	runners := mayprovkonfluxcidevv1alpha1.RunnerList{}
	Expect(cli.List(
		ctx,
		&runners,
		client.InNamespace(host.Namespace),
		client.MatchingLabels{constants.HostLabel: host.Name},
	)).To(Succeed())

	return runners
}
