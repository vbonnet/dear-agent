package override

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestValidateReasonRefusesUnauditableReasons(t *testing.T) {
	for name, reason := range map[string]string{
		"empty":                  "",
		"whitespace only":        "   \t\n ",
		"too short":              "too short",
		"padded to length":       "fix     \t   ",
		"boilerplate":            "workaround",
		"boilerplate cased":      "Just Testing",
		"boilerplate punctuated": "because!",
		"oversized ASCII":        strings.Repeat("a", MaxReasonBytes+1),
		"oversized UTF-8":        strings.Repeat("é", MaxReasonBytes/2+1),
	} {
		if _, err := ValidateReason(reason); err == nil {
			t.Errorf("%s: reason %q was accepted", name, reason)
		}
	}

	normalized, err := ValidateReason("  sandbox   path\nrotates per spawn, hooks can never be pre-trusted  ")
	if err != nil {
		t.Fatalf("real reason refused: %v", err)
	}
	if want := "sandbox path rotates per spawn, hooks can never be pre-trusted"; normalized != want {
		t.Fatalf("normalized = %q, want %q", normalized, want)
	}
}

func TestEncodeLedgerUseBoundsEveryCallerControlledField(t *testing.T) {
	base := Use{
		Kind:   KindAdmissionBrake,
		Reason: "disk watchdog clobbered the SRE hold, verifying the fix once",
		Actor:  "vroom-dispatch",
		AtUTC:  time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC),
	}
	if _, err := EncodeLedgerUse(base); err != nil {
		t.Fatalf("valid record refused: %v", err)
	}

	oversizedActor := base
	oversizedActor.Actor = strings.Repeat("a", MaxActorBytes+1)
	if _, err := EncodeLedgerUse(oversizedActor); !errors.Is(err, ErrLedgerRecord) {
		t.Fatalf("oversized actor error = %v, want ErrLedgerRecord", err)
	}

	oversizedSession := base
	oversizedSession.Session = strings.Repeat("s", MaxSessionBytes+1)
	if _, err := EncodeLedgerUse(oversizedSession); !errors.Is(err, ErrLedgerRecord) {
		t.Fatalf("oversized session error = %v, want ErrLedgerRecord", err)
	}
}

// An override with no human approval must refuse, and must not leave a ledger
// entry: a refused attempt is not a use.
func TestAuthorizeRefusesWithoutGrant(t *testing.T) {
	configureTestStore(t)

	_, err := Authorize(Request{
		Kind:   KindCodexHookTrust,
		Reason: "sandbox path rotates per spawn so hooks cannot be pre-trusted",
	})
	if !errors.Is(err, ErrNoGrant) {
		t.Fatalf("err = %v, want ErrNoGrant", err)
	}
	if uses, _ := LoadUses(time.Time{}); len(uses) != 0 {
		t.Fatalf("refused override was recorded: %+v", uses)
	}
}

func TestAuthorizeRefusesExpiredAndMismatchedGrants(t *testing.T) {
	configureTestStore(t)
	now := time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC)

	if err := SaveGrant(Grant{
		Kind:       KindCodexHookTrust,
		ApprovedBy: "valentin",
		ExpiresUTC: now.Add(-time.Minute),
	}); err != nil {
		t.Fatalf("save grant: %v", err)
	}
	_, err := Authorize(Request{
		Kind:   KindCodexHookTrust,
		Reason: "sandbox path rotates per spawn so hooks cannot be pre-trusted",
		Now:    now,
	})
	if !errors.Is(err, ErrGrantExpired) {
		t.Fatalf("err = %v, want ErrGrantExpired", err)
	}

	// A grant for one kind must not authorize another.
	grant := Grant{Kind: KindCodexHookTrust, ApprovedBy: "valentin", ExpiresUTC: now.Add(time.Hour)}
	if err := os.WriteFile(GrantPath(KindAdmissionBrake), mustJSON(t, grant), 0o600); err != nil {
		t.Fatalf("write cross-kind grant: %v", err)
	}
	if _, err := Authorize(Request{
		Kind:   KindAdmissionBrake,
		Reason: "disk watchdog clobbered the SRE hold, verifying the fix once",
		Now:    now,
	}); !errors.Is(err, ErrGrantKind) {
		t.Fatalf("err = %v, want ErrGrantKind", err)
	}
}

