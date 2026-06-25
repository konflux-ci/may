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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var _ = Describe("secretData", func() {
	DescribeTable("secret key lookup",
		func(data map[string][]byte, keys []string, expected string) {
			Expect(secretData(data, keys...)).Should(Equal(expected))
		},
		Entry("the first key is present",
			map[string][]byte{"access-key-id": []byte("AKIA")},
			[]string{"access-key-id", "aws_access_key_id"},
			"AKIA",
		),
		Entry("only the fallback key is present",
			map[string][]byte{"aws_access_key_id": []byte("LEGACY")},
			[]string{"access-key-id", "aws_access_key_id"},
			"LEGACY",
		),
		Entry("the matched key is empty",
			map[string][]byte{"access-key-id": []byte("")},
			[]string{"access-key-id", "aws_access_key_id"},
			"",
		),
		Entry("no matching keys exist",
			map[string][]byte{},
			[]string{"access-key-id"},
			"",
		),
	)
})

var _ = Describe("kubeSecretCredentialsProvider", func() {
	const (
		secretName = "aws-account"
		namespace  = "may-system"
	)

	var (
		ctx      context.Context
		provider *kubeSecretCredentialsProvider
	)

	BeforeEach(func() {
		ctx = context.Background()
	})

	When("the secret uses MPC key names", func() {
		BeforeEach(func() {
			provider = &kubeSecretCredentialsProvider{
				kubeClient: newTestKubeClient(awsCredentialsSecret(secretName, namespace, map[string][]byte{
					secretKeyAccessKeyID:     []byte("AKIAEXAMPLE"),
					secretKeySecretAccessKey: []byte("secret"),
					secretKeySessionToken:    []byte("token"),
				})),
				secretName:      secretName,
				secretNamespace: namespace,
			}
		})

		It("should return credentials including the session token", func() {
			creds, err := provider.Retrieve(ctx)

			Expect(err).ShouldNot(HaveOccurred())
			Expect(creds.AccessKeyID).Should(Equal("AKIAEXAMPLE"))
			Expect(creds.SecretAccessKey).Should(Equal("secret"))
			Expect(creds.SessionToken).Should(Equal("token"))
			Expect(creds.Source).Should(Equal("KubernetesSecret"))
			Expect(creds.CanExpire).Should(BeTrue())
			Expect(creds.Expires).ShouldNot(BeZero())
		})
	})

	When("the secret uses legacy key names", func() {
		BeforeEach(func() {
			provider = &kubeSecretCredentialsProvider{
				kubeClient: newTestKubeClient(awsCredentialsSecret("legacy-secret", namespace, map[string][]byte{
					legacySecretKeyAccessKeyID:     []byte("AKIALEGACY"),
					legacySecretKeySecretAccessKey: []byte("legacy-secret-value"),
				})),
				secretName:      "legacy-secret",
				secretNamespace: namespace,
			}
		})

		It("should return credentials from the legacy keys", func() {
			creds, err := provider.Retrieve(ctx)

			Expect(err).ShouldNot(HaveOccurred())
			Expect(creds.AccessKeyID).Should(Equal("AKIALEGACY"))
			Expect(creds.SecretAccessKey).Should(Equal("legacy-secret-value"))
			Expect(creds.Source).Should(Equal("KubernetesSecret"))
		})
	})

	When("the secret does not exist", func() {
		BeforeEach(func() {
			provider = &kubeSecretCredentialsProvider{
				kubeClient:      newTestKubeClient(),
				secretName:      "missing-secret",
				secretNamespace: namespace,
			}
		})

		It("should return an error", func() {
			_, err := provider.Retrieve(ctx)

			Expect(err).Should(HaveOccurred())
			Expect(err.Error()).Should(ContainSubstring("failed to get AWS credentials secret"))
		})
	})

	When("the secret is missing required keys", func() {
		BeforeEach(func() {
			provider = &kubeSecretCredentialsProvider{
				kubeClient: newTestKubeClient(awsCredentialsSecret("incomplete-secret", namespace, map[string][]byte{
					secretKeyAccessKeyID: []byte("AKIAEXAMPLE"),
				})),
				secretName:      "incomplete-secret",
				secretNamespace: namespace,
			}
		})

		It("should return an error", func() {
			_, err := provider.Retrieve(ctx)

			Expect(err).Should(HaveOccurred())
			Expect(err.Error()).Should(ContainSubstring("missing required keys"))
		})
	})
})

func awsCredentialsSecret(name, namespace string, data map[string][]byte) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Data: data,
	}
}

func newTestKubeClient(objects ...ctrlclient.Object) ctrlclient.Client {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	builder := fake.NewClientBuilder().WithScheme(scheme)
	if len(objects) > 0 {
		builder = builder.WithObjects(objects...)
	}
	return builder.Build()
}
