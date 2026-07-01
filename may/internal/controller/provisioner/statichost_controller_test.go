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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus/testutil"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	mayprovkonfluxcidevv1alpha1 "github.com/konflux-ci/may/api/v1alpha1"
)

var _ = Describe("Host Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName string = "test-resource"

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default", // TODO(user):Modify as needed
		}
		var (
			k8sClient client.Client
			host      *mayprovkonfluxcidevv1alpha1.StaticHost
		)

		scheme := runtime.NewScheme()
		utilruntime.Must(mayprovkonfluxcidevv1alpha1.AddToScheme(scheme))

		BeforeEach(func() {
			host = &mayprovkonfluxcidevv1alpha1.StaticHost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: "default",
				},
				Spec: mayprovkonfluxcidevv1alpha1.StaticHostSpec{
					HostCoreSpec: mayprovkonfluxcidevv1alpha1.HostCoreSpec{
						Flavor: "flavor",
						Status: mayprovkonfluxcidevv1alpha1.HostStatusPending,
					},
					Runners: mayprovkonfluxcidevv1alpha1.HostSpecRunners{
						Resources: v1.ResourceList{
							v1.ResourceCPU: resource.MustParse("1"),
						},
						Instances: 1,
					},
				},
			}

			k8sClient = fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(host).
				Build()
		})

		It("should successfully reconcile the resource", func(ctx context.Context) {
			By("Reconciling the created resource")
			controllerReconciler := &StaticHostReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			Expect(controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})).To(Equal(reconcile.Result{}))
		})

		When("Host is ready", func() {
			BeforeEach(func(ctx context.Context) {
				ready := mayprovkonfluxcidevv1alpha1.HostActualStateReady
				host.Status.State = &ready
				Expect(k8sClient.Update(ctx, host)).To(Succeed())
			})

			It("should ensure a runner is created", func(ctx context.Context) {
				By("Reconciling the created resource")
				controllerReconciler := &StaticHostReconciler{
					Client: k8sClient,
					Scheme: k8sClient.Scheme(),
				}

				Expect(controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})).To(Equal(reconcile.Result{}))

				By("Asserting a runner was created")
				runners := mayprovkonfluxcidevv1alpha1.RunnerList{}
				Expect(k8sClient.List(ctx, &runners)).To(Succeed())
				Expect(runners.Items).To(HaveLen(1))
			})

			It("should ensure runners remain on further reconciles", func(ctx context.Context) {
				By("Reconciling the created resource")
				controllerReconciler := &StaticHostReconciler{
					Client: k8sClient,
					Scheme: k8sClient.Scheme(),
				}

				Expect(controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})).To(Equal(reconcile.Result{}))

				By("Asserting the number of runners created")
				runners := mayprovkonfluxcidevv1alpha1.RunnerList{}
				Expect(k8sClient.List(ctx, &runners)).To(Succeed())
				Expect(runners.Items).To(HaveLen(1))

				By("Re-reconciling the created resource")
				// when re-reconciled, assert that no new runners have been created
				Expect(controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})).To(Equal(reconcile.Result{}))

				By("Asserting no further runners have been created")
				Expect(k8sClient.List(ctx, &runners)).To(Succeed())
				Expect(runners.Items).To(HaveLen(1))
			})

			It("should set owner references on created runners", func(ctx context.Context) {
				By("Reconciling the created resource")
				controllerReconciler := &StaticHostReconciler{
					Client: k8sClient,
					Scheme: k8sClient.Scheme(),
				}

				Expect(controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})).To(Equal(reconcile.Result{}))

				By("Asserting each runner has an owner reference")
				runners := mayprovkonfluxcidevv1alpha1.RunnerList{}
				Expect(k8sClient.List(ctx, &runners)).To(Succeed())
				Expect(runners.Items).To(HaveEach(Satisfy(func(runner mayprovkonfluxcidevv1alpha1.Runner) bool {
					found, err := controllerutil.HasOwnerReference(runner.OwnerReferences, host, k8sClient.Scheme())
					return err == nil && found
				})))
			})

			It("should ensure no runners have deletion timestamps set", func(ctx context.Context) {
				By("Reconciling the created resource")
				controllerReconciler := &StaticHostReconciler{
					Client: k8sClient,
					Scheme: k8sClient.Scheme(),
				}

				Expect(controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})).To(Equal(reconcile.Result{}))

				By("Asserting each runner has no deletion timestamp")
				runners := mayprovkonfluxcidevv1alpha1.RunnerList{}
				Expect(k8sClient.List(ctx, &runners)).To(Succeed())
				Expect(runners.Items).To(HaveEach(Satisfy(func(runner mayprovkonfluxcidevv1alpha1.Runner) bool {
					return runner.DeletionTimestamp == nil
				})))
			})

			// Serialize these tests to keep metrics counters consistent
			Context("Metrics tests", Serial, func() {
				It("should increment the created counter on runner creation", func(ctx context.Context) {
					controllerReconciler := &StaticHostReconciler{
						Client: k8sClient,
						Scheme: k8sClient.Scheme(),
					}

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
					k8sClient = fake.NewClientBuilder().
						WithScheme(scheme).
						WithObjects(host).
						WithInterceptorFuncs(interceptor.Funcs{
							Create: func(ctx context.Context, client client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
								_, ok := obj.(*mayprovkonfluxcidevv1alpha1.Runner)
								if ok {
									return errors.NewBadRequest("runners not allowed")
								}
								return client.Create(ctx, obj, opts...)
							},
						}).
						Build()

					By("Asserting the existence of the may_runner_created metric")
					oldValue := testutil.ToFloat64(runnersCreated)

					By("Reconciling the created resource")
					controllerReconciler := &StaticHostReconciler{
						Client: k8sClient,
						Scheme: k8sClient.Scheme(),
					}

					Expect(controllerReconciler.Reconcile(ctx, reconcile.Request{
						NamespacedName: typeNamespacedName,
					})).Error().To(HaveOccurred())

					By("Asserting runner creation metrics were not incremented")
					value := testutil.ToFloat64(runnersCreated)
					Expect(value).To(Equal(oldValue))
				})
			})
		})
	})
})
