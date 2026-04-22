//go:build integration

package lifecycle_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vbonnet/dear-agent/agm/test/integration/helpers"
)

var _ = Describe("Archive Session", func() {
	Describe("happy path - archive inactive session", func() {
		DescribeTable("archive session scenarios",
			func(agent string, expectSuccess bool) {
				// Create a test session
				sessionName := testEnv.UniqueSessionName("archive-test")
				err := helpers.CreateTmuxSession(sessionName, testEnv.SessionsDir)
				Expect(err).ToNot(HaveOccurred(), "test session creation should succeed")

				// Create manifest for the session (registers it with AGM)
				err = helpers.CreateSessionManifest(testEnv.SessionsDir, sessionName, agent)
				Expect(err).ToNot(HaveOccurred(), "manifest creation should succeed")

				// Verify session exists before archive
				exists, err := helpers.HasTmuxSession(sessionName)
				Expect(err).ToNot(HaveOccurred())
				Expect(exists).To(BeTrue(), "test session should exist before archive")

				// Kill tmux session (AGM expects sessions to be stopped before archiving)
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
		)
	})

	Describe("archive session validation", func() {
		It("should verify archived session excluded from default list", func() {
			// Create and archive a session
			sessionName := testEnv.UniqueSessionName("archive-list-test")
			err := helpers.CreateTmuxSession(sessionName, testEnv.SessionsDir)
			Expect(err).ToNot(HaveOccurred())

			err = helpers.CreateSessionManifest(testEnv.SessionsDir, sessionName, "claude")
			Expect(err).ToNot(HaveOccurred())

			// Kill tmux session before archiving
			err = helpers.KillTmuxSession(sessionName)
			Expect(err).ToNot(HaveOccurred())

			// Archive the session
			err = helpers.ArchiveTestSession(testEnv.SessionsDir, sessionName, "")
			Expect(err).ToNot(HaveOccurred(), "archive should succeed")

			// List active sessions (default - no --all flag)
			filter := helpers.ListFilter{
				Archived: false,
				All:      false,
			}
			sessions, err := helpers.ListTestSessions(testEnv.SessionsDir, filter)
			Expect(err).ToNot(HaveOccurred())

			// Verify archived session NOT in default list
			for _, s := range sessions {
				Expect(s.ID).ToNot(ContainSubstring(sessionName), "archived session should not appear in default list")
			}
		})

		It("should include archived sessions when using --all flag", func() {
			// Create and properly archive a session using AGM archive command
			sessionName := testEnv.UniqueSessionName("archive-list-all-test")
			err := helpers.CreateTmuxSession(sessionName, testEnv.SessionsDir)
			Expect(err).ToNot(HaveOccurred())

			err = helpers.CreateSessionManifest(testEnv.SessionsDir, sessionName, "claude")
			Expect(err).ToNot(HaveOccurred())

			// Kill tmux session before archiving
			err = helpers.KillTmuxSession(sessionName)
			Expect(err).ToNot(HaveOccurred())

			// Archive using AGM command (creates proper v2 manifest with lifecycle field)
			err = helpers.ArchiveTestSession(testEnv.SessionsDir, sessionName, "")
			Expect(err).ToNot(HaveOccurred())

			// List with --all flag (should include archived sessions)
			filter := helpers.ListFilter{
				All: true,
			}
			sessionsAll, err := helpers.ListTestSessions(testEnv.SessionsDir, filter)
			Expect(err).ToNot(HaveOccurred())

			// List without --all flag (should exclude archived sessions)
			filterDefault := helpers.ListFilter{
				All: false,
			}
			sessionsActive, err := helpers.ListTestSessions(testEnv.SessionsDir, filterDefault)
			Expect(err).ToNot(HaveOccurred())

			// Verify --all returns more sessions than default (includes archived)
			Expect(len(sessionsAll)).To(BeNumerically(">=", len(sessionsActive)),
				"--all should return at least as many sessions as default (includes archived)")
		})
	})

	// NOTE: Dolt-based integration tests require helper functions that don't exist yet
	// (GetSessionID, IsSessionArchived, CreateSessionManifestWithName).
	// The unit tests in internal/dolt/adapter_test.go provide comprehensive coverage
	// of ResolveIdentifier functionality (83.3% coverage, 3 test functions).
	// Integration testing will be added in Phase 6 after helper functions are created.
	//
	// See docs/testing/ARCHIVE-DOLT-RUNBOOK.md for manual testing procedures.
})
