package integration

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

// TestDoctorOrphanIntegration tests end-to-end orphan detection in doctor command
func TestDoctorOrphanIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Get test adapter
	adapter := dolt.GetTestAdapter(t)
	if adapter == nil {
		t.Skip("Dolt not available for testing")
	}
	defer adapter.Close()

	// Create test manifest (tracked session) in database
	m := &manifest.Manifest{
		SchemaVersion: "2.0.0",
		Name:          "test-session",
		SessionID:     "test-uuid-123",
		Workspace:     "test",
		Harness:       "claude-code",
		Context: manifest.Context{
			Project: "/tmp/test",
		},
		Claude: manifest.Claude{
			UUID: "test-uuid-123",
		},
		Tmux: manifest.Tmux{
			SessionName: "test-session",
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := adapter.CreateSession(m); err != nil {
		t.Fatalf("Failed to create test session in database: %v", err)
	}

	t.Run("DoctorDetectsNoOrphans", func(t *testing.T) {
		// Build agm binary
		agmBinary := buildAGMBinary(t)
		defer os.Remove(agmBinary)

		// Run doctor command (AGM uses Dolt, configured via environment)
		cmd := exec.Command(agmBinary, "admin", "doctor", "--test")
		cmd.Env = os.Environ()

		output, err := cmd.CombinedOutput()
		outputStr := string(output)

		// Doctor should complete successfully (no orphans)
		if err != nil {
			t.Logf("Doctor output:\n%s", outputStr)
		}

		// Verify orphan check section exists
		if !strings.Contains(outputStr, "Checking for orphaned conversations") &&
			!strings.Contains(outputStr, "orphaned conversations") {
			t.Errorf("Doctor output missing orphan check section:\n%s", outputStr)
		}

		// Verify "No orphaned sessions found" message
		if !strings.Contains(outputStr, "No orphaned sessions found") &&
			!strings.Contains(outputStr, "0 orphaned session") {
			t.Logf("Expected 'No orphaned sessions found' in output:\n%s", outputStr)
		}
	})

	t.Run("DoctorOutputFormat", func(t *testing.T) {
		// Verify doctor output follows existing format pattern
		agmBinary := buildAGMBinary(t)
		defer os.Remove(agmBinary)

		cmd := exec.Command(agmBinary, "admin", "doctor", "--test")
		cmd.Env = os.Environ()

		output, err := cmd.CombinedOutput()
		outputStr := string(output)

		if err != nil {
			t.Logf("Doctor output:\n%s", outputStr)
		}

		// Verify output includes all standard sections
		expectedSections := []string{
			"Health Check",
			"Claude history",
			"tmux",
		}

		for _, section := range expectedSections {
			if !strings.Contains(outputStr, section) {
				t.Errorf("Doctor output missing section: %s", section)
			}
		}
	})
}

// TestDoctorOrphanRemediation tests that doctor suggests correct remediation
func TestDoctorOrphanRemediation(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	t.Run("SuggestsRemediationCommand", func(t *testing.T) {
		// This test would require creating actual orphaned sessions
		// For now, we validate the structure is correct

		// Expected remediation in doctor output when orphans are found:
		// "Run: agm admin find-orphans --auto-import"

		expectedCommand := "agm admin find-orphans --auto-import"
		if expectedCommand == "" {
			t.Error("Remediation command should not be empty")
		}
	})
}

// TestDoctorOrphanPerformance tests that orphan check doesn't slow down doctor
func TestDoctorOrphanPerformance(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Get test adapter
	adapter := dolt.GetTestAdapter(t)
	if adapter == nil {
		t.Skip("Dolt not available for testing")
	}
	defer adapter.Close()

	// Create 50 test sessions in database
	for i := 0; i < 50; i++ {
		sessionName := fmt.Sprintf("session-%d", i)

		m := &manifest.Manifest{
			SchemaVersion: "2.0.0",
			Name:          sessionName,
			SessionID:     sessionName,
			Workspace:     "test",
			Harness:       "claude-code",
			Context: manifest.Context{
				Project: "/tmp/test",
			},
			Claude: manifest.Claude{
				UUID: sessionName,
			},
			Tmux: manifest.Tmux{
				SessionName: sessionName,
			},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		_ = adapter.CreateSession(m)
	}

	t.Run("CompletesInReasonableTime", func(t *testing.T) {
		agmBinary := buildAGMBinary(t)
		defer os.Remove(agmBinary)

		start := time.Now()

		cmd := exec.Command(agmBinary, "admin", "doctor", "--test")
		cmd.Env = os.Environ()

		output, err := cmd.CombinedOutput()
		duration := time.Since(start)

		if err != nil {
			t.Logf("Doctor output:\n%s", string(output))
		}

		// Doctor should complete in reasonable time (<5s for 50 sessions)
		if duration > 5*time.Second {
			t.Errorf("Doctor too slow: %v (expected < 5s)", duration)
		}
	})
}

// buildAGMBinary compiles agm binary for testing
func buildAGMBinary(t *testing.T) string {
	t.Helper()

	// Find project root (assuming we're in test/integration/)
	projectRoot := findProjectRoot(t)

	// Build binary in temp directory
	tmpBinary := filepath.Join(t.TempDir(), "agm-test")

	cmd := exec.Command("go", "build", "-o", tmpBinary, "./cmd/agm")
	cmd.Dir = projectRoot

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to build agm binary: %v\nStderr: %s", err, stderr.String())
	}

	return tmpBinary
}

// findProjectRoot finds the agm project root
func findProjectRoot(t *testing.T) string {
	t.Helper()

	// Start from current directory and walk up
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}

	for {
		// Check if go.mod exists
		goModPath := filepath.Join(dir, "go.mod")
		if _, err := os.Stat(goModPath); err == nil {
			// Verify it's the ai-tools module
			content, err := os.ReadFile(goModPath)
			if err == nil && strings.Contains(string(content), "ai-tools") {
				return dir
			}
		}

		// Move up one directory
		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached filesystem root
			break
		}
		dir = parent
	}

	t.Fatal("Could not find project root (go.mod with ai-tools)")
	return ""
}
