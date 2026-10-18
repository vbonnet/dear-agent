package deploy

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func readPulseLedgerNames(t *testing.T, hostPath string) []string {
	t.Helper()
	raw, err := os.ReadFile(pulseLedgerPath(hostPath))
	if err != nil {
		t.Fatalf("read pulse ledger: %v", err)
	}
	var names []string
	if err := json.Unmarshal(raw, &names); err != nil {
		t.Fatalf("parse pulse ledger: %v", err)
	}
	return names
}

func readPulseLedgerTransaction(t *testing.T, hostPath string) pulseLedgerTransaction {
	t.Helper()
	raw, err := os.ReadFile(pulseLedgerTransactionPath(hostPath))
	if err != nil {
		t.Fatalf("read pulse ledger transaction: %v", err)
	}
	var txn pulseLedgerTransaction
	if err := json.Unmarshal(raw, &txn); err != nil {
		t.Fatalf("parse pulse ledger transaction: %v", err)
	}
	return txn
}

func requirePathMissing(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("path %s exists or cannot be inspected: %v", path, err)
	}
}

func requireNames(t *testing.T, got []string, want ...string) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Fatalf("names = %v, want %v", got, want)
	}
}

func failFirstPulseLedgerWrite(hostPath string, failure error) (pulseFileOps, *bool) {
	ops := defaultPulseFileOps()
	realWrite := ops.atomicWrite
	failed := new(bool)
	ops.atomicWrite = func(path string, content []byte, mode os.FileMode, wantHash string) error {
		if path == pulseLedgerPath(hostPath) && !*failed {
			*failed = true
			return failure
		}
		return realWrite(path, content, mode, wantHash)
	}
	return ops, failed
}

func TestMergeRequiredPulses_CurrentLedgerWritesNothing(t *testing.T) {
	dir := t.TempDir()
	host := filepath.Join(dir, "host.json")
	hostRaw := validPulseConfig("existing")
	ledgerRaw := []byte(`["existing"]`)
	if err := os.WriteFile(host, hostRaw, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pulseLedgerPath(host), ledgerRaw, 0o644); err != nil {
		t.Fatal(err)
	}

	unexpectedWrite := errors.New("unexpected no-op ledger write")
	ops := defaultPulseFileOps()
	ops.atomicWrite = func(string, []byte, os.FileMode, string) error {
		return unexpectedWrite
	}
	ops.remove = func(string) error {
		return errors.New("unexpected no-op transaction removal")
	}

	added, err := mergeRequiredPulsesWithOps(
		host,
		hostRaw,
		0o644,
		map[string]bool{"existing": true},
		ops,
	)
	if err != nil {
		t.Fatalf("current ledger merge attempted a write: %v", err)
	}
	if len(added) != 0 {
		t.Fatalf("current ledger merge added = %v, want none", added)
	}
	currentLedger, err := os.ReadFile(pulseLedgerPath(host))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(currentLedger, ledgerRaw) {
		t.Fatalf("current ledger changed: got %s, want %s", currentLedger, ledgerRaw)
	}
	requirePathMissing(t, pulseLedgerTransactionPath(host))
}

