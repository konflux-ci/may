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

package client

import (
	"context"
	"os"

	maykonfluxcidevv1alpha1 "github.com/konflux-ci/may/api/v1alpha1"
	internalconfig "github.com/konflux-ci/may/drivers/aws/internal/config"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("validateAWSConfiguration", func() {
	When("the region is set", func() {
		It("should not return an error", func() {
			err := validateAWSConfiguration(internalconfig.AWSConfiguration{
				Region: "us-east-1",
			})

			Expect(err).ShouldNot(HaveOccurred())
		})
	})

	When("the region annotation is missing", func() {
		It("should return an error referencing the region annotation", func() {
			err := validateAWSConfiguration(internalconfig.AWSConfiguration{})

			Expect(err).Should(HaveOccurred())
			Expect(err.Error()).Should(ContainSubstring(internalconfig.AnnotationRegion))
		})
	})
})

var _ = Describe("validateCredentialEnvironment", func() {
	var (
		originalTokenFile string
		originalRoleARN   string
	)

	BeforeEach(func() {
		originalTokenFile, originalRoleARN = awsWebIdentityEnv()
	})

	AfterEach(func() {
		restoreAWSWebIdentityEnv(originalTokenFile, originalRoleARN)
	})

	When("web-identity env vars are unset", func() {
		It("should allow the SDK default credential chain", func() {
			clearAWSWebIdentityEnv()

			Expect(validateCredentialEnvironment()).ShouldNot(HaveOccurred())
		})
	})

	When("only AWS_WEB_IDENTITY_TOKEN_FILE is set", func() {
		It("should return an error", func() {
			Expect(setEnvOrUnset("AWS_WEB_IDENTITY_TOKEN_FILE", "/var/run/secrets/aws/token")).Should(Succeed())
			Expect(setEnvOrUnset("AWS_ROLE_ARN", "")).Should(Succeed())

			err := validateCredentialEnvironment()

			Expect(err).Should(HaveOccurred())
			Expect(err.Error()).Should(ContainSubstring("AWS_WEB_IDENTITY_TOKEN_FILE"))
			Expect(err.Error()).Should(ContainSubstring("AWS_ROLE_ARN"))
		})
	})

	When("only AWS_ROLE_ARN is set", func() {
		It("should return an error", func() {
			Expect(setEnvOrUnset("AWS_WEB_IDENTITY_TOKEN_FILE", "")).Should(Succeed())
			Expect(setEnvOrUnset("AWS_ROLE_ARN", "arn:aws:iam::123456789012:role/example")).Should(Succeed())

			err := validateCredentialEnvironment()

			Expect(err).Should(HaveOccurred())
			Expect(err.Error()).Should(ContainSubstring("AWS_WEB_IDENTITY_TOKEN_FILE"))
			Expect(err.Error()).Should(ContainSubstring("AWS_ROLE_ARN"))
		})
	})

	When("web-identity env vars are set but the token file is missing", func() {
		It("should return an error", func() {
			Expect(setEnvOrUnset("AWS_WEB_IDENTITY_TOKEN_FILE", "/tmp/missing-aws-web-identity-token")).Should(Succeed())
			Expect(setEnvOrUnset("AWS_ROLE_ARN", "arn:aws:iam::123456789012:role/example")).Should(Succeed())

			err := validateCredentialEnvironment()

			Expect(err).Should(HaveOccurred())
			Expect(err.Error()).Should(ContainSubstring("/tmp/missing-aws-web-identity-token"))
		})
	})

	When("web-identity env vars are set and the token file exists", func() {
		It("should not return an error", func() {
			tokenFile, err := os.CreateTemp("", "aws-web-identity-token")
			Expect(err).ShouldNot(HaveOccurred())
			DeferCleanup(func() {
				Expect(os.Remove(tokenFile.Name())).Should(Succeed())
			})

			Expect(setEnvOrUnset("AWS_WEB_IDENTITY_TOKEN_FILE", tokenFile.Name())).Should(Succeed())
			Expect(setEnvOrUnset("AWS_ROLE_ARN", "arn:aws:iam::123456789012:role/example")).Should(Succeed())

			Expect(validateCredentialEnvironment()).ShouldNot(HaveOccurred())
		})
	})
})

var _ = Describe("EC2 client construction", func() {
	var (
		originalTokenFile string
		originalRoleARN   string
	)

	BeforeEach(func() {
		originalTokenFile, originalRoleARN = awsWebIdentityEnv()
		clearAWSWebIdentityEnv()
	})

	AfterEach(func() {
		restoreAWSWebIdentityEnv(originalTokenFile, originalRoleARN)
	})

	var _ = Describe("newEC2Client", func() {
		When("the region is missing", func() {
			It("should return a validation error", func() {
				ec2Client, err := newEC2Client(context.Background(), internalconfig.AWSConfiguration{})

				Expect(err).Should(HaveOccurred())
				Expect(err.Error()).Should(ContainSubstring(internalconfig.AnnotationRegion))
				Expect(ec2Client).Should(BeNil())
			})
		})

		When("the region is configured", func() {
			It("should return an EC2 client for that region", func() {
				ec2Client, err := newEC2Client(context.Background(), internalconfig.AWSConfiguration{
					Region: "us-east-1",
				})

				Expect(err).ShouldNot(HaveOccurred())
				Expect(ec2Client).ShouldNot(BeNil())
				Expect(ec2Client.Options().Region).Should(Equal("us-east-1"))
			})
		})
	})

	var _ = Describe("NewStaticEC2Client", func() {
		When("the StaticHost has a region annotation", func() {
			It("should return an EC2 client for the configured region", func() {
				host := &maykonfluxcidevv1alpha1.StaticHost{
					ObjectMeta: metav1.ObjectMeta{
						Name: "aws-static-host",
						Annotations: map[string]string{
							internalconfig.AnnotationRegion: "us-east-1",
						},
					},
				}

				ec2Client, err := NewStaticEC2Client(context.Background(), host)

				Expect(err).ShouldNot(HaveOccurred())
				Expect(ec2Client).ShouldNot(BeNil())
				Expect(ec2Client.Options().Region).Should(Equal("us-east-1"))
			})
		})
	})

	var _ = Describe("NewDynamicEC2Client", func() {
		When("the DynamicHost has a region annotation", func() {
			It("should return an EC2 client for the configured region", func() {
				host := &maykonfluxcidevv1alpha1.DynamicHost{
					ObjectMeta: metav1.ObjectMeta{
						Name: "aws-dynamic-host",
						Annotations: map[string]string{
							internalconfig.AnnotationRegion: "eu-west-1",
						},
					},
				}

				ec2Client, err := NewDynamicEC2Client(context.Background(), host)

				Expect(err).ShouldNot(HaveOccurred())
				Expect(ec2Client).ShouldNot(BeNil())
				Expect(ec2Client.Options().Region).Should(Equal("eu-west-1"))
			})
		})
	})
})

func awsWebIdentityEnv() (tokenFile, roleARN string) {
	return os.Getenv("AWS_WEB_IDENTITY_TOKEN_FILE"), os.Getenv("AWS_ROLE_ARN")
}

func clearAWSWebIdentityEnv() {
	Expect(setEnvOrUnset("AWS_WEB_IDENTITY_TOKEN_FILE", "")).Should(Succeed())
	Expect(setEnvOrUnset("AWS_ROLE_ARN", "")).Should(Succeed())
}

func restoreAWSWebIdentityEnv(tokenFile, roleARN string) {
	Expect(setEnvOrUnset("AWS_WEB_IDENTITY_TOKEN_FILE", tokenFile)).Should(Succeed())
	Expect(setEnvOrUnset("AWS_ROLE_ARN", roleARN)).Should(Succeed())
}

func setEnvOrUnset(key, value string) error {
	if value == "" {
		return os.Unsetenv(key)
	}
	return os.Setenv(key, value)
}
