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
	"context"
	"errors"
	"fmt"
	"net"
	"time"
)

// SSHProbeError is returned when a TCP probe to sshPort fails.
type SSHProbeError struct {
	Addr string
	Err  error
}

func (e *SSHProbeError) Error() string {
	return fmt.Sprintf("ssh probe to %s: %s", e.Addr, e.Err.Error())
}

func (e *SSHProbeError) Unwrap() error {
	return e.Err
}

// IsSSHProbeError reports whether err is or wraps an SSHProbeError.
func IsSSHProbeError(err error) bool {
	var probeErr *SSHProbeError
	return errors.As(err, &probeErr)
}

const (
	// sshPort is the default SSH port probed for host readiness.
	sshPort = "22"

	sshDialTimeout = 5 * time.Second

	tcpProtocol = "tcp"
)

// SSHPortOpen checks whether host accepts TCP connections on sshPort.
func SSHPortOpen(ctx context.Context, host string) error {
	dialer := net.Dialer{Timeout: sshDialTimeout}
	addr := net.JoinHostPort(host, sshPort)
	conn, err := dialer.DialContext(ctx, tcpProtocol, addr)
	if err != nil {
		return &SSHProbeError{Addr: addr, Err: err}
	}
	_ = conn.Close()
	return nil
}
