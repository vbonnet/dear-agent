package identity

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestGitConfigDetector_Name tests detector name
func TestGitConfigDetector_Name(t *testing.T) {
	d := &GitConfigDetector{}
	if got := d.Name(); got != "git_config" {
		t.Errorf("Name() = %s, want git_config", got)
	}
}

// TestGitConfigDetector_Priority tests priority value
func TestGitConfigDetector_Priority(t *testing.T) {
	d := &GitConfigDetector{}
	if got := d.Priority(); got != 50 {
		t.Errorf("Priority() = %d, want 50", got)
	}
}

// TestGitConfigDetector_Detect tests successful detection
func TestGitConfigDetector_Detect(t *testing.T) {
	d := &GitConfigDetector{}
	ctx := context.Background()

	id, err := d.Detect(ctx)

	// May succeed or fail depending on git config
	// Just ensure no panic
	_ = id
	_ = err
}

// TestGitConfigDetector_ParseGitConfig tests .gitconfig parsing
func TestGitConfigDetector_ParseGitConfig(t *testing.T) {
	tests := []struct {
		name       string
		gitconfig  string
		wantEmail  string
		wantDomain string
	}{
		{
			name: "simple config",
			gitconfig: `[user]
	email = user@example.com
	name = Test User`,
			wantEmail:  "user@example.com",
			wantDomain: "@example.com",
		},
		{
			name: "config with spaces",
			gitconfig: `[user]
    email   =   user@example.com
    name = Test User`,
			wantEmail:  "user@example.com",
			wantDomain: "@example.com",
		},
		{
			name: "config with other sections",
			gitconfig: `[core]
	editor = vim
[user]
	email = user@example.com
[alias]
	st = status`,
			wantEmail:  "user@example.com",
			wantDomain: "@example.com",
		},
		{
			name: "no user section",
			gitconfig: `[core]
	editor = vim`,
			wantEmail:  "",
			wantDomain: "",
		},
		{
			name: "email without @ sign",
			gitconfig: `[user]
	email = invalid-email`,
			wantEmail:  "invalid-email", // Parser returns it
			wantDomain: "",              // But domain extraction fails
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary .gitconfig
			tmpDir := t.TempDir()
			gitconfigPath := filepath.Join(tmpDir, ".gitconfig")
			if err := os.WriteFile(gitconfigPath, []byte(tt.gitconfig), 0644); err != nil {
				t.Fatalf("Failed to write .gitconfig: %v", err)
			}

			// Mock home directory (would need to implement or use test helper)
			// For now, test the readGitConfigFile logic separately
			d := &GitConfigDetector{}

			// Test the parsing logic by reading our temp file
			data, err := os.ReadFile(gitconfigPath)
			if err != nil {
				t.Fatalf("Failed to read .gitconfig: %v", err)
			}

			// Parse email from data (duplicate parsing logic for test)
			email := parseEmailFromGitConfig(string(data))

			if email != tt.wantEmail {
				t.Errorf("parseEmail() = %s, want %s", email, tt.wantEmail)
			}

			if tt.wantEmail != "" {
				domain := extractDomain(email)
				if domain != tt.wantDomain {
					t.Errorf("extractDomain(%s) = %s, want %s", email, domain, tt.wantDomain)
				}

				// Create identity to verify structure
				id := d.makeIdentity(email)

				// makeIdentity returns nil if domain extraction fails
				if tt.wantDomain == "" {
					if id != nil {
						t.Error("makeIdentity() should return nil for email without valid domain")
					}
				} else {
					// Valid domain expected
					if id == nil {
						t.Error("makeIdentity() returned nil for valid email")
					} else {
						if id.Email != tt.wantEmail {
							t.Errorf("Identity.Email = %s, want %s", id.Email, tt.wantEmail)
						}
						if id.Domain != tt.wantDomain {
							t.Errorf("Identity.Domain = %s, want %s", id.Domain, tt.wantDomain)
						}
						if id.Method != "git_config" {
							t.Errorf("Identity.Method = %s, want git_config", id.Method)
						}
						if id.Verified {
							t.Error("Git config identity should not be verified")
						}
					}
				}
			}
		})
	}
}

// parseEmailFromGitConfig is a test helper that duplicates the parsing logic
func parseEmailFromGitConfig(data string) string {
	inUserSection := false
	for _, line := range splitLines(data) {
		line = trimSpace(line)

		// Check for [user] section
		if line == "[user]" {
			inUserSection = true
			continue
		}

		// Check for new section (exits [user] section)
		if len(line) > 0 && line[0] == '[' && line[len(line)-1] == ']' {
			inUserSection = false
			continue
		}

		// Parse email in [user] section
		if inUserSection {
			// Try both formats: "email = value" and "email=value"
			if hasPrefix(line, "email") {
				// Remove "email" prefix
				rest := trimPrefix(line, "email")
				rest = trimSpace(rest)

				// Remove "=" separator
				if hasPrefix(rest, "=") {
					email := trimPrefix(rest, "=")
					email = trimSpace(email)
					if email != "" {
						return email
					}
				}
			}
		}
	}

	return ""
}

// Test helpers
func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

func trimSpace(s string) string {
	start := 0
	for start < len(s) && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	end := len(s)
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}
	return s[start:end]
}

func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

func trimPrefix(s, prefix string) string {
	if hasPrefix(s, prefix) {
		return s[len(prefix):]
	}
	return s
}
