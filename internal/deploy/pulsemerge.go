package deploy

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"

	"github.com/vbonnet/dear-agent/pkg/absencealarm"
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
func MergeRequiredPulses(
	hostPath, defaultsPath string,
	seedMode os.FileMode,
	required map[string]bool,
) ([]string, error) {
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
	return mergeRequiredPulses(hostPath, defaultsRaw, seedMode, required)
}

// PulseMergeResult reports both the names added and whether this invocation
// created the live registry. Created is decided while holding the registry
// lock, so concurrent fresh-host deployments cannot both claim installation.
type PulseMergeResult struct {
	Added   []string
	Created bool
}

// MergeRequiredPulsesRendered is MergeRequiredPulses over already-rendered
// defaults. It retains the compact names-only API for callers that do not need
// a locked creation receipt.
func MergeRequiredPulsesRendered(
	hostPath string,
	defaultsRaw []byte,
	seedMode os.FileMode,
	required map[string]bool,
) ([]string, error) {
	result, err := MergeRequiredPulsesRenderedResult(hostPath, defaultsRaw, seedMode, required)
	return result.Added, err
}

// MergeRequiredPulsesRenderedResult returns the full locked merge outcome for
// deployment reporting while installing the same rendered bytes as the
// manifest-selected artifact.
func MergeRequiredPulsesRenderedResult(
	hostPath string,
	defaultsRaw []byte,
	seedMode os.FileMode,
	required map[string]bool,
) (PulseMergeResult, error) {
	unlock, err := lockPulseRegistry(hostPath)
	if err != nil {
		return PulseMergeResult{}, err
	}
	defer unlock()
	return mergeRequiredPulsesResultWithOps(hostPath, defaultsRaw, seedMode, required, defaultPulseFileOps())
}

func mergeRequiredPulses(
	hostPath string,
	defaultsRaw []byte,
	seedMode os.FileMode,
	required map[string]bool,
) ([]string, error) {
	return mergeRequiredPulsesWithOps(hostPath, defaultsRaw, seedMode, required, defaultPulseFileOps())
}

