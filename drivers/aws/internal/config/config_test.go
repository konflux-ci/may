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

	maykonfluxcidevv1alpha1 "github.com/konflux-ci/may/api/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("parseInt32", func() {
	DescribeTable("integer annotation parsing",
		func(input string, expected int32) {
			Expect(parseInt32(input)).Should(Equal(expected))
		},
		Entry("an empty string", "", int32(0)),
		Entry("a valid integer", "40", int32(40)),
		Entry("a non-numeric value", "not-a-number", int32(0)),
		Entry("a negative integer", "-1", int32(-1)),
		Entry("a value outside int32 range", "2147483648", int32(0)),
	)
})

var _ = Describe("parseBool", func() {
	DescribeTable("boolean annotation parsing",
		func(input string, expected bool) {
			Expect(parseBool(input)).Should(Equal(expected))
		},
		Entry("an empty string", "", false),
		Entry("true", "true", true),
		Entry("false", "false", false),
		Entry("an invalid value", "maybe", false),
		Entry("a numeric true value", "1", true),
	)
})

var _ = Describe("configurationFromAnnotations", func() {
	var (
		throughput int32 = 125
		iops       int32 = 3000
		userData         = "#!/bin/bash\necho hello"
	)

	When("annotations are nil", func() {
		It("should return a zero configuration", func() {
			Expect(configurationFromAnnotations(nil)).Should(Equal(AWSConfiguration{}))
		})
	})

	When("annotations are empty", func() {
		It("should return a zero configuration", func() {
			Expect(configurationFromAnnotations(map[string]string{})).Should(Equal(AWSConfiguration{}))
		})
	})

	When("all AWS annotations are present", func() {
		It("should map every field", func() {
			cfg := configurationFromAnnotations(map[string]string{
				AnnotationRegion:                  "us-east-1",
				AnnotationAmi:                     "ami-0123456789abcdef0",
				AnnotationInstanceType:            "m6a.4xlarge",
				AnnotationKeyName:                 "my-key",
				AnnotationSecret:                  "aws-account",
				AnnotationSystemNamespace:         "may-system",
				AnnotationSecurityGroup:           "launch-wizard-1",
				AnnotationSecurityGroupId:         "sg-0123456789abcdef0",
				AnnotationSubnetId:                "subnet-0123456789abcdef0",
				AnnotationDisk:                    "80",
				AnnotationMaxSpotInstancePrice:    "0.50",
				AnnotationInstanceProfileName:     "my-profile",
				AnnotationInstanceProfileArn:      "arn:aws:iam::123456789012:instance-profile/my-profile",
				AnnotationThroughput:              "125",
				AnnotationIops:                    "3000",
				AnnotationUserData:                userData,
				AnnotationTenancy:                 "host",
				AnnotationHostResourceGroupArn:    "arn:aws:resource-groups:us-east-1:123456789012:group/my-group",
				AnnotationLicenseConfigurationArn: "arn:aws:license-manager:us-east-1:123456789012:license-configuration:lic-0123456789abcdef0",
				AnnotationStrictPublicAddress:     "true",
			})

			Expect(cfg).Should(Equal(AWSConfiguration{
				Region:                  "us-east-1",
				Ami:                     "ami-0123456789abcdef0",
				InstanceType:            "m6a.4xlarge",
				KeyName:                 "my-key",
				Secret:                  "aws-account",
				SystemNamespace:         "may-system",
				SecurityGroup:           "launch-wizard-1",
				SecurityGroupId:         "sg-0123456789abcdef0",
				SubnetId:                "subnet-0123456789abcdef0",
				Disk:                    80,
				MaxSpotInstancePrice:    "0.50",
				InstanceProfileName:     "my-profile",
				InstanceProfileArn:      "arn:aws:iam::123456789012:instance-profile/my-profile",
				Throughput:              &throughput,
				Iops:                    &iops,
				UserData:                &userData,
				Tenancy:                 "host",
				HostResourceGroupArn:    "arn:aws:resource-groups:us-east-1:123456789012:group/my-group",
				LicenseConfigurationArn: "arn:aws:license-manager:us-east-1:123456789012:license-configuration:lic-0123456789abcdef0",
				StrictPublicAddress:     true,
			}))
		})
	})

	When("optional pointer annotations are absent", func() {
		It("should leave pointer fields nil", func() {
			cfg := configurationFromAnnotations(map[string]string{
				AnnotationRegion: "eu-west-1",
			})

			Expect(cfg).Should(Equal(AWSConfiguration{Region: "eu-west-1"}))
			Expect(cfg.Throughput).Should(BeNil())
			Expect(cfg.Iops).Should(BeNil())
			Expect(cfg.UserData).Should(BeNil())
		})
	})

	When("optional pointer annotations are present but empty", func() {
		It("should set pointer fields to zero values", func() {
			cfg := configurationFromAnnotations(map[string]string{
				AnnotationThroughput: "",
				AnnotationIops:       "",
				AnnotationUserData:   "",
			})

			Expect(cfg.Throughput).Should(Equal(int32Ptr(0)))
			Expect(cfg.Iops).Should(Equal(int32Ptr(0)))
			Expect(cfg.UserData).Should(Equal(strPtr("")))
		})
	})

	When("the disk annotation is invalid", func() {
		It("should default disk to zero", func() {
			cfg := configurationFromAnnotations(map[string]string{
				AnnotationDisk: "not-a-number",
			})

			Expect(cfg.Disk).Should(Equal(int32(0)))
		})
	})

	When("unrelated annotations are present", func() {
		It("should ignore them", func() {
			cfg := configurationFromAnnotations(map[string]string{
				AnnotationRegion:            "ap-southeast-2",
				"may.konflux-ci.dev/driver": "aws",
			})

			Expect(cfg).Should(Equal(AWSConfiguration{Region: "ap-southeast-2"}))
		})
	})
})

