package deploy

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// MergeRequiredPulses adds to the host's absence-alarm pulse config any pulse
// that the repository defaults define and the host is missing, and returns the
// names it added.
//
// The deployed config is declared absent-only in the manifest: once a host has
// one, neither `dear-deploy` nor `make install-absence-alarm-launchagent`
// rewrites it. That protects operator customization, and it also means a newly
// required pulse reaches the repository and never reaches the running alarm.
// The monitor then believes it watches something nothing emits, which is the
// blindness the pulse was added to remove.
//
// Merging by name is deliberately conservative. An entry the host already has
// is left exactly as it is, including a widened window or an edited path: this
// supplies what is missing, it does not reassert defaults over local choices.
// A host config that cannot be parsed is refused rather than replaced, because
// overwriting it would discard configuration this code cannot read.
func MergeRequiredPulses(hostPath, defaultsPath string) ([]string, error) {
	defaultsRaw, err := os.ReadFile(defaultsPath)
	if err != nil {
		return nil, fmt.Errorf("read pulse defaults %s: %w", defaultsPath, err)
	}
	var defaults pulseDoc
	if err := json.Unmarshal(defaultsRaw, &defaults); err != nil {
		return nil, fmt.Errorf("parse pulse defaults %s: %w", defaultsPath, err)
	}

	hostRaw, err := os.ReadFile(hostPath)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("read host pulses %s: %w", hostPath, err)
		}
		// No host config yet: the defaults are the whole answer.
		if err := writePulseDoc(hostPath, defaultsRaw); err != nil {
			return nil, err
		}
		return pulseNames(defaults.Pulses), nil
	}

	var host pulseDoc
	if err := json.Unmarshal(hostRaw, &host); err != nil {
		return nil, fmt.Errorf("parse host pulses %s (refusing to overwrite): %w", hostPath, err)
	}

	have := make(map[string]bool, len(host.Pulses))
	for _, p := range host.Pulses {
		have[pulseName(p)] = true
	}

	var added []string
	for _, p := range defaults.Pulses {
		name := pulseName(p)
		if name == "" || have[name] {
			continue
		}
		host.Pulses = append(host.Pulses, p)
		added = append(added, name)
	}
	if len(added) == 0 {
		return nil, nil
	}

	merged, err := json.MarshalIndent(host, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode merged pulses: %w", err)
	}
	if err := writePulseDoc(hostPath, append(merged, '\n')); err != nil {
		return nil, err
	}
	return added, nil
}

// pulseDoc keeps every field of every pulse verbatim. Decoding into a typed
// struct would silently drop any key this binary does not know about, which on
// a rewrite would delete operator configuration.
type pulseDoc struct {
	Pulses []map[string]any `json:"pulses"`
}

func pulseName(p map[string]any) string {
	if n, ok := p["name"].(string); ok {
		return n
	}
	return ""
}

func pulseNames(ps []map[string]any) []string {
	names := make([]string, 0, len(ps))
	for _, p := range ps {
		if n := pulseName(p); n != "" {
			names = append(names, n)
		}
	}
	return names
}

// writePulseDoc stages and renames so a failed write leaves the previous
// config in place, matching the atomicity every other deploy path guarantees.
func writePulseDoc(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir for %s: %w", path, err)
	}
	tmp := path + ".tmp"
	//nolint:gosec // deploy/manifest.yaml declares this artifact mode 0644; it is
	// world-readable configuration, and narrowing it here would drift from the
	// manifest the deploy status check compares against.
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("stage %s: %w", path, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("activate %s: %w", path, err)
	}
	return nil
}
