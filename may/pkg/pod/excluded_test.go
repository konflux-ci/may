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

package pod

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestPod(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Pod Suite")
}

var _ = Describe("IsExcludedFlavor", func() {
	DescribeTable("correctly identifies excluded flavors",
		func(flavor string, expected bool) {
			Expect(IsExcludedFlavor(flavor)).To(Equal(expected))
		},
		Entry("localhost is excluded", "localhost", true),
		Entry("local is excluded", "local", true),
		Entry("aws-linux-amd64 is not excluded", "aws-linux-amd64", false),
		Entry("aws-linux-arm64 is not excluded", "aws-linux-arm64", false),
		Entry("empty string is not excluded", "", false),
	)
})
