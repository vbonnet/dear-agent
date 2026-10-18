package deploy

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestMissingRegistryPreviewReportsSeedLedgerWriteEvenWhenLedgerBytesMatch(t *testing.T) {
	dir := t.TempDir()
	host := filepath.Join(dir, "nested", "host.json")
	defaultsRaw := validPulseConfig("a")
	if err := os.MkdirAll(filepath.Dir(host), 0o755); err != nil {
		t.Fatal(err)
	}
	ledgerRaw := []byte(`["a"]`)
	if err := os.WriteFile(pulseLedgerPath(host), ledgerRaw, 0o644); err != nil {
		t.Fatal(err)
	}

	preview, err := PendingPulseMergePreviewRendered(
		host,
		defaultsRaw,
		map[string]bool{"a": true},
	)
	if err != nil {
		t.Fatalf("preview missing registry: %v", err)
	}
	if !slices.Equal(preview.Added, []string{"a"}) || !preview.LedgerChanged {
		t.Fatalf("preview = %+v, want added [a] and ledger change", preview)
	}
	requirePathMissing(t, host)
	requirePathMissing(t, pulseLedgerTransactionPath(host))
	currentLedger, err := os.ReadFile(pulseLedgerPath(host))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(currentLedger, ledgerRaw) {
		t.Fatalf("preview changed ledger: got %q, want %q", currentLedger, ledgerRaw)
	}

	result, err := MergeRequiredPulsesRenderedResult(
		host,
		defaultsRaw,
		0o644,
		map[string]bool{"a": true},
	)
	if err != nil {
		t.Fatalf("seed missing registry: %v", err)
	}
	if !slices.Equal(result.Added, []string{"a"}) || !result.Created || !result.LedgerChanged || result.Reconciled {
		t.Fatalf("result = %+v, want created seed with ledger change", result)
	}
	registryRaw, err := os.ReadFile(host)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(registryRaw, defaultsRaw) {
		t.Fatalf("seeded registry = %q, want %q", registryRaw, defaultsRaw)
	}
	currentLedger, err = os.ReadFile(pulseLedgerPath(host))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(currentLedger, ledgerRaw) {
		t.Fatalf("seeded ledger = %q, want %q", currentLedger, ledgerRaw)
	}
	requirePathMissing(t, pulseLedgerTransactionPath(host))
}

func TestPendingPulseMergePreview_ReportsLedgerOnlyAdoption(t *testing.T) {
	dir := t.TempDir()
	host := filepath.Join(dir, "host.json")
	defaultsRaw := validPulseConfig("existing")
	if err := os.WriteFile(host, defaultsRaw, 0o644); err != nil {
		t.Fatal(err)
	}
	required := map[string]bool{"existing": true}

	preview, err := PendingPulseMergePreviewRendered(host, defaultsRaw, required)
	if err != nil {
		t.Fatalf("preview first adoption: %v", err)
	}
	if len(preview.Added) != 0 || !preview.LedgerChanged {
		t.Fatalf("preview = %+v, want ledger-only adoption", preview)
	}
	requirePathMissing(t, pulseLedgerPath(host))
	requirePathMissing(t, pulseLedgerTransactionPath(host))
	registryRaw, err := os.ReadFile(host)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(registryRaw, defaultsRaw) {
		t.Fatalf("preview changed registry: got %q, want %q", registryRaw, defaultsRaw)
	}

	result, err := MergeRequiredPulsesRenderedResult(host, defaultsRaw, 0o644, required)
	if err != nil {
		t.Fatalf("adopt ledger: %v", err)
	}
	if len(result.Added) != 0 || result.Created || !result.LedgerChanged || result.Reconciled {
		t.Fatalf("result = %+v, want ledger-only update", result)
	}
	registryRaw, err = os.ReadFile(host)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(registryRaw, defaultsRaw) {
		t.Fatalf("ledger adoption changed registry: got %q, want %q", registryRaw, defaultsRaw)
	}
	ledgerRaw, err := os.ReadFile(pulseLedgerPath(host))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(ledgerRaw, []byte(`["existing"]`)) {
		t.Fatalf("ledger = %q, want canonical existing entry", ledgerRaw)
	}
	requirePathMissing(t, pulseLedgerTransactionPath(host))

	preview, err = PendingPulseMergePreviewRendered(host, defaultsRaw, required)
	if err != nil {
		t.Fatalf("preview current ledger: %v", err)
	}
	if len(preview.Added) != 0 || preview.LedgerChanged {
		t.Fatalf("current preview = %+v, want clean", preview)
	}
	requirePathMissing(t, pulseLedgerTransactionPath(host))
	result, err = MergeRequiredPulsesRenderedResult(host, defaultsRaw, 0o644, required)
	if err != nil {
		t.Fatalf("repeat current merge: %v", err)
	}
	if len(result.Added) != 0 || result.Created || result.LedgerChanged || result.Reconciled {
		t.Fatalf("repeat result = %+v, want unchanged", result)
	}

	nonCanonical := []byte("[\n  \"existing\"\n]\n")
	if err := os.WriteFile(pulseLedgerPath(host), nonCanonical, 0o644); err != nil {
		t.Fatal(err)
	}
	preview, err = PendingPulseMergePreviewRendered(host, defaultsRaw, required)
	if err != nil {
		t.Fatalf("preview noncanonical ledger: %v", err)
	}
	if len(preview.Added) != 0 || !preview.LedgerChanged {
		t.Fatalf("noncanonical preview = %+v, want ledger-only rewrite", preview)
	}
	requirePathMissing(t, pulseLedgerTransactionPath(host))
	result, err = MergeRequiredPulsesRenderedResult(host, defaultsRaw, 0o644, required)
	if err != nil {
		t.Fatalf("canonicalize ledger: %v", err)
	}
	if len(result.Added) != 0 || result.Created || !result.LedgerChanged || result.Reconciled {
		t.Fatalf("canonicalization result = %+v, want ledger-only update", result)
	}
	ledgerRaw, err = os.ReadFile(pulseLedgerPath(host))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(ledgerRaw, []byte(`["existing"]`)) {
		t.Fatalf("canonicalized ledger = %q, want %q", ledgerRaw, []byte(`["existing"]`))
	}
}
