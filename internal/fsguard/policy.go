package fsguard

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// Environment variables that configure the policy at runtime. They let an
// operator widen or tighten the sandbox without recompiling — e.g. to protect
// an additional source tree on a different host, or to redirect the violation
// log. All path values support ~ and $HOME expansion and may be separated by
// the OS path-list separator (":" on Unix) or commas.
const (
	// EnvConfig points at a JSON config file (see Config). Loaded first; env
	// vars below layer on top of it.
	EnvConfig = "FSGUARD_CONFIG"
	// EnvProtected adds protected paths (writes blocked) on top of the defaults.
	EnvProtected = "FSGUARD_PROTECTED_PATHS"
	// EnvWritable adds always-writable carve-outs on top of the defaults.
	EnvWritable = "FSGUARD_WRITABLE_PATHS"
	// EnvWorktreesDir overrides the writable worktree root (default ~/worktrees).
	EnvWorktreesDir = "FSGUARD_WORKTREES_DIR"
	// EnvLog overrides the violation log path (default ~/.fsguard/violations.jsonl).
	EnvLog = "FSGUARD_LOG"
	// EnvDisableLog, when non-empty, disables violation logging entirely.
	EnvDisableLog = "FSGUARD_DISABLE_LOG"
)

// Policy is the configurable classification policy: which roots are writable
// and which are protected. A zero Policy is invalid; build one with
// DefaultPolicy and (optionally) layer config/env on top via LoadConfig.
//
// Classification precedence (see Guard.Classify) is: Writable carve-outs win
// over everything, then WorktreesDir is allowed, then Protected roots are
// blocked, then the home-dotfile rule, then a default deny.
type Policy struct {
	// WorktreesDir is the writable root for all agent work (default ~/worktrees).
	WorktreesDir string `json:"worktrees_dir"`
	// Protected roots block writes beneath them (default: ~/src). The ~/src
	// entry receives bespoke "create a worktree" guidance; other entries get a
	// generic protected-path message.
	Protected []string `json:"protected_paths"`
	// Writable carve-outs are always writable regardless of the policy: agent
	// scratch space and I/O plumbing necessary operations depend on. Defaults
	// cover /dev, the temp dirs, ~/.auto-memory, Cowork /sessions, and the
	// ~/beads tracker store.
	Writable []string `json:"writable_paths"`
}

// Config is the on-disk / env-resolved configuration: a Policy plus the
// violation log destination.
type Config struct {
	Policy  Policy `json:"-"`
	LogPath string `json:"log_path"`

	// Embedded policy fields for JSON decoding (a config file is flat, not
	// nested under "policy"). Decoded into the Policy by LoadConfig.
	WorktreesDir string   `json:"worktrees_dir"`
	Protected    []string `json:"protected_paths"`
	Writable     []string `json:"writable_paths"`
}

// DefaultPolicy returns the built-in worktree-only policy rooted at home. It
// preserves the historical carve-out set and adds the ~/beads tracker store so
// agents can run `bd`/`dolt` against the canonical Beads DB.
func DefaultPolicy(home string) Policy {
	return Policy{
		WorktreesDir: filepath.Join(home, "worktrees"),
		Protected:    []string{filepath.Join(home, "src")},
		Writable: []string{
			"/dev",
			filepath.Join(home, ".auto-memory"),
			"/tmp", "/private/tmp", "/var/tmp", "/private/var/tmp",
			"/var/folders", "/private/var/folders",
			"/sessions",
			filepath.Join(home, "beads"),
		},
	}
}

// DefaultLogPath is where violations are recorded unless overridden.
func DefaultLogPath(home string) string {
	return filepath.Join(home, ".fsguard", "violations.jsonl")
}

// LoadConfig builds the effective Config from defaults, an optional JSON config
// file (FSGUARD_CONFIG), and environment overrides, in that precedence order
// (later layers extend/override earlier ones).
//
// It never fails the caller into an unguarded state: if the config file is
// missing or malformed, it returns the default-derived Config alongside the
// error so the caller can keep enforcing with safe defaults while surfacing the
// problem. Home resolution failure falls back to "/".
func LoadConfig() (Config, error) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		home = "/"
	}
	if resolved, rerr := filepath.EvalSymlinks(home); rerr == nil {
		home = resolved
	}
	home = filepath.Clean(home)

	pol := DefaultPolicy(home)
	cfg := Config{Policy: pol, LogPath: DefaultLogPath(home)}

	var loadErr error
	if path := os.Getenv(EnvConfig); path != "" {
		if fileErr := mergeConfigFile(&cfg, expandHome(path, home), home); fileErr != nil {
			// Keep safe defaults; report the error.
			loadErr = fileErr
		}
	}
	applyEnvOverrides(&cfg, home)
	return cfg, loadErr
}

// mergeConfigFile reads a JSON config file and layers it onto cfg: list fields
// (protected/writable) are appended, scalar fields (worktrees_dir, log_path)
// override when non-empty. All paths are home-expanded and cleaned.
func mergeConfigFile(cfg *Config, path, home string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var fileCfg Config
	if err := json.Unmarshal(data, &fileCfg); err != nil {
		return err
	}
	if fileCfg.WorktreesDir != "" {
		cfg.Policy.WorktreesDir = expandHome(fileCfg.WorktreesDir, home)
	}
	cfg.Policy.Protected = append(cfg.Policy.Protected, expandAll(fileCfg.Protected, home)...)
	cfg.Policy.Writable = append(cfg.Policy.Writable, expandAll(fileCfg.Writable, home)...)
	if fileCfg.LogPath != "" {
		cfg.LogPath = expandHome(fileCfg.LogPath, home)
	}
	return nil
}

// applyEnvOverrides layers FSGUARD_* environment variables onto cfg.
func applyEnvOverrides(cfg *Config, home string) {
	if v := os.Getenv(EnvWorktreesDir); v != "" {
		cfg.Policy.WorktreesDir = expandHome(v, home)
	}
	cfg.Policy.Protected = append(cfg.Policy.Protected, expandAll(splitList(os.Getenv(EnvProtected)), home)...)
	cfg.Policy.Writable = append(cfg.Policy.Writable, expandAll(splitList(os.Getenv(EnvWritable)), home)...)
	if v := os.Getenv(EnvLog); v != "" {
		cfg.LogPath = expandHome(v, home)
	}
	if os.Getenv(EnvDisableLog) != "" {
		cfg.LogPath = ""
	}
}

// splitList splits a path list on the OS list separator, commas, and newlines,
// trimming blanks.
func splitList(s string) []string {
	if s == "" {
		return nil
	}
	fields := strings.FieldsFunc(s, func(r rune) bool {
		return r == os.PathListSeparator || r == ',' || r == '\n'
	})
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		if f = strings.TrimSpace(f); f != "" {
			out = append(out, f)
		}
	}
	return out
}

// expandHome resolves a leading ~ or $HOME to home and cleans the result.
func expandHome(p, home string) string {
	switch {
	case p == "~" || p == "$HOME" || p == "${HOME}":
		return filepath.Clean(home)
	case strings.HasPrefix(p, "~/"):
		p = home + p[1:]
	case strings.HasPrefix(p, "$HOME/"):
		p = home + p[len("$HOME"):]
	case strings.HasPrefix(p, "${HOME}/"):
		p = home + p[len("${HOME}"):]
	}
	return filepath.Clean(p)
}

func expandAll(paths []string, home string) []string {
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		out = append(out, expandHome(p, home))
	}
	return out
}
