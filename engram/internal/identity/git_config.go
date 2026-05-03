package identity

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// GitConfigDetector detects identity from git config user.email
type GitConfigDetector struct{}

// Name returns detector name
func (d *GitConfigDetector) Name() string {
	return "git_config"
}

// Priority returns 50 (medium priority, unverified)
func (d *GitConfigDetector) Priority() int {
	return 50
}

// Detect attempts to detect identity from git global config
func (d *GitConfigDetector) Detect(ctx context.Context) (*Identity, error) {
	// Try reading .gitconfig file directly first (faster than git command)
	email, err := d.readGitConfigFile()
	if err == nil && email != "" {
		return d.makeIdentity(email), nil
	}

	// Fall back to git command
	cmd := exec.CommandContext(ctx, "git", "config", "--global", "user.email")
	output, err := cmd.Output()
	if err != nil {
		// Git not installed or config not set, not an error
		return nil, nil //nolint:nilerr // intentional: caller signals via separate bool/optional
	}

	email = strings.TrimSpace(string(output))
	if email == "" {
		return nil, nil
	}

	return d.makeIdentity(email), nil
}

// readGitConfigFile reads email from ~/.gitconfig directly
func (d *GitConfigDetector) readGitConfigFile() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	configPath := filepath.Join(homeDir, ".gitconfig")
	data, err := os.ReadFile(configPath)
	if err != nil {
		return "", err
	}

	// Parse INI-style config
	// Format:
	// [user]
	//     email = user@example.com
	lines := strings.Split(string(data), "\n")
	inUserSection := false

	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Check for [user] section
		if line == "[user]" {
			inUserSection = true
			continue
		}

		// Check for new section
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			inUserSection = false
			continue
		}

		// Parse email in [user] section
		if inUserSection {
			if strings.HasPrefix(line, "email = ") || strings.HasPrefix(line, "email=") {
				email := strings.TrimPrefix(line, "email = ")
				email = strings.TrimPrefix(email, "email=")
				return strings.TrimSpace(email), nil
			}
		}
	}

	return "", fmt.Errorf("user.email not found in .gitconfig")
}

// makeIdentity creates Identity from email
func (d *GitConfigDetector) makeIdentity(email string) *Identity {
	domain := extractDomain(email)
	if domain == "" {
		return nil
	}

	return &Identity{
		Email:      email,
		Domain:     domain,
		Method:     "git_config",
		Verified:   false, // User can set git config to any value
		DetectedAt: time.Now(),
	}
}