func TestAuthorizeRecordsGrantedUse(t *testing.T) {
	configureTestStore(t)
	now := time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC)
	if err := SaveGrant(Grant{
		Kind:       KindCodexHookTrust,
		ApprovedBy: "valentin",
		ExpiresUTC: now.Add(time.Hour),
	}); err != nil {
		t.Fatalf("save grant: %v", err)
	}

	use, err := Authorize(Request{
		Kind:    KindCodexHookTrust,
		Reason:  "sandbox path rotates per spawn so hooks cannot be pre-trusted",
		Actor:   "vroom-dispatch",
		Session: "worker-ce-2ved",
		Now:     now,
	})
	if err != nil {
		t.Fatalf("authorize: %v", err)
	}
	if use.Actor != "vroom-dispatch" || use.Session != "worker-ce-2ved" {
		t.Fatalf("use lost attribution: %+v", use)
	}

	uses, err := LoadUses(time.Time{})
	if err != nil {
		t.Fatalf("load uses: %v", err)
	}
	if len(uses) != 1 || uses[0].Reason != use.Reason || uses[0].Kind != KindCodexHookTrust {
		t.Fatalf("ledger = %+v, want the single authorized use", uses)
	}
}

// The ledger is what the audit gate reads, so an override that cannot be
// recorded must not be treated as authorized.
func TestAuthorizeFailsClosedWhenLedgerUnwritable(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AGM_CONFIG_DIR", dir)
	configureTestGrantDir(t, filepath.Join(dir, "operator-grants"))
	now := time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC)
	if err := SaveGrant(Grant{Kind: KindAdmissionBrake, ApprovedBy: "v", ExpiresUTC: now.Add(time.Hour)}); err != nil {
		t.Fatalf("save grant: %v", err)
	}
	// Occupy the ledger path with a directory so the append cannot succeed.
	if err := os.MkdirAll(LedgerPath(), 0o700); err != nil {
		t.Fatalf("stage unwritable ledger: %v", err)
	}

	if _, err := Authorize(Request{
		Kind:   KindAdmissionBrake,
		Reason: "disk watchdog clobbered the SRE hold, verifying the fix once",
		Now:    now,
	}); err == nil {
		t.Fatal("override was authorized despite an unrecordable ledger")
	}
}

