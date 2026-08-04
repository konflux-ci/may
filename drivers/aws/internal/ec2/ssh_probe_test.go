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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("SSHPortOpen", func() {
	DescribeTable("Testing the ability to access a remote AWS ec2 instance via SSH",
		func(testInstanceIP string) {
			err := SSHPortOpen(context.Background(), testInstanceIP)
			Expect(err).Should(MatchError(And(
				ContainSubstring("ssh probe to"),
				ContainSubstring(":22"),
			)))
		},
		Entry("Negative test - no such IP address", "192.168.4.231"),
		Entry("Negative test - not an IP address", "Not an IP address"),
	)
})
