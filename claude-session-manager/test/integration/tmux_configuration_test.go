package integration_test

import (
	"os/exec"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vbonnet/ai-tools/claude-session-manager/internal/tmux"
	"github.com/vbonnet/ai-tools/claude-session-manager/test/integration/helpers"
)

var _ = Describe("Tmux Configuration", func() {
	var sessionName string
	var workDir string

	BeforeEach(func() {
		sessionName = testEnv.UniqueSessionName("config")
		workDir = "/tmp"

		// Create test tmux session
		err := helpers.CreateTmuxSession(sessionName, workDir)
		Expect(err).ToNot(HaveOccurred())

		// Apply CSM tmux settings (simulating what csm new does)
		// These are the settings from internal/tmux/tmux.go NewSession()
		setTmuxOption(sessionName, "aggressive-resize", "on")
		setTmuxOption(sessionName, "window-size", "latest")
		setTmuxOption(sessionName, "mouse", "on")
		// set-clipboard is a server option (-s flag)
		setTmuxServerOption("set-clipboard", "on")
	})

	AfterEach(func() {
		if !CurrentSpecReport().Failed() {
			helpers.KillTmuxSession(sessionName)
		}
	})

	Describe("Session-level tmux options", func() {
		Context("when verifying aggressive-resize setting", func() {
			It("should be set to 'on'", func() {
				value, err := helpers.GetTmuxOption(sessionName, "aggressive-resize")
				Expect(err).ToNot(HaveOccurred())
				Expect(value).To(Equal("on"))
			})
		})

		Context("when verifying window-size setting", func() {
			It("should be set to 'latest'", func() {
				value, err := helpers.GetTmuxOption(sessionName, "window-size")
				Expect(err).ToNot(HaveOccurred())
				Expect(value).To(Equal("latest"))
			})
		})

		Context("when verifying mouse setting", func() {
			It("should be set to 'on'", func() {
				value, err := helpers.GetTmuxOption(sessionName, "mouse")
				Expect(err).ToNot(HaveOccurred())
				Expect(value).To(Equal("on"))
			})
		})
	})

	Describe("Server-level tmux options", func() {
		Context("when verifying set-clipboard setting", func() {
			It("should be set to 'on'", func() {
				// Server options are queried differently (no -t session)
				cmd := exec.Command("tmux", "show-options", "-s", "set-clipboard")
				output, err := cmd.Output()
				Expect(err).ToNot(HaveOccurred())

				// Output format: "set-clipboard on"
				Expect(string(output)).To(ContainSubstring("on"))
			})
		})
	})

	Describe("CSM-created session configuration", func() {
		Context("when using internal/tmux package", func() {
			It("should create session with correct settings via NewSession", func() {
				// Create a session using the actual CSM tmux package
				csmSessionName := testEnv.UniqueSessionName("csm-direct")
				defer helpers.KillTmuxSession(csmSessionName)

				err := tmux.NewSession(csmSessionName, workDir)
				Expect(err).ToNot(HaveOccurred())

				// Verify session exists
				exists, err := helpers.HasTmuxSession(csmSessionName)
				Expect(err).ToNot(HaveOccurred())
				Expect(exists).To(BeTrue())

				// Verify CSM applies the expected settings
				Eventually(func() string {
					val, _ := helpers.GetTmuxOption(csmSessionName, "aggressive-resize")
					return val
				}, "5s", "500ms").Should(Equal("on"))

				Eventually(func() string {
					val, _ := helpers.GetTmuxOption(csmSessionName, "window-size")
					return val
				}, "5s", "500ms").Should(Equal("latest"))

				Eventually(func() string {
					val, _ := helpers.GetTmuxOption(csmSessionName, "mouse")
					return val
				}, "5s", "500ms").Should(Equal("on"))
			})
		})
	})
})

// Helper function to set tmux option for a session
func setTmuxOption(sessionName, option, value string) error {
	cmd := exec.Command("tmux", "set-option", "-t", sessionName, option, value)
	return cmd.Run()
}

// Helper function to set tmux server option
func setTmuxServerOption(option, value string) error {
	cmd := exec.Command("tmux", "set-option", "-s", option, value)
	return cmd.Run()
}
