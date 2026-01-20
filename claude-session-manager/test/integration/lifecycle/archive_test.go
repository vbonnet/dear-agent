package lifecycle_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vbonnet/ai-tools/claude-session-manager/test/integration/helpers"
)

var _ = Describe("Archive Session", func() {
	Describe("happy path - archive inactive session", func() {
		DescribeTable("archive session scenarios",
			func(agent string, expectSuccess bool) {
				// Create a test session
				sessionName := testEnv.UniqueSessionName("archive-test")
				err := helpers.CreateTmuxSession(sessionName, testEnv.SessionsDir)
				Expect(err).ToNot(HaveOccurred(), "test session creation should succeed")

				// Create manifest for the session (registers it with CSM)
				err = helpers.CreateSessionManifest(testEnv.SessionsDir, sessionName, agent)
				Expect(err).ToNot(HaveOccurred(), "manifest creation should succeed")

				// Verify session exists before archive
				exists, err := helpers.HasTmuxSession(sessionName)
				Expect(err).ToNot(HaveOccurred())
				Expect(exists).To(BeTrue(), "test session should exist before archive")

				// Kill tmux session (CSM expects sessions to be stopped before archiving)
				err = helpers.KillTmuxSession(sessionName)
				Expect(err).ToNot(HaveOccurred(), "tmux session kill should succeed")

				// Verify tmux session is gone
				exists, err = helpers.HasTmuxSession(sessionName)
				Expect(err).ToNot(HaveOccurred())
				Expect(exists).To(BeFalse(), "tmux session should be killed before archive")

				// Attempt to archive the session
				err = helpers.ArchiveTestSession(testEnv.SessionsDir, sessionName, "")

				if expectSuccess {
					Expect(err).ToNot(HaveOccurred(), "archive should succeed for valid session")
				} else {
					Expect(err).To(HaveOccurred(), "archive should fail for invalid session")
				}

				// Cleanup
				helpers.KillTmuxSession(sessionName) // Force cleanup if still exists
			},
			Entry("claude agent", "claude", true),
			// Phase 2 will add --reason flag test
			// Phase 3 will add: Entry("gemini agent", "gemini", true),
		)
	})
})
