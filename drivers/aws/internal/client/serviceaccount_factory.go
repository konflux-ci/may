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

	"github.com/aws/aws-sdk-go-v2/service/ec2"

	internalconfig "github.com/konflux-ci/may/drivers/aws/internal/config"
)

var errServiceAccountAuthNotImplemented = errors.New("service account authentication is not implemented yet")

// serviceAccountFactory builds EC2 clients using the controller pod's
// ServiceAccount credentials (for example IRSA on EKS).
type serviceAccountFactory struct{}

func (f *serviceAccountFactory) NewEC2Client(_ context.Context, cfg internalconfig.AWSConfiguration) (*ec2.Client, error) {
	if cfg.Region == "" {
		return nil, fmt.Errorf("missing required annotation %q", internalconfig.AnnotationRegion)
	}
	return nil, errServiceAccountAuthNotImplemented
}
