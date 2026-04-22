package uuid_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/vbonnet/dear-agent/agm/internal/history"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/uuid"
)

func TestUUIDDiscovery(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "UUID Discovery BDD Suite")
}

var _ = Describe("UUID Discovery", func() {
	var (
		tmpHome     string
		claudeDir   string
		historyPath string
	)

	BeforeEach(func() {
		// Setup temp home directory
		tmpHome = GinkgoT().TempDir()
		os.Setenv("HOME", tmpHome)
		claudeDir = filepath.Join(tmpHome, ".claude")
		Expect(os.MkdirAll(claudeDir, 0755)).To(Succeed())
		historyPath = filepath.Join(claudeDir, "history.jsonl")
	})

	AfterEach(func() {
		os.Unsetenv("HOME")
	})

	Context("when searching by /rename command", func() {
		It("should find the most recent session with matching rename", func() {
			// Given a history with multiple renames for the same session name
			now := time.Now()
			entries := []history.ConversationEntry{
				{
					SessionID: "old-uuid-1111-1111-1111-111111111111",
					Display:   "/rename test-session",
					Timestamp: now.Add(-2 * time.Hour).UnixMilli(),
				},
				{
					SessionID: "new-uuid-2222-2222-2222-222222222222",
					Display:   "/rename test-session",
					Timestamp: now.Add(-1 * time.Hour).UnixMilli(),
				},
			}
			writeHistory(historyPath, entries)

			// When searching for the session by name
			foundUUID, err := uuid.SearchHistoryByRename("test-session")

			// Then it should return the most recent UUID
			Expect(err).NotTo(HaveOccurred())
			Expect(foundUUID).To(Equal("new-uuid-2222-2222-2222-222222222222"))
		})

		It("should fail gracefully when session name not found", func() {
			// Given a history without the session name
			entries := []history.ConversationEntry{
				{
					SessionID: "uuid-3333-3333-3333-333333333333",
					Display:   "/rename other-session",
					Timestamp: time.Now().UnixMilli(),
				},
			}
			writeHistory(historyPath, entries)

			// When searching for a non-existent session
			foundUUID, err := uuid.SearchHistoryByRename("missing-session")

			// Then it should return an error
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("no rename found"))
			Expect(foundUUID).To(BeEmpty())
		})
	})

	Context("when timestamp search is NOT used (bug fix validation)", func() {
		It("should NOT return wrong UUID based on timestamp proximity", func() {
			// Given a manifest with empty UUID and a session created nearby in time
			now := time.Now()
			manifestSearchFunc := func(name string) (*manifest.Manifest, error) {
				return &manifest.Manifest{
					Claude: manifest.Claude{
						UUID: "", // Empty UUID
					},
					UpdatedAt: now,
				}, nil
			}

			// And history with a different session active around that time
			entries := []history.ConversationEntry{
				{
					SessionID: "wrong-uuid-4444-4444-4444-444444444444",
					Display:   "/rename other-session",
					Timestamp: now.Add(-5 * time.Minute).UnixMilli(), // Within timestamp window
				},
			}
			writeHistory(historyPath, entries)

			// When discovering UUID for empty-uuid-session
			foundUUID, err := uuid.Discover("empty-uuid-session", manifestSearchFunc, false)

			// Then it should fail rather than return the wrong UUID
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("UUID discovery failed"))
			Expect(foundUUID).NotTo(Equal("wrong-uuid-4444-4444-4444-444444444444"))
		})
	})

	Context("when manifest UUID conflicts with /rename", func() {
		It("should trust /rename over manifest UUID", func() {
			// Given a manifest with incorrect UUID
			manifestSearchFunc := func(name string) (*manifest.Manifest, error) {
				return &manifest.Manifest{
					Claude: manifest.Claude{
						UUID: "wrong-uuid-5555-5555-5555-555555555555",
					},
				}, nil
			}

			// And history with correct /rename pointing to different UUID
			entries := []history.ConversationEntry{
				{
					SessionID: "correct-uuid-6666-6666-6666-666666666666",
					Display:   "/rename conflicted-session",
					Timestamp: time.Now().UnixMilli(),
				},
			}
			writeHistory(historyPath, entries)

			// When discovering UUID
			foundUUID, err := uuid.Discover("conflicted-session", manifestSearchFunc, false)

			// Then it should return the /rename UUID, not the manifest UUID
			Expect(err).NotTo(HaveOccurred())
			Expect(foundUUID).To(Equal("correct-uuid-6666-6666-6666-666666666666"))
			Expect(foundUUID).NotTo(Equal("wrong-uuid-5555-5555-5555-555555555555"))
		})
	})

	Context("when manifest UUID cannot be verified", func() {
		It("should trust manifest UUID when /rename is unavailable", func() {
			// Given a manifest with UUID but no /rename in history
			manifestSearchFunc := func(name string) (*manifest.Manifest, error) {
				return &manifest.Manifest{
					Claude: manifest.Claude{
						UUID: "unverified-uuid-7777-7777-7777-777777777777",
					},
				}, nil
			}

			// And empty history (no /rename commands)
			writeHistory(historyPath, []history.ConversationEntry{})

			// When discovering UUID
			foundUUID, err := uuid.Discover("unverified-session", manifestSearchFunc, false)

			// Then it should trust the manifest UUID
			Expect(err).NotTo(HaveOccurred())
			Expect(foundUUID).To(Equal("unverified-uuid-7777-7777-7777-777777777777"))
		})
	})

	Context("given the which-vesion bug scenario", func() {
		It("should not return open-tasks UUID for which-vesion session", func() {
			// Given which-vesion manifest with empty UUID (created at 19:38:58)
			whichVesionTime := time.Date(2026, 1, 22, 19, 38, 58, 0, time.UTC)
			manifestSearchFunc := func(name string) (*manifest.Manifest, error) {
				if name == "which-vesion" {
					return &manifest.Manifest{
						Claude: manifest.Claude{
							UUID: "", // Empty UUID
						},
						UpdatedAt: whichVesionTime,
					}, nil
				}
				return nil, fmt.Errorf("not found")
			}

			// And open-tasks session active around that time
			entries := []history.ConversationEntry{
				{
					SessionID: "ff558af0-7468-4fc4-b8c2-bd5980a8be16", // open-tasks UUID
					Display:   "/rename open-tasks",
					Timestamp: whichVesionTime.Add(-5 * time.Minute).UnixMilli(),
				},
				{
					SessionID: "ff558af0-7468-4fc4-b8c2-bd5980a8be16",
					Display:   "some command",
					Timestamp: whichVesionTime.Add(2 * time.Minute).UnixMilli(),
				},
			}
			writeHistory(historyPath, entries)

			// When discovering UUID for which-vesion
			foundUUID, err := uuid.Discover("which-vesion", manifestSearchFunc, false)

			// Then it should fail rather than return open-tasks UUID
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("UUID discovery failed"))
			Expect(foundUUID).NotTo(Equal("ff558af0-7468-4fc4-b8c2-bd5980a8be16"))
		})
	})
})

// Helper function to write history entries
func writeHistory(path string, entries []history.ConversationEntry) {
	file, err := os.Create(path)
	Expect(err).NotTo(HaveOccurred())
	defer file.Close()

	for _, entry := range entries {
		data, err := json.Marshal(entry)
		Expect(err).NotTo(HaveOccurred())
		fmt.Fprintf(file, "%s\n", data)
	}
}
