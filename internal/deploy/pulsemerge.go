package deploy

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"syscall"

	"github.com/vbonnet/dear-agent/pkg/recoveryloop"
)

// MergeRequiredPulses adds to the host's absence-alarm pulse config any pulse
// that is REQUIRED, defined in the repository defaults, and missing from the
// host, and returns the names it added.
//
// Required means named by a recovery job. That narrowing is the whole reason
// this is safe to run unattended: a default probe an operator switched off
// stays off, because nothing depends on it, while a pulse some job's health is
// judged by cannot be silently absent. Without it, "missing from the host" on
// a machine that predates this code is indistinguishable from "deliberately
// removed", and every sync would reactivate probes somebody turned off.
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
func MergeRequiredPulses(hostPath, defaultsPath string, required map[string]bool) ([]string, error) {
	// Two post-merge deployments from different worktrees can run at once, and
	// the registry plus its ledger are a read-modify-write pair. Interleaving
	// them would let one run's additions be dropped by the other's write while
	// both ledgers record the names as offered, so neither run would ever add
	// them again: the pulses would be permanently missing and permanently
	// believed installed. One exclusive lock covers both files.
	unlock, err := lockPulseRegistry(hostPath)
	if err != nil {
		return nil, err
	}
	defer unlock()

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
		if !wantsPulse(pulseName(p), have, seen, required) {
			continue
		}
		host.Pulses = append(host.Pulses, p)
		added = append(added, pulseName(p))
	}

	if len(added) == 0 {
		// Nothing to install. The ledger still advances to the defaults this
		// host already HAS, so that removing one later is recognised as a
		// removal rather than as a host that predates the pulse.
		if err := savePulseLedger(hostPath, defaultNamesPresent(defaults, have)); err != nil {
			return nil, err
		}
		return nil, nil
	}

	merged, err := json.MarshalIndent(host, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode merged pulses: %w", err)
	}
	if err := writePulseDoc(hostPath, append(merged, '\n')); err != nil {
		return nil, err
	}
	// The ledger advances only after the registry is live, and records only
	// names now genuinely present in it. Recording an intent first would mean
	// a crash between the two writes leaves those names permanently in `seen`
	// while absent from the registry, so every later merge skips them and the
	// recovery loop stays unwired: a durable false green written by a failed
	// write.
	present := defaultNamesPresent(defaults, pulseNameSet(host.Pulses))
	if err := savePulseLedger(hostPath, present); err != nil {
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
func savePulseLedger(hostPath string, offered []string) error {
	seen, err := loadPulseLedger(hostPath)
	if err != nil {
		return err
	}
	for _, n := range offered {
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
	names := pulseNames(defaults.Pulses)
	if err := savePulseLedger(hostPath, names); err != nil {
		return nil, err
	}
	return names, nil
}

// RequiredPulseNames returns every pulse name a recovery job depends on.
//
// These are the only pulses this code will install unattended. A pulse nothing
// judges a job by is the operator's business; a pulse some job's health is
// judged by cannot be silently absent, because the recovery loop then has no
// evidence either way about that job, forever.
func RequiredPulseNames(repoRoot string) (map[string]bool, error) {
	return RequiredPulseNamesFrom(filepath.Join(repoRoot, "deploy", "recovery-loop", "jobs.json"))
}

// RequiredPulseNamesFrom reads the job registry at an explicit path, so a
// --manifest override that relocates or customises it is honoured instead of
// this code consulting a registry the caller never selected.
func RequiredPulseNamesFrom(jobsPath string) (map[string]bool, error) {
	required := make(map[string]bool)
	for _, j := range recoveryloop.DefaultJobs() {
		if j.Pulse != "" {
			required[j.Pulse] = true
		}
	}

	raw, err := os.ReadFile(jobsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return required, nil
		}
		return nil, fmt.Errorf("read recovery job registry: %w", err)
	}
	var doc struct {
		Jobs []recoveryloop.Job `json:"jobs"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("parse recovery job registry: %w", err)
	}
	for _, j := range doc.Jobs {
		if j.Pulse != "" {
			required[j.Pulse] = true
		}
	}
	return required, nil
}

// PendingPulseMerges reports which required pulses a sync would add, without
// writing anything, so `status` and `--dry-run` can surface a migration that
// has not happened yet.
func PendingPulseMerges(hostPath, defaultsPath string, required map[string]bool) ([]string, error) {
	defaultsRaw, err := os.ReadFile(defaultsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read pulse defaults %s: %w", defaultsPath, err)
	}
	var defaults pulseDoc
	if err := json.Unmarshal(defaultsRaw, &defaults); err != nil {
		return nil, fmt.Errorf("parse pulse defaults %s: %w", defaultsPath, err)
	}

	hostRaw, err := os.ReadFile(hostPath)
	if err != nil {
		if os.IsNotExist(err) {
			return pulseNames(defaults.Pulses), nil
		}
		return nil, fmt.Errorf("read host pulses %s: %w", hostPath, err)
	}
	var host pulseDoc
	if err := json.Unmarshal(hostRaw, &host); err != nil {
		return nil, fmt.Errorf("parse host pulses %s: %w", hostPath, err)
	}
	have := pulseNameSet(host.Pulses)
	seen, err := loadPulseLedger(hostPath)
	if err != nil {
		return nil, err
	}
	var pending []string
	for _, p := range defaults.Pulses {
		if n := pulseName(p); wantsPulse(n, have, seen, required) {
			pending = append(pending, n)
		}
	}
	return pending, nil
}

// defaultNamesPresent returns the default pulse names the host registry
// currently contains. Only these are safe to record as offered: a name in the
// ledger but absent from the registry would be skipped forever.
// wantsPulse reports whether a default pulse should be installed on this host:
// required by a recovery job, not already present, and never offered before
// (an offered-then-absent pulse was removed deliberately).
func wantsPulse(name string, have, seen, required map[string]bool) bool {
	return name != "" && required[name] && !have[name] && !seen[name]
}

func defaultNamesPresent(defaults pulseDoc, have map[string]bool) []string {
	var names []string
	for _, p := range defaults.Pulses {
		if n := pulseName(p); n != "" && have[n] {
			names = append(names, n)
		}
	}
	return names
}

// lockPulseRegistry takes an exclusive advisory lock covering the pulse
// registry and its ledger, and returns the release function.
func lockPulseRegistry(hostPath string) (func(), error) {
	if err := os.MkdirAll(filepath.Dir(hostPath), 0o755); err != nil {
		return nil, fmt.Errorf("mkdir for pulse lock: %w", err)
	}
	path := hostPath + ".lock"
	//nolint:gosec // the lock path is derived from the manifest-resolved registry path.
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open pulse lock %s: %w", path, err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		f.Close()
		return nil, fmt.Errorf("lock pulse registry %s: %w", path, err)
	}
	return func() {
		// Closing the descriptor releases the flock. The lock file carries no
		// data, but a close error still means the descriptor state is not what
		// this code believes, so it is reported rather than dropped (CodeQL:
		// writable handle closed without error handling).
		if unlockErr := syscall.Flock(int(f.Fd()), syscall.LOCK_UN); unlockErr != nil {
			fmt.Fprintf(os.Stderr, "deploy: unlock pulse registry %s: %v\n", path, unlockErr)
		}
		if closeErr := f.Close(); closeErr != nil {
			fmt.Fprintf(os.Stderr, "deploy: close pulse lock %s: %v\n", path, closeErr)
		}
	}, nil
}
