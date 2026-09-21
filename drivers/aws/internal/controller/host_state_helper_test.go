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
	"errors"

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
	launchInstance    func(context.Context, internalconfig.AWSConfiguration, string) (string, error)
	describeInstance  func(context.Context, string, bool) (internalec2.InstanceDetails, error)
	sshReady          func(context.Context, string, bool) (string, bool, error)
	terminateInstance func(context.Context, string) error
}

func (m *mockEC2Client) LaunchInstance(ctx context.Context, cfg internalconfig.AWSConfiguration, clientToken string) (string, error) {
	if m.launchInstance != nil {
		return m.launchInstance(ctx, cfg, clientToken)
	}
	return "", nil
}

func (m *mockEC2Client) DescribeInstance(ctx context.Context, instanceID string, strictPublicAddress bool) (internalec2.InstanceDetails, error) {
	if m.describeInstance != nil {
		return m.describeInstance(ctx, instanceID, strictPublicAddress)
	}
	return internalec2.InstanceDetails{}, nil
}

func (m *mockEC2Client) SSHReady(ctx context.Context, instanceID string, strictPublicAddress bool) (string, bool, error) {
	if m.sshReady != nil {
		return m.sshReady(ctx, instanceID, strictPublicAddress)
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
	GinkgoHelper()
	scheme := runtime.NewScheme()
	utilruntime.Must(maykonfluxcidevv1alpha1.AddToScheme(scheme))
	return scheme
}

func newTestStaticHost(name string, mutate func(*maykonfluxcidevv1alpha1.StaticHost)) *maykonfluxcidevv1alpha1.StaticHost {
	GinkgoHelper()
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
	GinkgoHelper()
	return HostStateHelper{Client: cl}
}

var _ = Describe("HostStateHelper", func() {
	var scheme *runtime.Scheme

	BeforeEach(func() {
		scheme = newTestScheme()
	})

	It("launches an EC2 instance when Ready is requested", func(ctx context.Context) {
		host := newTestStaticHost("launch-instance", func(h *maykonfluxcidevv1alpha1.StaticHost) {
			h.UID = "host-uid-1"
			h.Status.State = ptr.To(maykonfluxcidevv1alpha1.HostActualStatePending)
			h.Annotations = map[string]string{
				internalconfig.AnnotationRegion:       "us-east-1",
				internalconfig.AnnotationAmi:          "ami-0123456789abcdef0",
				internalconfig.AnnotationInstanceType: "m6a.large",
			}
		})

		var gotClientToken string
		mockEC2 := &mockEC2Client{
			launchInstance: func(_ context.Context, _ internalconfig.AWSConfiguration, clientToken string) (string, error) {
				gotClientToken = clientToken
				return "i-launch001", nil
			},
		}
		cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(host).WithStatusSubresource(host).Build()
		reconciler := newHostStateHelper(cl)

		result, _, err := reconciler.EnsureInstanceReady(ctx, mockEC2, host, func(context.Context) (internalconfig.AWSConfiguration, error) {
			return internalconfig.AWSConfiguration{
				Region:       "us-east-1",
				Ami:          "ami-0123456789abcdef0",
				InstanceType: "m6a.large",
			}, nil
		})
		Expect(err).ShouldNot(HaveOccurred())
		Expect(result.RequeueAfter).Should(Equal(instancePollInterval))

		updated := &maykonfluxcidevv1alpha1.StaticHost{}
		Expect(cl.Get(ctx, client.ObjectKeyFromObject(host), updated)).Should(Succeed())
		Expect(updated.Annotations[internalconfig.AnnotationInstanceID]).Should(Equal("i-launch001"))
		Expect(gotClientToken).Should(Equal("host-uid-1"))
	})

	It("requeues while waiting for SSH readiness", func(ctx context.Context) {
		host := newTestStaticHost("wait-ssh", func(h *maykonfluxcidevv1alpha1.StaticHost) {
			h.Status.State = ptr.To(maykonfluxcidevv1alpha1.HostActualStatePending)
			h.Annotations = map[string]string{
				internalconfig.AnnotationInstanceID: "i-wait001",
			}
		})

		mockEC2 := &mockEC2Client{
			sshReady: func(context.Context, string, bool) (string, bool, error) {
				return "203.0.113.10", false, nil
			},
		}
		cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(host).WithStatusSubresource(host).Build()
		reconciler := newHostStateHelper(cl)

		result, _, err := reconciler.EnsureInstanceReady(ctx, mockEC2, host, func(context.Context) (internalconfig.AWSConfiguration, error) {
			return internalconfig.AWSConfiguration{}, nil
		})
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
			sshReady: func(context.Context, string, bool) (string, bool, error) {
				return "203.0.113.10", true, nil
			},
		}
		cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(host).WithStatusSubresource(host).Build()
		reconciler := newHostStateHelper(cl)

		result, actualState, err := reconciler.EnsureInstanceReady(ctx, mockEC2, host, func(context.Context) (internalconfig.AWSConfiguration, error) {
			return internalconfig.AWSConfiguration{}, nil
		})
		Expect(err).ShouldNot(HaveOccurred())
		Expect(result.RequeueAfter).Should(BeZero())
		Expect(actualState).ShouldNot(BeNil())
		Expect(*actualState).Should(Equal(maykonfluxcidevv1alpha1.HostActualStateReady))

		updated := &maykonfluxcidevv1alpha1.StaticHost{}
		Expect(cl.Get(ctx, client.ObjectKeyFromObject(host), updated)).Should(Succeed())
		Expect(*updated.Status.State).Should(Equal(maykonfluxcidevv1alpha1.HostActualStatePending))
		Expect(updated.Annotations[internalconfig.AnnotationSSHAddress]).Should(Equal("203.0.113.10"))
	})

	It("requeues while the SSH probe fails", func(ctx context.Context) {
		host := newTestStaticHost("wait-ssh-probe", func(h *maykonfluxcidevv1alpha1.StaticHost) {
			h.Status.State = ptr.To(maykonfluxcidevv1alpha1.HostActualStatePending)
			h.Annotations = map[string]string{
				internalconfig.AnnotationInstanceID: "i-probe001",
			}
		})

		mockEC2 := &mockEC2Client{
			sshReady: func(context.Context, string, bool) (string, bool, error) {
				return "", false, &internalec2.SSHProbeError{
					Addr: "203.0.113.10:22",
					Err:  errors.New("connection refused"),
				}
			},
		}
		cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(host).WithStatusSubresource(host).Build()
		reconciler := newHostStateHelper(cl)

		result, _, err := reconciler.EnsureInstanceReady(ctx, mockEC2, host, func(context.Context) (internalconfig.AWSConfiguration, error) {
			return internalconfig.AWSConfiguration{}, nil
		})
		Expect(err).ShouldNot(HaveOccurred())
		Expect(result.RequeueAfter).Should(Equal(instancePollInterval))
	})

	It("returns a terminal SSHReady error", func(ctx context.Context) {
		host := newTestStaticHost("ssh-terminated", func(h *maykonfluxcidevv1alpha1.StaticHost) {
			h.Status.State = ptr.To(maykonfluxcidevv1alpha1.HostActualStatePending)
			h.Annotations = map[string]string{
				internalconfig.AnnotationInstanceID: "i-term-ready",
			}
		})

		expectedErr := errors.New("EC2 instance i-term-ready is terminated")
		mockEC2 := &mockEC2Client{
			sshReady: func(context.Context, string, bool) (string, bool, error) {
				return "", false, expectedErr
			},
		}
		cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(host).WithStatusSubresource(host).Build()
		reconciler := newHostStateHelper(cl)

		_, _, err := reconciler.EnsureInstanceReady(ctx, mockEC2, host, func(context.Context) (internalconfig.AWSConfiguration, error) {
			return internalconfig.AWSConfiguration{}, nil
		})
		Expect(err).Should(MatchError(expectedErr))
	})

	It("returns an AWS configuration error", func(ctx context.Context) {
		host := newTestStaticHost("bad-config", func(h *maykonfluxcidevv1alpha1.StaticHost) {
			h.Status.State = ptr.To(maykonfluxcidevv1alpha1.HostActualStatePending)
			h.Annotations = map[string]string{
				internalconfig.AnnotationInstanceID: "i-config001",
			}
		})

		expectedErr := errors.New("invalid AWS annotation")
		cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(host).WithStatusSubresource(host).Build()
		reconciler := newHostStateHelper(cl)

		_, _, err := reconciler.EnsureInstanceReady(ctx, &mockEC2Client{}, host, func(context.Context) (internalconfig.AWSConfiguration, error) {
			return internalconfig.AWSConfiguration{}, expectedErr
		})
		Expect(err).Should(MatchError(expectedErr))
	})

	It("returns a launch error", func(ctx context.Context) {
		host := newTestStaticHost("launch-err", func(h *maykonfluxcidevv1alpha1.StaticHost) {
			h.Status.State = ptr.To(maykonfluxcidevv1alpha1.HostActualStatePending)
			h.Annotations = map[string]string{
				internalconfig.AnnotationRegion:       "us-east-1",
				internalconfig.AnnotationAmi:          "ami-0123456789abcdef0",
				internalconfig.AnnotationInstanceType: "m6a.large",
			}
		})

		expectedErr := errors.New("RunInstances: quota exceeded")
		mockEC2 := &mockEC2Client{
			launchInstance: func(context.Context, internalconfig.AWSConfiguration, string) (string, error) {
				return "", expectedErr
			},
		}
		cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(host).WithStatusSubresource(host).Build()
		reconciler := newHostStateHelper(cl)

		_, _, err := reconciler.EnsureInstanceReady(ctx, mockEC2, host, func(context.Context) (internalconfig.AWSConfiguration, error) {
			return internalconfig.AWSConfiguration{
				Region:       "us-east-1",
				Ami:          "ami-0123456789abcdef0",
				InstanceType: "m6a.large",
			}, nil
		})
		Expect(err).Should(MatchError(expectedErr))
	})

	It("forwards strict public address to SSHReady", func(ctx context.Context) {
		host := newTestStaticHost("strict-ssh", func(h *maykonfluxcidevv1alpha1.StaticHost) {
			h.Status.State = ptr.To(maykonfluxcidevv1alpha1.HostActualStatePending)
			h.Annotations = map[string]string{
				internalconfig.AnnotationInstanceID: "i-strict001",
			}
		})

		var gotStrict bool
		mockEC2 := &mockEC2Client{
			sshReady: func(_ context.Context, _ string, strictPublicAddress bool) (string, bool, error) {
				gotStrict = strictPublicAddress
				return "203.0.113.10", true, nil
			},
		}
		cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(host).WithStatusSubresource(host).Build()
		reconciler := newHostStateHelper(cl)

		_, _, err := reconciler.EnsureInstanceReady(ctx, mockEC2, host, func(context.Context) (internalconfig.AWSConfiguration, error) {
			return internalconfig.AWSConfiguration{StrictPublicAddress: true}, nil
		})
		Expect(err).ShouldNot(HaveOccurred())
		Expect(gotStrict).Should(BeTrue())
	})

	It("errors when a Ready host's instance is stopped", func(ctx context.Context) {
		host := newTestStaticHost("not-running", func(h *maykonfluxcidevv1alpha1.StaticHost) {
			h.Annotations = map[string]string{
				internalconfig.AnnotationInstanceID: "i-stop001",
			}
		})

		mockEC2 := &mockEC2Client{
			describeInstance: func(_ context.Context, _ string, strictPublicAddress bool) (internalec2.InstanceDetails, error) {
				Expect(strictPublicAddress).Should(BeTrue())
				return internalec2.InstanceDetails{State: types.InstanceStateNameStopped}, nil
			},
		}
		cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(host).WithStatusSubresource(host).Build()
		reconciler := newHostStateHelper(cl)

		result, err := reconciler.EnsureInstanceStillRunning(ctx, mockEC2, host, true)
		Expect(err).Should(MatchError(And(
			ContainSubstring("stopped"),
			ContainSubstring("not running"),
		)))
		Expect(result.RequeueAfter).Should(BeZero())
	})

	It("requeues a running Ready host to catch later EC2 state changes", func(ctx context.Context) {
		host := newTestStaticHost("still-running", func(h *maykonfluxcidevv1alpha1.StaticHost) {
			h.Annotations = map[string]string{
				internalconfig.AnnotationInstanceID: "i-run001",
			}
		})

		mockEC2 := &mockEC2Client{
			describeInstance: func(context.Context, string, bool) (internalec2.InstanceDetails, error) {
				return internalec2.InstanceDetails{State: types.InstanceStateNameRunning}, nil
			},
		}
		cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(host).WithStatusSubresource(host).Build()
		reconciler := newHostStateHelper(cl)

		result, err := reconciler.EnsureInstanceStillRunning(ctx, mockEC2, host, false)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(result.RequeueAfter).Should(Equal(instanceHealthInterval))
	})

	It("returns context cancellation from SSHReady", func(ctx context.Context) {
		host := newTestStaticHost("ssh-canceled", func(h *maykonfluxcidevv1alpha1.StaticHost) {
			h.Status.State = ptr.To(maykonfluxcidevv1alpha1.HostActualStatePending)
			h.Annotations = map[string]string{
				internalconfig.AnnotationInstanceID: "i-cancel001",
			}
		})

		canceled, cancel := context.WithCancel(ctx)
		cancel()

		mockEC2 := &mockEC2Client{
			sshReady: func(context.Context, string, bool) (string, bool, error) {
				return "", false, &internalec2.SSHProbeError{Addr: "203.0.113.10:22", Err: context.Canceled}
			},
		}
		cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(host).WithStatusSubresource(host).Build()
		reconciler := newHostStateHelper(cl)

		_, _, err := reconciler.EnsureInstanceReady(canceled, mockEC2, host, func(context.Context) (internalconfig.AWSConfiguration, error) {
			return internalconfig.AWSConfiguration{}, nil
		})
		Expect(err).Should(MatchError(context.Canceled))
	})

	It("terminates the instance during deletion", func(ctx context.Context) {
		host := newTestStaticHost("finalize-terminate", func(h *maykonfluxcidevv1alpha1.StaticHost) {
			h.Annotations = map[string]string{
				internalconfig.AnnotationInstanceID:          "i-term001",
				internalconfig.AnnotationStrictPublicAddress: "true",
			}
		})

		terminated := false
		mockEC2 := &mockEC2Client{
			describeInstance: func(_ context.Context, _ string, strictPublicAddress bool) (internalec2.InstanceDetails, error) {
				Expect(strictPublicAddress).Should(BeTrue())
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
			describeInstance: func(context.Context, string, bool) (internalec2.InstanceDetails, error) {
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

	It("errors when the strict-public-address annotation is malformed", func(ctx context.Context) {
		host := newTestStaticHost("finalize-bad-strict", func(h *maykonfluxcidevv1alpha1.StaticHost) {
			h.Annotations = map[string]string{
				internalconfig.AnnotationInstanceID:          "i-term002",
				internalconfig.AnnotationStrictPublicAddress: "maybe",
			}
		})
		cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(host).WithStatusSubresource(host).Build()
		reconciler := newHostStateHelper(cl)

		result, done, err := reconciler.EnsureInstanceTerminated(ctx, &mockEC2Client{}, host)
		Expect(err).Should(MatchError(ContainSubstring(internalconfig.AnnotationStrictPublicAddress)))
		Expect(done).Should(BeFalse())
		Expect(result.RequeueAfter).Should(BeZero())
	})

	It("reports termination complete when no instance was created", func(ctx context.Context) {
		host := newTestStaticHost("finalize-no-instance", nil)
		cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(host).WithStatusSubresource(host).Build()
		reconciler := newHostStateHelper(cl)

		result, done, err := reconciler.EnsureInstanceTerminated(ctx, nil, host)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(done).Should(BeTrue())
		Expect(result.RequeueAfter).Should(BeZero())
	})

	It("errors when terminating an instance without an EC2 client", func(ctx context.Context) {
		host := newTestStaticHost("finalize-nil-ec2", func(h *maykonfluxcidevv1alpha1.StaticHost) {
			h.Annotations = map[string]string{
				internalconfig.AnnotationInstanceID: "i-term003",
			}
		})
		cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(host).WithStatusSubresource(host).Build()
		reconciler := newHostStateHelper(cl)

		result, done, err := reconciler.EnsureInstanceTerminated(ctx, nil, host)
		Expect(err).Should(MatchError(ContainSubstring("EC2 client is required")))
		Expect(done).Should(BeFalse())
		Expect(result.RequeueAfter).Should(BeZero())
	})

	DescribeTable("resets drain states to Pending when Ready is requested",
		func(ctx context.Context, actualState maykonfluxcidevv1alpha1.HostActualState) {
			host := newTestStaticHost("drain-to-ready", nil)
			cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(host).WithStatusSubresource(host).Build()
			reconciler := newHostStateHelper(cl)

			result, nextState, err := reconciler.EnsureReady(
				ctx,
				&mockEC2Client{},
				host,
				actualState,
				func(context.Context) (internalconfig.AWSConfiguration, error) {
					return internalconfig.AWSConfiguration{}, nil
				},
			)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(result.RequeueAfter).Should(BeZero())
			Expect(nextState).ShouldNot(BeNil())
			Expect(*nextState).Should(Equal(maykonfluxcidevv1alpha1.HostActualStatePending))
		},
		Entry("Draining", maykonfluxcidevv1alpha1.HostActualStateDraining),
		Entry("Drained", maykonfluxcidevv1alpha1.HostActualStateDrained),
	)

	It("errors when the actual state is not implemented", func(ctx context.Context) {
		host := newTestStaticHost("unknown-state", nil)
		cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(host).WithStatusSubresource(host).Build()
		reconciler := newHostStateHelper(cl)

		_, _, err := reconciler.EnsureReady(
			ctx,
			&mockEC2Client{},
			host,
			maykonfluxcidevv1alpha1.HostActualState("Unknown"),
			func(context.Context) (internalconfig.AWSConfiguration, error) {
				return internalconfig.AWSConfiguration{}, nil
			},
		)
		Expect(err).Should(MatchError(ContainSubstring(`unsupported host actual state "Unknown"`)))
	})
})
