package deploy

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"sort"
)

const pulseLedgerTransactionVersion = 1

const pulsePreviewSnapshotAttempts = 3

// pulseFileOps is an internal seam for the local-substitutable filesystem.
// The exported merge interface stays small; tests replace only atomic
// activation so interruption windows can be exercised without permission
// tricks.
type pulseFileOps struct {
	atomicWrite func(path string, content []byte, mode os.FileMode, wantHash string) error
	remove      func(path string) error
	syncDir     func(path string) error
}

func defaultPulseFileOps() pulseFileOps {
	return pulseFileOps{atomicWrite: atomicWrite, remove: durableRemove, syncDir: syncDirectory}
}

// pulseLedgerTransaction records the exact ledger projection that belongs to
// one registry byte image. It is persisted before that registry is activated,
// so a later run can distinguish the interruption windows without rolling back
// a required monitoring pulse.
type pulseLedgerTransaction struct {
	Version            int      `json:"version"`
	BaseRegistrySHA256 string   `json:"base_registry_sha256,omitempty"`
	RegistrySHA256     string   `json:"registry_sha256"`
	Offered            []string `json:"offered"`
}

type pulseReconcileResult struct {
	LedgerChanged bool
	Reconciled    bool
}

// pulseLedgerPath is a sidecar next to the registry recording every default
// pulse this host has been offered.
func pulseLedgerPath(hostPath string) string {
	return hostPath + ".offered"
}

// pulseLedgerTransactionPath is the pending intent written before registry
// activation. Its registry digest lets the next run distinguish an activation
// that happened from one that did not.
func pulseLedgerTransactionPath(hostPath string) string {
	return hostPath + ".offered.pending"
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
	return decodePulseLedger(raw), nil
}

func decodePulseLedger(raw []byte) map[string]bool {
	var names []string
	// A corrupt ledger degrades to "nothing offered yet", which can only
	// re-offer a default. Blocking a required pulse on an unreadable sidecar is
	// the failure this file prevents.
	if err := json.Unmarshal(raw, &names); err != nil {
		return map[string]bool{}
	}
	seen := make(map[string]bool, len(names))
	for _, n := range names {
		seen[n] = true
	}
	return seen
}

type pulseMergeSnapshot struct {
	registryRaw    []byte
	registryExists bool
	ledgerRaw      []byte
	ledgerExists   bool
	ledger         map[string]bool
}

type optionalPulseFile struct {
	raw    []byte
	exists bool
}

// readPulseMergeSnapshot takes a non-mutating, stable view of the registry and
// ledger. The pending marker brackets every writer transaction; double reads
// also catch a complete transaction that starts and finishes between marker
// checks, preventing a preview from combining an old registry with a new
// ledger (or vice versa).
func readPulseMergeSnapshot(hostPath, registryPath string) (pulseMergeSnapshot, error) {
	for range pulsePreviewSnapshotAttempts {
		if err := rejectPendingPulseLedgerTransaction(hostPath); err != nil {
			return pulseMergeSnapshot{}, err
		}
		registry, err := readOptionalPulseFile(registryPath)
		if err != nil {
			return pulseMergeSnapshot{}, fmt.Errorf("read host pulses %s: %w", hostPath, err)
		}
		ledger, err := readOptionalPulseFile(pulseLedgerPath(hostPath))
		if err != nil {
			return pulseMergeSnapshot{}, fmt.Errorf("read pulse ledger: %w", err)
		}
		if err := rejectPendingPulseLedgerTransaction(hostPath); err != nil {
			return pulseMergeSnapshot{}, err
		}
		registryCheck, err := readOptionalPulseFile(registryPath)
		if err != nil {
			return pulseMergeSnapshot{}, fmt.Errorf("re-read host pulses %s: %w", hostPath, err)
		}
		ledgerCheck, err := readOptionalPulseFile(pulseLedgerPath(hostPath))
		if err != nil {
			return pulseMergeSnapshot{}, fmt.Errorf("re-read pulse ledger: %w", err)
		}
		if err := rejectPendingPulseLedgerTransaction(hostPath); err != nil {
			return pulseMergeSnapshot{}, err
		}
		if sameOptionalPulseFile(registry, registryCheck) && sameOptionalPulseFile(ledger, ledgerCheck) {
			return pulseMergeSnapshot{
				registryRaw:    registryCheck.raw,
				registryExists: registryCheck.exists,
				ledgerRaw:      ledgerCheck.raw,
				ledgerExists:   ledgerCheck.exists,
				ledger:         decodePulseLedger(ledgerCheck.raw),
			}, nil
		}
	}
	return pulseMergeSnapshot{}, fmt.Errorf(
		"pulse registry or ledger changed repeatedly while computing preview",
	)
}

func readOptionalPulseFile(path string) (optionalPulseFile, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return optionalPulseFile{}, nil
		}
		return optionalPulseFile{}, err
	}
	return optionalPulseFile{raw: raw, exists: true}, nil
}

