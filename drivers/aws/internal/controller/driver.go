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

package controller

import (
	"context"
	"time"

	internalconfig "github.com/konflux-ci/may/drivers/aws/internal/config"
	internalec2 "github.com/konflux-ci/may/drivers/aws/internal/ec2"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// AWSDriverFinalizer is added to hosts managed by this driver.
	AWSDriverFinalizer = "drivers.may.konflux-ci.dev/aws"
	// DriverLabel selects hosts this driver should reconcile.
	DriverLabel = "may.konflux-ci.dev/driver"
	// DriverLabelValueAWS is the DriverLabel value for AWS-managed hosts.
	DriverLabelValueAWS = "aws"
)

const (
	instancePollInterval   = 15 * time.Second
	instanceHealthInterval = 30 * time.Minute
)

// hostEC2Client is the EC2 lifecycle surface used by host reconcilers.
type hostEC2Client interface {
	LaunchInstance(ctx context.Context, cfg internalconfig.AWSConfiguration, clientToken string) (string, error)
	DescribeInstance(ctx context.Context, instanceID string, strictPublicAddress bool) (internalec2.InstanceDetails, error)
	SSHReady(ctx context.Context, instanceID string, strictPublicAddress bool) (string, bool, error)
	TerminateInstance(ctx context.Context, instanceID string) error
}

func isAWSDriverHost(object client.Object) bool {
	return labels.
		SelectorFromSet(labels.Set{DriverLabel: DriverLabelValueAWS}).
		Matches(labels.Set(object.GetLabels()))
}
