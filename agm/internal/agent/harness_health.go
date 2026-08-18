package agent

import (
	"os"
	"path/filepath"
)

// HarnessHealth is a per-harness health summary produced for `agm admin doctor`.
//
// AGM officially supports more than one harness (claude-code, codex-cli, AGY,
// opencode-cli, pi-cli, plus deprecated gemini-cli), but the doctor's session checks have historically
// been Claude-centric. HarnessHealth separates the concerns that can fail
// independently for any harness — the CLI binary, the auth/config, and the
// on-disk config directory — so the doctor can render every harness in use in
// a single pass without privileging Claude.
type HarnessHealth struct {
	Harness        string // harness name, e.g. "gemini-cli"
	Known          bool   // whether Harness is a recognised AGM harness
	BinaryName     string // CLI binary expected on PATH, e.g. "gemini" ("" if unknown harness)
	BinaryPresent  bool   // binary resolved on PATH
	AuthConfigured bool   // API key / OAuth / Vertex / reachable server present
	AuthHint       string // remediation hint when auth is missing
	ConfigDir      string // harness config/home dir ("" if none to sanity-check)
	ConfigDirFound bool   // config dir exists on disk
}

// IsHealthy reports whether the harness is usable: its binary is present and
// its auth is configured. Config-dir presence is advisory only — a freshly
// installed harness may not have written its directory yet — so it does not
// gate health.
func (h HarnessHealth) IsHealthy() bool {
	return h.BinaryPresent && h.AuthConfigured
}

// harnessHealthBinary derives CLI harnesses from the availability registry.
// OpenCode is the only doctor-specific exception: availability is based on its
// server, while doctor also reports whether the optional CLI is installed.
func harnessHealthBinary(harness string) (string, bool) {
	if harness == "opencode-cli" {
		return "opencode", true
	}
	binaries := harnessBinaries[harness]
	if len(binaries) == 0 {
		return "", false
	}
	return binaries[0], true
}

// harnessConfigDir returns the harness's on-disk config/home directory under
// the given home dir, or "" when the harness keeps no local directory the
// doctor can sanity-check (opencode-cli is server-based). Paths mirror the
// locations the adapters already use elsewhere in this package
// (e.g. ~/.gemini, ~/.codex/auth.json).
func harnessConfigDir(home, harness string) string {
	switch harness {
	case "claude-code":
		return filepath.Join(home, ".claude")
	case "gemini-cli":
		return filepath.Join(home, ".gemini")
	case "codex-cli":
		return filepath.Join(home, ".codex")
	case "agy":
		return filepath.Join(home, ".gemini", "antigravity-cli")
	case "pi-cli":
		return filepath.Join(home, ".pi", "agent")
	default:
		// opencode-cli is server-based; unknown harnesses have no known dir.
		return ""
	}
}

// CheckHarnessHealth inspects a single harness for `agm admin doctor`.
//
// It never returns an error: every failure mode is captured in the returned
// struct so the caller can report all harnesses in one pass. Auth is delegated
// to ValidateHarnessAvailability, which treats a harness whose binary is on
// PATH as available (the CLI manages its own auth) — keeping the Claude path's
// behaviour unchanged.
func CheckHarnessHealth(harness string) HarnessHealth {
	harness = NormalizeHarnessName(harness)
	h := HarnessHealth{Harness: harness}

	if bin, ok := harnessHealthBinary(harness); ok {
		h.Known = true
		h.BinaryName = bin
		if _, err := lookPath(bin); err == nil {
			h.BinaryPresent = true
		}
	}

	if err := ValidateHarnessAvailability(harness); err != nil {
		h.AuthHint = err.Error()
	} else {
		h.AuthConfigured = true
	}

	if home, err := os.UserHomeDir(); err == nil {
		if dir := harnessConfigDir(home, harness); dir != "" {
			h.ConfigDir = dir
			if _, statErr := os.Stat(dir); statErr == nil {
				h.ConfigDirFound = true
			}
		}
	}

	return h
}
