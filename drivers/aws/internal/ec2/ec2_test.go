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

package ec2

import (
	"context"
	"encoding/base64"
	"errors"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsec2 "github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	internalconfig "github.com/konflux-ci/may/drivers/aws/internal/config"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type mockEC2API struct {
	runInstances       func(context.Context, *awsec2.RunInstancesInput, ...func(*awsec2.Options)) (*awsec2.RunInstancesOutput, error)
	describeInstances  func(context.Context, *awsec2.DescribeInstancesInput, ...func(*awsec2.Options)) (*awsec2.DescribeInstancesOutput, error)
	terminateInstances func(context.Context, *awsec2.TerminateInstancesInput, ...func(*awsec2.Options)) (*awsec2.TerminateInstancesOutput, error)
}

func (m *mockEC2API) RunInstances(ctx context.Context, params *awsec2.RunInstancesInput, optFns ...func(*awsec2.Options)) (*awsec2.RunInstancesOutput, error) {
	return m.runInstances(ctx, params, optFns...)
}

func (m *mockEC2API) DescribeInstances(ctx context.Context, params *awsec2.DescribeInstancesInput, optFns ...func(*awsec2.Options)) (*awsec2.DescribeInstancesOutput, error) {
	return m.describeInstances(ctx, params, optFns...)
}

func (m *mockEC2API) TerminateInstances(ctx context.Context, params *awsec2.TerminateInstancesInput, optFns ...func(*awsec2.Options)) (*awsec2.TerminateInstancesOutput, error) {
	return m.terminateInstances(ctx, params, optFns...)
}

func newMockClient(api *mockEC2API) *Client {
	return &Client{api: api}
}

func instanceOutput(instanceID string, state types.InstanceStateName, publicIP string) *awsec2.DescribeInstancesOutput {
	instance := types.Instance{
		InstanceId: aws.String(instanceID),
		State:      &types.InstanceState{Name: state},
	}
	if publicIP != "" {
		instance.PublicIpAddress = aws.String(publicIP)
	}
	return &awsec2.DescribeInstancesOutput{
		Reservations: []types.Reservation{
			{Instances: []types.Instance{instance}},
		},
	}
}

var validLaunchConfig = internalconfig.AWSConfiguration{
	Ami:          "ami-0123456789abcdef0",
	InstanceType: "m6a.large",
}

var _ = Describe("validateAWSConfiguration", func() {
	It("requires ami", func() {
		Expect(validateAWSConfiguration(internalconfig.AWSConfiguration{InstanceType: "m6a.large"})).Should(MatchError(ContainSubstring(internalconfig.AnnotationAmi)))
	})

	It("requires instance type", func() {
		Expect(validateAWSConfiguration(internalconfig.AWSConfiguration{Ami: "ami-0123456789abcdef0"})).Should(MatchError(ContainSubstring(internalconfig.AnnotationInstanceType)))
	})

	It("rejects both security group name and ID", func() {
		err := validateAWSConfiguration(internalconfig.AWSConfiguration{
			Ami:             "ami-0123456789abcdef0",
			InstanceType:    "m6a.large",
			SecurityGroup:   "my-sg",
			SecurityGroupId: "sg-0123456789abcdef0",
		})
		Expect(err).Should(MatchError(And(
			ContainSubstring(internalconfig.AnnotationSecurityGroup),
			ContainSubstring(internalconfig.AnnotationSecurityGroupId),
		)))
	})

	It("rejects security group name with subnet", func() {
		err := validateAWSConfiguration(internalconfig.AWSConfiguration{
			Ami:           "ami-0123456789abcdef0",
			InstanceType:  "m6a.large",
			SubnetId:      "subnet-0123456789abcdef0",
			SecurityGroup: "my-sg",
		})
		Expect(err).Should(MatchError(And(
			ContainSubstring(internalconfig.AnnotationSecurityGroup),
			ContainSubstring(internalconfig.AnnotationSubnetId),
			ContainSubstring(internalconfig.AnnotationSecurityGroupId),
		)))
	})
})

