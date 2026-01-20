package lifecycle_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vbonnet/ai-tools/claude-session-manager/test/integration/helpers"
)

var _ = Describe("Resume Session", func() {
	DescribeTable("resume session scenarios",
		func(agent string, expectSuccess bool) {
			// Create a test session
			sessionName := testEnv.UniqueSessionName("resume-test")
			err := helpers.CreateTmuxSession(sessionName, testEnv.SessionsDir)
			Expect(err).ToNot(HaveOccurred(), "test session creation should succeed")
			defer helpers.KillTmuxSession(sessionName)

			// Create manifest for the session (registers it with CSM)
			err = helpers.CreateSessionManifest(testEnv.SessionsDir, sessionName, agent)
			Expect(err).ToNot(HaveOccurred(), "manifest creation should succeed")

			// Detach from session (simulating a session that needs resume)
			// In real CSM, this would be a detached session
			exists, err := helpers.HasTmuxSession(sessionName)
			Expect(err).ToNot(HaveOccurred())
			Expect(exists).To(BeTrue(), "test session should exist before resume")

			// Attempt to resume the session
			err = helpers.ResumeTestSession(testEnv.SessionsDir, sessionName)

			if expectSuccess {
				Expect(err).ToNot(HaveOccurred(), "resume should succeed for valid session")

				// Verify session still exists after resume
				exists, err := helpers.HasTmuxSession(sessionName)
				Expect(err).ToNot(HaveOccurred())
				Expect(exists).To(BeTrue(), "session should exist after successful resume")
			} else {
				Expect(err).To(HaveOccurred(), "resume should fail for invalid session")
			}
		},
		Entry("claude agent (happy path)", "claude", true),
		// Phase 3 will add: Entry("gemini agent (happy path)", "gemini", true),
	)
})
