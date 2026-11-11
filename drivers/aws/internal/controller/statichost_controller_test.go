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
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ = Describe("StaticHost Controller", func() {
	It("ignores a missing host", func(ctx context.Context) {
		scheme := newTestScheme()
		cl := fake.NewClientBuilder().WithScheme(scheme).Build()
		reconciler := NewStaticHostReconciler(cl, scheme)

		res, err := reconciler.Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{Name: "missing", Namespace: "default"},
		})
		Expect(err).ShouldNot(HaveOccurred())
		Expect(res).Should(Equal(ctrl.Result{}))
	})

	It("adds the AWS driver finalizer", func(ctx context.Context) {
		host := newTestStaticHost("add-finalizer", nil)
		scheme := newTestScheme()
		cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(host).WithStatusSubresource(host).Build()
		reconciler := NewStaticHostReconciler(cl, scheme)

		res, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(host)})
		Expect(err).ShouldNot(HaveOccurred())
		Expect(res).Should(Equal(ctrl.Result{}))

		updated := &maykonfluxcidevv1alpha1.StaticHost{}
		Expect(cl.Get(ctx, client.ObjectKeyFromObject(host), updated)).Should(Succeed())
		Expect(updated.Finalizers).Should(ContainElement(AWSDriverFinalizer))
	})

	It("initializes status state to Pending", func(ctx context.Context) {
		host := newTestStaticHost("init-pending", func(h *maykonfluxcidevv1alpha1.StaticHost) {
			h.Finalizers = []string{AWSDriverFinalizer}
		})
		scheme := newTestScheme()
		cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(host).WithStatusSubresource(host).Build()
		reconciler := NewStaticHostReconciler(cl, scheme)

		res, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(host)})
		Expect(err).ShouldNot(HaveOccurred())
		Expect(res).Should(Equal(ctrl.Result{}))

		updated := &maykonfluxcidevv1alpha1.StaticHost{}
		Expect(cl.Get(ctx, client.ObjectKeyFromObject(host), updated)).Should(Succeed())
		Expect(updated.Status.State).Should(HaveValue(Equal(maykonfluxcidevv1alpha1.HostActualStatePending)))
	})

	It("leaves an initialized host unchanged", func(ctx context.Context) {
		pending := maykonfluxcidevv1alpha1.HostActualStatePending
		host := newTestStaticHost("already-pending", func(h *maykonfluxcidevv1alpha1.StaticHost) {
			h.Finalizers = []string{AWSDriverFinalizer}
			h.Status.State = &pending
		})
		scheme := newTestScheme()
		cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(host).WithStatusSubresource(host).Build()
		reconciler := NewStaticHostReconciler(cl, scheme)

		res, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(host)})
		Expect(err).ShouldNot(HaveOccurred())
		Expect(res).Should(Equal(ctrl.Result{}))

		updated := &maykonfluxcidevv1alpha1.StaticHost{}
		Expect(cl.Get(ctx, client.ObjectKeyFromObject(host), updated)).Should(Succeed())
		Expect(updated.Status.State).Should(HaveValue(Equal(maykonfluxcidevv1alpha1.HostActualStatePending)))
	})

	It("leaves a Ready host unchanged", func(ctx context.Context) {
		ready := maykonfluxcidevv1alpha1.HostActualStateReady
		host := newTestStaticHost("already-ready", func(h *maykonfluxcidevv1alpha1.StaticHost) {
			h.Finalizers = []string{AWSDriverFinalizer}
			h.Status.State = &ready
		})
		scheme := newTestScheme()
		cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(host).WithStatusSubresource(host).Build()
		reconciler := NewStaticHostReconciler(cl, scheme)

		res, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(host)})
		Expect(err).ShouldNot(HaveOccurred())
		Expect(res).Should(Equal(ctrl.Result{}))

		updated := &maykonfluxcidevv1alpha1.StaticHost{}
		Expect(cl.Get(ctx, client.ObjectKeyFromObject(host), updated)).Should(Succeed())
		Expect(updated.Status.State).Should(HaveValue(Equal(maykonfluxcidevv1alpha1.HostActualStateReady)))
	})

	It("removes the finalizer on delete", func(ctx context.Context) {
		now := metav1.Now()
		host := newTestStaticHost("static-delete", func(h *maykonfluxcidevv1alpha1.StaticHost) {
			h.Finalizers = []string{AWSDriverFinalizer, "may.konflux-ci.dev/other"}
			h.DeletionTimestamp = &now
		})
		scheme := newTestScheme()
		cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(host).WithStatusSubresource(host).Build()
		reconciler := NewStaticHostReconciler(cl, scheme)

		res, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(host)})
		Expect(err).ShouldNot(HaveOccurred())
		Expect(res).Should(Equal(ctrl.Result{}))

		updated := &maykonfluxcidevv1alpha1.StaticHost{}
		err = cl.Get(ctx, client.ObjectKeyFromObject(host), updated)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(updated.Finalizers).ShouldNot(ContainElement(AWSDriverFinalizer))
	})
})
