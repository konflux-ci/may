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

	internalconfig "github.com/konflux-ci/may/drivers/aws/internal/config"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("NewEC2ClientFactory", func() {
	When("token authentication is requested", func() {
		It("should return a token factory", func() {
			factory, err := NewEC2ClientFactory(AuthModeToken, newTestKubeClient())

			Expect(err).ShouldNot(HaveOccurred())
			Expect(factory).Should(BeAssignableToTypeOf(&tokenFactory{}))
		})
	})

	When("service account authentication is requested", func() {
		It("should return a service account factory", func() {
			factory, err := NewEC2ClientFactory(AuthModeServiceAccount, nil)

			Expect(err).ShouldNot(HaveOccurred())
			Expect(factory).Should(BeAssignableToTypeOf(&serviceAccountFactory{}))
		})
	})

	When("an unsupported auth mode is requested", func() {
		It("should return an error", func() {
			_, err := NewEC2ClientFactory(AuthMode("unknown"), nil)

			Expect(err).Should(HaveOccurred())
			Expect(err.Error()).Should(ContainSubstring("unsupported AWS auth mode"))
		})
	})
})

var _ = Describe("validateTokenAWSConfiguration", func() {
	When("region, secret, and host namespace are set", func() {
		It("should not return an error", func() {
			err := validateTokenAWSConfiguration(internalconfig.AWSConfiguration{
				Region:          "us-east-1",
				Secret:          "aws-account",
				SystemNamespace: "may-system",
			})

			Expect(err).ShouldNot(HaveOccurred())
		})
	})

	When("the region annotation is missing", func() {
		It("should return an error referencing the region annotation", func() {
			err := validateTokenAWSConfiguration(internalconfig.AWSConfiguration{
				Secret:          "aws-account",
				SystemNamespace: "may-system",
			})

			Expect(err).Should(HaveOccurred())
			Expect(err.Error()).Should(ContainSubstring(internalconfig.AnnotationRegion))
		})
	})

	When("the secret and host namespace are missing", func() {
		It("should return an error referencing both fields", func() {
			err := validateTokenAWSConfiguration(internalconfig.AWSConfiguration{
				Region: "us-east-1",
			})

			Expect(err).Should(HaveOccurred())
			Expect(err.Error()).Should(ContainSubstring(internalconfig.AnnotationSecret))
			Expect(err.Error()).Should(ContainSubstring("host namespace"))
		})
	})
})

var _ = Describe("tokenFactory", func() {
	const (
		secretName = "aws-account"
		namespace  = "may-system"
	)

	var (
		ctx        context.Context
		factory    *tokenFactory
		kubeClient = newTestKubeClient()
	)

	BeforeEach(func() {
		ctx = context.Background()
		factory = &tokenFactory{kubeClient: kubeClient}
	})

	When("required configuration is missing", func() {
		It("should return a validation error", func() {
			ec2Client, err := factory.NewEC2Client(ctx, internalconfig.AWSConfiguration{})

			Expect(err).Should(HaveOccurred())
			Expect(err.Error()).Should(ContainSubstring(internalconfig.AnnotationRegion))
			Expect(ec2Client).Should(BeNil())
		})
	})

	When("the configuration is valid and the credentials secret exists", func() {
		BeforeEach(func() {
			factory = &tokenFactory{kubeClient: newTestKubeClient(awsCredentialsSecret(secretName, namespace, map[string][]byte{
				secretKeyAccessKeyID:     []byte("AKIAEXAMPLE"),
				secretKeySecretAccessKey: []byte("secret"),
			}))}
		})

		It("should return a configured EC2 client", func() {
			ec2Client, err := factory.NewEC2Client(ctx, internalconfig.AWSConfiguration{
				Region:          "us-east-1",
				Secret:          secretName,
				SystemNamespace: namespace,
			})

			Expect(err).ShouldNot(HaveOccurred())
			Expect(ec2Client).ShouldNot(BeNil())
			Expect(ec2Client.Options().Region).Should(Equal("us-east-1"))

			creds, err := ec2Client.Options().Credentials.Retrieve(ctx)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(creds.AccessKeyID).Should(Equal("AKIAEXAMPLE"))
			Expect(creds.SecretAccessKey).Should(Equal("secret"))
			Expect(creds.CanExpire).Should(BeTrue())
		})
	})
})

var _ = Describe("serviceAccountFactory", func() {
	When("the region is configured", func() {
		It("should report that service account auth is not implemented yet", func() {
			factory := &serviceAccountFactory{}
			ec2Client, err := factory.NewEC2Client(context.Background(), internalconfig.AWSConfiguration{
				Region: "us-east-1",
			})

			Expect(err).Should(MatchError(errServiceAccountAuthNotImplemented))
			Expect(ec2Client).Should(BeNil())
		})
	})

	When("the region is missing", func() {
		It("should return a validation error", func() {
			factory := &serviceAccountFactory{}
			ec2Client, err := factory.NewEC2Client(context.Background(), internalconfig.AWSConfiguration{})

			Expect(err).Should(HaveOccurred())
			Expect(err.Error()).Should(ContainSubstring(internalconfig.AnnotationRegion))
			Expect(ec2Client).Should(BeNil())
		})
	})
})
