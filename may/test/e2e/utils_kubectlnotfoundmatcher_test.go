package e2e

import (
	"fmt"
	"strings"

	"github.com/onsi/gomega/matchers"
)

// kubectlNotFoundMatcher implements types.GomegaMatcher for kubectl "resource not found" errors.
// Match succeeds when actual is a non-nil error whose message contains "NotFound" or "not found".
type kubectlNotFoundMatcher struct {
	matchers.HaveOccurredMatcher
}

// BeKubectlNotFound returns a Gomega matcher that succeeds when the actual value is an error
// from kubectl get indicating the resource was not found (e.g. "Error from server (NotFound): ..."
// or "... not found"). Used by get*OrNotFound helpers to assert the error is NotFound, not API/network failure.
func BeKubectlNotFound() *kubectlNotFoundMatcher {
	return &kubectlNotFoundMatcher{}
}

func (m *kubectlNotFoundMatcher) Match(actual any) (success bool, err error) {
	// must be an non nil 'error' type
	if success, err := m.HaveOccurredMatcher.Match(actual); err != nil || !success {
		return success, err
	}

	msg := actual.(error).Error()
	return strings.Contains(msg, `failed with error "Error from server (NotFound): `), nil
}

func (m *kubectlNotFoundMatcher) FailureMessage(actual any) string {
	return fmt.Sprintf("Expected a kubectl NotFound error, got: %v", actual)
}

func (m *kubectlNotFoundMatcher) NegatedFailureMessage(actual any) string {
	return fmt.Sprintf("Expected not to get a kubectl NotFound error, got: %v", actual)
}
