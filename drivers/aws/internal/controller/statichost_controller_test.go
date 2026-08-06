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

	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	maykonfluxcidevv1alpha1 "github.com/konflux-ci/may/api/v1alpha1"
	internalconfig "github.com/konflux-ci/may/drivers/aws/internal/config"
	internalec2 "github.com/konflux-ci/may/drivers/aws/internal/ec2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ = Describe("StaticHost Controller", func() {
	It("adds the AWS driver finalizer", func(ctx context.Context) {
		host := newTestStaticHost("add-finalizer", nil)
		scheme := newTestScheme()
		cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(host).WithStatusSubresource(host).Build()
		reconciler := &StaticHostReconciler{Client: cl, Scheme: scheme, hostStateHelper: newHostStateHelper(cl)}

		_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(host)})
		Expect(err).ShouldNot(HaveOccurred())

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
		reconciler := &StaticHostReconciler{Client: cl, Scheme: scheme, hostStateHelper: newHostStateHelper(cl)}

		_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(host)})
		Expect(err).ShouldNot(HaveOccurred())

		updated := &maykonfluxcidevv1alpha1.StaticHost{}
		Expect(cl.Get(ctx, client.ObjectKeyFromObject(host), updated)).Should(Succeed())
		Expect(updated.Status.State).ShouldNot(BeNil())
		Expect(*updated.Status.State).Should(Equal(maykonfluxcidevv1alpha1.HostActualStatePending))
	})

	It("marks the host Ready when SSH is reachable", func(ctx context.Context) {
		host := newTestStaticHost("static-ready", func(h *maykonfluxcidevv1alpha1.StaticHost) {
			h.Finalizers = []string{AWSDriverFinalizer}
			h.Spec.Status = maykonfluxcidevv1alpha1.HostStatusReady
			h.Status.State = ptr.To(maykonfluxcidevv1alpha1.HostActualStatePending)
			h.Annotations = map[string]string{
				internalconfig.AnnotationInstanceID: "i-static001",
			}
		})
		scheme := newTestScheme()
		cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(host).WithStatusSubresource(host).Build()
		reconciler := &StaticHostReconciler{
			Client:          cl,
			Scheme:          scheme,
			hostStateHelper: newHostStateHelper(cl),
			newEC2client: func(context.Context, *maykonfluxcidevv1alpha1.StaticHost) (hostEC2Client, error) {
				return &mockEC2Client{
					sshReadyOnPublicIP: func(context.Context, string) (string, bool, error) {
						return "203.0.113.10", true, nil
					},
				}, nil
			},
		}

		_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(host)})
		Expect(err).ShouldNot(HaveOccurred())

		updated := &maykonfluxcidevv1alpha1.StaticHost{}
		Expect(cl.Get(ctx, client.ObjectKeyFromObject(host), updated)).Should(Succeed())
		Expect(updated.Status.State).ShouldNot(BeNil())
		Expect(*updated.Status.State).Should(Equal(maykonfluxcidevv1alpha1.HostActualStateReady))
	})

	It("removes the finalizer when the instance is terminated", func(ctx context.Context) {
		now := metav1.Now()
		host := newTestStaticHost("static-finalize", func(h *maykonfluxcidevv1alpha1.StaticHost) {
			h.Finalizers = []string{AWSDriverFinalizer}
			h.DeletionTimestamp = &now
			h.Annotations = map[string]string{
				internalconfig.AnnotationInstanceID: "i-gone001",
			}
		})
		scheme := newTestScheme()
		cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(host).WithStatusSubresource(host).Build()
		reconciler := &StaticHostReconciler{
			Client:          cl,
			Scheme:          scheme,
			hostStateHelper: newHostStateHelper(cl),
			newEC2client: func(context.Context, *maykonfluxcidevv1alpha1.StaticHost) (hostEC2Client, error) {
				return &mockEC2Client{
					describeInstance: func(context.Context, string) (internalec2.InstanceDetails, error) {
						return internalec2.InstanceDetails{State: types.InstanceStateNameTerminated}, nil
					},
				}, nil
			},
		}

		_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(host)})
		Expect(err).ShouldNot(HaveOccurred())

		updated := &maykonfluxcidevv1alpha1.StaticHost{}
		err = cl.Get(ctx, client.ObjectKeyFromObject(host), updated)
		Expect(client.IgnoreNotFound(err)).Should(Succeed())
		if err == nil {
			Expect(updated.Finalizers).ShouldNot(ContainElement(AWSDriverFinalizer))
		}
	})

	It("removes the finalizer on delete when no instance was created", func(ctx context.Context) {
		now := metav1.Now()
		host := newTestStaticHost("static-no-instance", func(h *maykonfluxcidevv1alpha1.StaticHost) {
			h.Finalizers = []string{AWSDriverFinalizer}
			h.DeletionTimestamp = &now
		})
		scheme := newTestScheme()
		cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(host).WithStatusSubresource(host).Build()
		reconciler := &StaticHostReconciler{Client: cl, Scheme: scheme, hostStateHelper: newHostStateHelper(cl)}

		_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(host)})
		Expect(err).ShouldNot(HaveOccurred())

		updated := &maykonfluxcidevv1alpha1.StaticHost{}
		err = cl.Get(ctx, client.ObjectKeyFromObject(host), updated)
		Expect(client.IgnoreNotFound(err)).Should(Succeed())
		if err == nil {
			Expect(updated.Finalizers).ShouldNot(ContainElement(AWSDriverFinalizer))
		}
	})
})