var _ = Describe("buildRunInstancesInput", func() {
	It("builds a minimal RunInstances request", func() {
		input := buildRunInstancesInput(validLaunchConfig)
		Expect(aws.ToString(input.ImageId)).Should(Equal(validLaunchConfig.Ami))
		Expect(input.InstanceType).Should(Equal(types.InstanceType(validLaunchConfig.InstanceType)))
		Expect(aws.ToInt32(input.MinCount)).Should(Equal(int32(1)))
		Expect(aws.ToInt32(input.MaxCount)).Should(Equal(int32(1)))
	})

	It("base64-encodes user data", func() {
		userData := "#!/bin/bash\necho hello"
		input := buildRunInstancesInput(internalconfig.AWSConfiguration{
			Ami:          validLaunchConfig.Ami,
			InstanceType: validLaunchConfig.InstanceType,
			UserData:     &userData,
		})
		Expect(aws.ToString(input.UserData)).Should(Equal(base64.StdEncoding.EncodeToString([]byte(userData))))
	})

	It("requests a public IP when a subnet is set and strict public address is enabled", func() {
		input := buildRunInstancesInput(internalconfig.AWSConfiguration{
			Ami:                 validLaunchConfig.Ami,
			InstanceType:        validLaunchConfig.InstanceType,
			SubnetId:            "subnet-0123456789abcdef0",
			SecurityGroupId:     "sg-0123456789abcdef0",
			StrictPublicAddress: true,
		})
		Expect(input.NetworkInterfaces).Should(HaveLen(1))
		Expect(aws.ToString(input.NetworkInterfaces[0].SubnetId)).Should(Equal("subnet-0123456789abcdef0"))
		Expect(aws.ToBool(input.NetworkInterfaces[0].AssociatePublicIpAddress)).Should(BeTrue())
		Expect(input.NetworkInterfaces[0].Groups).Should(Equal([]string{"sg-0123456789abcdef0"}))
		Expect(input.SubnetId).Should(BeNil())
	})

	It("does not override subnet public IP defaults when strict public address is disabled", func() {
		input := buildRunInstancesInput(internalconfig.AWSConfiguration{
			Ami:                 validLaunchConfig.Ami,
			InstanceType:        validLaunchConfig.InstanceType,
			SubnetId:            "subnet-0123456789abcdef0",
			SecurityGroupId:     "sg-0123456789abcdef0",
			StrictPublicAddress: false,
		})
		Expect(input.NetworkInterfaces).Should(HaveLen(1))
		Expect(input.NetworkInterfaces[0].AssociatePublicIpAddress).Should(BeNil())
	})

	It("sets security groups when no subnet is configured", func() {
		input := buildRunInstancesInput(internalconfig.AWSConfiguration{
			Ami:             validLaunchConfig.Ami,
			InstanceType:    validLaunchConfig.InstanceType,
			SecurityGroupId: "sg-0123456789abcdef0",
		})
		Expect(input.SecurityGroupIds).Should(Equal([]string{"sg-0123456789abcdef0"}))
		Expect(input.NetworkInterfaces).Should(BeNil())
	})

	It("sets security group name when no subnet is configured", func() {
		input := buildRunInstancesInput(internalconfig.AWSConfiguration{
			Ami:           validLaunchConfig.Ami,
			InstanceType:  validLaunchConfig.InstanceType,
			SecurityGroup: "my-sg",
		})
		Expect(input.SecurityGroups).Should(Equal([]string{"my-sg"}))
		Expect(input.NetworkInterfaces).Should(BeNil())
	})
})

