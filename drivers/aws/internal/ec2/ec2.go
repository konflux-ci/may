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
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsec2 "github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"

	internalconfig "github.com/konflux-ci/may/drivers/aws/internal/config"
)

// Client wraps an EC2 API client for instance lifecycle operations.
type Client struct {
	api ec2API
}

// NewClient returns a Client backed by a real AWS EC2 client.
func NewClient(c *awsec2.Client) *Client {
	return &Client{api: c}
}

// LaunchInstance starts a single EC2 instance from cfg and returns its instance ID.
func (c *Client) LaunchInstance(ctx context.Context, cfg internalconfig.AWSConfiguration) (string, error) {
	if err := validateAWSConfiguration(cfg); err != nil {
		return "", err
	}

	input := buildRunInstancesInput(cfg)

	out, err := c.api.RunInstances(ctx, input)
	if err != nil {
		return "", fmt.Errorf("RunInstances: %w", err)
	}

	if len(out.Instances) == 0 || out.Instances[0].InstanceId == nil {
		return "", fmt.Errorf("RunInstances returned no instance")
	}

	return aws.ToString(out.Instances[0].InstanceId), nil
}

// InstanceDetails holds EC2 instance fields used by the driver.
type InstanceDetails struct {
	State    types.InstanceStateName
	PublicIP string
}

// DescribeInstance returns driver-relevant details for instanceID.
func (c *Client) DescribeInstance(ctx context.Context, instanceID string) (InstanceDetails, error) {
	out, err := c.api.DescribeInstances(ctx, &awsec2.DescribeInstancesInput{
		InstanceIds: []string{instanceID},
	})
	if err != nil {
		return InstanceDetails{}, fmt.Errorf("DescribeInstances: %w", err)
	}

	for _, reservation := range out.Reservations {
		for _, instance := range reservation.Instances {
			if instance.InstanceId == nil || aws.ToString(instance.InstanceId) != instanceID {
				continue
			}
			details := InstanceDetails{}
			if instance.State != nil {
				details.State = instance.State.Name
			}
			if instance.PublicIpAddress != nil {
				details.PublicIP = aws.ToString(instance.PublicIpAddress)
			}
			return details, nil
		}
	}

	return InstanceDetails{}, fmt.Errorf("DescribeInstances: instance %q not found", instanceID)
}

// SSHReadyOnPublicIP reports whether instanceID is running, has a public IP, and accepts SSH.
func (c *Client) SSHReadyOnPublicIP(ctx context.Context, instanceID string) (string, bool, error) {
	details, err := c.DescribeInstance(ctx, instanceID)
	if err != nil {
		return "", false, err
	}

	switch details.State {
	case types.InstanceStateNameShuttingDown, types.InstanceStateNameTerminated:
		return "", false, fmt.Errorf("EC2 instance %s is %s before becoming ready", instanceID, details.State)
	case types.InstanceStateNameStopping, types.InstanceStateNameStopped:
		return "", false, fmt.Errorf("EC2 instance %s is %s and is not running", instanceID, details.State)
	case types.InstanceStateNameRunning:
		return c.sshReadyOnPublicIPRunning(ctx, details)
	default:
		return details.PublicIP, false, nil
	}
}

func (c *Client) sshReadyOnPublicIPRunning(ctx context.Context, details InstanceDetails) (string, bool, error) {
	if details.PublicIP == "" {
		return "", false, nil
	}

	if err := SSHPortOpen(ctx, details.PublicIP); err != nil {
		return "", false, err
	}

	return details.PublicIP, true, nil
}

// TerminateInstance requests termination of the given EC2 instance.
func (c *Client) TerminateInstance(ctx context.Context, instanceID string) error {
	_, err := c.api.TerminateInstances(ctx, &awsec2.TerminateInstancesInput{
		InstanceIds: []string{instanceID},
	})
	if err != nil {
		return fmt.Errorf("TerminateInstances: %w", err)
	}
	return nil
}

