//go:build integration

// DISABLED: Integration tests require real API keys (not dummy keys) to work.
// Tests time out waiting for Claude/agent processes that never start with test keys.
// E2E tests (test/e2e/) provide adequate coverage using mock agents.
// To run these tests: go test -tags=integration ./test/integration/...

package integration_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// TestAgentParity runs the agent parity test suite
func TestAgentParity(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Agent Feature Parity Suite")
}
