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
	"fmt"
	"net"
	"strconv"
	"time"
)

const (
	// SSHPort is the default SSH port probed for host readiness.
	SSHPort = 22

	sshDialTimeout = 5 * time.Second
)

// SSHPortOpen checks whether host accepts TCP connections on SSHPort.
func SSHPortOpen(ctx context.Context, host string) error {
	dialer := net.Dialer{Timeout: sshDialTimeout}
	conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(host, strconv.Itoa(SSHPort)))
	if err != nil {
		return fmt.Errorf("ssh probe to %s:%d: %w", host, SSHPort, err)
	}
	_ = conn.Close()
	return nil
}
