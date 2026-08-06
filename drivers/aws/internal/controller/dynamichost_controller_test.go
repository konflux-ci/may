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

package controller

import (
	"context"

	maykonfluxcidevv1alpha1 "github.com/konflux-ci/may/api/v1alpha1"
	internalconfig "github.com/konflux-ci/may/drivers/aws/internal/config"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ = Describe("DynamicHost Controller", func() {
	It("marks the host Ready when SSH is reachable", func(ctx context.Context) {
		host := &maykonfluxcidevv1alpha1.DynamicHost{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "dynamic-ready",
				Namespace:  "default",
				Finalizers: []string{AWSDriverFinalizer},
				Labels: map[string]string{
					DriverLabel: DriverLabelValueAWS,
				},
				Annotations: map[string]string{
					internalconfig.AnnotationInstanceID: "i-dynamic001",
				},
			},
			Spec: maykonfluxcidevv1alpha1.DynamicHostSpec{
				HostCoreSpec: maykonfluxcidevv1alpha1.HostCoreSpec{
					Flavor: "test-flavor",
					Status: maykonfluxcidevv1alpha1.HostStatusReady,
				},
				Runner: maykonfluxcidevv1alpha1.HostSpecRunner{
					Resources: corev1.ResourceList{
						corev1.ResourceCPU: resource.MustParse("1"),
					},
				},
			},
		}
		host.Status.State = ptr.To(maykonfluxcidevv1alpha1.HostActualStatePending)

		scheme := newTestScheme()
		cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(host).WithStatusSubresource(host).Build()
		reconciler := &DynamicHostReconciler{
			Client:          cl,
			Scheme:          scheme,
			hostStateHelper: newHostStateHelper(cl),
			newEC2client: func(context.Context, *maykonfluxcidevv1alpha1.DynamicHost) (hostEC2Client, error) {
				return &mockEC2Client{
					sshReadyOnPublicIP: func(context.Context, string) (string, bool, error) {
						return "203.0.113.11", true, nil
					},
				}, nil
			},
		}

		_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(host)})
		Expect(err).ShouldNot(HaveOccurred())

		updated := &maykonfluxcidevv1alpha1.DynamicHost{}
		Expect(cl.Get(ctx, client.ObjectKeyFromObject(host), updated)).Should(Succeed())
		Expect(updated.Status.State).ShouldNot(BeNil())
		Expect(*updated.Status.State).Should(Equal(maykonfluxcidevv1alpha1.HostActualStateReady))
		Expect(updated.Annotations[internalconfig.AnnotationPublicIPAddress]).Should(Equal("203.0.113.11"))
	})
})
