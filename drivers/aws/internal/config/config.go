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
	"strconv"

	maykonfluxcidevv1alpha1 "github.com/konflux-ci/may/api/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	// Annotation key prefix for all AWS configuration fields.
	annotationPrefix = "aws.may.konflux-ci.dev/"

	AnnotationRegion                  = annotationPrefix + "region"
	AnnotationAmi                     = annotationPrefix + "ami"
	AnnotationInstanceType            = annotationPrefix + "instance-type"
	AnnotationKeyName                 = annotationPrefix + "key-name"
	AnnotationSecret                  = annotationPrefix + "secret"
	AnnotationSystemNamespace         = annotationPrefix + "system-namespace"
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

	// Secret is the name of the Kubernetes Secret that contains the AWS access
	// key ID and secret access key.
	Secret string

	// SystemNamespace is the Kubernetes namespace of the host resource. AWS
	// credentials are read from a Secret in this namespace.
	SystemNamespace string

	// SecurityGroup is the name of the security group to be used on the instance.
	SecurityGroup string

	// SecurityGroupID is the unique identifier of the security group to be used on
	// the instance.
	SecurityGroupId string

	// SubnetId is the ID of the subnet to use when creating the instance.
	SubnetId string

	// Disk is the amount of permanent storage (in GB) to allocate the instance.
	Disk int32

	// MaxSpotInstancePrice is the maximum price (TODO: find out format) the user
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

	// UserData is the base64-encoded cloud-init or shell script passed to the
	// instance at launch.
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
func GetStaticAWSConfiguration(ctx context.Context, staticHost *maykonfluxcidevv1alpha1.StaticHost, _ client.Client) AWSConfiguration {
	l := logf.FromContext(ctx).WithValues("StaticHost", staticHost.Name)
	l.V(1).Info("building AWS configuration from StaticHost annotations")

	cfg := configurationFromAnnotations(staticHost.GetAnnotations())
	cfg.SystemNamespace = staticHost.Namespace

	l.V(1).Info("AWS configuration resolved",
		"region", cfg.Region,
		"ami", cfg.Ami,
		"instanceType", cfg.InstanceType,
	)
	return cfg
}

// GetDynamicAWSConfiguration returns the AWS configuration for a DynamicHost,
// sourced from the host's annotations.
func GetDynamicAWSConfiguration(ctx context.Context, dynamicHost *maykonfluxcidevv1alpha1.DynamicHost, _ client.Client) AWSConfiguration {
	l := logf.FromContext(ctx).WithValues("DynamicHost", dynamicHost.Name)
	l.V(1).Info("building AWS configuration from DynamicHost annotations")

	cfg := configurationFromAnnotations(dynamicHost.GetAnnotations())
	cfg.SystemNamespace = dynamicHost.Namespace

	l.V(1).Info("AWS configuration resolved",
		"region", cfg.Region,
		"ami", cfg.Ami,
		"instanceType", cfg.InstanceType,
	)
	return cfg
}

// configurationFromAnnotations extracts an AWSConfiguration from a map of
// Kubernetes annotations. Missing keys result in zero-values for the
// corresponding fields; pointer fields remain nil if their annotation is absent.
func configurationFromAnnotations(annotations map[string]string) AWSConfiguration {
	cfg := AWSConfiguration{
		Region:                  annotations[AnnotationRegion],
		Ami:                     annotations[AnnotationAmi],
		InstanceType:            annotations[AnnotationInstanceType],
		KeyName:                 annotations[AnnotationKeyName],
		Secret:                  annotations[AnnotationSecret],
		SecurityGroup:           annotations[AnnotationSecurityGroup],
		SecurityGroupId:         annotations[AnnotationSecurityGroupId],
		SubnetId:                annotations[AnnotationSubnetId],
		Disk:                    parseInt32(annotations[AnnotationDisk]),
		MaxSpotInstancePrice:    annotations[AnnotationMaxSpotInstancePrice],
		InstanceProfileName:     annotations[AnnotationInstanceProfileName],
		InstanceProfileArn:      annotations[AnnotationInstanceProfileArn],
		Tenancy:                 annotations[AnnotationTenancy],
		HostResourceGroupArn:    annotations[AnnotationHostResourceGroupArn],
		LicenseConfigurationArn: annotations[AnnotationLicenseConfigurationArn],
		StrictPublicAddress:     parseBool(annotations[AnnotationStrictPublicAddress]),
	}

	if v, ok := annotations[AnnotationThroughput]; ok {
		cfg.Throughput = parseOptionalInt32(v)
	}

	if v, ok := annotations[AnnotationIops]; ok {
		cfg.Iops = parseOptionalInt32(v)
	}

	if v, ok := annotations[AnnotationUserData]; ok {
		cfg.UserData = &v
	}

	return cfg
}

// parseInt32 parses a string as a base-10 int32. Returns 0 if the string is
// empty or cannot be parsed.
func parseInt32(s string) int32 {
	if s == "" {
		return 0
	}
	v, err := strconv.ParseInt(s, 10, 32)
	if err != nil {
		return 0
	}
	return int32(v)
}

// parseOptionalInt32 parses an optional int32 annotation. Empty strings return
// nil. Invalid values are ignored so callers can distinguish them from zero.
func parseOptionalInt32(s string) *int32 {
	if s == "" {
		return nil
	}
	v, err := strconv.ParseInt(s, 10, 32)
	if err != nil {
		return nil
	}
	parsed := int32(v)
	return &parsed
}

// parseBool parses a string as a boolean. Returns false if the string is empty
// or cannot be parsed.
func parseBool(s string) bool {
	if s == "" {
		return false
	}
	v, err := strconv.ParseBool(s)
	if err != nil {
		return false
	}
	return v
}