// resolvePulseRegistryPath preserves an operator-managed leaf symlink by
// directing atomic registry activation at its resolved regular-file target.
// The lock, ledger, and pending transaction stay beside the logical deployed
// path, so removal history remains owned by the declared artifact. A dangling
// link is not an absent registry: seeding over it would silently replace the
// operator's link, so it fails before any transaction is prepared.
func resolvePulseRegistryPath(hostPath string) (string, error) {
	info, err := os.Lstat(hostPath)
	if err != nil {
		if os.IsNotExist(err) {
			return hostPath, nil
		}
		return "", fmt.Errorf("inspect pulse registry %s: %w", hostPath, err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return hostPath, nil
	}
	resolved, err := filepath.EvalSymlinks(hostPath)
	if err != nil {
		return "", fmt.Errorf("resolve pulse registry symlink %s: %w", hostPath, err)
	}
	targetInfo, err := os.Stat(resolved)
	if err != nil {
		return "", fmt.Errorf("stat pulse registry symlink target %s: %w", resolved, err)
	}
	if !targetInfo.Mode().IsRegular() {
		return "", fmt.Errorf("pulse registry symlink target %s is not a regular file", resolved)
	}
	return resolved, nil
}

func mergeRequiredPulsesWithOps(
	hostPath string,
	defaultsRaw []byte,
	seedMode os.FileMode,
	required map[string]bool,
	ops pulseFileOps,
) ([]string, error) {
	result, err := mergeRequiredPulsesResultWithOps(hostPath, defaultsRaw, seedMode, required, ops)
	return result.Added, err
}

func mergeRequiredPulsesResultWithOps(
	hostPath string,
	defaultsRaw []byte,
	seedMode os.FileMode,
	required map[string]bool,
	ops pulseFileOps,
) (PulseMergeResult, error) {
	defaults, err := parseRequiredPulseDefaults(defaultsRaw, required, "pulse defaults")
	if err != nil {
		return PulseMergeResult{}, err
	}

	registryPath, err := resolvePulseRegistryPath(hostPath)
	if err != nil {
		return PulseMergeResult{}, err
	}
	if err := reconcilePulseLedgerTransaction(hostPath, registryPath, ops); err != nil {
		return PulseMergeResult{}, err
	}

	hostRaw, err := os.ReadFile(registryPath)
	if err != nil {
		if !os.IsNotExist(err) {
			return PulseMergeResult{}, fmt.Errorf("read host pulses %s: %w", hostPath, err)
		}
		added, seedErr := seedPulseConfig(hostPath, registryPath, defaultsRaw, defaults, seedMode, ops)
		if seedErr != nil {
			return PulseMergeResult{}, seedErr
		}
		return PulseMergeResult{Added: added, Created: true}, nil
	}
	hostInfo, err := os.Stat(registryPath)
	if err != nil {
		return PulseMergeResult{}, fmt.Errorf("stat host pulses %s: %w", hostPath, err)
	}
	hostMode := hostInfo.Mode().Perm()

	var host pulseDoc
	if err := json.Unmarshal(hostRaw, &host); err != nil {
		return PulseMergeResult{}, fmt.Errorf("parse host pulses %s (refusing to overwrite): %w", hostPath, err)
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
		return PulseMergeResult{}, err
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
		return PulseMergeResult{}, finishUnchangedPulseRegistry(hostPath, hostRaw, defaults, have, ops)
	}

	if err := activatePulseRegistryAdditions(
		hostPath,
		registryPath,
		hostRaw,
		host,
		hostMode,
		defaults,
		ops,
	); err != nil {
		return PulseMergeResult{}, err
	}
	return PulseMergeResult{Added: added}, nil
}

func finishUnchangedPulseRegistry(
	hostPath string,
	hostRaw []byte,
	defaults pulseDoc,
	have map[string]bool,
	ops pulseFileOps,
) error {
	if err := validatePulseConfigBytes(hostRaw, "live pulse registry"); err != nil {
		return err
	}
	// Nothing needs installation. The ledger still advances to the defaults
	// this host already HAS, so a later removal is recognised as deliberate
	// rather than mistaken for a host that predates the pulse.
	return advancePulseLedger(hostPath, hostRaw, defaultNamesPresent(defaults, have), ops)
}

func activatePulseRegistryAdditions(
	hostPath string,
	registryPath string,
	hostRaw []byte,
	host pulseDoc,
	hostMode os.FileMode,
	defaults pulseDoc,
	ops pulseFileOps,
) error {
	merged, err := marshalPulseDoc(host)
	if err != nil {
		return fmt.Errorf("encode merged pulses: %w", err)
	}
	if err := validatePulseConfigBytes(merged, "projected pulse registry"); err != nil {
		return err
	}
	present := defaultNamesPresent(defaults, pulseNameSet(host.Pulses))
	offered, err := nextPulseLedgerNames(hostPath, present)
	if err != nil {
		return err
	}
	txn, err := preparePulseLedgerTransaction(hostPath, sha256hex(hostRaw), merged, offered, ops)
	if err != nil {
		return err
	}
	if err := writePulseDoc(registryPath, merged, hostMode, ops); err != nil {
		return err
	}
	if err := commitPulseLedgerTransaction(hostPath, txn, ops); err != nil {
		return err
	}
	return nil
}

// pulseDoc decodes the pulses that merge logic needs while retaining every
// other top-level value as raw JSON. A rewrite replaces only the pulses member;
// fields this binary does not know about remain part of the host document
// without lossy number or type coercion.
type pulseDoc struct {
	fields map[string]json.RawMessage
	Pulses []map[string]any `json:"pulses"`
}

func (d *pulseDoc) UnmarshalJSON(data []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}

	var pulses []map[string]any
	if raw, ok := fields["pulses"]; ok {
		if err := json.Unmarshal(raw, &pulses); err != nil {
			return fmt.Errorf("parse pulses member: %w", err)
		}
	}
	d.fields = fields
	d.Pulses = pulses
	return nil
}

func (d pulseDoc) MarshalJSON() ([]byte, error) {
	fields := make(map[string]json.RawMessage, len(d.fields)+1)
	maps.Copy(fields, d.fields)
	pulses, err := json.Marshal(d.Pulses)
	if err != nil {
		return nil, err
	}
	fields["pulses"] = pulses
	return json.Marshal(fields)
}

