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

	maykonfluxcidevv1alpha1 "github.com/konflux-ci/may/api/v1alpha1"
	internalconfig "github.com/konflux-ci/may/drivers/aws/internal/config"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("NewStaticEC2Client", func() {
	When("the StaticHost has valid AWS annotations and the credentials secret exists", func() {
		It("should return an EC2 client for the configured region", func() {
			ctx := context.Background()
			host := staticHostWithAWSAnnotations()
			kubeClient := newTestKubeClient(awsCredentialsSecret("aws-account", host.Namespace, map[string][]byte{
				secretKeyAccessKeyID:     []byte("AKIASTATIC"),
				secretKeySecretAccessKey: []byte("static-secret"),
			}))

			ec2Client, err := NewStaticEC2Client(ctx, host, kubeClient)

			Expect(err).ShouldNot(HaveOccurred())
			Expect(ec2Client).ShouldNot(BeNil())
			Expect(ec2Client.Options().Region).Should(Equal("us-east-1"))
		})
	})
})

var _ = Describe("NewDynamicEC2Client", func() {
	When("the DynamicHost has valid AWS annotations and the credentials secret exists", func() {
		It("should return an EC2 client for the configured region", func() {
			ctx := context.Background()
			host := dynamicHostWithAWSAnnotations()
			kubeClient := newTestKubeClient(awsCredentialsSecret("aws-account", host.Namespace, map[string][]byte{
				secretKeyAccessKeyID:     []byte("AKIADYNAMIC"),
				secretKeySecretAccessKey: []byte("dynamic-secret"),
			}))

			ec2Client, err := NewDynamicEC2Client(ctx, host, kubeClient)

			Expect(err).ShouldNot(HaveOccurred())
			Expect(ec2Client).ShouldNot(BeNil())
			Expect(ec2Client.Options().Region).Should(Equal("eu-west-1"))
		})
	})
})

func staticHostWithAWSAnnotations() *maykonfluxcidevv1alpha1.StaticHost {
	return &maykonfluxcidevv1alpha1.StaticHost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "aws-static-host",
			Namespace: "may-system",
			Annotations: map[string]string{
				internalconfig.AnnotationRegion: "us-east-1",
				internalconfig.AnnotationSecret: "aws-account",
			},
		},
	}
}

func dynamicHostWithAWSAnnotations() *maykonfluxcidevv1alpha1.DynamicHost {
	return &maykonfluxcidevv1alpha1.DynamicHost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "aws-dynamic-host",
			Namespace: "may-system",
			Annotations: map[string]string{
				internalconfig.AnnotationRegion: "eu-west-1",
				internalconfig.AnnotationSecret: "aws-account",
			},
		},
	}
}