var _ = Describe("LaunchInstance", func() {
	It("returns a validation error before calling the API", func(ctx context.Context) {
		called := false
		client := newMockClient(&mockEC2API{
			runInstances: func(context.Context, *awsec2.RunInstancesInput, ...func(*awsec2.Options)) (*awsec2.RunInstancesOutput, error) {
				called = true
				return nil, nil
			},
		})

		_, err := client.LaunchInstance(ctx, internalconfig.AWSConfiguration{})
		Expect(err).Should(MatchError(ContainSubstring(internalconfig.AnnotationAmi)))
		Expect(called).Should(BeFalse())
	})

	It("rejects security group name with subnet before calling the API", func(ctx context.Context) {
		called := false
		client := newMockClient(&mockEC2API{
			runInstances: func(context.Context, *awsec2.RunInstancesInput, ...func(*awsec2.Options)) (*awsec2.RunInstancesOutput, error) {
				called = true
				return nil, nil
			},
		})

		_, err := client.LaunchInstance(ctx, internalconfig.AWSConfiguration{
			Ami:           validLaunchConfig.Ami,
			InstanceType:  validLaunchConfig.InstanceType,
			SubnetId:      "subnet-0123456789abcdef0",
			SecurityGroup: "my-sg",
		})
		Expect(err).Should(MatchError(And(
			ContainSubstring(internalconfig.AnnotationSecurityGroup),
			ContainSubstring(internalconfig.AnnotationSubnetId),
		)))
		Expect(called).Should(BeFalse())
	})

	It("rejects conflicting security groups before calling the API", func(ctx context.Context) {
		called := false
		client := newMockClient(&mockEC2API{
			runInstances: func(context.Context, *awsec2.RunInstancesInput, ...func(*awsec2.Options)) (*awsec2.RunInstancesOutput, error) {
				called = true
				return nil, nil
			},
		})

		_, err := client.LaunchInstance(ctx, internalconfig.AWSConfiguration{
			Ami:             validLaunchConfig.Ami,
			InstanceType:    validLaunchConfig.InstanceType,
			SubnetId:        "subnet-0123456789abcdef0",
			SecurityGroup:   "my-sg",
			SecurityGroupId: "sg-0123456789abcdef0",
		})
		Expect(err).Should(MatchError(And(
			ContainSubstring(internalconfig.AnnotationSecurityGroup),
			ContainSubstring(internalconfig.AnnotationSecurityGroupId),
		)))
		Expect(called).Should(BeFalse())
	})

	It("returns the instance ID from RunInstances", func(ctx context.Context) {
		instanceID := "i-launch001"
		client := newMockClient(&mockEC2API{
			runInstances: func(_ context.Context, input *awsec2.RunInstancesInput, _ ...func(*awsec2.Options)) (*awsec2.RunInstancesOutput, error) {
				Expect(aws.ToString(input.ImageId)).Should(Equal(validLaunchConfig.Ami))
				return &awsec2.RunInstancesOutput{
					Instances: []types.Instance{{InstanceId: aws.String(instanceID)}},
				}, nil
			},
		})

		gotInstanceID, err := client.LaunchInstance(ctx, validLaunchConfig)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(gotInstanceID).Should(Equal(instanceID))
	})

	It("wraps RunInstances API errors", func(ctx context.Context) {
		client := newMockClient(&mockEC2API{
			runInstances: func(context.Context, *awsec2.RunInstancesInput, ...func(*awsec2.Options)) (*awsec2.RunInstancesOutput, error) {
				return nil, errors.New("api unavailable")
			},
		})

		_, err := client.LaunchInstance(ctx, validLaunchConfig)
		Expect(err).Should(MatchError(And(
			ContainSubstring("RunInstances"),
			ContainSubstring("api unavailable"),
		)))
	})

	It("errors when RunInstances returns no instance", func(ctx context.Context) {
		client := newMockClient(&mockEC2API{
			runInstances: func(context.Context, *awsec2.RunInstancesInput, ...func(*awsec2.Options)) (*awsec2.RunInstancesOutput, error) {
				return &awsec2.RunInstancesOutput{}, nil
			},
		})

		_, err := client.LaunchInstance(ctx, validLaunchConfig)
		Expect(err).Should(MatchError(ContainSubstring("no instance")))
	})

	It("errors when RunInstances returns an instance without an ID", func(ctx context.Context) {
		client := newMockClient(&mockEC2API{
			runInstances: func(context.Context, *awsec2.RunInstancesInput, ...func(*awsec2.Options)) (*awsec2.RunInstancesOutput, error) {
				return &awsec2.RunInstancesOutput{
					Instances: []types.Instance{{InstanceId: nil}},
				}, nil
			},
		})

		_, err := client.LaunchInstance(ctx, validLaunchConfig)
		Expect(err).Should(MatchError(ContainSubstring("no instance")))
	})
})

