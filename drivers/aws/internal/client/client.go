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
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	maykonfluxcidevv1alpha1 "github.com/konflux-ci/may/api/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	internalconfig "github.com/konflux-ci/may/drivers/aws/internal/config"
)

// NewStaticEC2Client creates an EC2 client using the static AWS configuration.
// Credentials are loaded from the Kubernetes Secret referenced in the host
// annotations whenever the AWS SDK resolves credentials for this client.
func NewStaticEC2Client(ctx context.Context, staticHost *maykonfluxcidevv1alpha1.StaticHost, kubeClient client.Client) (*ec2.Client, error) {
	cfg := internalconfig.GetStaticAWSConfiguration(ctx, staticHost, kubeClient)
	return newEC2Client(ctx, kubeClient, cfg)
}

// NewDynamicEC2Client creates an EC2 client using the dynamic AWS configuration.
// Credentials are loaded from the Kubernetes Secret referenced in the host
// annotations whenever the AWS SDK resolves credentials for this client.
func NewDynamicEC2Client(ctx context.Context, dynamicHost *maykonfluxcidevv1alpha1.DynamicHost, kubeClient client.Client) (*ec2.Client, error) {
	cfg := internalconfig.GetDynamicAWSConfiguration(ctx, dynamicHost, kubeClient)
	return newEC2Client(ctx, kubeClient, cfg)
}

func newEC2Client(ctx context.Context, kubeClient client.Client, cfg internalconfig.AWSConfiguration) (*ec2.Client, error) {
	if err := validateAWSConfiguration(cfg); err != nil {
		return nil, err
	}

	provider := &kubeSecretCredentialsProvider{
		kubeClient:      kubeClient,
		secretName:      cfg.Secret,
		secretNamespace: cfg.SystemNamespace,
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(cfg.Region),
		awsconfig.WithCredentialsProvider(aws.NewCredentialsCache(provider)),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	return ec2.NewFromConfig(awsCfg), nil
}

func validateAWSConfiguration(cfg internalconfig.AWSConfiguration) error {
	var errs []error
	if cfg.Region == "" {
		errs = append(errs, fmt.Errorf("missing required annotation %q", internalconfig.AnnotationRegion))
	}
	if cfg.Secret == "" {
		errs = append(errs, fmt.Errorf("missing required annotation %q", internalconfig.AnnotationSecret))
	}
	if cfg.SystemNamespace == "" {
		errs = append(errs, fmt.Errorf("missing host namespace for AWS credentials secret lookup"))
	}
	return errors.Join(errs...)
}
