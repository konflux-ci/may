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
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type mockEC2Client struct {
	launchInstance     func(context.Context, internalconfig.AWSConfiguration) (string, error)
	describeInstance   func(context.Context, string) (internalec2.InstanceDetails, error)
	sshReadyOnPublicIP func(context.Context, string) (string, bool, error)
	terminateInstance  func(context.Context, string) error
}

func (m *mockEC2Client) LaunchInstance(ctx context.Context, cfg internalconfig.AWSConfiguration) (string, error) {
	if m.launchInstance != nil {
		return m.launchInstance(ctx, cfg)
	}
	return "", nil
}

func (m *mockEC2Client) DescribeInstance(ctx context.Context, instanceID string) (internalec2.InstanceDetails, error) {
	if m.describeInstance != nil {
		return m.describeInstance(ctx, instanceID)
	}
	return internalec2.InstanceDetails{}, nil
}

func (m *mockEC2Client) SSHReadyOnPublicIP(ctx context.Context, instanceID string) (string, bool, error) {
	if m.sshReadyOnPublicIP != nil {
		return m.sshReadyOnPublicIP(ctx, instanceID)
	}
	return "", false, nil
}

func (m *mockEC2Client) TerminateInstance(ctx context.Context, instanceID string) error {
	if m.terminateInstance != nil {
		return m.terminateInstance(ctx, instanceID)
	}
	return nil
}

func newTestScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	utilruntime.Must(maykonfluxcidevv1alpha1.AddToScheme(scheme))
	return scheme
}

func newTestStaticHost(name string, mutate func(*maykonfluxcidevv1alpha1.StaticHost)) *maykonfluxcidevv1alpha1.StaticHost {
	host := &maykonfluxcidevv1alpha1.StaticHost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
			Labels: map[string]string{
				DriverLabel: DriverLabelValueAWS,
			},
		},
		Spec: maykonfluxcidevv1alpha1.StaticHostSpec{
			HostCoreSpec: maykonfluxcidevv1alpha1.HostCoreSpec{
				Flavor: "test-flavor",
				Status: maykonfluxcidevv1alpha1.HostStatusPending,
			},
			Runners: maykonfluxcidevv1alpha1.HostSpecRunners{
				Resources: corev1.ResourceList{
					corev1.ResourceCPU: resource.MustParse("1"),
				},
				Instances: 1,
			},
		},
	}
	if mutate != nil {
		mutate(host)
	}
	return host
}

func newHostStateHelper(cl client.Client) HostStateHelper {
	return HostStateHelper{Client: cl}
}

