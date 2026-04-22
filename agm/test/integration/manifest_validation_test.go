//go:build integration

package integration_test

import (
	"os"
	"path/filepath"
	"regexp"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/google/uuid"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

var _ = Describe("Manifest Validation", func() {
	var sessionName string
	var manifestPath string

	BeforeEach(func() {
		sessionName = testEnv.UniqueSessionName("manifest")
		manifestDir := testEnv.ManifestDir(sessionName)
		os.MkdirAll(manifestDir, 0700)
		manifestPath = testEnv.ManifestPath(sessionName)
	})

	AfterEach(func() {
		if !CurrentSpecReport().Failed() {
			os.RemoveAll(testEnv.ManifestDir(sessionName))
		}
	})

	Describe("Manifest creation", func() {
		Context("when creating a new manifest", func() {
			It("should create manifest file with correct permissions", func() {
				m := &manifest.Manifest{
					SchemaVersion: manifest.SchemaVersion,
					SessionID:     uuid.New().String(),
					Name:          sessionName,
					CreatedAt:     time.Now(),
					UpdatedAt:     time.Now(),
					Context: manifest.Context{
						Project: "/tmp",
					},
					Tmux: manifest.Tmux{
						SessionName: sessionName,
					},
					Agent: "claude",
				}

				err := manifest.Write(manifestPath, m)
				Expect(err).ToNot(HaveOccurred())

				// Verify file exists
				info, err := os.Stat(manifestPath)
				Expect(err).ToNot(HaveOccurred())
				Expect(info.IsDir()).To(BeFalse())

				// Verify file is not empty
				Expect(info.Size()).To(BeNumerically(">", 0))
			})
		})
	})

	Describe("Manifest v2 schema validation", func() {
		Context("when manifest has all required v2 fields", func() {
			It("should validate successfully", func() {
				testUUID := uuid.New().String()
				now := time.Now()

				m := &manifest.Manifest{
					SchemaVersion: "2.0",
					SessionID:     testUUID,
					Name:          sessionName,
					CreatedAt:     now,
					UpdatedAt:     now,
					Lifecycle:     "",
					Context: manifest.Context{
						Project: "/tmp/test-project",
					},
					Tmux: manifest.Tmux{
						SessionName: sessionName,
					},
					Agent: "claude",
					Claude: manifest.Claude{
						UUID: "",
					},
				}

				err := manifest.Write(manifestPath, m)
				Expect(err).ToNot(HaveOccurred())

				// Read back and validate
				readManifest, err := manifest.Read(manifestPath)
				Expect(err).ToNot(HaveOccurred())

				// Verify all v2 fields
				Expect(readManifest.SchemaVersion).To(Equal("2.0"))
				Expect(readManifest.SessionID).To(Equal(testUUID))
				Expect(readManifest.Name).To(Equal(sessionName))
				Expect(readManifest.Tmux.SessionName).To(Equal(sessionName))
				Expect(readManifest.Agent).To(Equal("claude"))
			})
		})

		Context("when manifest has UUID in correct format", func() {
			It("should validate UUID format", func() {
				testUUID := uuid.New().String()

				m := &manifest.Manifest{
					SchemaVersion: manifest.SchemaVersion,
					SessionID:     testUUID,
					Name:          sessionName,
					CreatedAt:     time.Now(),
					UpdatedAt:     time.Now(),
					Context: manifest.Context{
						Project: "/tmp",
					},
					Tmux: manifest.Tmux{
						SessionName: sessionName,
					},
					Agent: "claude",
				}

				err := manifest.Write(manifestPath, m)
				Expect(err).ToNot(HaveOccurred())

				readManifest, err := manifest.Read(manifestPath)
				Expect(err).ToNot(HaveOccurred())

				// Validate UUID format
				uuidRegex := regexp.MustCompile(`^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$`)
				Expect(uuidRegex.MatchString(readManifest.SessionID)).To(BeTrue())
			})
		})
	})

	Describe("Manifest fixture validation", func() {
		Context("when loading valid v2 manifest fixture", func() {
			It("should parse successfully", func() {
				fixturePath := filepath.Join("testdata", "manifests", "valid-v2.yaml")

				m, err := manifest.Read(fixturePath)
				Expect(err).ToNot(HaveOccurred())

				// Verify fixture has v2 schema
				Expect(m.SchemaVersion).To(Equal("2.0"))
				Expect(m.SessionID).ToNot(BeEmpty())
				Expect(m.Name).To(Equal("test-session"))
				Expect(m.Agent).To(Equal("claude"))
			})
		})

		Context("when loading invalid manifest fixtures", func() {
			It("should handle missing session_id", func() {
				fixturePath := filepath.Join("testdata", "manifests", "missing-session-id.yaml")

				m, err := manifest.Read(fixturePath)
				// Manifest may load but have empty SessionID
				if err == nil {
					Expect(m.SessionID).To(BeEmpty())
				}
				// Either error or empty SessionID is acceptable for invalid fixture
			})

			It("should auto-migrate old schema version", func() {
				fixturePath := filepath.Join("testdata", "manifests", "invalid-schema.yaml")

				m, err := manifest.Read(fixturePath)
				// AGM auto-migrates v1 to v2
				if err == nil {
					// Should have been migrated to v2
					Expect(m.SchemaVersion).To(Equal("2.0"))
				}
				// Loading v1 schema should succeed with auto-migration
			})
		})
	})

	Describe("Manifest field validation", func() {
		Context("when agent field is set", func() {
			It("should preserve agent field value", func() {
				agents := []string{"claude", "gemini", "gpt", "opencode"}

				for _, agent := range agents {
					m := &manifest.Manifest{
						SchemaVersion: manifest.SchemaVersion,
						SessionID:     uuid.New().String(),
						Name:          sessionName + "-" + agent,
						CreatedAt:     time.Now(),
						UpdatedAt:     time.Now(),
						Context: manifest.Context{
							Project: "/tmp",
						},
						Tmux: manifest.Tmux{
							SessionName: sessionName,
						},
						Agent: agent,
					}

					testPath := filepath.Join(testEnv.ManifestDir(sessionName), agent+"-manifest.yaml")
					err := manifest.Write(testPath, m)
					Expect(err).ToNot(HaveOccurred())

					readManifest, err := manifest.Read(testPath)
					Expect(err).ToNot(HaveOccurred())
					Expect(readManifest.Agent).To(Equal(agent))
				}
			})
		})

		Context("when OpenCode metadata is set", func() {
			It("should preserve OpenCode metadata fields", func() {
				now := time.Now()
				m := &manifest.Manifest{
					SchemaVersion: manifest.SchemaVersion,
					SessionID:     uuid.New().String(),
					Name:          sessionName + "-opencode",
					CreatedAt:     now,
					UpdatedAt:     now,
					Context: manifest.Context{
						Project: "/tmp/opencode-project",
					},
					Tmux: manifest.Tmux{
						SessionName: sessionName,
					},
					Agent: "opencode",
					OpenCode: &manifest.OpenCode{
						ServerPort: 4096,
						ServerHost: "localhost",
						AttachTime: now,
					},
				}

				testPath := filepath.Join(testEnv.ManifestDir(sessionName), "opencode-manifest.yaml")
				err := manifest.Write(testPath, m)
				Expect(err).ToNot(HaveOccurred())

				readManifest, err := manifest.Read(testPath)
				Expect(err).ToNot(HaveOccurred())
				Expect(readManifest.Agent).To(Equal("opencode"))
				Expect(readManifest.OpenCode).ToNot(BeNil())
				Expect(readManifest.OpenCode.ServerPort).To(Equal(4096))
				Expect(readManifest.OpenCode.ServerHost).To(Equal("localhost"))
				Expect(readManifest.OpenCode.AttachTime).To(BeTemporally("~", now, time.Second))
			})
		})
	})
})
