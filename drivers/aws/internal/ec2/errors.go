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

package ec2

import (
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

// InstanceNotFoundError is returned when DescribeInstances cannot find the instance,
// including after AWS has purged a terminated record.
type InstanceNotFoundError struct {
	InstanceID string
	Err        error
}

func (e *InstanceNotFoundError) Error() string {
	if e == nil {
		return "EC2 instance not found"
	}
	return fmt.Sprintf("DescribeInstances: instance %q not found", e.InstanceID)
}

func (e *InstanceNotFoundError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// IsInstanceNotFoundError reports whether err is or wraps an InstanceNotFoundError.
func IsInstanceNotFoundError(err error) bool {
	_, ok := errors.AsType[*InstanceNotFoundError](err)
	return ok
}

// InstanceNotRunningError is returned when an instance will not become SSH-ready
// (stopped, terminated, and similar).
type InstanceNotRunningError struct {
	InstanceID string
	State      types.InstanceStateName
}

func (e *InstanceNotRunningError) Error() string {
	if e == nil {
		return "EC2 instance is not running"
	}
	state := instanceStateName(e.State)
	switch e.State {
	case types.InstanceStateNameShuttingDown, types.InstanceStateNameTerminated:
		return fmt.Sprintf("EC2 instance %s is %s", e.InstanceID, state)
	default:
		return fmt.Sprintf("EC2 instance %s is %s and is not running", e.InstanceID, state)
	}
}

func instanceStateName(state types.InstanceStateName) string {
	if state == "" {
		return "unknown"
	}
	return string(state)
}

// IsInstanceNotRunningError reports whether err is or wraps an InstanceNotRunningError.
func IsInstanceNotRunningError(err error) bool {
	_, ok := errors.AsType[*InstanceNotRunningError](err)
	return ok
}

// SSHProbeError is returned when a TCP probe to sshPort fails.
type SSHProbeError struct {
	Addr string
	Err  error
}

func (e *SSHProbeError) Error() string {
	if e == nil {
		return "ssh probe error"
	}
	if e.Err == nil {
		return fmt.Sprintf("ssh probe to %s", e.Addr)
	}
	return fmt.Sprintf("ssh probe to %s: %s", e.Addr, e.Err.Error())
}

func (e *SSHProbeError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// IsSSHProbeError reports whether err is or wraps an SSHProbeError.
func IsSSHProbeError(err error) bool {
	_, ok := errors.AsType[*SSHProbeError](err)
	return ok
}
