package manifest

import (
	"path/filepath"
	"testing"
	"time"
)

func TestValidateSandboxOwnership(t *testing.T) {
	sessionID := "owned-session"
	base := filepath.Join(t.TempDir(), ".agm", "sandboxes", sessionID)
	valid := SandboxConfig{
		Enabled:    true,
		ID:         sessionID,
		Provider:   "mock",
		MergedPath: filepath.Join(base, "merged"),
		WorkingDir: filepath.Join(base, "merged", "repo0"),
		CreatedAt:  time.Now(),
	}

	tests := []struct {
		name   string
		mutate func(*SandboxConfig)
	}{
		{name: "valid"},
		{name: "disabled", mutate: func(s *SandboxConfig) { s.Enabled = false }},
		{name: "wrong ID", mutate: func(s *SandboxConfig) { s.ID = "other-session" }},
		{name: "missing provider", mutate: func(s *SandboxConfig) { s.Provider = "" }},
		{name: "missing creation time", mutate: func(s *SandboxConfig) { s.CreatedAt = time.Time{} }},
		{name: "wrong merged boundary", mutate: func(s *SandboxConfig) { s.MergedPath = filepath.Join(base, "other") }},
		{name: "working directory outside merged", mutate: func(s *SandboxConfig) { s.WorkingDir = filepath.Dir(base) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := valid
			if tt.mutate != nil {
				tt.mutate(&got)
			}
			err := ValidateSandboxOwnership(sessionID, &got)
			if (err != nil) != (tt.mutate != nil) {
				t.Fatalf("ValidateSandboxOwnership() error = %v", err)
			}
		})
	}
}
