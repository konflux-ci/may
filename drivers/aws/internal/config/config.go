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

package config

import (
	"context"
	"fmt"
	"strconv"

	maykonfluxcidevv1alpha1 "github.com/konflux-ci/may/api/v1alpha1"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	// Annotation key prefix for all AWS configuration fields.
	annotationPrefix = "aws.may.konflux-ci.dev/"

	AnnotationRegion                  = annotationPrefix + "region"
	AnnotationAmi                     = annotationPrefix + "ami"
	AnnotationInstanceType            = annotationPrefix + "instance-type"
	AnnotationKeyName                 = annotationPrefix + "key-name"
	AnnotationSecurityGroup           = annotationPrefix + "security-group"
	AnnotationSecurityGroupId         = annotationPrefix + "security-group-id"
	AnnotationSubnetId                = annotationPrefix + "subnet-id"
	AnnotationDisk                    = annotationPrefix + "disk"
	AnnotationMaxSpotInstancePrice    = annotationPrefix + "max-spot-instance-price"
	AnnotationInstanceProfileName     = annotationPrefix + "instance-profile-name"
	AnnotationInstanceProfileArn      = annotationPrefix + "instance-profile-arn"
	AnnotationThroughput              = annotationPrefix + "throughput"
	AnnotationIops                    = annotationPrefix + "iops"
	AnnotationUserData                = annotationPrefix + "user-data"
	AnnotationTenancy                 = annotationPrefix + "tenancy"
	AnnotationHostResourceGroupArn    = annotationPrefix + "host-resource-group-arn"
	AnnotationLicenseConfigurationArn = annotationPrefix + "license-configuration-arn"
	AnnotationStrictPublicAddress     = annotationPrefix + "strict-public-address"
)

// AWSConfiguration holds the AWS-specific configuration for provisioning EC2 instances.
type AWSConfiguration struct {
	// Region is the geographical area to be associated with the instance.
	// See the [AWS region docs](https://docs.aws.amazon.com/AmazonRDS/latest/UserGuide/Concepts.RegionsAndAvailabilityZones.html)
	// for valid regions.
	Region string

	// Ami is the Amazon Machine Image used to provide the software to the instance.
	Ami string

	// InstanceType corresponds to the AWS instance type, which specifies the
	// hardware of the host computer used for the instance. See the
	// [AWS instance naming docs](https://docs.aws.amazon.com/ec2/latest/instancetypes/instance-type-names.html)
	// for proper instance type naming conventions.
	InstanceType string

	// KeyName is the name of the SSH key inside of AWS.
	KeyName string

	// SecurityGroup is the name of the security group to be used on the instance.
	SecurityGroup string

	// SecurityGroupId is the unique identifier of the security group to be used on
	// the instance.
	SecurityGroupId string

	// SubnetId is the ID of the subnet to use when creating the instance.
	SubnetId string

	// Disk is the amount of permanent storage (in GB) to allocate the instance.
	Disk int32

	// MaxSpotInstancePrice is the maximum price per hour in USD as a decimal string (e.g., "0.50")
	// is willing to pay for an EC2 [Spot instance](https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/using-spot-instances.html)
	MaxSpotInstancePrice string

	// InstanceProfileName is the name of the instance profile (a container for
	// an AWS IAM role attached to an EC2 instance).
	InstanceProfileName string

	// InstanceProfileArn is the Amazon Resource Name of the instance profile.
	InstanceProfileArn string

	// Throughput is the amount of traffic (in MiB/s) provisioned for the
	// instance's EBS volume(s).
	Throughput *int32

	// Iops is the number of input/output (I/O) operations per second provisioned
	// for the instance's EBS volume(s).
	Iops *int32

	// UserData is the raw cloud-init or shell script content for the instance
	// at launch. The annotation value is stored as-is; base64 encoding for the
	// EC2 RunInstances API happens when the request is built.
	UserData *string

	// Tenancy specifies the tenancy of the instance. Valid values are "default",
	// "dedicated", or "host". For Mac instances, use "host".
	Tenancy string

	// HostResourceGroupArn is the ARN of the host resource group in which to
	// launch the instance. Required when Tenancy is "host".
	HostResourceGroupArn string

	// LicenseConfigurationArn is the ARN of the license configuration to
	// associate with the instance.
	LicenseConfigurationArn string

	// StrictPublicAddress specifies whether the instance must use a public IP address.
	// If false, it would be also possible for the instance to use a private IP address.
	StrictPublicAddress bool
}

// GetStaticAWSConfiguration returns the AWS configuration for a StaticHost,
// sourced from the host's annotations.
func GetStaticAWSConfiguration(ctx context.Context, staticHost *maykonfluxcidevv1alpha1.StaticHost) (AWSConfiguration, error) {
	l := logf.FromContext(ctx).WithValues("StaticHost", staticHost.Name)
	l.V(1).Info("building AWS configuration from StaticHost annotations")

	cfg, err := configurationFromAnnotations(staticHost.GetAnnotations())
	if err != nil {
		return AWSConfiguration{}, err
	}

	l.V(1).Info("AWS configuration resolved",
		"region", cfg.Region,
		"ami", cfg.Ami,
		"instanceType", cfg.InstanceType,
	)
	return cfg, nil
}