var _ = Describe("GetStaticAWSConfiguration", func() {
	When("the StaticHost has AWS annotations", func() {
		It("should build configuration from host metadata", func() {
			host := &maykonfluxcidevv1alpha1.StaticHost{
				ObjectMeta: metav1.ObjectMeta{
					Name: "aws-host-arm64",
					Annotations: map[string]string{
						AnnotationRegion:       "us-west-2",
						AnnotationAmi:          "ami-static",
						AnnotationInstanceType: "t4g.medium",
						AnnotationSecret:       "aws-secret",
					},
				},
			}

			cfg := GetStaticAWSConfiguration(context.Background(), host, nil)

			Expect(cfg).Should(Equal(AWSConfiguration{
				Region:       "us-west-2",
				Ami:          "ami-static",
				InstanceType: "t4g.medium",
				Secret:       "aws-secret",
			}))
		})
	})
})

var _ = Describe("GetDynamicAWSConfiguration", func() {
	When("the DynamicHost has AWS annotations", func() {
		It("should build configuration from host metadata", func() {
			host := &maykonfluxcidevv1alpha1.DynamicHost{
				ObjectMeta: metav1.ObjectMeta{
					Name: "aws-host-amd64",
					Annotations: map[string]string{
						AnnotationRegion:          "us-east-1",
						AnnotationAmi:             "ami-dynamic",
						AnnotationInstanceType:    "m6a.4xlarge",
						AnnotationSecret:          "aws-account",
						AnnotationSystemNamespace: "may-system",
					},
				},
			}

			cfg := GetDynamicAWSConfiguration(context.Background(), host, nil)

			Expect(cfg).Should(Equal(AWSConfiguration{
				Region:          "us-east-1",
				Ami:             "ami-dynamic",
				InstanceType:    "m6a.4xlarge",
				Secret:          "aws-account",
				SystemNamespace: "may-system",
			}))
		})
	})
})

func int32Ptr(v int32) *int32 {
	return &v
}

func strPtr(v string) *string {
	return &v
}