func TestPulseLedgerTransactionSyncsEachRenameOwningDirectoryInOrder(t *testing.T) {
	dir := t.TempDir()
	logicalDir := filepath.Join(dir, "logical")
	targetDir := filepath.Join(dir, "target")
	if err := os.MkdirAll(logicalDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatal(err)
	}
	host := filepath.Join(logicalDir, "pulses.json")
	target := filepath.Join(targetDir, "pulses.json")
	if err := os.WriteFile(target, validPulseConfig("existing"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, host); err != nil {
		t.Fatal(err)
	}
	resolvedTarget, err := filepath.EvalSymlinks(target)
	if err != nil {
		t.Fatal(err)
	}

	var events []string
	ops := pulseFileOps{
		atomicWrite: func(path string, content []byte, mode os.FileMode, wantHash string) error {
			return atomicWriteWithDirSync(path, content, mode, wantHash, func(parent string) error {
				events = append(events, "write "+path+" sync "+parent)
				return nil
			})
		},
		remove: func(path string) error {
			return durableRemoveWithDirSync(path, func(parent string) error {
				events = append(events, "remove "+path+" sync "+parent)
				return nil
			})
		},
		syncDir: syncDirectory,
	}
	result, err := mergeRequiredPulsesResultWithOps(
		host,
		validPulseConfig("existing", "new"),
		0o644,
		map[string]bool{"existing": true, "new": true},
		ops,
	)
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	requireNames(t, result.Added, "new")
	want := []string{
		"write " + pulseLedgerTransactionPath(host) + " sync " + logicalDir,
		"write " + resolvedTarget + " sync " + filepath.Dir(resolvedTarget),
		"write " + pulseLedgerPath(host) + " sync " + logicalDir,
		"remove " + pulseLedgerTransactionPath(host) + " sync " + logicalDir,
	}
	if !slices.Equal(events, want) {
		t.Fatalf("durability events:\n got: %v\nwant: %v", events, want)
	}
}

func TestPulseLedgerTransactionRecoversPostRegistryRenameSyncFailure(t *testing.T) {
	dir := t.TempDir()
	logicalDir := filepath.Join(dir, "logical")
	targetDir := filepath.Join(dir, "target")
	if err := os.MkdirAll(logicalDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatal(err)
	}
	host := filepath.Join(logicalDir, "pulses.json")
	target := filepath.Join(targetDir, "pulses.json")
	base := validPulseConfig("existing")
	defaults := validPulseConfig("existing", "new")
	if err := os.WriteFile(target, base, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, host); err != nil {
		t.Fatal(err)
	}
	resolvedTarget, err := filepath.EvalSymlinks(target)
	if err != nil {
		t.Fatal(err)
	}

	injected := errors.New("injected registry directory sync failure")
	ops := defaultPulseFileOps()
	realWrite := ops.atomicWrite
	failed := false
	ops.atomicWrite = func(path string, content []byte, mode os.FileMode, wantHash string) error {
		if path == resolvedTarget && !failed {
			failed = true
			return atomicWriteWithDirSync(path, content, mode, wantHash, func(string) error {
				return injected
			})
		}
		return realWrite(path, content, mode, wantHash)
	}
	_, err = mergeRequiredPulsesResultWithOps(
		host,
		defaults,
		0o644,
		map[string]bool{"existing": true, "new": true},
		ops,
	)
	if !errors.Is(err, injected) {
		t.Fatalf("merge error = %v, want injected directory sync failure", err)
	}
	if !failed {
		t.Fatal("registry directory sync failure was not injected")
	}
	requireNames(t, readPulseNames(t, host), "existing", "new")
	requirePathMissing(t, pulseLedgerPath(host))
	_ = readPulseLedgerTransaction(t, host)

	confirmFailure := errors.New("injected registry confirmation failure")
	retryOps := defaultPulseFileOps()
	var confirmedDir string
	retryOps.syncDir = func(path string) error {
		confirmedDir = path
		return confirmFailure
	}
	_, err = mergeRequiredPulsesResultWithOps(
		host,
		defaults,
		0o644,
		map[string]bool{"existing": true, "new": true},
		retryOps,
	)
	if !errors.Is(err, confirmFailure) {
		t.Fatalf("confirmation retry error = %v, want injected failure", err)
	}
	if confirmedDir != filepath.Dir(resolvedTarget) {
		t.Fatalf("retry confirmed directory %q, want target directory %q", confirmedDir, filepath.Dir(resolvedTarget))
	}
	requirePathMissing(t, pulseLedgerPath(host))
	_ = readPulseLedgerTransaction(t, host)

	var events []string
	retryOps = defaultPulseFileOps()
	retryOps.syncDir = func(path string) error {
		events = append(events, "confirm "+path)
		return syncDirectory(path)
	}
	retryOps.atomicWrite = func(path string, content []byte, mode os.FileMode, wantHash string) error {
		return atomicWriteWithDirSync(path, content, mode, wantHash, func(parent string) error {
			events = append(events, "write "+path+" sync "+parent)
			return syncDirectory(parent)
		})
	}
	retryOps.remove = func(path string) error {
		return durableRemoveWithDirSync(path, func(parent string) error {
			events = append(events, "remove "+path+" sync "+parent)
			return syncDirectory(parent)
		})
	}
	result, err := mergeRequiredPulsesResultWithOps(
		host,
		defaults,
		0o644,
		map[string]bool{"existing": true, "new": true},
		retryOps,
	)
	if err != nil {
		t.Fatalf("successful retry: %v", err)
	}
	if len(result.Added) != 0 || result.Created || !result.LedgerChanged || !result.Reconciled {
		t.Fatalf("retry result = %+v, want reconciled ledger update", result)
	}
	wantEvents := []string{
		"confirm " + filepath.Dir(resolvedTarget),
		"write " + pulseLedgerPath(host) + " sync " + logicalDir,
		"remove " + pulseLedgerTransactionPath(host) + " sync " + logicalDir,
		"confirm " + logicalDir,
	}
	if !slices.Equal(events, wantEvents) {
		t.Fatalf("recovery durability events:\n got: %v\nwant: %v", events, wantEvents)
	}
	requireNames(t, readPulseLedgerNames(t, host), "existing", "new")
	requirePathMissing(t, pulseLedgerTransactionPath(host))
}

func TestPulseLedgerTransactionReportsPostRemoveSyncFailureAndRetriesSafely(t *testing.T) {
	dir := t.TempDir()
	host := filepath.Join(dir, "pulses.json")
	defaults := validPulseConfig("existing", "new")
	if err := os.WriteFile(host, validPulseConfig("existing"), 0o644); err != nil {
		t.Fatal(err)
	}

	injected := errors.New("injected pending removal directory sync failure")
	ops := defaultPulseFileOps()
	ops.remove = func(path string) error {
		return durableRemoveWithDirSync(path, func(string) error { return injected })
	}
	_, err := mergeRequiredPulsesResultWithOps(
		host,
		defaults,
		0o644,
		map[string]bool{"existing": true, "new": true},
		ops,
	)
	if !errors.Is(err, injected) {
		t.Fatalf("merge error = %v, want injected directory sync failure", err)
	}
	requireNames(t, readPulseNames(t, host), "existing", "new")
	requireNames(t, readPulseLedgerNames(t, host), "existing", "new")
	requirePathMissing(t, pulseLedgerTransactionPath(host))

	retryOps := defaultPulseFileOps()
	realSync := retryOps.syncDir
	var confirmedDir string
	retryOps.syncDir = func(path string) error {
		confirmedDir = path
		return realSync(path)
	}
	result, err := mergeRequiredPulsesResultWithOps(
		host,
		defaults,
		0o644,
		map[string]bool{"existing": true, "new": true},
		retryOps,
	)
	if err != nil {
		t.Fatalf("retry: %v", err)
	}
	if len(result.Added) != 0 || result.Created || result.LedgerChanged || result.Reconciled {
		t.Fatalf("retry result = %+v, want unchanged", result)
	}
	if confirmedDir != filepath.Dir(host) {
		t.Fatalf("retry confirmed directory %q, want %q", confirmedDir, filepath.Dir(host))
	}
}

func TestMergeRequiredPulses_RecoversExistingHostAfterLedgerWriteFailure(t *testing.T) {
	dir := t.TempDir()
	host := filepath.Join(dir, "host.json")
	hostRaw := validPulseConfig("existing")
	defaultsRaw := validPulseConfig("existing", "new")
	if err := os.WriteFile(host, hostRaw, 0o644); err != nil {
		t.Fatal(err)
	}

	injected := errors.New("injected pulse ledger write failure")
	ops, failed := failFirstPulseLedgerWrite(host, injected)
	_, err := mergeRequiredPulsesWithOps(
		host,
		defaultsRaw,
		0o644,
		map[string]bool{"existing": true, "new": true},
		ops,
	)
	if !errors.Is(err, injected) {
		t.Fatalf("merge error = %v, want injected ledger failure", err)
	}
	if !*failed {
		t.Fatal("ledger failure injection did not run")
	}
	requireNames(t, readPulseNames(t, host), "existing", "new")
	requirePathMissing(t, pulseLedgerPath(host))
	txn := readPulseLedgerTransaction(t, host)
	if txn.Version != pulseLedgerTransactionVersion {
		t.Errorf("transaction version = %d, want %d", txn.Version, pulseLedgerTransactionVersion)
	}
	if txn.BaseRegistrySHA256 != sha256hex(hostRaw) {
		t.Errorf("transaction base hash = %q, want %q", txn.BaseRegistrySHA256, sha256hex(hostRaw))
	}
	current, err := os.ReadFile(host)
	if err != nil {
		t.Fatal(err)
	}
	if txn.RegistrySHA256 != sha256hex(current) {
		t.Errorf("transaction target hash = %q, want %q", txn.RegistrySHA256, sha256hex(current))
	}
	requireNames(t, txn.Offered, "existing", "new")

	added, err := mergeRequiredPulses(
		host,
		hostRaw,
		0o644,
		map[string]bool{"existing": true},
	)
	if err != nil {
		t.Fatalf("recovery merge: %v", err)
	}
	if len(added) != 0 {
		t.Fatalf("recovery added = %v, want none", added)
	}
	requireNames(t, readPulseLedgerNames(t, host), "existing", "new")
	requirePathMissing(t, pulseLedgerTransactionPath(host))

	if err := os.WriteFile(host, hostRaw, 0o644); err != nil {
		t.Fatal(err)
	}
	added, err = mergeRequiredPulses(
		host,
		defaultsRaw,
		0o644,
		map[string]bool{"existing": true},
	)
	if err != nil {
		t.Fatalf("merge after operator removal: %v", err)
	}
	if len(added) != 0 {
		t.Fatalf("re-added deliberately removed pulses: %v", added)
	}
	requireNames(t, readPulseNames(t, host), "existing")
}

func TestMergeRequiredPulses_RecoversAbsentHostAfterLedgerWriteFailure(t *testing.T) {
	dir := t.TempDir()
	host := filepath.Join(dir, "nested", "host.json")
	defaultsRaw := validPulseConfig("a", "b")

	injected := errors.New("injected pulse ledger write failure")
	ops, failed := failFirstPulseLedgerWrite(host, injected)
	_, err := mergeRequiredPulsesWithOps(
		host,
		defaultsRaw,
		0o600,
		map[string]bool{"a": true, "b": true},
		ops,
	)
	if !errors.Is(err, injected) {
		t.Fatalf("seed error = %v, want injected ledger failure", err)
	}
	if !*failed {
		t.Fatal("ledger failure injection did not run")
	}
	current, err := os.ReadFile(host)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(current, defaultsRaw) {
		t.Errorf("seeded registry = %s, want exact defaults %s", current, defaultsRaw)
	}
	info, err := os.Stat(host)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("seeded mode = %04o, want 0600", got)
	}
	requirePathMissing(t, pulseLedgerPath(host))
	txn := readPulseLedgerTransaction(t, host)
	if txn.BaseRegistrySHA256 != "" {
		t.Errorf("absent-host base hash = %q, want empty", txn.BaseRegistrySHA256)
	}
	if txn.RegistrySHA256 != sha256hex(current) {
		t.Errorf("transaction target hash = %q, want %q", txn.RegistrySHA256, sha256hex(current))
	}
	requireNames(t, txn.Offered, "a", "b")

	added, err := mergeRequiredPulses(
		host,
		validPulseConfig("a"),
		0o600,
		map[string]bool{"a": true},
	)
	if err != nil {
		t.Fatalf("seed recovery: %v", err)
	}
	if len(added) != 0 {
		t.Fatalf("seed recovery added = %v, want none", added)
	}
	requireNames(t, readPulseLedgerNames(t, host), "a", "b")
	requirePathMissing(t, pulseLedgerTransactionPath(host))

	if err := os.WriteFile(host, validPulseConfig("a"), 0o600); err != nil {
		t.Fatal(err)
	}
	added, err = mergeRequiredPulses(
		host,
		defaultsRaw,
		0o600,
		map[string]bool{"a": true},
	)
	if err != nil {
		t.Fatalf("merge after operator removal: %v", err)
	}
	if len(added) != 0 {
		t.Fatalf("re-added deliberately removed pulses: %v", added)
	}
	requireNames(t, readPulseNames(t, host), "a")
}

