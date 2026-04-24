//go:build integration

package integration_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vbonnet/dear-agent/agm/internal/daemon"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/messages"
	"github.com/vbonnet/dear-agent/agm/test/integration/helpers"
)

var _ = Describe("AGM Send Interrupt Control", func() {
	var sessionName string
	var workDir string
	var agmBinary string
	var homeDir string
	var pidFile string

	BeforeEach(func() {
		sessionName = testEnv.UniqueSessionName("send-interrupt")
		workDir = "/tmp"

		// Get home directory for daemon PID file
		var err error
		homeDir, err = os.UserHomeDir()
		Expect(err).ToNot(HaveOccurred())
		pidFile = filepath.Join(homeDir, ".agm", "daemon.pid")

		// Build AGM binary for testing
		agmBinary = filepath.Join(GinkgoT().TempDir(), "agm")
		buildCmd := exec.Command("go", "build", "-o", agmBinary, "./cmd/agm")
		buildCmd.Dir = filepath.Join(os.Getenv("HOME"), "src/ws/oss/repos/ai-tools/main/agm")
		output, err := buildCmd.CombinedOutput()
		if err != nil {
			GinkgoWriter.Printf("Build output: %s\n", output)
			Fail(fmt.Sprintf("Failed to build AGM binary: %v", err))
		}
	})

	AfterEach(func() {
		// Preserve session on test failure for debugging
		if CurrentSpecReport().Failed() {
			GinkgoWriter.Printf("Test failed, preserving session: %s\n", sessionName)
			GinkgoWriter.Printf("Attach with: tmux attach -t %s\n", sessionName)
			return
		}

		// Clean up on success
		helpers.KillTmuxSession(sessionName)
		os.RemoveAll(testEnv.ManifestDir(sessionName))
	})

	Describe("Flag Verification", func() {
		It("should show --interrupt flag in help text", func() {
			cmd := exec.Command(agmBinary, "session", "send", "--help")
			output, err := cmd.CombinedOutput()
			Expect(err).ToNot(HaveOccurred())

			helpText := string(output)
			Expect(helpText).To(ContainSubstring("--interrupt"))
			Expect(helpText).To(ContainSubstring("Interrupt session immediately"))
			Expect(helpText).To(ContainSubstring("default: queue for later"))
		})
	})

	Describe("Non-Interrupt Mode (Default)", func() {
		var manifestPath string

		BeforeEach(func() {
			// Create tmux session
			err := helpers.CreateTmuxSession(sessionName, workDir)
			Expect(err).ToNot(HaveOccurred())

			// Create manifest with READY state
			manifestDir := testEnv.ManifestDir(sessionName)
			err = os.MkdirAll(manifestDir, 0700)
			Expect(err).ToNot(HaveOccurred())

			manifestPath = testEnv.ManifestPath(sessionName)
			m := &manifest.Manifest{
				SchemaVersion: manifest.SchemaVersion,
				SessionID:     "test-uuid-" + sessionName,
				Name:          sessionName,
				Context: manifest.Context{
					Project: workDir,
				},
				Tmux: manifest.Tmux{
					SessionName: sessionName,
				},
				Agent:          "claude",
				State:          manifest.StateDone,
				StateUpdatedAt: time.Now(),
			}

			err = manifest.Write(manifestPath, m)
			Expect(err).ToNot(HaveOccurred())
		})

		Context("when session is READY", func() {
			It("should queue message by default (non-interrupt)", func() {
				// Send message without --interrupt flag
				cmd := exec.Command(agmBinary, "session", "send", sessionName,
					"--sender", "test-sender",
					"--prompt", "Test message (should queue)")
				output, err := cmd.CombinedOutput()

				// Command should succeed
				Expect(err).ToNot(HaveOccurred())

				// Output should indicate queueing
				outputStr := string(output)
				Expect(outputStr).To(ContainSubstring("⏳ Queued to"))
				Expect(outputStr).To(ContainSubstring(sessionName))
			})
		})

		Context("when session is THINKING", func() {
			BeforeEach(func() {
				// Update manifest to THINKING state
				m, err := manifest.Read(manifestPath)
				Expect(err).ToNot(HaveOccurred())
				m.State = manifest.StateWorking
				m.StateUpdatedAt = time.Now()
				err = manifest.Write(manifestPath, m)
				Expect(err).ToNot(HaveOccurred())
			})

			It("should queue message", func() {
				cmd := exec.Command(agmBinary, "session", "send", sessionName,
					"--sender", "test-sender",
					"--prompt", "Test message during THINKING")
				output, err := cmd.CombinedOutput()

				Expect(err).ToNot(HaveOccurred())
				outputStr := string(output)
				Expect(outputStr).To(ContainSubstring("⏳ Queued to"))
				Expect(outputStr).To(ContainSubstring("WORKING"))
			})
		})

		Context("when daemon is offline", func() {
			BeforeEach(func() {
				// Ensure daemon is not running
				if daemon.IsRunning(pidFile) {
					stopCmd := exec.Command(agmBinary, "session", "daemon", "stop")
					stopCmd.Run()
					time.Sleep(500 * time.Millisecond)
				}
			})

			It("should queue message with warning", func() {
				cmd := exec.Command(agmBinary, "session", "send", sessionName,
					"--sender", "test-sender",
					"--prompt", "Message with daemon offline")
				output, err := cmd.CombinedOutput()

				Expect(err).ToNot(HaveOccurred())
				outputStr := string(output)
				Expect(outputStr).To(ContainSubstring("⏳ Queued to"))
				Expect(outputStr).To(ContainSubstring("⚠️"))
				Expect(outputStr).To(ContainSubstring("daemon is NOT running"))
				Expect(outputStr).To(ContainSubstring("agm session daemon start"))
			})
		})
	})

	Describe("Interrupt Mode (Explicit)", func() {
		var manifestPath string

		BeforeEach(func() {
			// Create tmux session
			err := helpers.CreateTmuxSession(sessionName, workDir)
			Expect(err).ToNot(HaveOccurred())

			// Create manifest with READY state
			manifestDir := testEnv.ManifestDir(sessionName)
			err = os.MkdirAll(manifestDir, 0700)
			Expect(err).ToNot(HaveOccurred())

			manifestPath = testEnv.ManifestPath(sessionName)
			m := &manifest.Manifest{
				SchemaVersion: manifest.SchemaVersion,
				SessionID:     "test-uuid-" + sessionName,
				Name:          sessionName,
				Context: manifest.Context{
					Project: workDir,
				},
				Tmux: manifest.Tmux{
					SessionName: sessionName,
				},
				Agent:          "claude",
				State:          manifest.StateDone,
				StateUpdatedAt: time.Now(),
			}

			err = manifest.Write(manifestPath, m)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should send message immediately with --interrupt flag", func() {
			cmd := exec.Command(agmBinary, "session", "send", sessionName,
				"--interrupt",
				"--sender", "test-sender",
				"--prompt", "Urgent message")
			output, err := cmd.CombinedOutput()

			Expect(err).ToNot(HaveOccurred())
			outputStr := string(output)
			Expect(outputStr).To(ContainSubstring("✓ Sent to"))
			Expect(outputStr).To(ContainSubstring(sessionName))
			Expect(outputStr).To(ContainSubstring("via: tmux"))
			Expect(outputStr).ToNot(ContainSubstring("⏳ Queued"))
		})
	})

	Describe("Compaction Protection", func() {
		var manifestPath string

		BeforeEach(func() {
			// Create tmux session
			err := helpers.CreateTmuxSession(sessionName, workDir)
			Expect(err).ToNot(HaveOccurred())

			// Create manifest with COMPACTING state
			manifestDir := testEnv.ManifestDir(sessionName)
			err = os.MkdirAll(manifestDir, 0700)
			Expect(err).ToNot(HaveOccurred())

			manifestPath = testEnv.ManifestPath(sessionName)
			m := &manifest.Manifest{
				SchemaVersion: manifest.SchemaVersion,
				SessionID:     "test-uuid-" + sessionName,
				Name:          sessionName,
				Context: manifest.Context{
					Project: workDir,
				},
				Tmux: manifest.Tmux{
					SessionName: sessionName,
				},
				Agent:          "claude",
				State:          manifest.StateCompacting,
				StateUpdatedAt: time.Now(),
			}

			err = manifest.Write(manifestPath, m)
			Expect(err).ToNot(HaveOccurred())
		})

		Context("without --interrupt flag", func() {
			It("should reject message with compaction error", func() {
				cmd := exec.Command(agmBinary, "session", "send", sessionName,
					"--sender", "test-sender",
					"--prompt", "Message during compaction")
				output, err := cmd.CombinedOutput()

				// Command should fail
				Expect(err).To(HaveOccurred())

				outputStr := string(output)
				Expect(outputStr).To(ContainSubstring("❌ Cannot send"))
				Expect(outputStr).To(ContainSubstring("COMPACTING"))
				Expect(outputStr).To(ContainSubstring("30-60 seconds"))
			})
		})

		Context("with --interrupt flag", func() {
			It("should still reject message (compaction protection overrides interrupt)", func() {
				cmd := exec.Command(agmBinary, "session", "send", sessionName,
					"--interrupt",
					"--sender", "test-sender",
					"--prompt", "Urgent during compaction")
				output, err := cmd.CombinedOutput()

				// Command should fail
				Expect(err).To(HaveOccurred())

				outputStr := string(output)
				Expect(outputStr).To(ContainSubstring("❌ Cannot send"))
				Expect(outputStr).To(ContainSubstring("COMPACTING"))
			})
		})
	})

	Describe("Message Queue Integration", func() {
		var queue *messages.MessageQueue
		var manifestPath string

		BeforeEach(func() {
			// Create tmux session
			err := helpers.CreateTmuxSession(sessionName, workDir)
			Expect(err).ToNot(HaveOccurred())

			// Create manifest with READY state
			manifestDir := testEnv.ManifestDir(sessionName)
			err = os.MkdirAll(manifestDir, 0700)
			Expect(err).ToNot(HaveOccurred())

			manifestPath = testEnv.ManifestPath(sessionName)
			m := &manifest.Manifest{
				SchemaVersion: manifest.SchemaVersion,
				SessionID:     "test-uuid-" + sessionName,
				Name:          sessionName,
				Context: manifest.Context{
					Project: workDir,
				},
				Tmux: manifest.Tmux{
					SessionName: sessionName,
				},
				Agent:          "claude",
				State:          manifest.StateDone,
				StateUpdatedAt: time.Now(),
			}

			err = manifest.Write(manifestPath, m)
			Expect(err).ToNot(HaveOccurred())

			// Initialize message queue
			queue, err = messages.NewMessageQueue()
			Expect(err).ToNot(HaveOccurred())
		})

		AfterEach(func() {
			if queue != nil {
				queue.Close()
			}
		})

		It("should create queue entry when sending without --interrupt", func() {
			// Send message (should queue)
			cmd := exec.Command(agmBinary, "session", "send", sessionName,
				"--sender", "test-sender",
				"--prompt", "Queued test message")
			_, err := cmd.CombinedOutput()
			Expect(err).ToNot(HaveOccurred())

			// Verify queue entry exists
			entries, err := queue.GetAllPending()
			Expect(err).ToNot(HaveOccurred())
			Expect(len(entries)).To(BeNumerically(">", 0))

			// Find our entry
			found := false
			for _, entry := range entries {
				if entry.To == sessionName && entry.From == "test-sender" {
					found = true
					Expect(entry.Message).To(ContainSubstring("Queued test message"))
					break
				}
			}
			Expect(found).To(BeTrue(), "Queue entry not found")
		})

		It("should NOT create queue entry when using --interrupt", func() {
			// Get initial queue size
			initialEntries, err := queue.GetAllPending()
			Expect(err).ToNot(HaveOccurred())
			initialCount := len(initialEntries)

			// Send message with --interrupt (should send directly)
			cmd := exec.Command(agmBinary, "session", "send", sessionName,
				"--interrupt",
				"--sender", "test-sender",
				"--prompt", "Interrupt test message")
			_, err = cmd.CombinedOutput()
			Expect(err).ToNot(HaveOccurred())

			// Verify queue did not grow
			finalEntries, err := queue.GetAllPending()
			Expect(err).ToNot(HaveOccurred())
			Expect(len(finalEntries)).To(Equal(initialCount), "Queue should not have new entries for --interrupt sends")
		})
	})

	Describe("Backward Compatibility", func() {
		var manifestPath string

		BeforeEach(func() {
			// Create tmux session
			err := helpers.CreateTmuxSession(sessionName, workDir)
			Expect(err).ToNot(HaveOccurred())

			// Create manifest
			manifestDir := testEnv.ManifestDir(sessionName)
			err = os.MkdirAll(manifestDir, 0700)
			Expect(err).ToNot(HaveOccurred())

			manifestPath = testEnv.ManifestPath(sessionName)
			m := &manifest.Manifest{
				SchemaVersion: manifest.SchemaVersion,
				SessionID:     "test-uuid-" + sessionName,
				Name:          sessionName,
				Context: manifest.Context{
					Project: workDir,
				},
				Tmux: manifest.Tmux{
					SessionName: sessionName,
				},
				Agent:          "claude",
				State:          manifest.StateDone,
				StateUpdatedAt: time.Now(),
			}

			err = manifest.Write(manifestPath, m)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should work without --interrupt flag (legacy behavior)", func() {
			// Send message without --interrupt (old command format)
			cmd := exec.Command(agmBinary, "session", "send", sessionName,
				"--sender", "legacy-script",
				"--prompt", "Legacy message")
			output, err := cmd.CombinedOutput()

			// Command should succeed
			Expect(err).ToNot(HaveOccurred())

			// New default behavior: queue instead of interrupt
			outputStr := string(output)
			Expect(outputStr).To(ContainSubstring("⏳ Queued to"))
		})
	})
})
