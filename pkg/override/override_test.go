package override

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
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

// An override with no human approval must refuse, and must not leave a ledger
// entry: a refused attempt is not a use.
func TestAuthorizeRefusesWithoutGrant(t *testing.T) {
	t.Setenv("AGM_CONFIG_DIR", t.TempDir())

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
	t.Setenv("AGM_CONFIG_DIR", t.TempDir())
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
	t.Setenv("AGM_CONFIG_DIR", t.TempDir())
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

func TestGrantPathsLiveUnderConfigDir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AGM_CONFIG_DIR", dir)
	if want := filepath.Join(dir, "overrides", "grants", "codex-hook-trust.json"); GrantPath(KindCodexHookTrust) != want {
		t.Fatalf("GrantPath = %q, want %q", GrantPath(KindCodexHookTrust), want)
	}
	if want := filepath.Join(dir, "overrides", "ledger.jsonl"); LedgerPath() != want {
		t.Fatalf("LedgerPath = %q, want %q", LedgerPath(), want)
	}
}

func mustJSON(t *testing.T, grant Grant) []byte {
	t.Helper()
	data, err := json.Marshal(grant)
	if err != nil {
		t.Fatalf("marshal grant: %v", err)
	}
	return data
}
