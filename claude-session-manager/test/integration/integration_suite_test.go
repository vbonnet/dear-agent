package integration_test

import (
	"os/exec"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vbonnet/ai-tools/claude-session-manager/test/integration/helpers"
)

func TestIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "CSM Integration Suite")
}

var testEnv *helpers.TestEnv

var _ = BeforeSuite(func() {
	// Verify tmux is installed
	_, err := exec.LookPath("tmux")
	Expect(err).ToNot(HaveOccurred(), "tmux must be installed for integration tests")

	// Setup test environment
	testEnv = helpers.NewTestEnv(GinkgoT())

	// Clean up any leftover test sessions from previous runs
	err = testEnv.Cleanup(GinkgoT())
	if err != nil {
		GinkgoWriter.Printf("Warning: failed to cleanup before suite: %v\n", err)
	}
})

var _ = AfterSuite(func() {
	// Final cleanup
	if testEnv != nil {
		err := testEnv.Cleanup(GinkgoT())
		if err != nil {
			GinkgoWriter.Printf("Warning: failed to cleanup after suite: %v\n", err)
		}
	}
})
