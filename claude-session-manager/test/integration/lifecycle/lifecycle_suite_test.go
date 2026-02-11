package lifecycle_test

import (
	"os/exec"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vbonnet/ai-tools/claude-session-manager/test/integration/helpers"
)

func TestLifecycle(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "CSM Lifecycle Tests Suite")
}

var testEnv *helpers.TestEnv

var _ = BeforeSuite(func() {
	// Verify tmux is installed
	_, err := exec.LookPath("tmux")
	Expect(err).ToNot(HaveOccurred(), "tmux must be installed for lifecycle tests")

	// Verify agm command is available
	_, err = exec.LookPath("agm")
	Expect(err).ToNot(HaveOccurred(), "agm command must be available for lifecycle tests")

	// Setup test environment
	testEnv = helpers.NewTestEnv(nil)

	// Clean up any leftover test sessions from previous runs
	err = testEnv.Cleanup(nil)
	if err != nil {
		GinkgoWriter.Printf("Warning: failed to cleanup before suite: %v\n", err)
	}
})

var _ = AfterSuite(func() {
	// Final cleanup
	if testEnv != nil {
		err := testEnv.Cleanup(nil)
		if err != nil {
			GinkgoWriter.Printf("Warning: failed to cleanup after suite: %v\n", err)
		}
	}
})
