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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ = Describe("DynamicHost Controller", func() {
	It("ignores a missing host", func(ctx context.Context) {
		scheme := newTestScheme()
		cl := fake.NewClientBuilder().WithScheme(scheme).Build()
		reconciler := NewDynamicHostReconciler(cl, scheme)

		_, err := reconciler.Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{Name: "missing", Namespace: "default"},
		})
		Expect(err).ShouldNot(HaveOccurred())
	})

	It("adds the AWS driver finalizer", func(ctx context.Context) {
		host := newTestDynamicHost("add-finalizer", nil)
		scheme := newTestScheme()
		cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(host).WithStatusSubresource(host).Build()
		reconciler := NewDynamicHostReconciler(cl, scheme)

		_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(host)})
		Expect(err).ShouldNot(HaveOccurred())

		updated := &maykonfluxcidevv1alpha1.DynamicHost{}
		Expect(cl.Get(ctx, client.ObjectKeyFromObject(host), updated)).Should(Succeed())
		Expect(updated.Finalizers).Should(ContainElement(AWSDriverFinalizer))
	})

	It("initializes status state to Pending", func(ctx context.Context) {
		host := newTestDynamicHost("init-pending", func(h *maykonfluxcidevv1alpha1.DynamicHost) {
			h.Finalizers = []string{AWSDriverFinalizer}
		})
		scheme := newTestScheme()
		cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(host).WithStatusSubresource(host).Build()
		reconciler := NewDynamicHostReconciler(cl, scheme)

		_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(host)})
		Expect(err).ShouldNot(HaveOccurred())

		updated := &maykonfluxcidevv1alpha1.DynamicHost{}
		Expect(cl.Get(ctx, client.ObjectKeyFromObject(host), updated)).Should(Succeed())
		Expect(updated.Status.State).ShouldNot(BeNil())
		Expect(*updated.Status.State).Should(Equal(maykonfluxcidevv1alpha1.HostActualStatePending))
	})

	It("removes the finalizer on delete", func(ctx context.Context) {
		now := metav1.Now()
		host := newTestDynamicHost("dynamic-delete", func(h *maykonfluxcidevv1alpha1.DynamicHost) {
			h.Finalizers = []string{AWSDriverFinalizer}
			h.DeletionTimestamp = &now
		})
		scheme := newTestScheme()
		cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(host).WithStatusSubresource(host).Build()
		reconciler := NewDynamicHostReconciler(cl, scheme)

		_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(host)})
		Expect(err).ShouldNot(HaveOccurred())

		updated := &maykonfluxcidevv1alpha1.DynamicHost{}
		err = cl.Get(ctx, client.ObjectKeyFromObject(host), updated)
		Expect(client.IgnoreNotFound(err)).Should(Succeed())
		if err == nil {
			Expect(updated.Finalizers).ShouldNot(ContainElement(AWSDriverFinalizer))
		}
	})
})