var _ = Describe("DescribeInstance", func() {
	It("returns state and public IP", func(ctx context.Context) {
		instanceID := "i-describe-running"
		publicIP := "203.0.113.10"
		client := newMockClient(&mockEC2API{
			describeInstances: func(_ context.Context, input *awsec2.DescribeInstancesInput, _ ...func(*awsec2.Options)) (*awsec2.DescribeInstancesOutput, error) {
				Expect(input.InstanceIds).Should(Equal([]string{instanceID}))
				return instanceOutput(instanceID, types.InstanceStateNameRunning, publicIP), nil
			},
		})

		details, err := client.DescribeInstance(ctx, instanceID)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(details.State).Should(Equal(types.InstanceStateNameRunning))
		Expect(details.PublicIP).Should(Equal(publicIP))
	})

	It("errors when the instance is missing", func(ctx context.Context) {
		client := newMockClient(&mockEC2API{
			describeInstances: func(context.Context, *awsec2.DescribeInstancesInput, ...func(*awsec2.Options)) (*awsec2.DescribeInstancesOutput, error) {
				return &awsec2.DescribeInstancesOutput{}, nil
			},
		})

		_, err := client.DescribeInstance(ctx, "i-missing")
		Expect(err).Should(MatchError(And(
			ContainSubstring("DescribeInstances"),
			ContainSubstring(`instance "i-missing" not found`),
		)))
	})

	It("returns empty state when the instance state is missing", func(ctx context.Context) {
		instanceID := "i-describe-nil-state"
		client := newMockClient(&mockEC2API{
			describeInstances: func(context.Context, *awsec2.DescribeInstancesInput, ...func(*awsec2.Options)) (*awsec2.DescribeInstancesOutput, error) {
				return &awsec2.DescribeInstancesOutput{
					Reservations: []types.Reservation{{
						Instances: []types.Instance{{
							InstanceId: aws.String(instanceID),
						}},
					}},
				}, nil
			},
		})

		details, err := client.DescribeInstance(ctx, instanceID)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(details.State).Should(BeEmpty())
	})

	It("wraps DescribeInstances API errors", func(ctx context.Context) {
		instanceID := "i-describe-api-err"
		client := newMockClient(&mockEC2API{
			describeInstances: func(context.Context, *awsec2.DescribeInstancesInput, ...func(*awsec2.Options)) (*awsec2.DescribeInstancesOutput, error) {
				return nil, errors.New("api unavailable")
			},
		})

		_, err := client.DescribeInstance(ctx, instanceID)
		Expect(err).Should(MatchError(And(
			ContainSubstring("DescribeInstances"),
			ContainSubstring("api unavailable"),
		)))
	})
})

var _ = Describe("SSHReadyOnPublicIP", func() {
	It("waits while the instance is pending", func(ctx context.Context) {
		instanceID := "i-ssh-pending"
		client := newMockClient(&mockEC2API{
			describeInstances: func(context.Context, *awsec2.DescribeInstancesInput, ...func(*awsec2.Options)) (*awsec2.DescribeInstancesOutput, error) {
				return instanceOutput(instanceID, types.InstanceStateNamePending, ""), nil
			},
		})

		publicIP, ready, err := client.SSHReadyOnPublicIP(ctx, instanceID)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(ready).Should(BeFalse())
		Expect(publicIP).Should(BeEmpty())
	})

	It("waits while the instance has no public IP", func(ctx context.Context) {
		instanceID := "i-ssh-no-public-ip"
		client := newMockClient(&mockEC2API{
			describeInstances: func(context.Context, *awsec2.DescribeInstancesInput, ...func(*awsec2.Options)) (*awsec2.DescribeInstancesOutput, error) {
				return instanceOutput(instanceID, types.InstanceStateNameRunning, ""), nil
			},
		})

		publicIP, ready, err := client.SSHReadyOnPublicIP(ctx, instanceID)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(ready).Should(BeFalse())
		Expect(publicIP).Should(BeEmpty())
	})

	It("waits while SSH is not reachable on the public IP", func() {
		instanceID := "i-ssh-unreachable"
		// Invalid IP fails the probe quickly without waiting for a TCP timeout.
		publicIP := "999.999.999.999"
		client := newMockClient(&mockEC2API{
			describeInstances: func(context.Context, *awsec2.DescribeInstancesInput, ...func(*awsec2.Options)) (*awsec2.DescribeInstancesOutput, error) {
				return instanceOutput(instanceID, types.InstanceStateNameRunning, publicIP), nil
			},
		})

		gotPublicIP, ready, err := client.SSHReadyOnPublicIP(context.Background(), instanceID)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(ready).Should(BeFalse())
		Expect(gotPublicIP).Should(BeZero())
	})

	It("returns context cancellation from the SSH probe", func() {
		instanceID := "i-ssh-ctx-cancel"
		publicIP := "999.999.999.999"
		client := newMockClient(&mockEC2API{
			describeInstances: func(context.Context, *awsec2.DescribeInstancesInput, ...func(*awsec2.Options)) (*awsec2.DescribeInstancesOutput, error) {
				return instanceOutput(instanceID, types.InstanceStateNameRunning, publicIP), nil
			},
		})

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		gotPublicIP, ready, err := client.SSHReadyOnPublicIP(ctx, instanceID)
		Expect(err).Should(MatchError(context.Canceled))
		Expect(ready).Should(BeFalse())
		Expect(gotPublicIP).Should(BeZero())
	})

	It("errors when the instance terminates before becoming ready", func(ctx context.Context) {
		instanceID := "i-ssh-terminated"
		client := newMockClient(&mockEC2API{
			describeInstances: func(context.Context, *awsec2.DescribeInstancesInput, ...func(*awsec2.Options)) (*awsec2.DescribeInstancesOutput, error) {
				return instanceOutput(instanceID, types.InstanceStateNameTerminated, ""), nil
			},
		})

		_, ready, err := client.SSHReadyOnPublicIP(ctx, instanceID)
		Expect(err).Should(MatchError(ContainSubstring("terminated")))
		Expect(ready).Should(BeFalse())
	})

	It("errors when the instance is shutting down before becoming ready", func(ctx context.Context) {
		instanceID := "i-ssh-shutting-down"
		publicIP := "203.0.113.11"
		client := newMockClient(&mockEC2API{
			describeInstances: func(context.Context, *awsec2.DescribeInstancesInput, ...func(*awsec2.Options)) (*awsec2.DescribeInstancesOutput, error) {
				return instanceOutput(instanceID, types.InstanceStateNameShuttingDown, publicIP), nil
			},
		})

		gotPublicIP, ready, err := client.SSHReadyOnPublicIP(ctx, instanceID)
		Expect(err).Should(MatchError(ContainSubstring("shutting-down")))
		Expect(ready).Should(BeFalse())
		Expect(gotPublicIP).Should(BeZero())
	})

	It("errors when the instance is stopped before becoming ready", func(ctx context.Context) {
		instanceID := "i-ssh-stopped"
		publicIP := "203.0.113.12"
		client := newMockClient(&mockEC2API{
			describeInstances: func(context.Context, *awsec2.DescribeInstancesInput, ...func(*awsec2.Options)) (*awsec2.DescribeInstancesOutput, error) {
				return instanceOutput(instanceID, types.InstanceStateNameStopped, publicIP), nil
			},
		})

		gotPublicIP, ready, err := client.SSHReadyOnPublicIP(ctx, instanceID)
		Expect(err).Should(MatchError(And(
			ContainSubstring("stopped"),
			ContainSubstring("not running"),
		)))
		Expect(ready).Should(BeFalse())
		Expect(gotPublicIP).Should(BeZero())
	})

	It("errors when the instance is stopping before becoming ready", func(ctx context.Context) {
		instanceID := "i-ssh-stopping"
		client := newMockClient(&mockEC2API{
			describeInstances: func(context.Context, *awsec2.DescribeInstancesInput, ...func(*awsec2.Options)) (*awsec2.DescribeInstancesOutput, error) {
				return instanceOutput(instanceID, types.InstanceStateNameStopping, ""), nil
			},
		})

		_, ready, err := client.SSHReadyOnPublicIP(ctx, instanceID)
		Expect(err).Should(MatchError(And(
			ContainSubstring("stopping"),
			ContainSubstring("not running"),
		)))
		Expect(ready).Should(BeFalse())
	})

	It("returns an error when DescribeInstance fails", func(ctx context.Context) {
		instanceID := "i-ssh-describe-err"
		client := newMockClient(&mockEC2API{
			describeInstances: func(context.Context, *awsec2.DescribeInstancesInput, ...func(*awsec2.Options)) (*awsec2.DescribeInstancesOutput, error) {
				return nil, errors.New("api unavailable")
			},
		})

		_, ready, err := client.SSHReadyOnPublicIP(ctx, instanceID)
		Expect(err).Should(MatchError(And(
			ContainSubstring("DescribeInstances"),
			ContainSubstring("api unavailable"),
		)))
		Expect(ready).Should(BeFalse())
	})
})