func marshalPulseDoc(doc pulseDoc) ([]byte, error) {
	raw, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(raw, '\n'), nil
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
func writePulseDoc(path string, data []byte, mode os.FileMode, ops pulseFileOps) error {
	return ops.atomicWrite(path, data, mode, sha256hex(data))
}

// seedPulseConfig installs the defaults verbatim on a host that has no pulse
// registry yet.
func seedPulseConfig(
	hostPath string,
	registryPath string,
	defaultsRaw []byte,
	defaults pulseDoc,
	seedMode os.FileMode,
	ops pulseFileOps,
) ([]string, error) {
	names := pulseNames(defaults.Pulses)
	offered, err := nextPulseLedgerNames(hostPath, names)
	if err != nil {
		return nil, err
	}
	txn, err := preparePulseLedgerTransaction(hostPath, "", defaultsRaw, offered, ops)
	if err != nil {
		return nil, err
	}
	if err := writePulseDoc(registryPath, defaultsRaw, seedMode, ops); err != nil {
		return nil, err
	}
	if err := commitPulseLedgerTransaction(hostPath, txn, ops); err != nil {
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
	raw, err := os.ReadFile(jobsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return defaultRequiredPulseNames(), nil
		}
		return nil, fmt.Errorf("read recovery job registry: %w", err)
	}
	return RequiredPulseNamesRendered(raw)
}

// RequiredPulseNamesRendered returns every pulse named by the already-rendered
// job registry. An explicit registry replaces the built-in jobs at runtime, so
// its rendered bytes are authoritative; built-ins are only the fallback when
// no registry source exists.
func RequiredPulseNamesRendered(raw []byte) (map[string]bool, error) {
	required := make(map[string]bool)
	cfg, err := recoveryloop.ParseConfig(raw)
	if err != nil {
		return nil, fmt.Errorf("parse recovery job registry: %w", err)
	}
	for _, j := range cfg.Jobs {
		if j.Pulse != "" {
			required[j.Pulse] = true
		}
	}
	return required, nil
}

// validateRequiredPulseDefinitions rejects a recovery job whose health names
// a pulse the selected defaults cannot publish. Silently ignoring such a name
// would let the jobs registry deploy successfully while its recovery decision
// can never receive evidence.
func validateRequiredPulseDefinitions(defaults pulseDoc, required map[string]bool) error {
	defined := pulseNameSet(defaults.Pulses)
	missing := make([]string, 0)
	for name, wanted := range required {
		if wanted && !defined[name] {
			missing = append(missing, name)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	sort.Strings(missing)
	return fmt.Errorf("required recovery pulses are not defined in pulse defaults: %s", strings.Join(missing, ", "))
}

func parseRequiredPulseDefaults(raw []byte, required map[string]bool, label string) (pulseDoc, error) {
	var defaults pulseDoc
	if err := json.Unmarshal(raw, &defaults); err != nil {
		return pulseDoc{}, fmt.Errorf("parse %s: %w", label, err)
	}
	if err := validatePulseConfigBytes(raw, label); err != nil {
		return pulseDoc{}, err
	}
	if err := validateRequiredPulseDefinitions(defaults, required); err != nil {
		return pulseDoc{}, err
	}
	return defaults, nil
}

func validatePulseConfigBytes(raw []byte, label string) error {
	if _, err := absencealarm.ParsePulseConfig(raw); err != nil {
		return fmt.Errorf("validate %s: %w", label, err)
	}
	return nil
}

func defaultRequiredPulseNames() map[string]bool {
	required := make(map[string]bool)
	for _, j := range recoveryloop.DefaultJobs() {
		if j.Pulse != "" {
			required[j.Pulse] = true
		}
	}
	return required
}

// PendingPulseMerges reports which required pulses a sync would add, without
// writing anything, so `status` and `--dry-run` can surface a migration that
// has not happened yet.
func PendingPulseMerges(hostPath, defaultsPath string, required map[string]bool) ([]string, error) {
	if err := rejectPendingPulseLedgerTransaction(hostPath); err != nil {
		return nil, err
	}
	defaultsRaw, err := os.ReadFile(defaultsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read pulse defaults %s: %w", defaultsPath, err)
	}
	return PendingPulseMergesRendered(hostPath, defaultsRaw, required)
}

// PendingPulseMergesRendered is PendingPulseMerges over already-rendered
// defaults. Status and dry-run use this path so their preview is based on the
// same token-substituted bytes a real sync would merge.
func PendingPulseMergesRendered(
	hostPath string,
	defaultsRaw []byte,
	required map[string]bool,
) ([]string, error) {
	defaults, err := parseRequiredPulseDefaults(defaultsRaw, required, "rendered pulse defaults")
	if err != nil {
		return nil, err
	}

	registryPath, err := resolvePulseRegistryPath(hostPath)
	if err != nil {
		return nil, err
	}
	snapshot, err := readPulseMergeSnapshot(hostPath, registryPath)
	if err != nil {
		return nil, err
	}
	if !snapshot.registryExists {
		return pulseNames(defaults.Pulses), nil
	}
	var host pulseDoc
	if err := json.Unmarshal(snapshot.registryRaw, &host); err != nil {
		return nil, fmt.Errorf("parse host pulses %s: %w", hostPath, err)
	}
	have := pulseNameSet(host.Pulses)
	var pending []string
	for _, p := range defaults.Pulses {
		if n := pulseName(p); wantsPulse(n, have, snapshot.ledger, required) {
			pending = append(pending, n)
			host.Pulses = append(host.Pulses, p)
		}
	}
	projected := snapshot.registryRaw
	if len(pending) > 0 {
		projected, err = marshalPulseDoc(host)
		if err != nil {
			return nil, fmt.Errorf("encode projected pulses: %w", err)
		}
	}
	if err := validatePulseConfigBytes(projected, "projected pulse registry"); err != nil {
		return nil, err
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
