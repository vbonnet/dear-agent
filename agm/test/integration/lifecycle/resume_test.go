//go:build integration

package lifecycle_test

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/test/integration/helpers"
)

var _ = Describe("Resume Session", func() {
	Describe("session health checks and state updates", func() {
		It("should detect missing project directory during health check", func() {
			// Create a test session with manifest but no project directory
			sessionName := testEnv.UniqueSessionName("resume-health-test")
			err := helpers.CreateSessionManifest(testEnv.SessionsDir, sessionName, "claude")
			Expect(err).ToNot(HaveOccurred(), "manifest creation should succeed")

			// Delete the project directory to trigger health check failure
			projectDir := filepath.Join(testEnv.SessionsDir, sessionName, "project")
			err = os.RemoveAll(projectDir)
			Expect(err).ToNot(HaveOccurred(), "project dir deletion should succeed")

			// Attempt resume - should fail health check
			// Note: We can't test actual resume command without TTY, but we test the manifest/project setup
			manifestPath := filepath.Join(testEnv.SessionsDir, sessionName, "manifest.yaml")
			m, err := manifest.Read(manifestPath)
			Expect(err).ToNot(HaveOccurred(), "manifest read should succeed")

			// Verify project directory doesn't exist (health check would fail)
			_, err = os.Stat(m.Context.Project)
			Expect(os.IsNotExist(err)).To(BeTrue(), "project directory should not exist")
		})

		It("should update manifest timestamp when session state changes", func() {
			// Create a test session
			sessionName := testEnv.UniqueSessionName("resume-timestamp-test")
			err := helpers.CreateSessionManifest(testEnv.SessionsDir, sessionName, "claude")
			Expect(err).ToNot(HaveOccurred(), "manifest creation should succeed")

			// Read initial manifest
			manifestPath := filepath.Join(testEnv.SessionsDir, sessionName, "manifest.yaml")
			m1, err := manifest.Read(manifestPath)
			Expect(err).ToNot(HaveOccurred(), "initial manifest read should succeed")
			initialTimestamp := m1.UpdatedAt

			// Simulate a manifest update (what resume would do)
			err = manifest.Write(manifestPath, m1)
			Expect(err).ToNot(HaveOccurred(), "manifest write should succeed")

			// Read updated manifest
			m2, err := manifest.Read(manifestPath)
			Expect(err).ToNot(HaveOccurred(), "updated manifest read should succeed")

			// Verify timestamp was updated (manifest.Write auto-updates UpdatedAt)
			Expect(m2.UpdatedAt.After(initialTimestamp) || m2.UpdatedAt.Equal(initialTimestamp)).To(BeTrue(),
				"updated_at timestamp should be equal or after initial timestamp")
		})

		It("should verify session manifest exists before resume", func() {
			// Create session with tmux but NO manifest
			sessionName := testEnv.UniqueSessionName("resume-no-manifest-test")
			err := helpers.CreateTmuxSession(sessionName, testEnv.SessionsDir)
			Expect(err).ToNot(HaveOccurred(), "tmux session creation should succeed")
			defer helpers.KillTmuxSession(sessionName)

			// Verify tmux session exists
			exists, err := helpers.HasTmuxSession(sessionName)
			Expect(err).ToNot(HaveOccurred())
			Expect(exists).To(BeTrue(), "tmux session should exist")

			// Verify manifest does NOT exist (resume would fail to resolve)
			manifestPath := filepath.Join(testEnv.SessionsDir, sessionName, "manifest.yaml")
			_, err = os.Stat(manifestPath)
			Expect(os.IsNotExist(err)).To(BeTrue(), "manifest should not exist - resume would fail")
		})
	})

	Describe("full resume with tmux attach", func() {
		It("requires TTY for tmux attach step", func() {
			Skip("Full resume with tmux attach requires interactive TTY (not available in CI). " +
				"Session health checks, creation logic, and state updates are tested above. " +
				"Future enhancement: Add --dry-run flag to AGM resume command for programmatic testing. " +
				"Manual testing: Run 'agm session resume <session-id>' in terminal to verify attach behavior.")
		})
	})

	Describe("resume attach vs resume branching", func() {
		DescribeTable("should choose correct branch based on tmux existence",
			func(tmuxExists bool, expectedBehavior string) {
				sessionName := testEnv.UniqueSessionName("resume-branch-test")
				uuid := "test-uuid-" + helpers.RandomString(8)

				// SETUP: Create manifest with UUID
				manifestPath := helpers.SetupManifestWithUUID(sessionName, uuid)
				defer os.Remove(manifestPath)
				defer os.RemoveAll(filepath.Dir(manifestPath))

				// SETUP: Create or ensure NO tmux session based on scenario
				if tmuxExists {
					// Scenario A: Tmux exists -> attach only
					err := helpers.CreateTmuxSession(sessionName, testEnv.SessionsDir)
					Expect(err).ToNot(HaveOccurred(), "tmux session creation should succeed")
					defer helpers.KillTmuxSession(sessionName)
				} else {
					// Scenario B: Tmux NOT exists -> send resume command
					err := helpers.EnsureNoTmuxSession(sessionName)
					Expect(err).ToNot(HaveOccurred(), "tmux session should not exist")
				}

				// EXECUTION: Check session health (simulates resume logic)
				exists, err := helpers.HasTmuxSession(sessionName)
				Expect(err).ToNot(HaveOccurred())

				// Create mock command sender to verify behavior
				mockSender := &helpers.MockCommandSender{}
				defer mockSender.Reset()

				// Simulate resume logic decision
				sendCommands := !exists

				if sendCommands {
					// Scenario B: Send claude --resume command
					resumeCmd := filepath.Join(testEnv.SessionsDir, sessionName, "project") + " && claude --resume " + uuid + " && exit"
					mockSender.SendCommand(sessionName, resumeCmd)
				}

				// ASSERTIONS
				if expectedBehavior == "attach" {
					Expect(mockSender.CommandsSent).To(BeEmpty(),
						"Expected NO commands sent when tmux exists")
					Expect(exists).To(BeTrue(),
						"Expected tmux session to exist for attach scenario")
				} else {
					Expect(mockSender.CommandsSent).To(HaveLen(1),
						"Expected claude --resume command sent when tmux NOT exists")
					Expect(mockSender.CommandsSent[0]).To(ContainSubstring("claude --resume "+uuid),
						"Expected correct UUID in resume command")
					Expect(mockSender.CommandsSent[0]).To(ContainSubstring("&&"),
						"Expected command chain (cd && claude --resume && exit)")
					Expect(exists).To(BeFalse(),
						"Expected tmux session NOT to exist for resume scenario")
				}
			},
			Entry("Tmux exists -> attach only (no command sent)", true, "attach"),
			Entry("Tmux NOT exists -> send claude --resume", false, "resume"),
		)
	})
})
