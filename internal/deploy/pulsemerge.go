package deploy

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
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
		return seedPulseConfig(hostPath, defaultsRaw, defaults)
	}

	var host pulseDoc
	if err := json.Unmarshal(hostRaw, &host); err != nil {
		return nil, fmt.Errorf("parse host pulses %s (refusing to overwrite): %w", hostPath, err)
	}

	have := pulseNameSet(host.Pulses)

	// "Missing from the host" is ambiguous on its own: it means either that
	// this host predates the pulse or that an operator turned it off. The
	// ledger records which defaults this host has already been offered, so a
	// name that was offered and is now absent was removed deliberately and is
	// left alone. Re-adding it on every sync would silently reactivate probes
	// somebody switched off.
	seen, err := loadPulseLedger(hostPath)
	if err != nil {
		return nil, err
	}

	var added []string
	for _, p := range defaults.Pulses {
		name := pulseName(p)
		if name == "" || have[name] || seen[name] {
			continue
		}
		host.Pulses = append(host.Pulses, p)
		added = append(added, name)
	}

	// The ledger always advances to the current defaults, including on a
	// no-op run: the first run after this code ships adopts whatever the host
	// has now, and every removal after that is respected.
	if err := savePulseLedger(hostPath, defaults.Pulses); err != nil {
		return nil, err
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

func pulseNameSet(ps []map[string]any) map[string]bool {
	set := make(map[string]bool, len(ps))
	for _, p := range ps {
		set[pulseName(p)] = true
	}
	return set
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

// writePulseDoc routes through the package's stage, verify, activate path so a
// merged registry gets the same read-back hash check every other deployed
// artifact does (internal/deploy SPEC DEP-01). A mangled staged write must not
// be able to replace a working pulse registry with invalid JSON, which would
// make absence-alarm reject every subsequent tick.
func writePulseDoc(path string, data []byte) error {
	// deploy/manifest.yaml declares absence-alarm-pulses mode 0644: it is
	// world-readable configuration, and narrowing it here would drift from the
	// manifest the deploy status check compares against.
	return atomicWrite(path, data, 0o644, sha256hex(data))
}

// pulseLedgerPath is a sidecar next to the registry recording every default
// pulse this host has been offered.
func pulseLedgerPath(hostPath string) string {
	return hostPath + ".offered"
}

// loadPulseLedger reads the set of default pulses already offered to this host.
// A missing ledger is not an error: it means this host has never been merged,
// and the first run adopts the current defaults.
func loadPulseLedger(hostPath string) (map[string]bool, error) {
	raw, err := os.ReadFile(pulseLedgerPath(hostPath))
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]bool{}, nil
		}
		return nil, fmt.Errorf("read pulse ledger: %w", err)
	}
	var names []string
	//nolint:nilerr // A corrupt ledger degrades to "nothing offered yet", which
	// can only re-offer a default. Propagating this would let an unreadable
	// sidecar block a required pulse, which is the failure this file prevents.
	if err := json.Unmarshal(raw, &names); err != nil {
		return map[string]bool{}, nil
	}
	seen := make(map[string]bool, len(names))
	for _, n := range names {
		seen[n] = true
	}
	return seen, nil
}

// savePulseLedger records the current default pulse names, unioned with what
// was already offered so a pulse retired from the defaults is not forgotten
// and then resurrected if it returns.
func savePulseLedger(hostPath string, defaults []map[string]any) error {
	seen, err := loadPulseLedger(hostPath)
	if err != nil {
		return err
	}
	for _, n := range pulseNames(defaults) {
		seen[n] = true
	}
	names := make([]string, 0, len(seen))
	for n := range seen {
		names = append(names, n)
	}
	sort.Strings(names)
	raw, err := json.Marshal(names)
	if err != nil {
		return fmt.Errorf("encode pulse ledger: %w", err)
	}
	return atomicWrite(pulseLedgerPath(hostPath), raw, 0o644, sha256hex(raw))
}

// seedPulseConfig installs the defaults verbatim on a host that has no pulse
// registry yet.
func seedPulseConfig(hostPath string, defaultsRaw []byte, defaults pulseDoc) ([]string, error) {
	if err := writePulseDoc(hostPath, defaultsRaw); err != nil {
		return nil, err
	}
	if err := savePulseLedger(hostPath, defaults.Pulses); err != nil {
		return nil, err
	}
	return pulseNames(defaults.Pulses), nil
}
