package agent

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/BurntSushi/toml"
)

// Codex refuses to work in directories it does not consider trusted. For the
// interactive TUI (which is what every AGM tmux launch runs) an untrusted
// directory blocks startup on an interactive "Do you trust the contents of
// this directory?" onboarding prompt, and for `codex exec` it is a hard exit
// ("Not inside a trusted directory and --skip-git-repo-check was not
// specified"). Fresh sandbox merged/ dirs are neither git repos nor recorded
// in Codex's trusted-projects config, so codex-harness supervisors/workers
// brick on spawn (ce-cmsq, observed 2026-07-03).
//
// The `--skip-git-repo-check` flag is NOT a usable fix for AGM's launches: it
// exists only on `codex exec` — the interactive TUI and `codex resume` reject
// it with a clap "unexpected argument" error (verified against codex-cli
// 0.142.5), which would brick every launch including healthy ones. Git-initing
// the sandbox is also insufficient: Codex records a trust decision per project
// root even for git repos.
//
// The mechanism Codex itself uses when a user answers "Yes, continue" is a
// per-directory entry in $CODEX_HOME/config.toml:
//
//	[projects."/abs/work/dir"]
//	trust_level = "trusted"
//
// EnsureCodexWorkdirTrusted writes that entry ahead of the launch so the TUI
// starts straight into the composer. The workspace-write sandbox still
// confines what the launched agent may touch; trust only gates startup and
// project-local config loading.
var codexTrustMu sync.Mutex

// codexConfigPath returns $CODEX_HOME/config.toml, defaulting CODEX_HOME to
// ~/.codex exactly like the codex CLI does.
func codexConfigPath() (string, error) {
	if home := os.Getenv("CODEX_HOME"); home != "" {
		return filepath.Join(home, "config.toml"), nil
	}
	userHome, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir for codex config: %w", err)
	}
	return filepath.Join(userHome, ".codex", "config.toml"), nil
}

// EnsureCodexWorkdirTrusted records workDir as a trusted Codex project in
// $CODEX_HOME/config.toml (default ~/.codex/config.toml) so a Codex launch in
// that directory does not block on the interactive trust prompt (ce-cmsq).
//
// Behavior:
//   - workDir is resolved to an absolute path ("" and "." mean the cwd).
//   - If the config already has ANY projects entry for the directory — trusted
//     or not — it is left untouched: an explicit "untrusted" record is a user
//     decision AGM must not overturn.
//   - Otherwise a trust entry is appended, preserving the existing config
//     bytes, and the file is replaced atomically (temp file + rename).
//   - If the existing config fails to parse, the file is left untouched and an
//     error is returned; appending to a broken config could mask the problem
//     and duplicate-table appends would break codex entirely.
//
// Failures are returned for the caller to log; callers should treat this as
// best-effort and still attempt the launch (a git-repo workdir the user
// already trusted works without any entry).
func EnsureCodexWorkdirTrusted(workDir string) error {
	if workDir == "" {
		workDir = "."
	}
	abs, err := filepath.Abs(workDir)
	if err != nil {
		return fmt.Errorf("resolve codex workdir %q: %w", workDir, err)
	}

	configPath, err := codexConfigPath()
	if err != nil {
		return err
	}

	// Serialize writers within this process; cross-process collisions are
	// guarded by the sentinel lock below.
	codexTrustMu.Lock()
	defer codexTrustMu.Unlock()

	unlock, err := acquireCodexConfigLock(configPath)
	if err != nil {
		return err
	}
	defer unlock()

	data, err := os.ReadFile(configPath) // #nosec G304 -- path derived from CODEX_HOME/homedir, not user input.
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("read codex config %s: %w", configPath, err)
	}

	trusted, err := codexConfigHasProjectEntry(data, abs)
	if err != nil {
		return fmt.Errorf("parse codex config %s: %w", configPath, err)
	}
	if trusted {
		return nil
	}

	updated := appendCodexTrustEntry(data, abs)
	return writeFileAtomic(configPath, updated)
}

// codexConfigHasProjectEntry reports whether config data already contains a
// projects table for dir (any trust level).
func codexConfigHasProjectEntry(data []byte, dir string) (bool, error) {
	if len(data) == 0 {
		return false, nil
	}
	var cfg struct {
		Projects map[string]struct {
			TrustLevel string `toml:"trust_level"`
		} `toml:"projects"`
	}
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return false, err
	}
	_, ok := cfg.Projects[dir]
	return ok, nil
}

// appendCodexTrustEntry returns data with a trusted projects entry for dir
// appended, matching the format codex itself writes.
func appendCodexTrustEntry(data []byte, dir string) []byte {
	var b strings.Builder
	b.Write(data)
	if len(data) > 0 && data[len(data)-1] != '\n' {
		b.WriteByte('\n')
	}
	if len(data) > 0 {
		b.WriteByte('\n')
	}
	fmt.Fprintf(&b, "[projects.%s]\ntrust_level = \"trusted\"\n", tomlQuoteKey(dir))
	return []byte(b.String())
}

// tomlQuoteKey renders s as a TOML basic-string key segment.
func tomlQuoteKey(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`)
	return `"` + r.Replace(s) + `"`
}

// acquireCodexConfigLock takes a best-effort cross-process sentinel lock next
// to the config file. A concurrent duplicate append would produce a duplicate
// TOML table, which breaks every subsequent codex launch — worth a lock.
func acquireCodexConfigLock(configPath string) (func(), error) {
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		return nil, fmt.Errorf("create codex config dir: %w", err)
	}
	lockPath := configPath + ".agm-lock"
	for {
		f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600) // #nosec G304 -- sibling of the config path.
		if err == nil {
			_ = f.Close()
			return func() { _ = os.Remove(lockPath) }, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, fmt.Errorf("acquire codex config lock: %w", err)
		}
		// A stale lock (crashed holder) must not brick launches forever.
		// Staleness is judged by the lock file's own age, not this waiter's
		// wait time: a per-waiter deadline could steal a lock another waiter
		// just legitimately acquired.
		if fi, statErr := os.Stat(lockPath); statErr == nil && time.Since(fi.ModTime()) > 2*time.Second {
			_ = os.Remove(lockPath)
		}
		time.Sleep(25 * time.Millisecond)
	}
}

// writeFileAtomic replaces path with data via a same-directory temp file and
// rename, so readers never observe a torn config.
func writeFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".agm-*")
	if err != nil {
		return fmt.Errorf("create temp codex config: %w", err)
	}
	tmpName := tmp.Name()
	defer func() {
		_ = os.Remove(tmpName) // no-op after successful rename
	}()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temp codex config: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp codex config: %w", err)
	}
	if err := os.Chmod(tmpName, 0o600); err != nil {
		return fmt.Errorf("chmod temp codex config: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("replace codex config: %w", err)
	}
	return nil
}