func validateAWSConfiguration(cfg internalconfig.AWSConfiguration) error {
	if cfg.Ami == "" {
		return fmt.Errorf("missing required annotation %q", internalconfig.AnnotationAmi)
	}
	if cfg.InstanceType == "" {
		return fmt.Errorf("missing required annotation %q", internalconfig.AnnotationInstanceType)
	}
	if cfg.SecurityGroup != "" && cfg.SecurityGroupId != "" {
		return fmt.Errorf("cannot set both %q and %q",
			internalconfig.AnnotationSecurityGroup,
			internalconfig.AnnotationSecurityGroupId,
		)
	}
	if cfg.SubnetId != "" && cfg.SecurityGroup != "" && cfg.SecurityGroupId == "" {
		return fmt.Errorf("%q (security group name) cannot be used with %q; set %q instead",
			internalconfig.AnnotationSecurityGroup,
			internalconfig.AnnotationSubnetId,
			internalconfig.AnnotationSecurityGroupId,
		)
	}
	return nil
}

func buildRunInstancesInput(cfg internalconfig.AWSConfiguration) *awsec2.RunInstancesInput {
	input := &awsec2.RunInstancesInput{
		ImageId:      aws.String(cfg.Ami),
		InstanceType: types.InstanceType(cfg.InstanceType),
		MinCount:     aws.Int32(1),
		MaxCount:     aws.Int32(1),
	}

	if cfg.KeyName != "" {
		input.KeyName = aws.String(cfg.KeyName)
	}

	if cfg.UserData != nil {
		input.UserData = aws.String(base64.StdEncoding.EncodeToString([]byte(*cfg.UserData)))
	}

	if cfg.InstanceProfileArn != "" || cfg.InstanceProfileName != "" {
		profile := &types.IamInstanceProfileSpecification{}
		if cfg.InstanceProfileArn != "" {
			profile.Arn = aws.String(cfg.InstanceProfileArn)
		} else {
			profile.Name = aws.String(cfg.InstanceProfileName)
		}
		input.IamInstanceProfile = profile
	}

	if cfg.MaxSpotInstancePrice != "" {
		input.InstanceMarketOptions = &types.InstanceMarketOptionsRequest{
			MarketType: types.MarketTypeSpot,
			SpotOptions: &types.SpotMarketOptions{
				MaxPrice: aws.String(cfg.MaxSpotInstancePrice),
			},
		}
	}

	if cfg.Tenancy != "" || cfg.HostResourceGroupArn != "" {
		placement := &types.Placement{}
		if cfg.Tenancy != "" {
			placement.Tenancy = types.Tenancy(cfg.Tenancy)
		}
		if cfg.HostResourceGroupArn != "" {
			placement.HostResourceGroupArn = aws.String(cfg.HostResourceGroupArn)
		}
		input.Placement = placement
	}

	if cfg.LicenseConfigurationArn != "" {
		input.LicenseSpecifications = []types.LicenseConfigurationRequest{
			{
				LicenseConfigurationArn: aws.String(cfg.LicenseConfigurationArn),
			},
		}
	}

	if cfg.Disk > 0 || cfg.Throughput != nil || cfg.Iops != nil {
		ebs := &types.EbsBlockDevice{
			VolumeType:          types.VolumeTypeGp3,
			DeleteOnTermination: aws.Bool(true),
		}
		if cfg.Disk > 0 {
			ebs.VolumeSize = aws.Int32(cfg.Disk)
		}
		if cfg.Throughput != nil {
			ebs.Throughput = cfg.Throughput
		}
		if cfg.Iops != nil {
			ebs.Iops = cfg.Iops
		}
		input.BlockDeviceMappings = []types.BlockDeviceMapping{
			{
				DeviceName: aws.String("/dev/sda1"),
				Ebs:        ebs,
			},
		}
	}

	if cfg.SubnetId != "" {
		ni := types.InstanceNetworkInterfaceSpecification{
			DeviceIndex: aws.Int32(0),
			SubnetId:    aws.String(cfg.SubnetId),
		}
		if cfg.StrictPublicAddress {
			ni.AssociatePublicIpAddress = aws.Bool(true)
		}
		if cfg.SecurityGroupId != "" {
			ni.Groups = []string{cfg.SecurityGroupId}
		}
		input.NetworkInterfaces = []types.InstanceNetworkInterfaceSpecification{ni}
		return input
	}

	switch {
	case cfg.SecurityGroupId != "":
		input.SecurityGroupIds = []string{cfg.SecurityGroupId}
	case cfg.SecurityGroup != "":
		input.SecurityGroups = []string{cfg.SecurityGroup}
	}

	return input
}
