// Package claudetrust pre-authorizes the workspaces AGM launches Claude Code
// in, so the interactive trust dialog never gates an unattended spawn.
//
// Claude Code records per-directory trust in ~/.claude.json under
// projects.<dir>.hasTrustDialogAccepted, keyed by the *resolved* working
// directory. AGM hands sandboxed sessions a path under the sandbox's "merged"
// symlink, so seeding the path AGM knows about leaves the path Claude actually
// looks up untrusted — see SPEC.md.
package claudetrust

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// trustKey is the field Claude Code reads to decide whether to show the
// directory trust dialog.
const trustKey = "hasTrustDialogAccepted"

// configLockTimeout bounds the wait for the config lock. Claude itself rewrites
// ~/.claude.json on exit and concurrent spawns contend for it, so the write is
// serialized; a spawn should fail fast rather than hang on a stuck lock.
const configLockTimeout = 10 * time.Second

// SeedWorkspaceTrust pre-authorizes workDir in the user's Claude Code config and
// returns the path that was actually trusted.
//
// The returned path is the caller's receipt: it is the key Claude will look up,
// which is not necessarily the path that was passed in.
func SeedWorkspaceTrust(workDir string) (string, error) {
	configPath, err := ConfigPath()
	if err != nil {
		return "", err
	}
	return seedWorkspaceTrustAt(configPath, workDir)
}

// ConfigPath returns the Claude Code config file AGM seeds trust in.
func ConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory for Claude config: %w", err)
	}
	return filepath.Join(home, ".claude.json"), nil
}

func seedWorkspaceTrustAt(configPath, workDir string) (string, error) {
	// Resolve before anything else: this is the difference between trusting the
	// directory Claude will run in and trusting a symlink pointing at it.
	// EvalSymlinks also fails when the directory does not exist, which is worth
	// surfacing — an absent workspace means the launch is already broken.
	resolved, err := filepath.EvalSymlinks(workDir)
	if err != nil {
		return "", fmt.Errorf("resolve workspace %q for trust seeding: %w", workDir, err)
	}
	resolved, err = filepath.Abs(resolved)
	if err != nil {
		return "", fmt.Errorf("absolutize workspace %q for trust seeding: %w", resolved, err)
	}

	unlock, err := lockConfig(configPath)
	if err != nil {
		return "", err
	}
	defer unlock()

	config, err := readConfig(configPath)
	if err != nil {
		return "", err
	}

	projects, _ := config["projects"].(map[string]any)
	if projects == nil {
		projects = map[string]any{}
	}
	entry, _ := projects[resolved].(map[string]any)
	if entry == nil {
		entry = map[string]any{}
	}
	if entry[trustKey] == true {
		return resolved, nil
	}
	entry[trustKey] = true
	projects[resolved] = entry
	config["projects"] = projects

	if err := writeConfig(configPath, config); err != nil {
		return "", err
	}
	return resolved, nil
}

// lockConfig serializes the read-modify-write against concurrent spawns, which
// is the realistic case: the supervisor mesh brings sessions up in parallel and
// each one seeds its own workspace into the same file. Without the lock the
// last writer wins and the other sessions launch untrusted.
func lockConfig(configPath string) (func(), error) {
	lockPath := configPath + ".agm-trust.lock"
	file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open Claude config lock %q: %w", lockPath, err)
	}
	deadline := time.Now().Add(configLockTimeout)
	for {
		err = syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return func() {
				_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
				_ = file.Close()
			}, nil
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) || time.Now().After(deadline) {
			_ = file.Close()
			return nil, fmt.Errorf("lock Claude config %q after %v: %w", configPath, configLockTimeout, err)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// readConfig loads the config as a generic map so every field AGM does not know
// about survives the round trip. A missing file is a fresh config; a corrupt one
// is an error, because overwriting it would discard the user's account state.
func readConfig(configPath string) (map[string]any, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]any{}, nil
		}
		return nil, fmt.Errorf("read Claude config %q: %w", configPath, err)
	}
	if len(data) == 0 {
		return map[string]any{}, nil
	}
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("parse Claude config %q (refusing to overwrite it): %w", configPath, err)
	}
	if config == nil {
		config = map[string]any{}
	}
	return config, nil
}

// writeConfig replaces the config atomically. A partial write here would corrupt
// the file Claude reads on every start, for every project.
func writeConfig(configPath string, config map[string]any) error {
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("encode Claude config: %w", err)
	}
	temp, err := os.CreateTemp(filepath.Dir(configPath), ".claude.json.agm-*")
	if err != nil {
		return fmt.Errorf("create temporary Claude config: %w", err)
	}
	tempPath := temp.Name()
	defer func() { _ = os.Remove(tempPath) }()

	if _, err := temp.Write(data); err != nil {
		_ = temp.Close()
		return fmt.Errorf("write temporary Claude config: %w", err)
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return fmt.Errorf("sync temporary Claude config: %w", err)
	}
	// Set the mode on the open descriptor, before Close. Chmod'ing tempPath
	// after the handle is gone is a path-based operation on a name that no
	// longer has to refer to the file we wrote (TOCTOU); fchmod cannot be
	// redirected that way.
	if err := temp.Chmod(configFileMode(configPath)); err != nil {
		_ = temp.Close()
		return fmt.Errorf("set permissions on temporary Claude config: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close temporary Claude config: %w", err)
	}
	if err := os.Rename(tempPath, configPath); err != nil {
		return fmt.Errorf("replace Claude config %q: %w", configPath, err)
	}
	return nil
}

// configFileMode keeps the existing file's permissions when there is one; a new
// config is created private, since it carries account state.
func configFileMode(configPath string) os.FileMode {
	if info, err := os.Stat(configPath); err == nil {
		return info.Mode().Perm()
	}
	return 0o600
}
