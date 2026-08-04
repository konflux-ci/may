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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("SSHPortOpen", func() {
	DescribeTable("returns a wrapped error when the SSH port is unreachable",
		func(host string) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
			defer cancel()

			err := SSHPortOpen(ctx, host)
			Expect(err).Should(MatchError(And(
				ContainSubstring("ssh probe to"),
				ContainSubstring(":22"),
			)))
		},
		Entry("invalid IP address", "999.999.999.999"),
		Entry("invalid hostname", "not-a-host"),
	)
})
