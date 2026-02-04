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
