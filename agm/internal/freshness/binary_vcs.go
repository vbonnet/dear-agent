package freshness

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ReadBinaryVCSRevision extracts the embedded vcs.revision from a Go binary
// using "go version -m". Returns ("", nil) when the binary exists but was
// built without VCS stamps. Returns an error only when the binary is
// unreadable or "go" itself fails.
func ReadBinaryVCSRevision(binaryPath string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), gitTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "go", "version", "-m", binaryPath).Output()
	if err != nil {
		return "", fmt.Errorf("go version -m %s: %w", binaryPath, err)
	}

	var rev string
	var modified bool
	for line := range strings.SplitSeq(string(out), "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "build\tvcs.revision="):
			r := strings.TrimPrefix(line, "build\tvcs.revision=")
			if len(r) > 12 {
				r = r[:12]
			}
			rev = r
		case line == "build\tvcs.modified=true":
			modified = true
		}
	}
	if rev != "" && modified {
		return rev + "-dirty", nil
	}
	return rev, nil // "" when vcs stamps absent
}

// FindDearAgentRepoPath locates the dear-agent source repository root (the
// parent of the agm/ subdirectory). It checks DEAR_AGENT_SOURCE_DIR first,
// then the known default location.
func FindDearAgentRepoPath() (string, error) {
	if dir := os.Getenv("DEAR_AGENT_SOURCE_DIR"); dir != "" {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		return "", fmt.Errorf("DEAR_AGENT_SOURCE_DIR=%s does not contain go.mod", dir)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}

	knownPath := filepath.Join(home, "src", "dear-agent")
	if _, err := os.Stat(filepath.Join(knownPath, "go.mod")); err == nil {
		return knownPath, nil
	}

	return "", fmt.Errorf("dear-agent source repository not found (set DEAR_AGENT_SOURCE_DIR to override)")
}