// GetDynamicAWSConfiguration returns the AWS configuration for a DynamicHost,
// sourced from the host's annotations.
func GetDynamicAWSConfiguration(ctx context.Context, dynamicHost *maykonfluxcidevv1alpha1.DynamicHost) (AWSConfiguration, error) {
	l := logf.FromContext(ctx).WithValues("DynamicHost", dynamicHost.Name)
	l.V(1).Info("building AWS configuration from DynamicHost annotations")

	cfg, err := configurationFromAnnotations(dynamicHost.GetAnnotations())
	if err != nil {
		return AWSConfiguration{}, err
	}

	l.V(1).Info("AWS configuration resolved",
		"region", cfg.Region,
		"ami", cfg.Ami,
		"instanceType", cfg.InstanceType,
	)
	return cfg, nil
}

// configurationFromAnnotations extracts an AWSConfiguration from a map of
// Kubernetes annotations. Missing keys result in zero-values for the
// corresponding fields; pointer fields remain nil if their annotation is absent.
// Present annotations with invalid values return an error.
func configurationFromAnnotations(annotations map[string]string) (AWSConfiguration, error) {
	if annotations == nil {
		return AWSConfiguration{}, nil
	}

	cfg := AWSConfiguration{
		Region:                  annotations[AnnotationRegion],
		Ami:                     annotations[AnnotationAmi],
		InstanceType:            annotations[AnnotationInstanceType],
		KeyName:                 annotations[AnnotationKeyName],
		SecurityGroup:           annotations[AnnotationSecurityGroup],
		SecurityGroupId:         annotations[AnnotationSecurityGroupId],
		SubnetId:                annotations[AnnotationSubnetId],
		MaxSpotInstancePrice:    annotations[AnnotationMaxSpotInstancePrice],
		InstanceProfileName:     annotations[AnnotationInstanceProfileName],
		InstanceProfileArn:      annotations[AnnotationInstanceProfileArn],
		Tenancy:                 annotations[AnnotationTenancy],
		HostResourceGroupArn:    annotations[AnnotationHostResourceGroupArn],
		LicenseConfigurationArn: annotations[AnnotationLicenseConfigurationArn],
	}

	if v, ok := annotations[AnnotationDisk]; ok {
		disk, err := parseInt32(AnnotationDisk, v)
		if err != nil {
			return AWSConfiguration{}, err
		}
		cfg.Disk = disk
	}

	if v, ok := annotations[AnnotationStrictPublicAddress]; ok {
		strictPublicAddress, err := parseBool(AnnotationStrictPublicAddress, v)
		if err != nil {
			return AWSConfiguration{}, err
		}
		cfg.StrictPublicAddress = strictPublicAddress
	}

	if v, ok := annotations[AnnotationThroughput]; ok {
		throughput, err := parseOptionalInt32(AnnotationThroughput, v)
		if err != nil {
			return AWSConfiguration{}, err
		}
		cfg.Throughput = throughput
	}

	if v, ok := annotations[AnnotationIops]; ok {
		iops, err := parseOptionalInt32(AnnotationIops, v)
		if err != nil {
			return AWSConfiguration{}, err
		}
		cfg.Iops = iops
	}

	if v, ok := annotations[AnnotationUserData]; ok {
		if v == "" {
			return AWSConfiguration{}, fmt.Errorf("invalid AWS annotation %q: empty value", AnnotationUserData)
		}
		// Raw script/cloud-init content; base64 encoding is deferred to EC2 API call time.
		cfg.UserData = &v
	}

	return cfg, nil
}

// parseInt32 parses a string as a base-10 int32 when the annotation is present.
func parseInt32(annotation, value string) (int32, error) {
	if value == "" {
		return 0, fmt.Errorf("invalid AWS annotation %q: empty value", annotation)
	}
	v, err := strconv.ParseInt(value, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("invalid AWS annotation %q: %w", annotation, err)
	}
	if v < 0 {
		return 0, fmt.Errorf("invalid AWS annotation %q: negative value", annotation)
	}
	return int32(v), nil
}

// parseOptionalInt32 parses an optional int32 annotation when the key is present.
func parseOptionalInt32(annotation, value string) (*int32, error) {
	if value == "" {
		return nil, fmt.Errorf("invalid AWS annotation %q: empty value", annotation)
	}
	v, err := strconv.ParseInt(value, 10, 32)
	if err != nil {
		return nil, fmt.Errorf("invalid AWS annotation %q: %w", annotation, err)
	}
	if v < 0 {
		return nil, fmt.Errorf("invalid AWS annotation %q: negative value", annotation)
	}
	parsed := int32(v)
	return &parsed, nil
}

// parseBool parses a string as a boolean when the annotation is present.
func parseBool(annotation, value string) (bool, error) {
	if value == "" {
		return false, fmt.Errorf("invalid AWS annotation %q: empty value", annotation)
	}
	v, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("invalid AWS annotation %q: %w", annotation, err)
	}
	return v, nil
}
