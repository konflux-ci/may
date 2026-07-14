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

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	maykonfluxcidevv1alpha1 "github.com/konflux-ci/may/api/v1alpha1"

	internalconfig "github.com/konflux-ci/may/drivers/aws/internal/config"
)

// NewStaticEC2Client creates an EC2 client for a StaticHost.
//
// AWS credentials are not read from Kubernetes Secrets or host annotations.
// See newEC2Client for how credentials are resolved on OpenShift.
func NewStaticEC2Client(ctx context.Context, staticHost *maykonfluxcidevv1alpha1.StaticHost) (*ec2.Client, error) {
	cfg := internalconfig.GetStaticAWSConfiguration(ctx, staticHost)
	return newEC2Client(ctx, cfg)
}

// NewDynamicEC2Client creates an EC2 client for a DynamicHost.
//
// AWS credentials are not read from Kubernetes Secrets or host annotations.
// See newEC2Client for how credentials are resolved on OpenShift.
func NewDynamicEC2Client(ctx context.Context, dynamicHost *maykonfluxcidevv1alpha1.DynamicHost) (*ec2.Client, error) {
	cfg := internalconfig.GetDynamicAWSConfiguration(ctx, dynamicHost)
	return newEC2Client(ctx, cfg)
}

// newEC2Client builds an EC2 client for the given AWS configuration.
//
// Only the region is taken from host annotations. Authentication is delegated
// to aws-sdk-go-v2's LoadDefaultConfig default credential chain. The driver
// does not read AWS keys from Kubernetes Secrets or host annotations.
//
// Deployment model: standalone OpenShift → AWS EC2 API
//
// The controller runs on OpenShift (not EKS) and calls the EC2 API in AWS.
// Credentials are obtained through web-identity federation: OpenShift proves
// the pod's ServiceAccount identity, AWS IAM trusts that identity via an OIDC
// provider, and STS issues short-lived credentials for the EC2 API only.
//
// Runtime flow:
//  1. The controller pod runs as the controller-manager ServiceAccount.
//  2. The Deployment projects a ServiceAccount token with audience
//     sts.amazonaws.com into the pod (see config/manager/aws_web_identity_patch.yaml).
//  3. AWS_ROLE_ARN and AWS_WEB_IDENTITY_TOKEN_FILE tell the SDK to call STS
//     AssumeRoleWithWebIdentity using that token.
//  4. AWS returns temporary credentials for the trusted IAM role; the SDK
//     uses them for EC2 API calls.
//
// Operator setup (cluster/platform team, outside this package):
//
//  AWS:
//   - Register an IAM OIDC identity provider for the OpenShift service-account
//     issuer URL (the API server --service-account-issuer value).
//   - Create an IAM role with AssumeRoleWithWebIdentity trust, scoped to
//     subject system:serviceaccount:<controller-namespace>:controller-manager.
//   - Attach a least-privilege IAM policy for the EC2 actions this driver needs.
//
//  OpenShift:
//   - Deploy the controller with the controller-manager ServiceAccount.
//   - Enable the aws_web_identity_patch.yaml overlay and set AWS_ROLE_ARN for
//     the target environment.
//
// SDK credential resolution order (first match wins when AWS_PROFILE is unset):
//   a. Static env vars: AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY
//      [and optionally AWS_SESSION_TOKEN] — local development only.
//   b. Web identity: AWS_WEB_IDENTITY_TOKEN_FILE + AWS_ROLE_ARN
//      — production on OpenShift.
//   c. Container credentials URIs (AWS_CONTAINER_CREDENTIALS_*).
//   d. EC2 instance metadata — not used for this deployment model.
//
// When AWS_PROFILE is set, the shared config profile takes precedence over (a)
// and (b). Local development typically uses (a) or a profile; production on
// OpenShift uses (b). See AGENTS.md ("Local development") for setup details.
func newEC2Client(ctx context.Context, cfg internalconfig.AWSConfiguration) (*ec2.Client, error) {
	if err := validateAWSConfiguration(cfg); err != nil {
		return nil, err
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(cfg.Region),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	return ec2.NewFromConfig(awsCfg), nil
}

func validateAWSConfiguration(cfg internalconfig.AWSConfiguration) error {
	if cfg.Region == "" {
		return errors.Join(fmt.Errorf("missing required annotation %q", internalconfig.AnnotationRegion))
	}
	return nil
}
