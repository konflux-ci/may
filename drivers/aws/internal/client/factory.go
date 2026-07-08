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

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	internalconfig "github.com/konflux-ci/may/drivers/aws/internal/config"
)

// AuthMode identifies how the driver authenticates to AWS.
type AuthMode string

const (
	// AuthModeToken loads credentials from a Kubernetes Secret referenced on the host.
	AuthModeToken AuthMode = "token"
	// AuthModeServiceAccount uses the controller pod's ServiceAccount (IRSA on EKS).
	AuthModeServiceAccount AuthMode = "serviceaccount"
)

// EC2ClientFactory builds EC2 clients for a specific authentication mode.
type EC2ClientFactory interface {
	NewEC2Client(ctx context.Context, cfg internalconfig.AWSConfiguration) (*ec2.Client, error)
}

// NewEC2ClientFactory returns a factory for the requested authentication mode.
func NewEC2ClientFactory(mode AuthMode, kubeClient ctrlclient.Client) (EC2ClientFactory, error) {
	switch mode {
	case AuthModeToken, "":
		return &tokenFactory{kubeClient: kubeClient}, nil
	case AuthModeServiceAccount:
		return &serviceAccountFactory{}, nil
	default:
		return nil, fmt.Errorf("unsupported AWS auth mode %q", mode)
	}
}
