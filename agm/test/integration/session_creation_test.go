//go:build integration

package integration_test

import (
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/test/integration/helpers"
)

var _ = Describe("Session Creation", func() {
	var sessionName string
	var workDir string

	BeforeEach(func() {
		sessionName = testEnv.UniqueSessionName("creation")
		workDir = "/tmp"
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

	Describe("Basic session creation", func() {
		Context("when creating session outside tmux", func() {
			It("should create new tmux session", func() {
				// Create tmux session
				err := helpers.CreateTmuxSession(sessionName, workDir)
				Expect(err).ToNot(HaveOccurred())

				// Verify session exists
				exists, err := helpers.HasTmuxSession(sessionName)
				Expect(err).ToNot(HaveOccurred())
				Expect(exists).To(BeTrue())
			})
		})

		Context("when creating session with manifest", func() {
			It("should create manifest file with v2 schema", func() {
				// Create tmux session
				err := helpers.CreateTmuxSession(sessionName, workDir)
				Expect(err).ToNot(HaveOccurred())

				// Create manifest directory
				manifestDir := testEnv.ManifestDir(sessionName)
				err = os.MkdirAll(manifestDir, 0700)
				Expect(err).ToNot(HaveOccurred())

				// Create manifest
				manifestPath := testEnv.ManifestPath(sessionName)
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
					Agent: "claude",
				}

				err = manifest.Write(manifestPath, m)
				Expect(err).ToNot(HaveOccurred())

				// Verify manifest exists
				_, err = os.Stat(manifestPath)
				Expect(err).ToNot(HaveOccurred())

				// Read and verify manifest
				readManifest, err := manifest.Read(manifestPath)
				Expect(err).ToNot(HaveOccurred())
				Expect(readManifest.Name).To(Equal(sessionName))
				Expect(readManifest.Agent).To(Equal("claude"))
			})
		})
	})

	Describe("Multi-agent session creation", func() {
		DescribeTable("creates session for multiple agents",
			func(agent string) {
				// Create unique session for this agent test
				agentSessionName := testEnv.UniqueSessionName("agent-" + agent)
				defer func() {
					if !CurrentSpecReport().Failed() {
						helpers.KillTmuxSession(agentSessionName)
						os.RemoveAll(testEnv.ManifestDir(agentSessionName))
					}
				}()

				// Create tmux session
				err := helpers.CreateTmuxSession(agentSessionName, workDir)
				Expect(err).ToNot(HaveOccurred())

				// Create manifest with agent field
				manifestDir := testEnv.ManifestDir(agentSessionName)
				err = os.MkdirAll(manifestDir, 0700)
				Expect(err).ToNot(HaveOccurred())

				manifestPath := testEnv.ManifestPath(agentSessionName)
				m := &manifest.Manifest{
					SchemaVersion: manifest.SchemaVersion,
					SessionID:     "test-uuid-" + agentSessionName,
					Name:          agentSessionName,
					Context: manifest.Context{
						Project: workDir,
					},
					Tmux: manifest.Tmux{
						SessionName: agentSessionName,
					},
					Agent: agent,
				}

				err = manifest.Write(manifestPath, m)
				Expect(err).ToNot(HaveOccurred())

				// Read and verify agent field
				readManifest, err := manifest.Read(manifestPath)
				Expect(err).ToNot(HaveOccurred())
				Expect(readManifest.Agent).To(Equal(agent))
			},
			Entry("claude agent", "claude"),
			Entry("gemini agent", "gemini"),
		)
	})

	Describe("Claude mock integration", func() {
		Context("when using mock Claude", func() {
			It("should simulate Claude startup", func() {
				mockClaude := testEnv.Claude

				// Simulate starting Claude
				err := mockClaude.Start(sessionName)
				Expect(err).ToNot(HaveOccurred())

				// Verify Claude is ready
				ready := mockClaude.IsReady(sessionName)
				Expect(ready).To(BeTrue())

				// Stop Claude
				err = mockClaude.Stop(sessionName)
				Expect(err).ToNot(HaveOccurred())
			})
		})
	})
})