var _ = Describe("TerminateInstance", func() {
	It("requests instance termination", func(ctx context.Context) {
		instanceID := "i-terminate001"
		var gotInstanceID string
		client := newMockClient(&mockEC2API{
			terminateInstances: func(_ context.Context, input *awsec2.TerminateInstancesInput, _ ...func(*awsec2.Options)) (*awsec2.TerminateInstancesOutput, error) {
				Expect(input.InstanceIds).Should(HaveLen(1))
				gotInstanceID = input.InstanceIds[0]
				return &awsec2.TerminateInstancesOutput{}, nil
			},
		})

		Expect(client.TerminateInstance(ctx, instanceID)).Should(Succeed())
		Expect(gotInstanceID).Should(Equal(instanceID))
	})

	It("wraps TerminateInstances API errors", func(ctx context.Context) {
		instanceID := "i-terminate-api-err"
		client := newMockClient(&mockEC2API{
			terminateInstances: func(context.Context, *awsec2.TerminateInstancesInput, ...func(*awsec2.Options)) (*awsec2.TerminateInstancesOutput, error) {
				return nil, errors.New("api unavailable")
			},
		})

		err := client.TerminateInstance(ctx, instanceID)
		Expect(err).Should(MatchError(And(
			ContainSubstring("TerminateInstances"),
			ContainSubstring("api unavailable"),
		)))
	})
})
