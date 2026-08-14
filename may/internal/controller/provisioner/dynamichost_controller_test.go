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
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	maykonfluxcidevv1alpha1 "github.com/konflux-ci/may/api/v1alpha1"
	runner "github.com/konflux-ci/may/pkg/runner"
)

var _ = Describe("DynamicHost Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default", // TODO(user):Modify as needed
		}
		dynamichost := &maykonfluxcidevv1alpha1.DynamicHost{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind DynamicHost")
			err := k8sClient.Get(ctx, typeNamespacedName, dynamichost)
			if err != nil && errors.IsNotFound(err) {
				dynamichost = &maykonfluxcidevv1alpha1.DynamicHost{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: maykonfluxcidevv1alpha1.DynamicHostSpec{
						HostCoreSpec: maykonfluxcidevv1alpha1.HostCoreSpec{
							Flavor: "flavor",
							Status: maykonfluxcidevv1alpha1.HostStatusPending,
						},
						Runner: maykonfluxcidevv1alpha1.HostSpecRunner{
							Hooks: nil,
							Resources: v1.ResourceList{
								v1.ResourceCPU: resource.MustParse("1"),
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, dynamichost)).To(Succeed())
			}
		})

		AfterEach(func() {
			// TODO(user): Cleanup logic after each test, like removing the resource instance.
			resource := &maykonfluxcidevv1alpha1.DynamicHost{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleaning up any Runner objects associated with a DynamicHost")
			runners := &maykonfluxcidevv1alpha1.RunnerList{}
			err = k8sClient.List(ctx, runners)
			Expect(err).NotTo(HaveOccurred())
			for _, runner := range runners.Items {
				err = k8sClient.Delete(ctx, &runner)
				Expect(err).NotTo(HaveOccurred())
			}

			By("Cleanup the specific resource instance DynamicHost")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})
		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &DynamicHostReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			// TODO(user): Add more specific assertions depending on your controller's reconciliation logic.
			// Example: If you expect a certain status condition after reconciliation, verify it here.
		})

		It("should allocate a Runner", func() {
			By(`Creating a Runner object when no Runner object already exists 
				and the DynamicHost's status is Ready`)

			Expect(_reconcile(ctx, typeNamespacedName)).NotTo(HaveOccurred())

			dynamicHost := &maykonfluxcidevv1alpha1.DynamicHost{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, dynamicHost)).NotTo(HaveOccurred())

			dynamicHost.Status.State = ptr.To(maykonfluxcidevv1alpha1.HostActualStateReady)
			Expect(k8sClient.Status().Update(ctx, dynamicHost)).NotTo(HaveOccurred())
			Expect(_reconcile(ctx, typeNamespacedName)).NotTo(HaveOccurred())

			By("updating the DynamicHost if the Runner's status is Ready")
			r := &maykonfluxcidevv1alpha1.Runner{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, r)).NotTo(HaveOccurred())
			Expect(runner.SetReady(r)).To(BeTrue())
			Expect(k8sClient.Status().Update(ctx, r)).NotTo(HaveOccurred())
			Expect(k8sClient.Get(ctx, typeNamespacedName, r)).NotTo(HaveOccurred())
			Expect(len(r.Status.Conditions)).NotTo(BeZero())
			Expect(_reconcile(ctx, typeNamespacedName)).NotTo(HaveOccurred())

			By("updating the DynamicHost if the Runner has a Pipeline")
			r.Labels["tekton.dev/pipeline"] = "test-pipeline"
			Expect(k8sClient.Update(ctx, r)).NotTo(HaveOccurred())

			Expect(_reconcile(ctx, typeNamespacedName)).NotTo(HaveOccurred())
			Expect(k8sClient.Get(ctx, typeNamespacedName, dynamicHost)).NotTo(HaveOccurred())
			Expect(dynamicHost.Status.Pipeline).To(Equal("test-pipeline"))

			By("updating the DynamicHost if the Runner is marked for deletion")
			Expect(k8sClient.Delete(ctx, r)).NotTo(HaveOccurred())
			Expect(_reconcile(ctx, typeNamespacedName))
			Expect(k8sClient.Get(ctx, typeNamespacedName, dynamicHost)).NotTo(HaveOccurred())
			Expect(*dynamicHost.Status.State).To(Equal(maykonfluxcidevv1alpha1.HostActualStateDraining))
		})
	})
})

func _reconcile(ctx context.Context, namespacedName types.NamespacedName) error {
	controllerReconciler := &DynamicHostReconciler{
		Client: k8sClient,
		Scheme: k8sClient.Scheme(),
	}
	_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
		NamespacedName: namespacedName,
	})
	return err
}
