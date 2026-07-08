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
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	internalconfig "github.com/konflux-ci/may/drivers/aws/internal/config"
)

// tokenFactory builds EC2 clients using credentials from a Kubernetes Secret.
type tokenFactory struct {
	kubeClient ctrlclient.Client
}

func (f *tokenFactory) NewEC2Client(ctx context.Context, cfg internalconfig.AWSConfiguration) (*ec2.Client, error) {
	if err := validateTokenAWSConfiguration(cfg); err != nil {
		return nil, err
	}

	provider := &kubeSecretCredentialsProvider{
		kubeClient:      f.kubeClient,
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

func validateTokenAWSConfiguration(cfg internalconfig.AWSConfiguration) error {
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
