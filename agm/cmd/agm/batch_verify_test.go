package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

func TestVerifySingleSession_Verified(t *testing.T) {
	// Create a temp repo where the purpose assertions pass
	repoDir := t.TempDir()
	writeTestFile(t, filepath.Join(repoDir, "handler.go"), `package main

func broadcastFromMPC() {
	// fixed implementation
}
`)

	m := &manifest.Manifest{
		SessionID: "test-session-1",
		Name:      "fix-broadcast",
		Context: manifest.Context{
			Purpose: "fix broadcastFromMPC function",
			Project: repoDir,
		},
	}

	// Set repo dir to the temp dir
	oldRepoDir := batchVerifyRepoDir
	batchVerifyRepoDir = repoDir
	defer func() { batchVerifyRepoDir = oldRepoDir }()

	result := verifySingleSession(m)

	assert.Equal(t, "VERIFIED", result.Status)
	assert.Equal(t, "fix-broadcast", result.SessionName)
	assert.Equal(t, "test-session-1", result.SessionID)
	assert.Greater(t, result.PassCount, 0)
	assert.Equal(t, 0, result.FailCount)
}

func TestVerifySingleSession_NeedsRemediation(t *testing.T) {
	// Create a temp repo where removal assertions fail (dependency still present)
	repoDir := t.TempDir()
	writeTestFile(t, filepath.Join(repoDir, "go.mod"), `module example.com/test

go 1.21

require go.temporal.io/sdk v1.25.0
`)
	writeTestFile(t, filepath.Join(repoDir, "worker.go"), `package main

import "go.temporal.io/sdk/client"

func startWorker() {
	c, _ := client.Dial(client.Options{})
	defer c.Close()
}
`)

	m := &manifest.Manifest{
		SessionID: "test-session-2",
		Name:      "remove-temporal",
		Context: manifest.Context{
			Purpose: "Remove go.temporal.io/sdk dependency from the project",
			Project: repoDir,
		},
	}

	oldRepoDir := batchVerifyRepoDir
	batchVerifyRepoDir = repoDir
	defer func() { batchVerifyRepoDir = oldRepoDir }()

	result := verifySingleSession(m)

	assert.Equal(t, "NEEDS_REMEDIATION", result.Status)
	assert.Greater(t, result.FailCount, 0)

	// Should have details about failures
	var foundFailDetail bool
	for _, d := range result.Details {
		if !d.Pass {
			foundFailDetail = true
			break
		}
	}
	assert.True(t, foundFailDetail, "expected at least one failed detail")
}

func TestVerifySingleSession_SkippedNoPurpose(t *testing.T) {
	m := &manifest.Manifest{
		SessionID: "test-session-3",
		Name:      "no-purpose",
		Context: manifest.Context{
			Purpose: "",
			Project: "/tmp/nonexistent",
		},
	}

	result := verifySingleSession(m)

	assert.Equal(t, "SKIPPED", result.Status)
}

func TestVerifyBatch_MixedResults(t *testing.T) {
	repoDir := t.TempDir()
	writeTestFile(t, filepath.Join(repoDir, "handler.go"), `package main

func broadcastFromMPC() {}
`)

	manifests := []*manifest.Manifest{
		{
			SessionID: "s1",
			Name:      "fix-broadcast",
			Context: manifest.Context{
				Purpose: "fix broadcastFromMPC function",
				Project: repoDir,
			},
		},
		{
			SessionID: "s2",
			Name:      "no-purpose",
			Context:   manifest.Context{},
		},
	}

	oldRepoDir := batchVerifyRepoDir
	batchVerifyRepoDir = repoDir
	defer func() { batchVerifyRepoDir = oldRepoDir }()

	report := verifyBatch(manifests)

	assert.Equal(t, 2, report.Summary.Total)
	assert.Equal(t, 1, report.Summary.Verified)
	assert.Equal(t, 1, report.Summary.Skipped)
	assert.Equal(t, 0, report.Summary.NeedsRemediation)
}

func TestSaveReport(t *testing.T) {
	report := &BatchVerifyReport{
		Timestamp: "2026-03-31T12:00:00Z",
		RepoDir:   "/tmp/test",
		Results: []BatchVerifyResult{
			{
				SessionName: "test-session",
				SessionID:   "abc123",
				Purpose:     "fix something",
				Status:      "VERIFIED",
				PassCount:   2,
				FailCount:   0,
			},
		},
		Summary: BatchVerifySummary{
			Total:    1,
			Verified: 1,
		},
	}

	reportPath, err := saveReport(report)
	require.NoError(t, err)
	defer os.Remove(reportPath)

	// Verify file exists and is valid JSON
	data, err := os.ReadFile(reportPath)
	require.NoError(t, err)

	var loaded BatchVerifyReport
	err = json.Unmarshal(data, &loaded)
	require.NoError(t, err)

	assert.Equal(t, 1, loaded.Summary.Total)
	assert.Equal(t, 1, loaded.Summary.Verified)
	assert.Equal(t, "test-session", loaded.Results[0].SessionName)
}

func TestSessionDisplayName(t *testing.T) {
	tests := []struct {
		name     string
		manifest *manifest.Manifest
		expected string
	}{
		{
			name: "prefers Name",
			manifest: &manifest.Manifest{
				SessionID: "abc",
				Name:      "my-session",
				Tmux:      manifest.Tmux{SessionName: "tmux-name"},
			},
			expected: "my-session",
		},
		{
			name: "falls back to tmux name",
			manifest: &manifest.Manifest{
				SessionID: "abc",
				Tmux:      manifest.Tmux{SessionName: "tmux-name"},
			},
			expected: "tmux-name",
		},
		{
			name: "falls back to session ID",
			manifest: &manifest.Manifest{
				SessionID: "abc-123",
			},
			expected: "abc-123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, sessionDisplayName(tt.manifest))
		})
	}
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	dir := filepath.Dir(path)
	require.NoError(t, os.MkdirAll(dir, 0755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))
}
