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
	DescribeTable("valid integer annotation parsing",
		func(input string, expected int32) {
			value, err := parseInt32(AnnotationDisk, input)

			Expect(err).ShouldNot(HaveOccurred())
			Expect(value).Should(Equal(expected))
		},
		Entry("a valid integer", "40", int32(40)),
		Entry("zero", "0", int32(0)),
	)

	DescribeTable("invalid integer annotation parsing",
		func(input string) {
			_, err := parseInt32(AnnotationDisk, input)

			Expect(err).Should(HaveOccurred())
			Expect(err.Error()).Should(ContainSubstring(AnnotationDisk))
		},
		Entry("an empty string", ""),
		Entry("a non-numeric value", "not-a-number"),
		Entry("a negative integer", "-1"),
		Entry("a value outside int32 range", "2147483648"),
	)
})

var _ = Describe("parseOptionalInt32", func() {
	DescribeTable("valid optional integer annotation parsing",
		func(input string, expected *int32) {
			value, err := parseOptionalInt32(AnnotationThroughput, input)

			Expect(err).ShouldNot(HaveOccurred())
			Expect(value).Should(Equal(expected))
		},
		Entry("a valid integer", "125", int32Ptr(125)),
	)

	DescribeTable("invalid optional integer annotation parsing",
		func(input string) {
			_, err := parseOptionalInt32(AnnotationThroughput, input)

			Expect(err).Should(HaveOccurred())
			Expect(err.Error()).Should(ContainSubstring(AnnotationThroughput))
		},
		Entry("an empty string", ""),
		Entry("a non-numeric value", "not-a-number"),
		Entry("a negative integer", "-1"),
	)
})

var _ = Describe("parseBool", func() {
	DescribeTable("valid boolean annotation parsing",
		func(input string, expected bool) {
			value, err := parseBool(AnnotationStrictPublicAddress, input)

			Expect(err).ShouldNot(HaveOccurred())
			Expect(value).Should(Equal(expected))
		},
		Entry("true", "true", true),
		Entry("false", "false", false),
		Entry("a numeric true value", "1", true),
	)

	DescribeTable("invalid boolean annotation parsing",
		func(input string) {
			_, err := parseBool(AnnotationStrictPublicAddress, input)

			Expect(err).Should(HaveOccurred())
			Expect(err.Error()).Should(ContainSubstring(AnnotationStrictPublicAddress))
		},
		Entry("an empty string", ""),
		Entry("an invalid value", "maybe"),
	)
})

