// Package codexarchive bridges AGM session archival to Codex CLI saved-session
// archival.
package codexarchive

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

const (
	defaultTimeout        = 30 * time.Second
	defaultRemoteEndpoint = "unix://"
	envRemote             = "AGM_CODEX_REMOTE"
	// #nosec G101 -- this is the name of an environment variable, not a credential.
	envRemoteToken = "AGM_CODEX_REMOTE_AUTH_TOKEN_ENV"
)

var (
	runCodexRemoteArchiveFn = runCodexRemoteArchive
)

// Result describes the Codex-side archive operation paired with AGM archive.
type Result struct {
	Skipped         bool
	AlreadyArchived bool
	Target          string
	TranscriptPath  string
}

// ArchiveManifest archives the Codex CLI saved session that belongs to m.
func ArchiveManifest(ctx context.Context, m *manifest.Manifest) (*Result, error) {
	if m == nil {
		return nil, errors.New("manifest is nil")
	}
	req := Request{
		Harness:          m.Harness,
		Name:             m.Name,
		AGMSessionID:     m.SessionID,
		WorkingDirectory: m.WorkingDirectory,
		ProjectDirectory: m.Context.Project,
	}
	if m.Codex != nil {
		req.CodexSessionID = m.Codex.SessionID
	}
	if m.Sandbox != nil {
		req.SandboxMergedPath = m.Sandbox.MergedPath
	}
	return Archive(ctx, req)
}

// Request contains the minimal session fields needed to map AGM archive to
// Codex's saved-session archive command.
type Request struct {
	Harness           string
	Name              string
	AGMSessionID      string
	CodexSessionID    string
	WorkingDirectory  string
	SandboxMergedPath string
	ProjectDirectory  string
}

// Archive invokes `codex archive` for Codex CLI sessions. It uses the Codex
// transcript metadata cwd to resolve the real Codex session UUID, because Codex
// thread names are generated and do not reliably match AGM session names.
func Archive(ctx context.Context, req Request) (*Result, error) {
	if req.Harness != "codex-cli" {
		return &Result{Skipped: true}, nil
	}

	if req.CodexSessionID != "" {
		if err := runCodexRemoteArchiveFn(ctx, req.CodexSessionID); err != nil {
			return nil, err
		}
		return &Result{Target: req.CodexSessionID}, nil
	}

	match, err := findSessionByCWD(req.candidateDirs())
	if err != nil {
		return nil, err
	}
	if match.alreadyArchived {
		return &Result{AlreadyArchived: true, Target: match.id, TranscriptPath: match.path}, nil
	}

	target := firstNonEmpty(match.id, req.Name, req.AGMSessionID)
	if target == "" {
		return nil, errors.New("could not determine Codex session archive target")
	}
	if err := runCodexArchive(ctx, target); err != nil {
		return nil, err
	}
	return &Result{Target: target, TranscriptPath: match.path}, nil
}

func (r Request) candidateDirs() []string {
	return uniqueCleanPaths([]string{
		r.WorkingDirectory,
		r.SandboxMergedPath,
		r.ProjectDirectory,
	})
}

func runCodexArchive(ctx context.Context, target string) error {
	return runCodexArchiveWithRemote(ctx, target, os.Getenv(envRemote))
}

func runCodexRemoteArchive(ctx context.Context, target string) error {
	return runCodexArchiveWithRemote(ctx, target, firstNonEmpty(os.Getenv(envRemote), defaultRemoteEndpoint))
}

func runCodexArchiveWithRemote(ctx context.Context, target, remote string) error {
	timeoutCtx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	args := []string{"archive"}
	if remote != "" {
		args = append(args, "--remote", remote)
		if tokenEnv := os.Getenv(envRemoteToken); tokenEnv != "" {
			args = append(args, "--remote-auth-token-env", tokenEnv)
		}
	}
	args = append(args, target)

	cmd := exec.CommandContext(timeoutCtx, "codex", args...) // #nosec G702 -- fixed executable; args are passed without a shell.
	output, err := cmd.CombinedOutput()
	if timeoutCtx.Err() != nil {
		return fmt.Errorf("codex archive timed out after %s", defaultTimeout)
	}
	if err != nil {
		out := strings.TrimSpace(string(output))
		if out != "" {
			return fmt.Errorf("codex archive %q failed: %w: %s", target, err, out)
		}
		return fmt.Errorf("codex archive %q failed: %w", target, err)
	}
	return nil
}

type codexMatch struct {
	id              string
	path            string
	mtime           time.Time
	alreadyArchived bool
}

func findSessionByCWD(candidateDirs []string) (codexMatch, error) {
	if len(candidateDirs) == 0 {
		return codexMatch{}, errors.New("cannot resolve Codex session: no AGM working directory recorded")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return codexMatch{}, fmt.Errorf("cannot resolve Codex home: %w", err)
	}
	codexHome := filepath.Join(home, ".codex")

	active, err := findInTree(filepath.Join(codexHome, "sessions"), candidateDirs, false)
	if err != nil {
		return codexMatch{}, err
	}
	if active.id != "" {
		return active, nil
	}

	archived, err := findInTree(filepath.Join(codexHome, "archived_sessions"), candidateDirs, true)
	if err != nil {
		return codexMatch{}, err
	}
	if archived.id != "" {
		return archived, nil
	}

	return codexMatch{}, fmt.Errorf("cannot resolve Codex session for AGM working directories: %s", strings.Join(candidateDirs, ", "))
}

func findInTree(root string, candidateDirs []string, archived bool) (codexMatch, error) {
	var best codexMatch
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil //nolint:nilerr // Ignore unreadable transcript entries and continue scanning.
		}
		if d.IsDir() || !strings.HasSuffix(path, ".jsonl") {
			return nil
		}
		id, cwd, ok := readSessionMeta(path)
		if !ok || !matchesAnyPath(cwd, candidateDirs) {
			return nil
		}
		info, statErr := d.Info()
		if statErr != nil {
			return nil //nolint:nilerr // Ignore unreadable transcript entries and continue scanning.
		}
		if best.id == "" || info.ModTime().After(best.mtime) {
			best = codexMatch{
				id:              id,
				path:            path,
				mtime:           info.ModTime(),
				alreadyArchived: archived,
			}
		}
		return nil
	})
	if os.IsNotExist(err) {
		return codexMatch{}, nil
	}
	return best, err
}

func readSessionMeta(path string) (id string, cwd string, ok bool) {
	f, err := os.Open(path)
	if err != nil {
		return "", "", false
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 2*1024*1024)
	for scanner.Scan() {
		var entry struct {
			Type    string `json:"type"`
			Payload struct {
				SessionID string `json:"session_id"`
				ID        string `json:"id"`
				CWD       string `json:"cwd"`
			} `json:"payload"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			continue
		}
		if entry.Type != "session_meta" {
			continue
		}
		id = firstNonEmpty(entry.Payload.SessionID, entry.Payload.ID)
		cwd = cleanPath(entry.Payload.CWD)
		return id, cwd, id != "" && cwd != ""
	}
	return "", "", false
}

func matchesAnyPath(path string, candidates []string) bool {
	path = cleanPath(path)
	return slices.Contains(candidates, path)
}

func uniqueCleanPaths(paths []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, p := range paths {
		p = cleanPath(p)
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	return out
}

func cleanPath(path string) string {
	if path == "" {
		return ""
	}
	return filepath.Clean(path)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
