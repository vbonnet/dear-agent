package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCompletion_ZshOutput checks that the zsh completion subcommand produces
// a valid zsh completion script (starts with the expected #compdef header).
func TestCompletion_ZshOutput(t *testing.T) {
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout = w

	rootCmd.SetArgs([]string{"completion", "zsh"})
	t.Cleanup(func() {
		rootCmd.SetArgs(nil)
		os.Stdout = orig
	})
	if err := rootCmd.Execute(); err != nil {
		w.Close()
		os.Stdout = orig
		t.Fatalf("completion zsh: %v", err)
	}

	w.Close()
	os.Stdout = orig

	var buf bytes.Buffer
	if _, err := buf.ReadFrom(r); err != nil {
		t.Fatalf("read pipe: %v", err)
	}
	out := buf.String()
	if !strings.HasPrefix(out, "#compdef agm") {
		t.Errorf("zsh completion output should start with '#compdef agm', got:\n%s", out[:min(80, len(out))])
	}
}

// TestCompletion_CacheFlag checks that --cache writes the completion script
// to ~/.cache/agm-completion.zsh (respecting XDG_CACHE_HOME if set).
func TestCompletion_CacheFlag(t *testing.T) {
	cacheDir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cacheDir)

	rootCmd.SetArgs([]string{"completion", "zsh", "--cache"})
	t.Cleanup(func() { rootCmd.SetArgs(nil) })

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("completion zsh --cache: %v", err)
	}

	dest := filepath.Join(cacheDir, "agm-completion.zsh")
	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("cache file not written: %v", err)
	}
	if !strings.HasPrefix(string(data), "#compdef agm") {
		t.Errorf("cached completion should start with '#compdef agm'")
	}
}

// TestCompletion_NoPersistentPreRunE verifies that the completion command
// does not trigger the root PersistentPreRunE (which would attempt DB/health
// initialisation). We detect this by checking that the config is NOT loaded
// (cfg remains nil after running completion).
func TestCompletion_NoPersistentPreRunE(t *testing.T) {
	// Save and restore global cfg so we don't pollute other tests.
	savedCfg := cfg
	t.Cleanup(func() { cfg = savedCfg })

	// Reset cfg to nil to detect if PersistentPreRunE ran.
	cfg = nil

	cacheDir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cacheDir)

	rootCmd.SetArgs([]string{"completion", "zsh", "--cache"})
	t.Cleanup(func() { rootCmd.SetArgs(nil) })

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("completion zsh: %v", err)
	}

	if cfg != nil {
		t.Error("completion command triggered PersistentPreRunE (cfg was initialised); " +
			"it should bypass the root hook to avoid DB/health-check overhead")
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
