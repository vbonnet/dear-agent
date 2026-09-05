package agent

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCheckHarnessHealth_BinaryPresence(t *testing.T) {
	clearClaudeEnv(t)
	// Only the claude binary is on PATH; gemini/codex/opencode are absent.
	mockLookPath(t, map[string]bool{"claude": true})
	home := t.TempDir()

	claude := CheckHarnessHealthAtHome("claude-code", home)
	if !claude.Known {
		t.Fatalf("claude-code should be a known harness")
	}
	if claude.BinaryName != "claude" {
		t.Errorf("BinaryName = %q, want %q", claude.BinaryName, "claude")
	}
	if !claude.BinaryPresent {
		t.Errorf("claude binary should be reported present")
	}
	// Binary on PATH ⇒ available (CLI manages its own auth), so healthy even
	// with no API key set.
	if !claude.AuthConfigured {
		t.Errorf("claude-code with binary on PATH should be auth-configured")
	}
	if !claude.IsHealthy() {
		t.Errorf("claude-code should be healthy when its binary is present")
	}

	gemini := CheckHarnessHealthAtHome("gemini-cli", home)
	if gemini.BinaryName != "gemini" {
		t.Errorf("BinaryName = %q, want %q", gemini.BinaryName, "gemini")
	}
	if gemini.BinaryPresent {
		t.Errorf("gemini binary should be reported absent")
	}
	if gemini.IsHealthy() {
		t.Errorf("gemini-cli should be unhealthy when its binary is absent and no API key is set")
	}
}

func TestCheckHarnessHealth_OpenCodeBinaryTracked(t *testing.T) {
	clearClaudeEnv(t)
	// opencode-cli is omitted from the availability map, but the doctor still
	// reports its binary. Verify the binary name is wired up.
	mockLookPath(t, map[string]bool{"opencode": true})

	oc := CheckHarnessHealthAtHome("opencode-cli", t.TempDir())
	if !oc.Known {
		t.Fatalf("opencode-cli should be a known harness")
	}
	if oc.BinaryName != "opencode" {
		t.Errorf("BinaryName = %q, want %q", oc.BinaryName, "opencode")
	}
	if !oc.BinaryPresent {
		t.Errorf("opencode binary should be reported present")
	}
	// Auth (server reachability) is environment-dependent; only assert that an
	// unreachable server yields a hint rather than a silent pass.
	if !oc.AuthConfigured && oc.AuthHint == "" {
		t.Errorf("unavailable opencode-cli should carry an AuthHint")
	}
}

func TestHarnessHealthUsesCanonicalBinaryRegistry(t *testing.T) {
	clearClaudeEnv(t)
	mockLookPath(t, map[string]bool{})
	home := t.TempDir()

	for harness, binaries := range harnessBinaries {
		if len(binaries) == 0 {
			t.Fatalf("canonical binary registry has no binaries for %q", harness)
		}
		h := CheckHarnessHealthAtHome(harness, home)
		if !h.Known || h.BinaryName != binaries[0] {
			t.Fatalf("doctor binary for %q = known %v name %q, want true %q", harness, h.Known, h.BinaryName, binaries[0])
		}
	}
}

func TestCheckHarnessHealth_AgyPresent(t *testing.T) {
	clearClaudeEnv(t)
	t.Setenv("GEMINI_API_KEY", "")
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	configDir := filepath.Join(home, ".gemini", "antigravity-cli")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("create AGY config directory: %v", err)
	}
	mockLookPath(t, map[string]bool{"agy": true})

	h := CheckHarnessHealthAtHome("agy", home)
	if !h.Known || h.BinaryName != "agy" || !h.BinaryPresent {
		t.Fatalf("AGY binary health = %+v", h)
	}
	if !h.AuthConfigured || !h.IsHealthy() {
		t.Fatalf("installed AGY should manage its own auth: %+v", h)
	}
	if h.ConfigDir != configDir || !h.ConfigDirFound {
		t.Fatalf("AGY config health = path %q found %v, want %q true", h.ConfigDir, h.ConfigDirFound, configDir)
	}
}

func TestCheckHarnessHealth_AgyAbsent(t *testing.T) {
	clearClaudeEnv(t)
	t.Setenv("GEMINI_API_KEY", "")
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	mockLookPath(t, map[string]bool{})

	h := CheckHarnessHealthAtHome("agy", home)
	if !h.Known || h.BinaryName != "agy" {
		t.Fatalf("AGY should remain a known harness when absent: %+v", h)
	}
	if h.BinaryPresent || h.AuthConfigured || h.IsHealthy() {
		t.Fatalf("absent unauthenticated AGY should be unhealthy: %+v", h)
	}
	if h.AuthHint == "" {
		t.Fatal("absent AGY should include an auth remediation hint")
	}
	wantConfigDir := filepath.Join(home, ".gemini", "antigravity-cli")
	if h.ConfigDir != wantConfigDir || h.ConfigDirFound {
		t.Fatalf("AGY absent config health = path %q found %v, want %q false", h.ConfigDir, h.ConfigDirFound, wantConfigDir)
	}
}