func TestMergeRequiredPulses_DiscardsTransactionBeforeRegistryActivation(t *testing.T) {
	dir := t.TempDir()
	host := filepath.Join(dir, "host.json")
	baseRaw := validPulseConfig("existing")
	targetRaw := validPulseConfig("existing", "new")
	defaultsRaw := targetRaw
	if err := os.WriteFile(host, baseRaw, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := preparePulseLedgerTransaction(
		host,
		sha256hex(baseRaw),
		targetRaw,
		[]string{"existing", "new"},
		defaultPulseFileOps(),
	); err != nil {
		t.Fatalf("prepare transaction: %v", err)
	}

	added, err := mergeRequiredPulses(
		host,
		defaultsRaw,
		0o644,
		map[string]bool{"existing": true, "new": true},
	)
	if err != nil {
		t.Fatalf("merge after pre-activation interruption: %v", err)
	}
	requireNames(t, added, "new")
	requireNames(t, readPulseNames(t, host), "existing", "new")
	requireNames(t, readPulseLedgerNames(t, host), "existing", "new")
	requirePathMissing(t, pulseLedgerTransactionPath(host))
}

func TestMergeRequiredPulses_ReportsMarkerOnlyReconciliation(t *testing.T) {
	dir := t.TempDir()
	host := filepath.Join(dir, "host.json")
	baseRaw := validPulseConfig("existing")
	targetRaw := validPulseConfig("existing", "new")
	if err := os.WriteFile(host, baseRaw, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pulseLedgerPath(host), []byte(`["existing"]`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := preparePulseLedgerTransaction(
		host,
		sha256hex(baseRaw),
		targetRaw,
		[]string{"existing", "new"},
		defaultPulseFileOps(),
	); err != nil {
		t.Fatalf("prepare transaction: %v", err)
	}

	result, err := MergeRequiredPulsesRenderedResult(
		host,
		baseRaw,
		0o644,
		map[string]bool{"existing": true},
	)
	if err != nil {
		t.Fatalf("marker-only reconciliation: %v", err)
	}
	if len(result.Added) != 0 || result.Created || result.LedgerChanged || !result.Reconciled {
		t.Fatalf("result = %+v, want marker-only reconciliation", result)
	}
	requirePathMissing(t, pulseLedgerTransactionPath(host))
	requireNames(t, readPulseLedgerNames(t, host), "existing")
	requireNames(t, readPulseNames(t, host), "existing")
}

func TestMergeRequiredPulses_CompletesTransactionAfterRegistryActivation(t *testing.T) {
	dir := t.TempDir()
	host := filepath.Join(dir, "host.json")
	baseRaw := validPulseConfig("existing")
	targetRaw := validPulseConfig("existing", "new")
	if err := os.WriteFile(host, baseRaw, 0o644); err != nil {
		t.Fatal(err)
	}
	ops := defaultPulseFileOps()
	if _, err := preparePulseLedgerTransaction(
		host,
		sha256hex(baseRaw),
		targetRaw,
		[]string{"existing", "new"},
		ops,
	); err != nil {
		t.Fatalf("prepare transaction: %v", err)
	}
	if err := writePulseDoc(host, targetRaw, 0o644, ops); err != nil {
		t.Fatalf("activate registry: %v", err)
	}

	added, err := mergeRequiredPulses(
		host,
		baseRaw,
		0o644,
		map[string]bool{"existing": true},
	)
	if err != nil {
		t.Fatalf("merge after post-activation interruption: %v", err)
	}
	if len(added) != 0 {
		t.Fatalf("post-activation recovery added = %v, want none", added)
	}
	requireNames(t, readPulseLedgerNames(t, host), "existing", "new")
	requirePathMissing(t, pulseLedgerTransactionPath(host))

	if err := os.WriteFile(host, baseRaw, 0o644); err != nil {
		t.Fatal(err)
	}
	added, err = mergeRequiredPulses(
		host,
		targetRaw,
		0o644,
		map[string]bool{"existing": true},
	)
	if err != nil {
		t.Fatalf("merge after operator removal: %v", err)
	}
	if len(added) != 0 {
		t.Fatalf("re-added deliberately removed pulses: %v", added)
	}
	requireNames(t, readPulseNames(t, host), "existing")
}

func TestMergeRequiredPulses_PreservesAmbiguousTransaction(t *testing.T) {
	dir := t.TempDir()
	host := filepath.Join(dir, "host.json")
	baseRaw := validPulseConfig("existing")
	targetRaw := validPulseConfig("existing", "new")
	operatorRaw := validPulseConfig("existing", "operator-local")
	if err := os.WriteFile(host, baseRaw, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := preparePulseLedgerTransaction(
		host,
		sha256hex(baseRaw),
		targetRaw,
		[]string{"existing", "new"},
		defaultPulseFileOps(),
	); err != nil {
		t.Fatalf("prepare transaction: %v", err)
	}
	if err := os.WriteFile(host, operatorRaw, 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := mergeRequiredPulses(
		host,
		targetRaw,
		0o644,
		map[string]bool{"existing": true, "new": true},
	)
	if err == nil || !strings.Contains(err.Error(), "matches neither") {
		t.Fatalf("ambiguous merge error = %v, want preserved-state failure", err)
	}
	current, readErr := os.ReadFile(host)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if !bytes.Equal(current, operatorRaw) {
		t.Errorf("ambiguous registry changed: got %s, want %s", current, operatorRaw)
	}
	if _, statErr := os.Stat(pulseLedgerTransactionPath(host)); statErr != nil {
		t.Fatalf("ambiguous transaction was not preserved: %v", statErr)
	}
	requirePathMissing(t, pulseLedgerPath(host))
}

func TestPendingPulseMerges_ReportsReconciliationDebt(t *testing.T) {
	dir := t.TempDir()
	host := filepath.Join(dir, "host.json")
	baseRaw := validPulseConfig("existing")
	targetRaw := validPulseConfig("existing", "new")
	if err := os.WriteFile(host, baseRaw, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := preparePulseLedgerTransaction(
		host,
		sha256hex(baseRaw),
		targetRaw,
		[]string{"existing", "new"},
		defaultPulseFileOps(),
	); err != nil {
		t.Fatalf("prepare transaction: %v", err)
	}

	_, err := PendingPulseMergesRendered(
		host,
		targetRaw,
		map[string]bool{"existing": true, "new": true},
	)
	if err == nil || !strings.Contains(err.Error(), "reconciliation pending") {
		t.Fatalf("pending preview error = %v, want reconciliation debt", err)
	}
	if _, statErr := os.Stat(pulseLedgerTransactionPath(host)); statErr != nil {
		t.Fatalf("read-only preview changed the transaction: %v", statErr)
	}
}

func TestMergeRequiredPulses_RecoversLedgerOnlyAdvancement(t *testing.T) {
	dir := t.TempDir()
	host := filepath.Join(dir, "host.json")
	registryRaw := validPulseConfig("existing")
	if err := os.WriteFile(host, registryRaw, 0o644); err != nil {
		t.Fatal(err)
	}

	injected := errors.New("injected ledger-only write failure")
	ops, failed := failFirstPulseLedgerWrite(host, injected)
	_, err := mergeRequiredPulsesWithOps(
		host,
		registryRaw,
		0o644,
		map[string]bool{"existing": true},
		ops,
	)
	if !errors.Is(err, injected) {
		t.Fatalf("ledger-only merge error = %v, want injected failure", err)
	}
	if !*failed {
		t.Fatal("ledger failure injection did not run")
	}
	requireNames(t, readPulseNames(t, host), "existing")
	requirePathMissing(t, pulseLedgerPath(host))
	txn := readPulseLedgerTransaction(t, host)
	if txn.BaseRegistrySHA256 != txn.RegistrySHA256 {
		t.Errorf(
			"ledger-only transaction base = %q, target = %q; want identical",
			txn.BaseRegistrySHA256,
			txn.RegistrySHA256,
		)
	}
	requireNames(t, txn.Offered, "existing")
	if _, previewErr := PendingPulseMergesRendered(
		host,
		registryRaw,
		map[string]bool{"existing": true},
	); previewErr == nil || !strings.Contains(previewErr.Error(), "reconciliation pending") {
		t.Fatalf("pending preview error = %v, want reconciliation debt", previewErr)
	}

	added, err := mergeRequiredPulses(host, registryRaw, 0o644, map[string]bool{})
	if err != nil {
		t.Fatalf("ledger-only recovery: %v", err)
	}
	if len(added) != 0 {
		t.Fatalf("ledger-only recovery added = %v, want none", added)
	}
	requireNames(t, readPulseLedgerNames(t, host), "existing")
	requirePathMissing(t, pulseLedgerTransactionPath(host))

	operatorOnly := validPulseConfig("operator-local")
	if err := os.WriteFile(host, operatorOnly, 0o644); err != nil {
		t.Fatal(err)
	}
	added, err = mergeRequiredPulses(
		host,
		registryRaw,
		0o644,
		map[string]bool{"operator-local": true},
	)
	if err != nil {
		t.Fatalf("merge after operator removal: %v", err)
	}
	if len(added) != 0 {
		t.Fatalf("re-added deliberately removed pulse: %v", added)
	}
	if got := readPulseNames(t, host); !slices.Equal(got, []string{"operator-local"}) {
		t.Fatalf("host pulses = %v, want [operator-local]", got)
	}
}

func TestMergeRequiredPulses_PreservesTransactionWhenRegistryActivationFails(t *testing.T) {
	dir := t.TempDir()
	host := filepath.Join(dir, "host.json")
	baseRaw := validPulseConfig("existing")
	defaultsRaw := validPulseConfig("existing", "new")
	if err := os.WriteFile(host, baseRaw, 0o644); err != nil {
		t.Fatal(err)
	}

	injected := errors.New("injected registry activation failure")
	ops := defaultPulseFileOps()
	realWrite := ops.atomicWrite
	failed := false
	ops.atomicWrite = func(path string, content []byte, mode os.FileMode, wantHash string) error {
		if path == host && !failed {
			failed = true
			return injected
		}
		return realWrite(path, content, mode, wantHash)
	}
	_, err := mergeRequiredPulsesWithOps(
		host,
		defaultsRaw,
		0o644,
		map[string]bool{"existing": true, "new": true},
		ops,
	)
	if !errors.Is(err, injected) {
		t.Fatalf("merge error = %v, want injected registry failure", err)
	}
	if !failed {
		t.Fatal("registry failure injection did not run")
	}
	current, readErr := os.ReadFile(host)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if !bytes.Equal(current, baseRaw) {
		t.Errorf("failed activation changed registry: got %s, want %s", current, baseRaw)
	}
	if _, statErr := os.Stat(pulseLedgerTransactionPath(host)); statErr != nil {
		t.Fatalf("pre-activation transaction was not preserved: %v", statErr)
	}
	requirePathMissing(t, pulseLedgerPath(host))

	added, err := mergeRequiredPulses(
		host,
		defaultsRaw,
		0o644,
		map[string]bool{"existing": true, "new": true},
	)
	if err != nil {
		t.Fatalf("retry after registry failure: %v", err)
	}
	requireNames(t, added, "new")
	requireNames(t, readPulseNames(t, host), "existing", "new")
	requireNames(t, readPulseLedgerNames(t, host), "existing", "new")
	requirePathMissing(t, pulseLedgerTransactionPath(host))
}

func TestMergeRequiredPulses_RetriesTransactionCleanup(t *testing.T) {
	dir := t.TempDir()
	host := filepath.Join(dir, "host.json")
	baseRaw := validPulseConfig("existing")
	defaultsRaw := validPulseConfig("existing", "new")
	if err := os.WriteFile(host, baseRaw, 0o644); err != nil {
		t.Fatal(err)
	}

	injected := errors.New("injected transaction cleanup failure")
	ops := defaultPulseFileOps()
	realRemove := ops.remove
	failed := false
	ops.remove = func(path string) error {
		if path == pulseLedgerTransactionPath(host) && !failed {
			failed = true
			return injected
		}
		return realRemove(path)
	}
	_, err := mergeRequiredPulsesWithOps(
		host,
		defaultsRaw,
		0o644,
		map[string]bool{"existing": true, "new": true},
		ops,
	)
	if !errors.Is(err, injected) {
		t.Fatalf("merge error = %v, want injected cleanup failure", err)
	}
	if !failed {
		t.Fatal("cleanup failure injection did not run")
	}
	requireNames(t, readPulseNames(t, host), "existing", "new")
	requireNames(t, readPulseLedgerNames(t, host), "existing", "new")
	if _, statErr := os.Stat(pulseLedgerTransactionPath(host)); statErr != nil {
		t.Fatalf("transaction was not preserved after cleanup failure: %v", statErr)
	}

	added, err := mergeRequiredPulses(
		host,
		defaultsRaw,
		0o644,
		map[string]bool{"existing": true, "new": true},
	)
	if err != nil {
		t.Fatalf("retry after cleanup failure: %v", err)
	}
	if len(added) != 0 {
		t.Fatalf("cleanup retry added = %v, want none", added)
	}
	requireNames(t, readPulseLedgerNames(t, host), "existing", "new")
	requirePathMissing(t, pulseLedgerTransactionPath(host))
}

func TestMergeRequiredPulses_PreservesInvalidTransactions(t *testing.T) {
	validHash := strings.Repeat("a", 64)
	cases := map[string][]byte{
		"malformed":       []byte(`{"version":`),
		"future version":  []byte(`{"version":2,"registry_sha256":"` + validHash + `","offered":["a"]}`),
		"missing offered": []byte(`{"version":1,"registry_sha256":"` + validHash + `"}`),
		"unsorted names":  []byte(`{"version":1,"registry_sha256":"` + validHash + `","offered":["b","a"]}`),
	}
	for name, transactionRaw := range cases {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			host := filepath.Join(dir, "host.json")
			hostRaw := validPulseConfig("existing")
			if err := os.WriteFile(host, hostRaw, 0o644); err != nil {
				t.Fatal(err)
			}
			transactionPath := pulseLedgerTransactionPath(host)
			if err := os.WriteFile(transactionPath, transactionRaw, 0o644); err != nil {
				t.Fatal(err)
			}

			_, err := mergeRequiredPulses(host, hostRaw, 0o644, map[string]bool{"existing": true})
			if err == nil {
				t.Fatal("invalid transaction was accepted")
			}
			current, readErr := os.ReadFile(host)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if !bytes.Equal(current, hostRaw) {
				t.Errorf("invalid transaction changed registry: got %s, want %s", current, hostRaw)
			}
			preserved, readErr := os.ReadFile(transactionPath)
			if readErr != nil {
				t.Fatalf("invalid transaction was not preserved: %v", readErr)
			}
			if !bytes.Equal(preserved, transactionRaw) {
				t.Errorf("invalid transaction changed: got %s, want %s", preserved, transactionRaw)
			}
		})
	}
}
