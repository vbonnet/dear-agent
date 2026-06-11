package claude

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

// uuidPattern matches a canonical Claude session UUID (8-4-4-4-12 hex). It is
// used to reject malformed identifiers before they reach filesystem globs.
var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// FindTranscriptCwd locates the transcript for a Claude conversation UUID under
// ~/.claude/projects/<project-slug>/<uuid>.jsonl and returns the original
// working directory the conversation was started in (its "cwd" field).
//
// This matters for resume: `claude --resume <uuid>` is scoped to the current
// directory's project slug. If a session is resumed from a directory whose slug
// differs from where the conversation actually lives, Claude reports
// "No conversation found". Resuming from the recorded cwd avoids that.
//
// homeDir is the user home directory (so callers can inject one in tests).
// Returns an error if the transcript cannot be found or has no cwd entry.
func FindTranscriptCwd(homeDir, uuid string) (string, error) {
	if uuid == "" {
		return "", fmt.Errorf("empty UUID")
	}
	// Reject malformed identifiers before globbing: a value containing path
	// separators or glob metacharacters could match unintended transcripts.
	if !uuidPattern.MatchString(uuid) {
		return "", fmt.Errorf("invalid UUID format: %q", uuid)
	}

	projectsDir := filepath.Join(homeDir, ".claude", "projects")
	matches, err := filepath.Glob(filepath.Join(projectsDir, "*", uuid+".jsonl"))
	if err != nil {
		return "", fmt.Errorf("failed to search transcripts: %w", err)
	}
	if len(matches) == 0 {
		return "", fmt.Errorf("no transcript found for UUID %s under %s", uuid, projectsDir)
	}

	// If multiple project dirs contain a same-named transcript (rare), prefer
	// the first; they should refer to the same conversation.
	return readCwdFromTranscript(matches[0])
}

// readCwdFromTranscript scans a transcript file for the first entry carrying a
// non-empty "cwd" field and returns it. Summary/meta lines at the top of a
// compacted transcript have no cwd, so we cannot rely on the first line.
func readCwdFromTranscript(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("failed to open transcript %s: %w", path, err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), scannerBufSize)

	for scanner.Scan() {
		var entry struct {
			Cwd string `json:"cwd"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			continue // tolerate malformed lines
		}
		if entry.Cwd != "" {
			return entry.Cwd, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("failed to scan transcript %s: %w", path, err)
	}

	return "", fmt.Errorf("no cwd entry found in transcript %s", path)
}