// A write-only ledger is not auditable. O_APPEND|O_WRONLY used to authorize
// this case even though every subsequent audit failed to read the record.
func TestAuthorizeFailsClosedWhenLedgerUnreadable(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses the mode-bit read denial")
	}
	dir := t.TempDir()
	t.Setenv("AGM_CONFIG_DIR", dir)
	configureTestGrantDir(t, filepath.Join(dir, "operator-grants"))
	now := time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC)
	if err := SaveGrant(Grant{Kind: KindAdmissionBrake, ApprovedBy: "v", ExpiresUTC: now.Add(time.Hour)}); err != nil {
		t.Fatalf("save grant: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(LedgerPath()), 0o700); err != nil {
		t.Fatalf("create ledger dir: %v", err)
	}
	if err := os.WriteFile(LedgerPath(), nil, 0o200); err != nil {
		t.Fatalf("stage write-only ledger: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(LedgerPath(), 0o600) })

	if _, err := Authorize(Request{
		Kind:   KindAdmissionBrake,
		Reason: "disk watchdog clobbered the SRE hold, verifying the fix once",
		Now:    now,
	}); err == nil {
		t.Fatal("override was authorized despite an unreadable ledger")
	}
}

func TestAuditAlertsPerKindAndRanksRepeatedReasons(t *testing.T) {
	now := time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC)
	repeated := "sandbox path rotates per spawn so hooks cannot be pre-trusted"
	uses := []Use{
		{Kind: KindCodexHookTrust, Reason: repeated, AtUTC: now},
		{Kind: KindCodexHookTrust, Reason: repeated, AtUTC: now},
		{Kind: KindCodexHookTrust, Reason: repeated, AtUTC: now},
		{Kind: KindCodexHookTrust, Reason: "one-off during the 0.146 upgrade window", AtUTC: now},
		{Kind: KindAdmissionBrake, Reason: "disk watchdog clobbered the SRE hold once", AtUTC: now},
	}

	report := Audit(uses, 7*24*time.Hour, 3, now)
	if !report.Breached {
		t.Fatal("threshold of 3 was not reported as breached at 4 hook-trust uses")
	}
	if len(report.Breaches) != 1 || report.Breaches[0].Kind != KindCodexHookTrust {
		t.Fatalf("breaches = %+v, want only codex-hook-trust", report.Breaches)
	}
	// The brake's single use must not be offset into a breach by the other kind.
	if report.ByKind[KindAdmissionBrake] != 1 {
		t.Fatalf("brake count = %d, want 1", report.ByKind[KindAdmissionBrake])
	}
	if len(report.ByReason) == 0 || report.ByReason[0].Reason != repeated || report.ByReason[0].Count != 3 {
		t.Fatalf("top reason = %+v, want the thrice-repeated one", report.ByReason)
	}

	if Audit(uses, 7*24*time.Hour, 0, now).Breached {
		t.Fatal("threshold 0 should disable the alert")
	}
}

func TestConfiguredTestGrantAndLedgerUseSeparatePaths(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AGM_CONFIG_DIR", dir)
	operatorDir := filepath.Join(dir, "operator-grants")
	configureTestGrantDir(t, operatorDir)
	if want := filepath.Join(operatorDir, "dear-agent-override-codex-hook-trust.json"); GrantPath(KindCodexHookTrust) != want {
		t.Fatalf("GrantPath = %q, want %q", GrantPath(KindCodexHookTrust), want)
	}
	if want := filepath.Join(dir, "overrides", "ledger.jsonl"); LedgerPath() != want {
		t.Fatalf("LedgerPath = %q, want %q", LedgerPath(), want)
	}
}

func TestProductionStoresIgnoreAgentConfigDir(t *testing.T) {
	t.Setenv("AGM_CONFIG_DIR", t.TempDir())
	oldDir, oldEnforcement := grantDirPath, enforceOperatorOwnership
	oldLedger, oldLedgerEnforcement := ledgerFilePath, enforceOperatorLedger
	grantDirPath, enforceOperatorOwnership = operatorGrantDir, true
	ledgerFilePath, enforceOperatorLedger = operatorLedgerPath, true
	t.Cleanup(func() {
		grantDirPath, enforceOperatorOwnership = oldDir, oldEnforcement
		ledgerFilePath, enforceOperatorLedger = oldLedger, oldLedgerEnforcement
	})
	if got := GrantDir(); got != operatorGrantDir {
		t.Fatalf("GrantDir = %q, want operator-owned %q", got, operatorGrantDir)
	}
	if got := LedgerPath(); got != operatorLedgerPath {
		t.Fatalf("LedgerPath = %q, want operator-owned %q", got, operatorLedgerPath)
	}
}

func TestLoadGrantRejectsSameUserJSON(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("cannot create a non-root-owned fixture while running as root")
	}
	dir := t.TempDir()
	oldDir, oldEnforcement := grantDirPath, enforceOperatorOwnership
	grantDirPath, enforceOperatorOwnership = dir, true
	t.Cleanup(func() {
		grantDirPath, enforceOperatorOwnership = oldDir, oldEnforcement
	})
	grant := Grant{
		Kind:       KindCodexHookTrust,
		ApprovedBy: "unattended-agent",
		ExpiresUTC: time.Now().Add(time.Hour),
	}
	if err := os.WriteFile(GrantPath(grant.Kind), mustJSON(t, grant), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadGrant(grant.Kind); !errors.Is(err, ErrGrantUntrusted) {
		t.Fatalf("LoadGrant error = %v, want ErrGrantUntrusted", err)
	}
}

func TestLoadUsesRejectsSameUserLedger(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("cannot create a non-root-owned fixture while running as root")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "ledger.jsonl")
	oldPath, oldEnforcement := ledgerFilePath, enforceOperatorLedger
	ledgerFilePath, enforceOperatorLedger = path, true
	t.Cleanup(func() {
		ledgerFilePath, enforceOperatorLedger = oldPath, oldEnforcement
	})
	if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadUses(time.Time{}); !errors.Is(err, ErrLedgerUntrusted) {
		t.Fatalf("LoadUses error = %v, want ErrLedgerUntrusted", err)
	}
	if err := Record(Use{
		Kind:   KindAdmissionBrake,
		Reason: "disk watchdog clobbered the SRE hold, verifying the fix once",
		Actor:  "test",
		AtUTC:  time.Now(),
	}); !errors.Is(err, ErrLedgerUntrusted) {
		t.Fatalf("Record error = %v, want ErrLedgerUntrusted", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "{}\n" {
		t.Fatalf("same-user ledger changed after rejected append: %q", data)
	}
}

func configureTestStore(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("AGM_CONFIG_DIR", dir)
	configureTestGrantDir(t, filepath.Join(dir, "operator-grants"))
}

func configureTestGrantDir(t *testing.T, dir string) {
	t.Helper()
	oldDir, oldEnforcement := grantDirPath, enforceOperatorOwnership
	oldLedger, oldLedgerEnforcement := ledgerFilePath, enforceOperatorLedger
	grantDirPath, enforceOperatorOwnership = dir, false
	ledgerFilePath = filepath.Join(filepath.Dir(dir), "overrides", "ledger.jsonl")
	enforceOperatorLedger = false
	t.Cleanup(func() {
		grantDirPath, enforceOperatorOwnership = oldDir, oldEnforcement
		ledgerFilePath, enforceOperatorLedger = oldLedger, oldLedgerEnforcement
	})
}

func mustJSON(t *testing.T, grant Grant) []byte {
	t.Helper()
	data, err := json.Marshal(grant)
	if err != nil {
		t.Fatalf("marshal grant: %v", err)
	}
	return data
}
