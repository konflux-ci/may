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

// --- Option 1: Static exclusion list ---
var _ = Describe("IsLocalFlavor", func() {
	DescribeTable("correctly identifies local flavors",
		func(flavor string, expected bool) {
			Expect(IsLocalFlavor(flavor)).To(Equal(expected))
		},
		Entry("localhost is local", "localhost", true),
		Entry("local is local", "local", true),
		Entry("aws-linux-amd64 is not local", "aws-linux-amd64", false),
		Entry("aws-linux-arm64 is not local", "aws-linux-arm64", false),
		Entry("empty string is not local", "", false),
	)
})
