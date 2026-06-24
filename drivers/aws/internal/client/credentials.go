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
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// kubeSecretCredentialsProvider implements aws.CredentialsProvider by reading
// AWS credentials from a Kubernetes Secret whenever the SDK requests them.
type kubeSecretCredentialsProvider struct {
	kubeClient      client.Client
	secretName      string
	secretNamespace string
}

// Secret keys follow the multi-platform-controller convention. Legacy AWS-style
// key names are accepted as a fallback.
const (
	secretKeyAccessKeyID     = "access-key-id"
	secretKeySecretAccessKey = "secret-access-key"
	secretKeySessionToken    = "session-token"

	legacySecretKeyAccessKeyID     = "aws_access_key_id"
	legacySecretKeySecretAccessKey = "aws_secret_access_key"
	legacySecretKeySessionToken    = "aws_session_token"
)

// Retrieve reads AWS credentials from the configured Kubernetes Secret.
func (p *kubeSecretCredentialsProvider) Retrieve(ctx context.Context) (aws.Credentials, error) {
	secret := &corev1.Secret{}
	if err := p.kubeClient.Get(ctx, client.ObjectKey{
		Namespace: p.secretNamespace,
		Name:      p.secretName,
	}, secret); err != nil {
		return aws.Credentials{}, fmt.Errorf("failed to get AWS credentials secret %s/%s: %w",
			p.secretNamespace, p.secretName, err)
	}

	accessKeyID := secretData(secret.Data, secretKeyAccessKeyID, legacySecretKeyAccessKeyID)
	secretAccessKey := secretData(secret.Data, secretKeySecretAccessKey, legacySecretKeySecretAccessKey)
	if accessKeyID == "" || secretAccessKey == "" {
		return aws.Credentials{}, fmt.Errorf(
			"secret %s/%s is missing required keys (%q and/or %q, or legacy %q and/or %q)",
			p.secretNamespace, p.secretName,
			secretKeyAccessKeyID, secretKeySecretAccessKey,
			legacySecretKeyAccessKeyID, legacySecretKeySecretAccessKey)
	}

	return aws.Credentials{
		AccessKeyID:     accessKeyID,
		SecretAccessKey: secretAccessKey,
		SessionToken:    secretData(secret.Data, secretKeySessionToken, legacySecretKeySessionToken),
		Source:          "KubernetesSecret",
	}, nil
}

func secretData(data map[string][]byte, keys ...string) string {
	for _, key := range keys {
		if value, ok := data[key]; ok && len(value) > 0 {
			return string(value)
		}
	}
	return ""
}