var _ = Describe("HostStateHelper", func() {
	var scheme *runtime.Scheme

	BeforeEach(func() {
		scheme = newTestScheme()
	})

	It("launches an EC2 instance when Ready is requested", func(ctx context.Context) {
		host := newTestStaticHost("launch-instance", func(h *maykonfluxcidevv1alpha1.StaticHost) {
			h.Status.State = ptr.To(maykonfluxcidevv1alpha1.HostActualStatePending)
			h.Annotations = map[string]string{
				internalconfig.AnnotationRegion:       "us-east-1",
				internalconfig.AnnotationAmi:          "ami-0123456789abcdef0",
				internalconfig.AnnotationInstanceType: "m6a.large",
			}
		})

		mockEC2 := &mockEC2Client{
			launchInstance: func(context.Context, internalconfig.AWSConfiguration) (string, error) {
				return "i-launch001", nil
			},
		}
		cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(host).WithStatusSubresource(host).Build()
		reconciler := newHostStateHelper(cl)

		result, err := reconciler.EnsureInstanceReady(ctx, mockEC2, host, func(context.Context) (internalconfig.AWSConfiguration, error) {
			return internalconfig.AWSConfiguration{
				Region:       "us-east-1",
				Ami:          "ami-0123456789abcdef0",
				InstanceType: "m6a.large",
			}, nil
		}, &host.Status.State)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(result.RequeueAfter).Should(Equal(instancePollInterval))

		updated := &maykonfluxcidevv1alpha1.StaticHost{}
		Expect(cl.Get(ctx, client.ObjectKeyFromObject(host), updated)).Should(Succeed())
		Expect(updated.Annotations[internalconfig.AnnotationInstanceID]).Should(Equal("i-launch001"))
	})

	It("requeues while waiting for SSH readiness", func(ctx context.Context) {
		host := newTestStaticHost("wait-ssh", func(h *maykonfluxcidevv1alpha1.StaticHost) {
			h.Status.State = ptr.To(maykonfluxcidevv1alpha1.HostActualStatePending)
			h.Annotations = map[string]string{
				internalconfig.AnnotationInstanceID: "i-wait001",
			}
		})

		mockEC2 := &mockEC2Client{
			sshReadyOnPublicIP: func(context.Context, string) (string, bool, error) {
				return "203.0.113.10", false, nil
			},
		}
		cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(host).WithStatusSubresource(host).Build()
		reconciler := newHostStateHelper(cl)

		result, err := reconciler.EnsureInstanceReady(ctx, mockEC2, host, func(context.Context) (internalconfig.AWSConfiguration, error) {
			return internalconfig.AWSConfiguration{}, nil
		}, &host.Status.State)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(result.RequeueAfter).Should(Equal(instancePollInterval))
	})

	It("marks the host Ready when SSH is reachable", func(ctx context.Context) {
		host := newTestStaticHost("host-ready", func(h *maykonfluxcidevv1alpha1.StaticHost) {
			h.Status.State = ptr.To(maykonfluxcidevv1alpha1.HostActualStatePending)
			h.Annotations = map[string]string{
				internalconfig.AnnotationInstanceID: "i-ready001",
			}
		})

		mockEC2 := &mockEC2Client{
			sshReadyOnPublicIP: func(context.Context, string) (string, bool, error) {
				return "203.0.113.10", true, nil
			},
		}
		cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(host).WithStatusSubresource(host).Build()
		reconciler := newHostStateHelper(cl)

		result, err := reconciler.EnsureInstanceReady(ctx, mockEC2, host, func(context.Context) (internalconfig.AWSConfiguration, error) {
			return internalconfig.AWSConfiguration{}, nil
		}, &host.Status.State)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(result.RequeueAfter).Should(BeZero())

		updated := &maykonfluxcidevv1alpha1.StaticHost{}
		Expect(cl.Get(ctx, client.ObjectKeyFromObject(host), updated)).Should(Succeed())
		Expect(updated.Status.State).ShouldNot(BeNil())
		Expect(*updated.Status.State).Should(Equal(maykonfluxcidevv1alpha1.HostActualStateReady))
		Expect(updated.Annotations[internalconfig.AnnotationPublicIPAddress]).Should(Equal("203.0.113.10"))
	})

	It("requeues when a Ready host's instance stops running", func(ctx context.Context) {
		host := newTestStaticHost("not-running", func(h *maykonfluxcidevv1alpha1.StaticHost) {
			h.Annotations = map[string]string{
				internalconfig.AnnotationInstanceID: "i-stop001",
			}
		})

		mockEC2 := &mockEC2Client{
			describeInstance: func(context.Context, string) (internalec2.InstanceDetails, error) {
				return internalec2.InstanceDetails{State: types.InstanceStateNameStopped}, nil
			},
		}
		cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(host).WithStatusSubresource(host).Build()
		reconciler := newHostStateHelper(cl)

		result, err := reconciler.EnsureInstanceStillRunning(ctx, mockEC2, host)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(result.RequeueAfter).Should(Equal(instancePollInterval))
	})

	It("terminates the instance during deletion", func(ctx context.Context) {
		host := newTestStaticHost("finalize-terminate", func(h *maykonfluxcidevv1alpha1.StaticHost) {
			h.Annotations = map[string]string{
				internalconfig.AnnotationInstanceID: "i-term001",
			}
		})

		terminated := false
		mockEC2 := &mockEC2Client{
			describeInstance: func(context.Context, string) (internalec2.InstanceDetails, error) {
				return internalec2.InstanceDetails{State: types.InstanceStateNameRunning}, nil
			},
			terminateInstance: func(context.Context, string) error {
				terminated = true
				return nil
			},
		}
		cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(host).WithStatusSubresource(host).Build()
		reconciler := newHostStateHelper(cl)

		result, done, err := reconciler.EnsureInstanceTerminated(ctx, mockEC2, host)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(done).Should(BeFalse())
		Expect(result.RequeueAfter).Should(Equal(instancePollInterval))
		Expect(terminated).Should(BeTrue())
	})

	It("reports termination complete when the instance is terminated", func(ctx context.Context) {
		host := newTestStaticHost("finalize-done", func(h *maykonfluxcidevv1alpha1.StaticHost) {
			h.Annotations = map[string]string{
				internalconfig.AnnotationInstanceID: "i-gone001",
			}
		})

		mockEC2 := &mockEC2Client{
			describeInstance: func(context.Context, string) (internalec2.InstanceDetails, error) {
				return internalec2.InstanceDetails{State: types.InstanceStateNameTerminated}, nil
			},
		}
		cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(host).WithStatusSubresource(host).Build()
		reconciler := newHostStateHelper(cl)

		result, done, err := reconciler.EnsureInstanceTerminated(ctx, mockEC2, host)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(done).Should(BeTrue())
		Expect(result.RequeueAfter).Should(BeZero())
	})

	It("reports termination complete when no instance was created", func(ctx context.Context) {
		host := newTestStaticHost("finalize-no-instance", nil)
		cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(host).WithStatusSubresource(host).Build()
		reconciler := newHostStateHelper(cl)

		result, done, err := reconciler.EnsureInstanceTerminated(ctx, &mockEC2Client{}, host)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(done).Should(BeTrue())
		Expect(result.RequeueAfter).Should(BeZero())
	})
})
