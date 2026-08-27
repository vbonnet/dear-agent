package compaction

import (
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/vbonnet/dear-agent/agm/internal/fileutil"
)

// PromptInput holds data used to generate a compaction prompt.
type PromptInput struct {
	SessionName string
	Project     string
	Purpose     string
	Tags        []string
	Notes       string
	Harness     string
	FocusText   string // from --focus flag
}

// GeneratePrompt builds a /compact command string with structured preservation instructions.
func GeneratePrompt(input *PromptInput) string {
	var parts []string

	if input.SessionName != "" || input.Project != "" {
		parts = append(parts, fmt.Sprintf("Session: %s, Project: %s", input.SessionName, input.Project))
	}
	if input.Purpose != "" {
		parts = append(parts, fmt.Sprintf("Purpose: %s", input.Purpose))
	}
	if len(input.Tags) > 0 {
		parts = append(parts, fmt.Sprintf("Tags: %s", strings.Join(input.Tags, ", ")))
	}
	if input.Notes != "" {
		parts = append(parts, fmt.Sprintf("Notes: %s", input.Notes))
	}
	if input.FocusText != "" {
		parts = append(parts, fmt.Sprintf("Focus: %s", input.FocusText))
	}

	if len(parts) == 0 {
		return "/compact"
	}

	var sb strings.Builder
	sb.WriteString("/compact Preserve the following context during compaction:\n")
	for _, p := range parts {
		sb.WriteString("- ")
		sb.WriteString(p)
		sb.WriteString("\n")
	}
	return strings.TrimRight(sb.String(), "\n")
}

// promptDir returns the compaction-prompts directory under baseDir.
func promptDir(baseDir string) string {
	return filepath.Join(baseDir, "compaction-prompts")
}

// promptFilePattern matches "<session>-compact-<N>.md"
var promptFilePattern = regexp.MustCompile(`-compact-(\d+)\.md$`)

// PromptAllocation is an exclusively-created prompt audit record.
type PromptAllocation struct {
	Number int
	Path   string
}

// NextPromptNumber scans legacy display-name-keyed prompts for the next number.
// Delivery callers use AllocatePromptExclusive to close the scan/write race.
func NextPromptNumber(baseDir, sessionName string) (int, error) {
	if err := validateStorageKey("session name", sessionName); err != nil {
		return 0, err
	}
	dir := promptDir(baseDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 1, nil
		}
		return 0, fmt.Errorf("read prompt dir: %w", err)
	}

	prefix := sessionName + "-compact-"
	maxN := 0
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), prefix) {
			continue
		}
		matches := promptFilePattern.FindStringSubmatch(e.Name())
		if len(matches) == 2 {
			n, parseErr := strconv.Atoi(matches[1])
			if parseErr != nil {
				continue
			}
			if n > maxN {
				maxN = n
			}
		}
	}
	if maxN == math.MaxInt {
		return 0, fmt.Errorf("prompt number exhausted for session %q", sessionName)
	}
	return maxN + 1, nil
}

// SavePrompt writes a legacy display-name-keyed prompt audit file. Delivery
// callers use AllocatePromptExclusive so an existing audit is never truncated.
func SavePrompt(baseDir, sessionName string, promptNumber int, content string) (string, error) {
	if err := validateStorageKey("session name", sessionName); err != nil {
		return "", err
	}
	if promptNumber < 1 {
		return "", fmt.Errorf("prompt number must be positive")
	}
	dir := promptDir(baseDir)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("create prompt dir: %w", err)
	}
	filename := fmt.Sprintf("%s-compact-%d.md", sessionName, promptNumber)
	path := filepath.Join(dir, filename)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		return "", fmt.Errorf("write prompt file: %w", err)
	}
	return path, nil
}

// AllocatePromptExclusive durably creates one stable-session-ID-keyed prompt
// audit file. It uses O_EXCL and retries the number when another invocation wins
// the same candidate, so it never truncates another delivery's prompt.
func AllocatePromptExclusive(baseDir, sessionID, content string) (PromptAllocation, error) {
	if err := validateStorageKey("session ID", sessionID); err != nil {
		return PromptAllocation{}, err
	}
	dir := promptDir(baseDir)
	if err := fileutil.MkdirAllDurable(dir, 0o700); err != nil {
		return PromptAllocation{}, fmt.Errorf("create prompt dir: %w", err)
	}
	// MkdirAll intentionally preserves the mode of an existing directory. Prompt
	// audits can contain sensitive preservation context, so tighten a legacy or
	// pre-created directory before allocating a file and persist that metadata.
	// #nosec G302 -- directories require execute bits; 0700 is owner-only.
	if err := os.Chmod(dir, 0o700); err != nil {
		return PromptAllocation{}, fmt.Errorf("secure prompt dir: %w", err)
	}
	if err := fileutil.SyncDir(dir); err != nil {
		return PromptAllocation{}, fmt.Errorf("persist prompt dir permissions: %w", err)
	}
	number, err := NextPromptNumber(baseDir, sessionID)
	if err != nil {
		return PromptAllocation{}, err
	}
	for {
		filename := fmt.Sprintf("%s-compact-%d.md", sessionID, number)
		path := filepath.Join(dir, filename)
		file, openErr := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if errors.Is(openErr, os.ErrExist) {
			if number == math.MaxInt {
				return PromptAllocation{}, fmt.Errorf("prompt number exhausted for session %q", sessionID)
			}
			number++
			continue
		}
		if openErr != nil {
			return PromptAllocation{}, fmt.Errorf("allocate prompt file: %w", openErr)
		}
		if err := writeExclusivePrompt(file, path, content); err != nil {
			return PromptAllocation{}, err
		}
		return PromptAllocation{Number: number, Path: path}, nil
	}
}

func writeExclusivePrompt(file *os.File, path, content string) error {
	return writeExclusivePromptWithDirSync(file, path, content, fileutil.SyncDir)
}

func writeExclusivePromptWithDirSync(file *os.File, path, content string, syncDir func(string) error) error {
	remove := true
	defer func() {
		if remove {
			_ = os.Remove(path)
		}
	}()
	if _, err := file.WriteString(content); err != nil {
		_ = file.Close()
		return fmt.Errorf("write prompt file: %w", err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return fmt.Errorf("sync prompt file: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close prompt file: %w", err)
	}
	if err := syncDir(filepath.Dir(path)); err != nil {
		return fmt.Errorf("sync prompt directory: %w", err)
	}
	remove = false
	return nil
}