var _ = Describe("configurationFromAnnotations", func() {
	var (
		throughput int32 = 125
		iops       int32 = 3000
		// Raw user-data script; base64 encoding is not applied during parsing.
		userData = "#!/bin/bash\necho hello"
	)

	When("annotations are nil", func() {
		It("should return a zero configuration", func() {
			cfg, err := configurationFromAnnotations(nil)

			Expect(err).ShouldNot(HaveOccurred())
			Expect(cfg).Should(Equal(AWSConfiguration{}))
		})
	})

	When("annotations are empty", func() {
		It("should return a zero configuration", func() {
			cfg, err := configurationFromAnnotations(map[string]string{})

			Expect(err).ShouldNot(HaveOccurred())
			Expect(cfg).Should(Equal(AWSConfiguration{}))
		})
	})

	When("all AWS annotations are present", func() {
		It("should map every field", func() {
			cfg, err := configurationFromAnnotations(map[string]string{
				AnnotationRegion:                  "us-east-1",
				AnnotationAmi:                     "ami-0123456789abcdef0",
				AnnotationInstanceType:            "m6a.4xlarge",
				AnnotationKeyName:                 "my-key",
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

			Expect(err).ShouldNot(HaveOccurred())
			Expect(cfg).Should(Equal(AWSConfiguration{
				Region:                  "us-east-1",
				Ami:                     "ami-0123456789abcdef0",
				InstanceType:            "m6a.4xlarge",
				KeyName:                 "my-key",
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
			cfg, err := configurationFromAnnotations(map[string]string{
				AnnotationRegion: "eu-west-1",
			})

			Expect(err).ShouldNot(HaveOccurred())
			Expect(cfg).Should(Equal(AWSConfiguration{Region: "eu-west-1"}))
			Expect(cfg.Throughput).Should(BeNil())
			Expect(cfg.Iops).Should(BeNil())
			Expect(cfg.UserData).Should(BeNil())
		})
	})

	When("throughput or iops annotations are invalid", func() {
		It("should return an error for invalid throughput", func() {
			_, err := configurationFromAnnotations(map[string]string{
				AnnotationThroughput: "not-a-number",
			})

			Expect(err).Should(HaveOccurred())
			Expect(err.Error()).Should(ContainSubstring(AnnotationThroughput))
		})

		It("should return an error for invalid iops", func() {
			_, err := configurationFromAnnotations(map[string]string{
				AnnotationIops: "also-invalid",
			})

			Expect(err).Should(HaveOccurred())
			Expect(err.Error()).Should(ContainSubstring(AnnotationIops))
		})
	})

	When("present optional annotations are empty", func() {
		It("should return an error for empty throughput", func() {
			_, err := configurationFromAnnotations(map[string]string{
				AnnotationThroughput: "",
			})

			Expect(err).Should(HaveOccurred())
			Expect(err.Error()).Should(ContainSubstring(AnnotationThroughput))
		})

		It("should return an error for empty user-data", func() {
			_, err := configurationFromAnnotations(map[string]string{
				AnnotationUserData: "",
			})

			Expect(err).Should(HaveOccurred())
			Expect(err.Error()).Should(ContainSubstring(AnnotationUserData))
		})
	})

	When("the disk annotation is invalid", func() {
		It("should return an error", func() {
			_, err := configurationFromAnnotations(map[string]string{
				AnnotationDisk: "not-a-number",
			})

			Expect(err).Should(HaveOccurred())
			Expect(err.Error()).Should(ContainSubstring(AnnotationDisk))
		})
	})

	When("the strict-public-address annotation is invalid", func() {
		It("should return an error", func() {
			_, err := configurationFromAnnotations(map[string]string{
				AnnotationStrictPublicAddress: "maybe",
			})

			Expect(err).Should(HaveOccurred())
			Expect(err.Error()).Should(ContainSubstring(AnnotationStrictPublicAddress))
		})
	})

	When("unrelated annotations are present", func() {
		It("should ignore them", func() {
			cfg, err := configurationFromAnnotations(map[string]string{
				AnnotationRegion:            "ap-southeast-2",
				"may.konflux-ci.dev/driver": "aws",
			})

			Expect(err).ShouldNot(HaveOccurred())
			Expect(cfg).Should(Equal(AWSConfiguration{Region: "ap-southeast-2"}))
		})
	})
})

var _ = Describe("GetStaticAWSConfiguration", func() {
	When("the StaticHost has AWS annotations", func() {
		It("should build configuration from host metadata", func() {
			host := &maykonfluxcidevv1alpha1.StaticHost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "aws-host-arm64",
					Namespace: "may-system",
					Annotations: map[string]string{
						AnnotationRegion:       "us-west-2",
						AnnotationAmi:          "ami-static",
						AnnotationInstanceType: "t4g.medium",
					},
				},
			}

			cfg, err := GetStaticAWSConfiguration(context.Background(), host)

			Expect(err).ShouldNot(HaveOccurred())
			Expect(cfg).Should(Equal(AWSConfiguration{
				Region:       "us-west-2",
				Ami:          "ami-static",
				InstanceType: "t4g.medium",
			}))
		})
	})

	When("the StaticHost has invalid AWS annotations", func() {
		It("should return an error", func() {
			host := &maykonfluxcidevv1alpha1.StaticHost{
				ObjectMeta: metav1.ObjectMeta{
					Name: "aws-host-arm64",
					Annotations: map[string]string{
						AnnotationDisk: "not-a-number",
					},
				},
			}

			_, err := GetStaticAWSConfiguration(context.Background(), host)

			Expect(err).Should(HaveOccurred())
			Expect(err.Error()).Should(ContainSubstring(AnnotationDisk))
		})
	})
})

var _ = Describe("GetDynamicAWSConfiguration", func() {
	When("the DynamicHost has AWS annotations", func() {
		It("should build configuration from host metadata", func() {
			host := &maykonfluxcidevv1alpha1.DynamicHost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "aws-host-amd64",
					Namespace: "may-system",
					Annotations: map[string]string{
						AnnotationRegion:       "us-east-1",
						AnnotationAmi:          "ami-dynamic",
						AnnotationInstanceType: "m6a.4xlarge",
					},
				},
			}

			cfg, err := GetDynamicAWSConfiguration(context.Background(), host)

			Expect(err).ShouldNot(HaveOccurred())
			Expect(cfg).Should(Equal(AWSConfiguration{
				Region:       "us-east-1",
				Ami:          "ami-dynamic",
				InstanceType: "m6a.4xlarge",
			}))
		})
	})
})

func int32Ptr(v int32) *int32 {
	return &v
}
