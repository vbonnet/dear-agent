// agm-statusline is an extensible status line compositor for Claude Code.
//
// It replaces agm-statusline-capture with additional functionality:
//  1. Persists CC session JSON to /tmp/agm-context/{session_id}.json (existing behavior)
//  2. Runs provider scripts from ~/.config/agm/statusline/providers.d/
//  3. Composes their output into a single status line
//
// Provider protocol:
//   - Executables named NN-name (e.g., 10-orchestrator, 20-wayfinder)
//   - Receive CC session JSON on stdin
//   - Output a single line to stdout (ANSI colors OK, empty = skip)
//   - Exit 0 = include, non-zero = skip
//   - Must complete within 500ms
//   - Env vars: AGM_SESSION_NAME, AGM_SESSION_ID, AGM_WORKSPACE
//
// Performance target: <50ms total (excluding provider timeout budget).
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Overridable for testing.
var (
	statusLineDir   = "/tmp/agm-context"
	providersDir    = ""
	configPath      = ""
	providerTimeout = 500 * time.Millisecond
	cacheTTL        = 5 * time.Second
	timeNow         = time.Now // overridable for testing
)

// sessionData is the subset of CC's statusLine JSON we need.
type sessionData struct {
	SessionID   string `json:"session_id"`
	SessionName string `json:"session_name"`
	Workspace   struct {
		CurrentDir string `json:"current_dir"`
	} `json:"workspace"`
}

// config holds compositor configuration from config.yaml.
type config struct {
	Separator  string   `yaml:"separator"`
	TimeoutMs  int      `yaml:"timeout_ms"`
	Disable    []string `yaml:"disable"`
}

func defaultProvidersDir() string {
	if dir := os.Getenv("AGM_STATUSLINE_PROVIDERS_DIR"); dir != "" {
		return dir
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "agm", "statusline", "providers.d")
}

func defaultConfigPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "agm", "statusline", "config.yaml")
}

func loadConfig() config {
	cfg := config{Separator: " │ "}
	path := configPath
	if path == "" {
		path = defaultConfigPath()
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg
	}
	_ = yaml.Unmarshal(data, &cfg)
	if cfg.Separator == "" {
		cfg.Separator = " │ "
	}
	if cfg.TimeoutMs > 0 {
		providerTimeout = time.Duration(cfg.TimeoutMs) * time.Millisecond
	}
	return cfg
}

func main() {
	raw, sd, err := readAndPersist()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "agm-statusline: %v\n", err)
	}

	// Check provider output cache before running providers.
	if sd.SessionID != "" {
		if cached, ok := readCache(sd.SessionID); ok {
			fmt.Print(cached)
			return
		}
	}

	cfg := loadConfig()
	segments := runProviders(cfg, raw, sd)

	output := ""
	if len(segments) > 0 {
		output = strings.Join(segments, cfg.Separator)
	}

	// Cache the composed output for subsequent calls.
	if sd.SessionID != "" {
		writeCache(sd.SessionID, output)
	}

	if output != "" {
		fmt.Print(output)
	}
}

// cachePath returns the path to the provider output cache file for a session.
func cachePath(sessionID string) string {
	return filepath.Join(statusLineDir, sessionID+".statusline-cache")
}

// readCache returns cached provider output if the cache is fresh (within cacheTTL).
func readCache(sessionID string) (string, bool) {
	p := cachePath(sessionID)
	info, err := os.Stat(p)
	if err != nil {
		return "", false
	}
	if timeNow().Sub(info.ModTime()) > cacheTTL {
		return "", false
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return "", false
	}
	return string(data), true
}

// writeCache writes the composed provider output to the cache file.
func writeCache(sessionID, output string) {
	p := cachePath(sessionID)
	// Best-effort; don't fail the status line if caching fails.
	_ = os.MkdirAll(statusLineDir, 0o700)
	_ = os.WriteFile(p, []byte(output), 0o600)
}

