package override

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func testCodexHookSource() *CodexHookSource {
	return &CodexHookSource{
		Repository: "/reviewed/dear-agent",
		Commit:     strings.Repeat("a", 40),
		Digest:     strings.Repeat("b", 64),
	}
}

func testCodexHookSubject(t *testing.T) string {
	t.Helper()
	subject, err := testCodexHookSource().Subject()
	if err != nil {
		t.Fatal(err)
	}
	return subject
}

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

func TestKindsIncludesEverySharedDangerousOverride(t *testing.T) {
	kinds := Kinds()
	if got := len(kinds); got != MaxLedgerUsesPerTransaction {
		t.Fatalf("len(Kinds()) = %d, transaction bound = %d; keep them synchronized",
			got, MaxLedgerUsesPerTransaction)
	}
	for _, want := range []Kind{
		KindCodexHookTrust,
		KindAdmissionBrake,
		KindSupervisorOAuthCheck,
	} {
		if !slices.Contains(kinds, want) {
			t.Errorf("Kinds() = %v, missing %q", kinds, want)
		}
		if !want.Valid() {
			t.Errorf("shared override kind %q is not valid", want)
		}
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

func TestPrivilegedAppendRequestIsCanonicalAndLauncherBound(t *testing.T) {
	use := Use{
		Kind:            KindAdmissionBrake,
		Reason:          "host recovered and the operator is verifying one guarded spawn",
		Actor:           "vroom-dispatch",
		Session:         "vroom-orchestrator",
		AuthorizationID: strings.Repeat("a", AuthorizationIDBytes*2),
		AtUTC:           time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC),
	}
	request, err := EncodePrivilegedAppendRequest([]Use{use}, 4242)
	if err != nil {
		t.Fatal(err)
	}
	uses, transaction, launcherPID, err := DecodePrivilegedAppendRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	if launcherPID != 4242 || len(uses) != 1 || uses[0] != use {
		t.Fatalf("decoded request = uses %+v pid %d", uses, launcherPID)
	}
	wantTransaction, err := EncodeLedgerUses([]Use{use})
	if err != nil {
		t.Fatal(err)
	}
	if string(transaction) != string(wantTransaction) {
		t.Fatalf("decoded transaction = %q, want %q", transaction, wantTransaction)
	}

	nonCanonical := append([]byte(" "), request...)
	if _, _, _, err := DecodePrivilegedAppendRequest(nonCanonical); err == nil {
		t.Fatal("non-canonical privileged append request was accepted")
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
		CodexHooks: testCodexHookSource(),
	}); err != nil {
		t.Fatalf("save grant: %v", err)
	}

	use, err := Authorize(Request{
		Kind:    KindCodexHookTrust,
		Reason:  "sandbox path rotates per spawn so hooks cannot be pre-trusted",
		Actor:   "vroom-dispatch",
		Session: "worker-ce-2ved",
		Subject: testCodexHookSubject(t),
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

func TestCodexHookGrantIsBoundToReviewedBytes(t *testing.T) {
	configureTestStore(t)
	now := time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC)
	if err := SaveGrant(Grant{
		Kind:       KindCodexHookTrust,
		ApprovedBy: "valentin",
		ExpiresUTC: now.Add(time.Hour),
		CodexHooks: testCodexHookSource(),
	}); err != nil {
		t.Fatalf("save grant: %v", err)
	}
	request := Request{
		Kind:    KindCodexHookTrust,
		Reason:  "sandbox path rotates per spawn so hooks cannot be pre-trusted",
		Subject: testCodexHookSubject(t),
		Now:     now,
	}
	request.Subject = "codex-hooks:sha256:" + strings.Repeat("c", 64)
	if _, err := Reserve(request); !errors.Is(err, ErrGrantSubject) {
		t.Fatalf("mismatched hook subject error = %v, want ErrGrantSubject", err)
	}

	if err := SaveGrant(Grant{
		Kind:       KindCodexHookTrust,
		ApprovedBy: "valentin",
		ExpiresUTC: now.Add(time.Hour),
	}); err != nil {
		t.Fatalf("save legacy generic grant: %v", err)
	}
	request.Subject = testCodexHookSubject(t)
	if _, err := Reserve(request); !errors.Is(err, ErrGrantSubject) {
		t.Fatalf("generic hook grant error = %v, want ErrGrantSubject", err)
	}
}

func TestCommitProofsRevalidatesAndRecordsExactReservation(t *testing.T) {
	configureTestStore(t)
	now := time.Now().UTC()
	if err := SaveGrant(Grant{
		Kind:       KindCodexHookTrust,
		ApprovedBy: "valentin",
		ExpiresUTC: now.Add(time.Hour),
		CodexHooks: testCodexHookSource(),
	}); err != nil {
		t.Fatalf("save grant: %v", err)
	}
	reservation, err := Reserve(Request{
		Kind:    KindCodexHookTrust,
		Reason:  "sandbox path rotates per spawn so hooks cannot be pre-trusted",
		Actor:   "vroom-dispatch",
		Session: "worker-ce-6xfu",
		Subject: testCodexHookSubject(t),
		Now:     now,
	})
	if err != nil {
		t.Fatalf("reserve: %v", err)
	}
	proof := reservation.Proof()
	if _, err := CommitProofs(proof); err != nil {
		t.Fatalf("commit prepared proof: %v", err)
	}
	if uses, err := LoadUses(time.Time{}); err != nil || len(uses) != 1 ||
		uses[0].AuthorizationID != proof.AuthorizationID {
		t.Fatalf("committed proof uses = %+v, err = %v", uses, err)
	}

	proof.Session = "different-worker"
	proof.Subject = "codex-hooks:sha256:" + strings.Repeat("c", 64)
	if _, err := CommitProofs(proof); !errors.Is(err, ErrGrantSubject) {
		t.Fatalf("tampered prepared proof error = %v, want ErrGrantSubject", err)
	}

	unknown := proof
	unknown.Kind = Kind("future-override")
	if _, err := CommitProofs(unknown); !errors.Is(err, ErrUnknownKind) {
		t.Fatalf("unknown prepared proof error = %v, want ErrUnknownKind", err)
	}

	duplicate := reservation.Proof()
	duplicate.Kind = KindAdmissionBrake
	duplicate.Subject = ""
	if _, err := CommitProofs(reservation.Proof(), duplicate); !errors.Is(err, ErrLedgerRecord) {
		t.Fatalf("duplicate authorization ID error = %v, want ErrLedgerRecord", err)
	}
}

func TestReservationRecordsOnlyAfterCommit(t *testing.T) {
	configureTestStore(t)
	now := time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC)
	if err := SaveGrant(Grant{
		Kind:       KindAdmissionBrake,
		ApprovedBy: "valentin",
		ExpiresUTC: now.Add(time.Hour),
	}); err != nil {
		t.Fatalf("save grant: %v", err)
	}

	reservation, err := Reserve(Request{
		Kind:    KindAdmissionBrake,
		Reason:  "host recovered and the operator is verifying one guarded spawn",
		Actor:   "vroom-dispatch",
		Session: "worker-ce-6xfu",
		Now:     now,
	})
	if err != nil {
		t.Fatalf("reserve: %v", err)
	}
	if uses, err := LoadUses(time.Time{}); err != nil || len(uses) != 0 {
		t.Fatalf("reservation consumed ledger quota before final check: uses=%+v err=%v", uses, err)
	}

	use, err := reservation.Commit()
	if err != nil {
		t.Fatalf("commit: %v", err)
	}
	if use.Session != "worker-ce-6xfu" {
		t.Fatalf("committed use lost attribution: %+v", use)
	}
	uses, err := LoadUses(time.Time{})
	if err != nil {
		t.Fatalf("load uses: %v", err)
	}
	if len(uses) != 1 || uses[0].Reason != use.Reason {
		t.Fatalf("committed ledger = %+v, want one reserved use", uses)
	}
	if _, err := reservation.Commit(); !errors.Is(err, ErrReservationCommitted) {
		t.Fatalf("second commit error = %v, want ErrReservationCommitted", err)
	}
}

func TestCommitAllRecordsCombinedReservationsAtomically(t *testing.T) {
	configureTestStore(t)
	for _, kind := range []Kind{KindAdmissionBrake, KindSupervisorOAuthCheck} {
		if err := SaveGrant(Grant{
			Kind:       kind,
			ApprovedBy: "valentin",
			ExpiresUTC: time.Now().Add(time.Hour),
		}); err != nil {
			t.Fatalf("save %s grant: %v", kind, err)
		}
	}
	reserveBoth := func() (*Reservation, *Reservation) {
		t.Helper()
		brake, err := Reserve(Request{
			Kind:    KindAdmissionBrake,
			Reason:  "host recovered and the operator is verifying one guarded spawn",
			Session: "vroom-orchestrator",
		})
		if err != nil {
			t.Fatalf("reserve brake: %v", err)
		}
		oauth, err := Reserve(Request{
			Kind:    KindSupervisorOAuthCheck,
			Reason:  "validating a development supervisor without stored OAuth",
			Session: "vroom-orchestrator",
		})
		if err != nil {
			t.Fatalf("reserve OAuth: %v", err)
		}
		return brake, oauth
	}

	brake, oauth := reserveBoth()
	if err := SaveGrant(Grant{
		Kind:       KindSupervisorOAuthCheck,
		ApprovedBy: "valentin",
		ExpiresUTC: time.Now().Add(-time.Minute),
	}); err != nil {
		t.Fatalf("expire OAuth grant: %v", err)
	}
	if _, err := CommitAll(brake, oauth); !errors.Is(err, ErrGrantExpired) {
		t.Fatalf("combined commit error = %v, want ErrGrantExpired", err)
	}
	if uses, err := LoadUses(time.Time{}); err != nil || len(uses) != 0 {
		t.Fatalf("failed combined commit partially recorded: uses=%+v err=%v", uses, err)
	}

	if err := SaveGrant(Grant{
		Kind:       KindSupervisorOAuthCheck,
		ApprovedBy: "valentin",
		ExpiresUTC: time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatalf("restore OAuth grant: %v", err)
	}
	brake, oauth = reserveBoth()
	uses, err := CommitAll(brake, oauth)
	if err != nil {
		t.Fatalf("commit combined reservations: %v", err)
	}
	if len(uses) != 2 {
		t.Fatalf("combined uses = %+v, want two", uses)
	}
	recorded, err := LoadUses(time.Time{})
	if err != nil {
		t.Fatalf("load combined uses: %v", err)
	}
	if len(recorded) != 2 ||
		recorded[0].Kind != KindAdmissionBrake ||
		recorded[1].Kind != KindSupervisorOAuthCheck {
		t.Fatalf("combined ledger = %+v, want brake and OAuth records", recorded)
	}
}

func TestCommitAllDoesNotSpendBrakeWhenCodexGrantExpires(t *testing.T) {
	configureTestStore(t)
	now := time.Now()
	if err := SaveGrant(Grant{
		Kind:       KindAdmissionBrake,
		ApprovedBy: "valentin",
		ExpiresUTC: now.Add(time.Hour),
	}); err != nil {
		t.Fatalf("save brake grant: %v", err)
	}
	if err := SaveGrant(Grant{
		Kind:       KindCodexHookTrust,
		ApprovedBy: "valentin",
		ExpiresUTC: now.Add(time.Hour),
		CodexHooks: testCodexHookSource(),
	}); err != nil {
		t.Fatalf("save Codex grant: %v", err)
	}
	brake, err := Reserve(Request{
		Kind:    KindAdmissionBrake,
		Reason:  "host recovered and the operator is verifying one guarded spawn",
		Session: "worker-ce-6xfu",
	})
	if err != nil {
		t.Fatalf("reserve brake: %v", err)
	}
	hooks, err := Reserve(Request{
		Kind:    KindCodexHookTrust,
		Reason:  "sandbox path rotates per spawn so hooks cannot be pre-trusted",
		Session: "worker-ce-6xfu",
		Subject: testCodexHookSubject(t),
	})
	if err != nil {
		t.Fatalf("reserve Codex hooks: %v", err)
	}
	if err := SaveGrant(Grant{
		Kind:       KindCodexHookTrust,
		ApprovedBy: "valentin",
		ExpiresUTC: now.Add(-time.Minute),
		CodexHooks: testCodexHookSource(),
	}); err != nil {
		t.Fatalf("expire Codex grant: %v", err)
	}
	if _, err := CommitAll(brake, hooks); !errors.Is(err, ErrGrantExpired) {
		t.Fatalf("combined brake/Codex commit error = %v, want ErrGrantExpired", err)
	}
	if uses, err := LoadUses(time.Time{}); err != nil || len(uses) != 0 {
		t.Fatalf("expired Codex grant spent brake use: uses=%+v err=%v", uses, err)
	}
}

func TestAbandonedReservationDoesNotConsumeLedgerQuota(t *testing.T) {
	configureTestStore(t)
	now := time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC)
	if err := SaveGrant(Grant{
		Kind:       KindAdmissionBrake,
		ApprovedBy: "valentin",
		ExpiresUTC: now.Add(time.Hour),
	}); err != nil {
		t.Fatalf("save grant: %v", err)
	}
	if _, err := Reserve(Request{
		Kind:   KindAdmissionBrake,
		Reason: "concurrent stagger gate changed before the final live check",
		Now:    now,
	}); err != nil {
		t.Fatalf("reserve: %v", err)
	}
	if uses, err := LoadUses(time.Time{}); err != nil || len(uses) != 0 {
		t.Fatalf("abandoned reservation was recorded: uses=%+v err=%v", uses, err)
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
