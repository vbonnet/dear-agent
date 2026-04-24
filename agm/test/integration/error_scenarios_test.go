//go:build integration

package integration_test

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/google/uuid"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/test/integration/helpers"
)

var _ = Describe("Error Scenarios", func() {
	var sessionName string

	BeforeEach(func() {
		sessionName = testEnv.UniqueSessionName("error")
	})

	AfterEach(func() {
		if !CurrentSpecReport().Failed() {
			helpers.KillTmuxSession(sessionName)
			os.RemoveAll(testEnv.ManifestDir(sessionName))
		}
	})

	Describe("Session name conflicts", func() {
		Context("when session already exists", func() {
			It("should detect existing session", func() {
				// Create first session
				err := helpers.CreateTmuxSession(sessionName, "/tmp")
				Expect(err).ToNot(HaveOccurred())

				// Verify session exists
				exists, err := helpers.HasTmuxSession(sessionName)
				Expect(err).ToNot(HaveOccurred())
				Expect(exists).To(BeTrue())

				// Attempting to create again should be detectable
				// (actual behavior depends on AGM implementation)
				// For now, verify we can detect the session exists
				exists, err = helpers.HasTmuxSession(sessionName)
				Expect(err).ToNot(HaveOccurred())
				Expect(exists).To(BeTrue())
			})
		})
	})

	Describe("Invalid session names", func() {
		Context("when session name is empty", func() {
			It("should be rejected by tmux", func() {
				err := helpers.CreateTmuxSession("", "/tmp")
				Expect(err).To(HaveOccurred())
			})
		})

		Context("when session name contains invalid characters", func() {
			It("should handle special characters", func() {
				// Tmux has restrictions on session names (e.g., no colons, periods are ok)
				// Test that we can detect invalid names
				invalidNames := []string{
					"session:with:colons",
					"session.with.dots", // This may actually be valid
				}

				for _, name := range invalidNames {
					err := helpers.CreateTmuxSession(name, "/tmp")
					// Some may fail, some may succeed depending on tmux version
					// We're just verifying the error handling works
					if err != nil {
						Expect(err.Error()).To(ContainSubstring("failed to create tmux session"))
					} else {
						// Clean up if it succeeded
						helpers.KillTmuxSession(name)
					}
				}
			})
		})
	})

	Describe("Manifest write failures", func() {
		Context("when manifest directory is not writable", func() {
			It("should return error on write", func() {
				// Create read-only directory
				readOnlyDir := filepath.Join(os.TempDir(), "readonly-test-"+uuid.New().String())
				err := os.MkdirAll(readOnlyDir, 0400) // read-only
				Expect(err).ToNot(HaveOccurred())
				defer os.RemoveAll(readOnlyDir)

				// Attempt to write manifest to read-only directory
				manifestPath := filepath.Join(readOnlyDir, "manifest.yaml")
				m := &manifest.Manifest{
					SchemaVersion: manifest.SchemaVersion,
					SessionID:     uuid.New().String(),
					Name:          sessionName,
				}

				err = manifest.Write(manifestPath, m)
				Expect(err).To(HaveOccurred())
			})
		})
	})

	Describe("Tmux operation errors", func() {
		Context("when querying non-existent session", func() {
			It("should return false without error", func() {
				nonExistentSession := "agm-test-nonexistent-" + uuid.New().String()

				exists, err := helpers.HasTmuxSession(nonExistentSession)
				Expect(err).ToNot(HaveOccurred())
				Expect(exists).To(BeFalse())
			})
		})

		Context("when killing non-existent session", func() {
			It("should handle gracefully", func() {
				nonExistentSession := "agm-test-nonexistent-" + uuid.New().String()

				err := helpers.KillTmuxSession(nonExistentSession)
				Expect(err).ToNot(HaveOccurred()) // Should not error for non-existent session
			})
		})

		Context("when getting option from non-existent session", func() {
			It("should return error", func() {
				nonExistentSession := "agm-test-nonexistent-" + uuid.New().String()

				_, err := helpers.GetTmuxOption(nonExistentSession, "aggressive-resize")
				Expect(err).To(HaveOccurred())
			})
		})
	})

	Describe("Mock Claude error handling", func() {
		Context("when Claude operations fail gracefully", func() {
			It("should handle stop on non-started session", func() {
				mockClaude := testEnv.Claude

				// Stop Claude that was never started (should not error)
				err := mockClaude.Stop("never-started-session")
				Expect(err).ToNot(HaveOccurred())
			})

			It("should report not ready for non-started session", func() {
				mockClaude := testEnv.Claude

				ready := mockClaude.IsReady("never-started-session")
				Expect(ready).To(BeFalse())
			})
		})
	})

	Describe("Manifest read errors", func() {
		Context("when manifest file doesn't exist", func() {
			It("should return error", func() {
				nonExistentPath := filepath.Join(os.TempDir(), "nonexistent-"+uuid.New().String()+".yaml")

				_, err := manifest.Read(nonExistentPath)
				Expect(err).To(HaveOccurred())
			})
		})

		Context("when manifest file is corrupt", func() {
			It("should return error on invalid YAML", func() {
				corruptPath := filepath.Join(os.TempDir(), "corrupt-"+uuid.New().String()+".yaml")
				defer os.Remove(corruptPath)

				// Write invalid YAML
				err := os.WriteFile(corruptPath, []byte("invalid: yaml: content: [[["), 0600)
				Expect(err).ToNot(HaveOccurred())

				_, err = manifest.Read(corruptPath)
				Expect(err).To(HaveOccurred())
			})
		})
	})
})
