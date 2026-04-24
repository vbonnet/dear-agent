//go:build integration

package integration_test

import (
	"os/exec"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vbonnet/dear-agent/agm/internal/tmux"
	"github.com/vbonnet/dear-agent/agm/test/integration/helpers"
)

var _ = Describe("Tmux Configuration", func() {
	var sessionName string
	var workDir string

	BeforeEach(func() {
		// Clean up any stale tmux lock from previous tests
		tmux.ReleaseTmuxLock()

		sessionName = testEnv.UniqueSessionName("config")
		workDir = "/tmp"

		// Create test tmux session
		err := helpers.CreateTmuxSession(sessionName, workDir)
		Expect(err).ToNot(HaveOccurred())

		// Apply AGM tmux settings (simulating what agm session new does)
		// These are the settings from internal/tmux/tmux.go NewSession()
		setTmuxOption(sessionName, "aggressive-resize", "on")
		setTmuxOption(sessionName, "window-size", "latest")
		setTmuxOption(sessionName, "mouse", "on")
		// Server options (-s flag)
		setTmuxServerOption("set-clipboard", "on")
		setTmuxServerOption("escape-time", "10")
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

		Context("when verifying escape-time setting", func() {
			It("should be set to 10ms to avoid copy-mode lag", func() {
				cmd := exec.Command("tmux", "show-options", "-s", "escape-time")
				output, err := cmd.Output()
				Expect(err).ToNot(HaveOccurred())

				// Output format: "escape-time 10"
				Expect(string(output)).To(ContainSubstring("10"))
			})
		})
	})

	Describe("AGM-created session configuration", func() {
		Context("when using internal/tmux package", func() {
			It("should create session with correct settings via NewSession", func() {
				// Create a session using the actual AGM tmux package
				agmSessionName := testEnv.UniqueSessionName("agm-direct")
				defer func() {
					// Use AGM socket for cleanup
					exec.Command("tmux", "-S", tmux.GetSocketPath(), "kill-session", "-t", agmSessionName).Run()
				}()

				err := tmux.NewSession(agmSessionName, workDir)
				Expect(err).ToNot(HaveOccurred())

				// Verify session exists (using AGM's HasSession which uses the isolated socket)
				exists, err := tmux.HasSession(agmSessionName)
				Expect(err).ToNot(HaveOccurred())
				Expect(exists).To(BeTrue())

				// Verify AGM applies the expected settings (using AGM socket)
				socketPath := tmux.GetSocketPath()

				Eventually(func() string {
					cmd := exec.Command("tmux", "-S", socketPath, "show-options", "-t", agmSessionName, "aggressive-resize")
					output, err := cmd.Output()
					if err != nil {
						return ""
					}
					parts := strings.SplitN(strings.TrimSpace(string(output)), " ", 2)
					if len(parts) < 2 {
						return ""
					}
					return strings.TrimSpace(parts[1])
				}, "5s", "500ms").Should(Equal("on"))

				Eventually(func() string {
					cmd := exec.Command("tmux", "-S", socketPath, "show-options", "-t", agmSessionName, "window-size")
					output, err := cmd.Output()
					if err != nil {
						return ""
					}
					parts := strings.SplitN(strings.TrimSpace(string(output)), " ", 2)
					if len(parts) < 2 {
						return ""
					}
					return strings.TrimSpace(parts[1])
				}, "5s", "500ms").Should(Equal("latest"))

				Eventually(func() string {
					cmd := exec.Command("tmux", "-S", socketPath, "show-options", "-t", agmSessionName, "mouse")
					output, err := cmd.Output()
					if err != nil {
						return ""
					}
					parts := strings.SplitN(strings.TrimSpace(string(output)), " ", 2)
					if len(parts) < 2 {
						return ""
					}
					return strings.TrimSpace(parts[1])
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