func sameOptionalPulseFile(a, b optionalPulseFile) bool {
	return a.exists == b.exists && bytes.Equal(a.raw, b.raw)
}

// rejectPendingPulseLedgerTransaction keeps read-only status and dry-run from
// reporting a clean preview while a prior write still needs reconciliation.
// Only a real sync, under the registry lock, may complete or discard the
// transaction.
func rejectPendingPulseLedgerTransaction(hostPath string) error {
	path := pulseLedgerTransactionPath(hostPath)
	if _, err := os.Lstat(path); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("inspect pulse ledger transaction %s: %w", path, err)
	}
	return fmt.Errorf("pulse ledger reconciliation pending at %s; run sync before trusting this preview", path)
}

// nextPulseLedgerNames returns the current default pulse names unioned with
// what was already offered, so a pulse retired from the defaults is not
// forgotten and then resurrected if it returns.
func nextPulseLedgerNames(hostPath string, offered []string) ([]string, error) {
	seen, err := loadPulseLedger(hostPath)
	if err != nil {
		return nil, err
	}
	names, _, err := pulseLedgerProjection(seen, offered)
	return names, err
}

// pulseLedgerProjection returns the canonical ledger union and its exact JSON
// encoding without mutating the observed ledger map. Preview and publication
// share this calculation so status cannot call a ledger adoption clean while a
// real sync would write a sidecar.
func pulseLedgerProjection(seen map[string]bool, offered []string) ([]string, []byte, error) {
	projected := make(map[string]bool, len(seen)+len(offered))
	maps.Copy(projected, seen)
	for _, n := range offered {
		projected[n] = true
	}
	names := make([]string, 0, len(projected))
	for name, present := range projected {
		if present {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	raw, err := json.Marshal(names)
	if err != nil {
		return nil, nil, fmt.Errorf("encode pulse ledger projection: %w", err)
	}
	return names, raw, nil
}

// advancePulseLedger journals a ledger-only projection against an unchanged
// registry. Base and target hashes are identical, so reconciliation completes
// the ledger rather than mistaking a failed write for a clean deployment.
func advancePulseLedger(
	hostPath string,
	registryRaw []byte,
	offered []string,
	ops pulseFileOps,
) (bool, error) {
	current, err := readOptionalPulseFile(pulseLedgerPath(hostPath))
	if err != nil {
		return false, fmt.Errorf("read current pulse ledger projection: %w", err)
	}
	names, projectedRaw, err := pulseLedgerProjection(decodePulseLedger(current.raw), offered)
	if err != nil {
		return false, err
	}
	if current.exists && bytes.Equal(current.raw, projectedRaw) {
		// A prior pending-marker removal may have succeeded before its parent
		// directory sync failed. Confirm the logical ledger namespace before a
		// no-op retry reports the transaction current.
		if err := ops.syncDir(filepath.Dir(hostPath)); err != nil {
			return false, fmt.Errorf("confirm pulse ledger directory: %w", err)
		}
		return false, nil
	}
	registrySHA256 := sha256hex(registryRaw)
	txn, err := preparePulseLedgerTransaction(
		hostPath,
		registrySHA256,
		registryRaw,
		names,
		ops,
	)
	if err != nil {
		return false, err
	}
	if err := commitPulseLedgerTransaction(hostPath, txn, ops); err != nil {
		return false, err
	}
	return true, nil
}

// preparePulseLedgerTransaction persists the exact ledger projection before
// the corresponding registry bytes can become live.
func preparePulseLedgerTransaction(
	hostPath string,
	baseRegistrySHA256 string,
	registryRaw []byte,
	offered []string,
	ops pulseFileOps,
) (pulseLedgerTransaction, error) {
	txn := pulseLedgerTransaction{
		Version:            pulseLedgerTransactionVersion,
		BaseRegistrySHA256: baseRegistrySHA256,
		RegistrySHA256:     sha256hex(registryRaw),
		Offered:            append([]string{}, offered...),
	}
	raw, err := json.Marshal(txn)
	if err != nil {
		return pulseLedgerTransaction{}, fmt.Errorf("encode pulse ledger transaction: %w", err)
	}
	if err := ops.atomicWrite(
		pulseLedgerTransactionPath(hostPath), raw, 0o644, sha256hex(raw),
	); err != nil {
		return pulseLedgerTransaction{}, fmt.Errorf("prepare pulse ledger transaction: %w", err)
	}
	return txn, nil
}

// commitPulseLedgerTransaction installs the transaction's exact ledger and
// clears its intent only after the ledger activation succeeds. A failure leaves
// the intent in place for the next run to reconcile.
func commitPulseLedgerTransaction(
	hostPath string,
	txn pulseLedgerTransaction,
	ops pulseFileOps,
) error {
	raw, err := json.Marshal(txn.Offered)
	if err != nil {
		return fmt.Errorf("encode pulse ledger transaction projection: %w", err)
	}
	if err := ops.atomicWrite(pulseLedgerPath(hostPath), raw, 0o644, sha256hex(raw)); err != nil {
		return fmt.Errorf("commit pulse ledger transaction: %w", err)
	}
	return removePulseLedgerTransaction(hostPath, ops)
}

// reconcilePulseLedgerTransaction completes or discards an interrupted
// registry-plus-ledger projection. A target digest match proves registry
// activation happened, so the ledger is completed. For a registry-changing
// transaction, an exact base-state match is treated as a pre-activation
// interruption and the intent is discarded; out-of-band ABA edits cannot be
// distinguished with two artifacts. Any third state is ambiguous operator
// drift and fails loud without destroying the recovery evidence.
func reconcilePulseLedgerTransaction(
	hostPath, registryPath string,
	ops pulseFileOps,
) (pulseReconcileResult, error) {
	raw, err := os.ReadFile(pulseLedgerTransactionPath(hostPath))
	if err != nil {
		if os.IsNotExist(err) {
			return pulseReconcileResult{}, nil
		}
		return pulseReconcileResult{}, fmt.Errorf("read pulse ledger transaction: %w", err)
	}
	var txn pulseLedgerTransaction
	if err := json.Unmarshal(raw, &txn); err != nil {
		return pulseReconcileResult{}, fmt.Errorf("parse pulse ledger transaction: %w", err)
	}
	if err := validatePulseLedgerTransaction(txn); err != nil {
		return pulseReconcileResult{}, err
	}

	registryRaw, err := os.ReadFile(registryPath)
	if err != nil {
		if os.IsNotExist(err) {
			if txn.BaseRegistrySHA256 == "" {
				if err := removePulseLedgerTransaction(hostPath, ops); err != nil {
					return pulseReconcileResult{}, err
				}
				return pulseReconcileResult{Reconciled: true}, nil
			}
			return pulseReconcileResult{}, fmt.Errorf(
				"pulse registry changed from the pending transaction base; preserving %s for manual reconciliation",
				pulseLedgerTransactionPath(hostPath),
			)
		}
		return pulseReconcileResult{}, fmt.Errorf("read registry for pulse ledger transaction: %w", err)
	}
	registrySHA256 := sha256hex(registryRaw)
	if registrySHA256 == txn.RegistrySHA256 {
		// A previous registry activation can expose the target bytes before its
		// parent-directory sync succeeds. Reconfirm the actual registry target
		// directory (which may differ from hostPath through a leaf symlink)
		// before making the corresponding ledger/pending cleanup durable.
		if err := ops.syncDir(filepath.Dir(registryPath)); err != nil {
			return pulseReconcileResult{}, fmt.Errorf("confirm activated pulse registry directory: %w", err)
		}
		if err := commitPulseLedgerTransaction(hostPath, txn, ops); err != nil {
			return pulseReconcileResult{}, err
		}
		return pulseReconcileResult{LedgerChanged: true, Reconciled: true}, nil
	}
	if txn.BaseRegistrySHA256 != "" && registrySHA256 == txn.BaseRegistrySHA256 {
		if err := removePulseLedgerTransaction(hostPath, ops); err != nil {
			return pulseReconcileResult{}, err
		}
		return pulseReconcileResult{Reconciled: true}, nil
	}
	return pulseReconcileResult{}, fmt.Errorf(
		"pulse registry matches neither the pending transaction base nor target; preserving %s for manual reconciliation",
		pulseLedgerTransactionPath(hostPath),
	)
}

func validatePulseLedgerTransaction(txn pulseLedgerTransaction) error {
	if txn.Version != pulseLedgerTransactionVersion {
		return fmt.Errorf(
			"unsupported pulse ledger transaction version %d (want %d)",
			txn.Version,
			pulseLedgerTransactionVersion,
		)
	}
	if txn.BaseRegistrySHA256 != "" {
		baseDigest, err := hex.DecodeString(txn.BaseRegistrySHA256)
		if err != nil || len(baseDigest) != sha256.Size {
			return fmt.Errorf(
				"invalid pulse ledger transaction base_registry_sha256 %q",
				txn.BaseRegistrySHA256,
			)
		}
	}
	digest, err := hex.DecodeString(txn.RegistrySHA256)
	if err != nil || len(digest) != sha256.Size {
		return fmt.Errorf("invalid pulse ledger transaction registry_sha256 %q", txn.RegistrySHA256)
	}
	if txn.Offered == nil {
		return fmt.Errorf("pulse ledger transaction is missing offered names")
	}
	for i, name := range txn.Offered {
		if name == "" {
			return fmt.Errorf("pulse ledger transaction contains an empty offered name")
		}
		if i > 0 && txn.Offered[i-1] >= name {
			return fmt.Errorf("pulse ledger transaction offered names are not canonical")
		}
	}
	return nil
}

func removePulseLedgerTransaction(hostPath string, ops pulseFileOps) error {
	path := pulseLedgerTransactionPath(hostPath)
	if err := ops.remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove pulse ledger transaction %s: %w", path, err)
	}
	return nil
}
