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

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	maykonfluxcidevv1alpha1 "github.com/konflux-ci/may/api/v1alpha1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	internalconfig "github.com/konflux-ci/may/drivers/aws/internal/config"
)

// NewStaticEC2Client creates an EC2 client for a StaticHost using token-based
// authentication. Use NewEC2ClientFactory when a different auth mode is needed.
func NewStaticEC2Client(ctx context.Context, staticHost *maykonfluxcidevv1alpha1.StaticHost, kubeClient ctrlclient.Client) (*ec2.Client, error) {
	cfg := internalconfig.GetStaticAWSConfiguration(ctx, staticHost, kubeClient)
	return newHostEC2Client(ctx, kubeClient, AuthModeToken, cfg)
}

// NewDynamicEC2Client creates an EC2 client for a DynamicHost using token-based
// authentication. Use NewEC2ClientFactory when a different auth mode is needed.
func NewDynamicEC2Client(ctx context.Context, dynamicHost *maykonfluxcidevv1alpha1.DynamicHost, kubeClient ctrlclient.Client) (*ec2.Client, error) {
	cfg := internalconfig.GetDynamicAWSConfiguration(ctx, dynamicHost, kubeClient)
	return newHostEC2Client(ctx, kubeClient, AuthModeToken, cfg)
}

func newHostEC2Client(ctx context.Context, kubeClient ctrlclient.Client, mode AuthMode, cfg internalconfig.AWSConfiguration) (*ec2.Client, error) {
	factory, err := NewEC2ClientFactory(mode, kubeClient)
	if err != nil {
		return nil, err
	}
	return factory.NewEC2Client(ctx, cfg)
}