func TestCheckHarnessHealth_AgyAliases(t *testing.T) {
	clearClaudeEnv(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	mockLookPath(t, map[string]bool{"agy": true})

	for _, alias := range []string{"agy-cli", "antigravity"} {
		t.Run(alias, func(t *testing.T) {
			h := CheckHarnessHealthAtHome(alias, home)
			if h.Harness != "agy" || !h.Known || h.BinaryName != "agy" || !h.IsHealthy() {
				t.Fatalf("health for alias %q = %+v", alias, h)
			}
			wantConfigDir := filepath.Join(home, ".gemini", "antigravity-cli")
			if h.ConfigDir != wantConfigDir {
				t.Fatalf("config dir for alias %q = %q, want %q", alias, h.ConfigDir, wantConfigDir)
			}
		})
	}
}

func TestCheckHarnessHealth_UnknownHarness(t *testing.T) {
	clearClaudeEnv(t)
	mockLookPath(t, map[string]bool{})

	h := CheckHarnessHealthAtHome("totally-made-up", t.TempDir())
	if h.Known {
		t.Errorf("unknown harness should not be marked Known")
	}
	if h.BinaryName != "" {
		t.Errorf("unknown harness should have no BinaryName, got %q", h.BinaryName)
	}
	if h.IsHealthy() {
		t.Errorf("unknown harness should never be healthy")
	}
	if h.AuthHint == "" {
		t.Errorf("unknown harness should carry an AuthHint explaining the failure")
	}
}

func TestCheckHarnessHealth_GeminiAuthViaEnv(t *testing.T) {
	clearClaudeEnv(t)
	mockLookPath(t, map[string]bool{}) // no binaries on PATH
	t.Setenv("GEMINI_API_KEY", "test-key")

	gemini := CheckHarnessHealthAtHome("gemini-cli", t.TempDir())
	if !gemini.AuthConfigured {
		t.Errorf("gemini-cli should be auth-configured when GEMINI_API_KEY is set")
	}
	// Binary still absent ⇒ not healthy: a CLI harness needs its binary.
	if gemini.IsHealthy() {
		t.Errorf("gemini-cli should be unhealthy without its binary even when API key is set")
	}
}

func TestCheckHarnessHealthAtHomeUsesRetainedCodexHome(t *testing.T) {
	clearClaudeEnv(t)
	mockLookPath(t, map[string]bool{})
	t.Setenv("OPENAI_API_KEY", "")

	retainedHome := t.TempDir()
	driftHome := t.TempDir()
	codexDir := filepath.Join(retainedHome, ".codex")
	if err := os.MkdirAll(codexDir, 0o700); err != nil {
		t.Fatalf("create retained Codex directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(codexDir, "auth.json"), []byte("{}"), 0o600); err != nil {
		t.Fatalf("create retained Codex credentials: %v", err)
	}
	t.Setenv("HOME", driftHome)
	t.Setenv("USERPROFILE", driftHome)

	h := CheckHarnessHealthAtHome("codex-cli", retainedHome)
	if !h.AuthConfigured {
		t.Fatalf("retained Codex OAuth should configure auth: %+v", h)
	}
	if h.ConfigDir != codexDir || !h.ConfigDirFound {
		t.Fatalf("Codex config health = path %q found %v, want %q true", h.ConfigDir, h.ConfigDirFound, codexDir)
	}
}

func TestCheckHarnessHealthAtHomeRejectsRelativeHome(t *testing.T) {
	clearClaudeEnv(t)
	mockLookPath(t, map[string]bool{})
	t.Setenv("OPENAI_API_KEY", "")
	originalStat := statPath
	statCalls := 0
	statPath = func(string) (os.FileInfo, error) {
		statCalls++
		return nil, os.ErrNotExist
	}
	t.Cleanup(func() { statPath = originalStat })

	for _, home := range []string{"", "."} {
		h := CheckHarnessHealthAtHome("codex-cli", home)
		if h.AuthConfigured {
			t.Fatalf("CheckHarnessHealthAtHome(%q) unexpectedly configured Codex auth: %+v", home, h)
		}
		if h.ConfigDir != "" || h.ConfigDirFound {
			t.Fatalf("CheckHarnessHealthAtHome(%q) config = %q found %v, want no relative config path", home, h.ConfigDir, h.ConfigDirFound)
		}
	}
	if statCalls != 0 {
		t.Fatalf("non-absolute HOME triggered %d filesystem lookup(s), want none", statCalls)
	}
}

func TestHarnessConfigDir(t *testing.T) {
	home := "/home/tester"
	cases := map[string]string{
		"claude-code":  filepath.Join(home, ".claude"),
		"gemini-cli":   filepath.Join(home, ".gemini"),
		"codex-cli":    filepath.Join(home, ".codex"),
		"agy":          filepath.Join(home, ".gemini", "antigravity-cli"),
		"opencode-cli": "", // server-based, no local dir
		"unknown":      "",
	}
	for harness, want := range cases {
		if got := harnessConfigDir(home, harness); got != want {
			t.Errorf("harnessConfigDir(%q) = %q, want %q", harness, got, want)
		}
	}
}
