//go:build integration

package lifecycle_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vbonnet/dear-agent/agm/test/integration/helpers"
)

var _ = Describe("List Sessions", func() {
	Describe("list active sessions", func() {
		DescribeTable("list scenarios",
			func(agent string, createCount int, expectMinCount int) {
				// Create test sessions
				var sessionNames []string
				for i := 0; i < createCount; i++ {
					sessionName := testEnv.UniqueSessionName("list-test")
					err := helpers.CreateTmuxSession(sessionName, testEnv.SessionsDir)
					Expect(err).ToNot(HaveOccurred())

					// Register session with AGM (create manifest)
					err = helpers.CreateSessionManifest(testEnv.SessionsDir, sessionName, agent)
					Expect(err).ToNot(HaveOccurred())

					sessionNames = append(sessionNames, sessionName)
				}
				defer func() {
					for _, name := range sessionNames {
						helpers.KillTmuxSession(name)
					}
				}()

				filter := helpers.ListFilter{
					Archived: false,
					All:      false,
				}
				sessions, err := helpers.ListTestSessions(testEnv.SessionsDir, filter)
				Expect(err).ToNot(HaveOccurred(), "list should succeed")

				// Verify at least the expected count (may include other test sessions)
				Expect(len(sessions)).To(BeNumerically(">=", expectMinCount),
					"should list at least %d sessions", expectMinCount)
			},
			Entry("claude agent - 3 sessions", "claude", 3, 3),
		)
	})

	Describe("list archived sessions only (--archived flag)", func() {
		It("is not supported - use --all instead", func() {
			Skip("AGM does not have a --archived flag (only --all exists). " +
				"The --all flag shows both active and archived sessions. " +
				"To list archived sessions only, use 'agm session list --all' and filter by status. " +
				"See list.go:94 for flag definition.")
		})
	})

	Describe("list all sessions", func() {
		It("should list both active and archived sessions with --all flag", func() {
			// Create one active session with manifest
			activeSessionName := testEnv.UniqueSessionName("list-all-active")
			err := helpers.CreateTmuxSession(activeSessionName, testEnv.SessionsDir)
			Expect(err).ToNot(HaveOccurred())
			defer helpers.KillTmuxSession(activeSessionName)

			// Create manifest for active session
			err = helpers.CreateSessionManifest(testEnv.SessionsDir, activeSessionName, "claude")
			Expect(err).ToNot(HaveOccurred())

			// Create one archived session
			archivedSessionID := "test-archived-all-001"
			err = helpers.CreateArchivedSession(testEnv, archivedSessionID, "claude")
			Expect(err).ToNot(HaveOccurred())
			defer helpers.CleanupArchivedSession(testEnv, archivedSessionID)

			// List all sessions (including archived)
			filter := helpers.ListFilter{
				All: true,
			}
			sessions, err := helpers.ListTestSessions(testEnv.SessionsDir, filter)
			Expect(err).ToNot(HaveOccurred(), "agm session list --all should succeed")

			// Verify both active and archived in list
			Expect(len(sessions)).To(BeNumerically(">=", 2), "should list at least 2 sessions (active + archived)")
		})
	})
})
