package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/vbonnet/dear-agent/agm/internal/config"
)

func TestResolveDoctorHomePathRetainsLoadedAuthority(t *testing.T) {
	loaded, retainedHome := loadDoctorConfigThroughSymlinkedHome(t)

	driftHome := filepath.Join(t.TempDir(), "drift-home")
	if err := os.MkdirAll(driftHome, 0o700); err != nil {
		t.Fatalf("create drift HOME: %v", err)
	}
	t.Setenv("HOME", driftHome)
	t.Setenv("USERPROFILE", driftHome)

	got, err := resolveDoctorHomePath(loaded)
	if err != nil {
		t.Fatalf("resolveDoctorHomePath() error = %v", err)
	}
	if got != retainedHome {
		t.Fatalf("resolveDoctorHomePath() = %q, want retained physical HOME %q", got, retainedHome)
	}
}

func TestResolveDoctorHomePathRejectsMissingAuthority(t *testing.T) {
	unloaded := &config.Config{}
	for name, candidate := range map[string]*config.Config{
		"nil config":      nil,
		"unloaded config": unloaded,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := resolveDoctorHomePath(candidate)
			if !errors.Is(err, config.ErrRuntimeAuthorityUnavailable) {
				t.Fatalf("resolveDoctorHomePath() error = %v, want %v", err, config.ErrRuntimeAuthorityUnavailable)
			}
		})
	}
}

func TestGetDoctorSessionsDirAtHome(t *testing.T) {
	originalCfg, originalTestMode := cfg, doctorTestMode
	t.Cleanup(func() {
		cfg = originalCfg
		doctorTestMode = originalTestMode
	})

	home := t.TempDir()
	cfg = &config.Config{}
	doctorTestMode = false
	if got, want := getDoctorSessionsDirAtHome(home), filepath.Join(home, "sessions"); got != want {
		t.Fatalf("default sessions dir = %q, want %q", got, want)
	}

	doctorTestMode = true
	if got, want := getDoctorSessionsDirAtHome(home), filepath.Join(home, "sessions-test"); got != want {
		t.Fatalf("test sessions dir = %q, want %q", got, want)
	}

	doctorTestMode = false
	cfg.SessionsDir = filepath.Join(t.TempDir(), "configured-sessions")
	if got := getDoctorSessionsDirAtHome(home); got != cfg.SessionsDir {
		t.Fatalf("configured sessions dir = %q, want %q", got, cfg.SessionsDir)
	}
}

func TestGetDoctorSessionsDirHandlesMissingLiveHome(t *testing.T) {
	originalCfg, originalTestMode := cfg, doctorTestMode
	t.Cleanup(func() {
		cfg = originalCfg
		doctorTestMode = originalTestMode
	})
	t.Setenv("HOME", "")
	t.Setenv("USERPROFILE", "")
	doctorTestMode = false
	cfg = &config.Config{}

	if got, err := getDoctorSessionsDir(); err == nil || got != "" {
		t.Fatalf("getDoctorSessionsDir() = %q, %v; want empty path and HOME error", got, err)
	}

	cfg.SessionsDir = filepath.Join(t.TempDir(), "configured-sessions")
	if got, err := getDoctorSessionsDir(); err != nil || got != cfg.SessionsDir {
		t.Fatalf("configured getDoctorSessionsDir() = %q, %v; want %q, nil", got, err, cfg.SessionsDir)
	}
}

func TestRunInstallChecksUsesRetainedRuntimeHome(t *testing.T) {
	loaded, retainedHome := loadDoctorConfigThroughSymlinkedHome(t)
	home, err := resolveDoctorHomePath(loaded)
	if err != nil {
		t.Fatalf("resolveDoctorHomePath() error = %v", err)
	}
	if home != retainedHome {
		t.Fatalf("resolved doctor HOME = %q, want physical %q", home, retainedHome)
	}

	binDir := filepath.Join(t.TempDir(), "bin")
	if err := os.MkdirAll(binDir, 0o700); err != nil {
		t.Fatalf("create test bin directory: %v", err)
	}
	for _, name := range append(append([]string{}, requiredBinaries...), "go") {
		writeDoctorExecutable(t, filepath.Join(binDir, name))
	}
	t.Setenv("PATH", binDir)

	hooksDir := filepath.Join(home, ".claude", "hooks")
	hookPaths := []string{
		filepath.Join(hooksDir, "posttool-agm-state-notify"),
		filepath.Join(hooksDir, "pretool-test-session-guard"),
		filepath.Join(hooksDir, "pretool-agm-mode-tracker"),
		filepath.Join(hooksDir, "session-start", "agm-state-ready"),
		filepath.Join(hooksDir, "session-start", "agm-plan-continuity"),
	}
	for _, path := range hookPaths {
		writeDoctorExecutable(t, path)
	}
	settings := `{"hooks":{"test":[{"hooks":[` +
		`{"command":"~/.claude/hooks/posttool-agm-state-notify"},` +
		`{"command":"~/.claude/hooks/pretool-agm-mode-tracker"},` +
		`{"command":"~/.claude/hooks/pretool-test-session-guard"},` +
		`{"command":"~/.claude/hooks/session-start/agm-state-ready"},` +
		`{"command":"~/.claude/hooks/session-start/agm-plan-continuity"}` +
		`]}]}}`
	if err := os.WriteFile(filepath.Join(home, ".claude", "settings.json"), []byte(settings), 0o600); err != nil {
		t.Fatalf("write retained settings: %v", err)
	}

	driftHome := filepath.Join(t.TempDir(), "drift-home")
	if err := os.MkdirAll(driftHome, 0o700); err != nil {
		t.Fatalf("create drift HOME: %v", err)
	}
	t.Setenv("HOME", driftHome)
	t.Setenv("USERPROFILE", driftHome)

	if !runInstallChecks(home) {
		t.Fatal("runInstallChecks() ignored the retained runtime HOME")
	}
}

func loadDoctorConfigThroughSymlinkedHome(t *testing.T) (*config.Config, string) {
	t.Helper()
	root := t.TempDir()
	physicalHome := filepath.Join(root, "physical-home")
	if err := os.MkdirAll(physicalHome, 0o700); err != nil {
		t.Fatalf("create physical HOME: %v", err)
	}
	logicalHome := filepath.Join(root, "logical-home")
	if err := os.Symlink(physicalHome, logicalHome); err != nil {
		t.Skipf("symlinked HOME unsupported: %v", err)
	}
	t.Setenv("HOME", logicalHome)
	t.Setenv("USERPROFILE", logicalHome)
	loaded, err := config.Load("")
	if err != nil {
		t.Fatalf("config.Load() error = %v", err)
	}
	resolvedHome, err := filepath.EvalSymlinks(physicalHome)
	if err != nil {
		t.Fatalf("resolve physical HOME: %v", err)
	}
	return loaded, resolvedHome
}

func writeDoctorExecutable(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("create executable parent %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatalf("write executable %s: %v", path, err)
	}
}