// readAndPersist reads stdin, parses session data, and persists the raw JSON.
func readAndPersist() ([]byte, sessionData, error) {
	var sd sessionData
	raw, err := io.ReadAll(os.Stdin)
	if err != nil {
		return nil, sd, fmt.Errorf("read stdin: %w", err)
	}
	if len(raw) == 0 {
		return nil, sd, nil
	}

	if err := json.Unmarshal(raw, &sd); err != nil {
		return raw, sd, fmt.Errorf("parse JSON: %w", err)
	}

	if sd.SessionID != "" {
		if persistErr := persistJSON(sd.SessionID, raw); persistErr != nil {
			_, _ = fmt.Fprintf(os.Stderr, "agm-statusline: persist: %v\n", persistErr)
		}
	}

	return raw, sd, nil
}

// persistJSON atomically writes the raw JSON to the status line directory.
func persistJSON(sessionID string, raw []byte) error {
	if err := os.MkdirAll(statusLineDir, 0o700); err != nil {
		return fmt.Errorf("mkdir %s: %w", statusLineDir, err)
	}
	dst := filepath.Join(statusLineDir, sessionID+".json")
	tmp := dst + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return fmt.Errorf("write tmp: %w", err)
	}
	if err := os.Rename(tmp, dst); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename: %w", err)
	}
	return nil
}

// providerResult holds a provider's output and sort key.
type providerResult struct {
	name   string
	output string
}

// providerEntry captures a discovered provider script.
type providerEntry struct {
	name string
	path string
}

// discoverProviders enumerates executable provider scripts in dir, skipping
// directories, non-executables, and entries listed in disabled.
func discoverProviders(dir string, disabled map[string]bool) []providerEntry {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var providers []providerEntry
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if disabled[name] || disabled[stripPrefix(name)] {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.Mode()&0o111 == 0 {
			continue
		}
		providers = append(providers, providerEntry{name: name, path: filepath.Join(dir, name)})
	}
	sort.Slice(providers, func(i, j int) bool {
		return providers[i].name < providers[j].name
	})
	return providers
}

// runProviders discovers and executes provider scripts in parallel.
func runProviders(cfg config, raw []byte, sd sessionData) []string {
	dir := providersDir
	if dir == "" {
		dir = defaultProvidersDir()
	}

	disabled := make(map[string]bool, len(cfg.Disable))
	for _, d := range cfg.Disable {
		disabled[d] = true
	}

	providers := discoverProviders(dir, disabled)
	if len(providers) == 0 {
		return nil
	}

	// Run all providers in parallel.
	results := make([]providerResult, len(providers))
	done := make(chan int, len(providers))

	for i, p := range providers {
		go func(idx int, p providerEntry) {
			defer func() { done <- idx }()
			ctx, cancel := context.WithTimeout(context.Background(), providerTimeout)
			defer cancel()

			cmd := exec.CommandContext(ctx, p.path)
			cmd.Stdin = bytes.NewReader(raw)
			cmd.Env = append(os.Environ(),
				"AGM_SESSION_NAME="+sd.SessionName,
				"AGM_SESSION_ID="+sd.SessionID,
				"AGM_WORKSPACE="+sd.Workspace.CurrentDir,
			)

			out, err := cmd.Output()
			if err != nil {
				return
			}
			line := strings.TrimSpace(string(out))
			if line != "" {
				results[idx] = providerResult{name: p.name, output: line}
			}
		}(i, p)
	}

	// Wait for all providers.
	for range providers {
		<-done
	}

	// Collect non-empty results in order.
	var segments []string
	for _, r := range results {
		if r.output != "" {
			segments = append(segments, r.output)
		}
	}
	return segments
}

// stripPrefix removes NN- prefix from provider name.
func stripPrefix(name string) string {
	for i, c := range name {
		if c == '-' && i > 0 {
			return name[i+1:]
		}
		if c < '0' || c > '9' {
			break
		}
	}
	return name
}
