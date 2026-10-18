package override

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultJudge_RejectsLowEffortReasons(t *testing.T) {
	tests := []struct {
		name   string
		reason string
		want   bool // allow?
	}{
		{"empty", "", false},
		{"whitespace", "   \t ", false},
		{"single junk x", "x", false},
		{"junk force", "force", false},
		{"junk because", "because", false},
		{"junk n/a", "N/A", false},
		{"junk trust me", "trust me", false},
		{"dots only", "....", false},
		{"repeated char", "aaaaaaaaaaaa", false},
		{"too short word", "fix", false},
		{"too short sentence", "do it now", false}, // 9 chars
		{"real reason gemini down", "gemini bot is unreachable due to quota exhaustion", true},
		{"real reason migrate", "re-running migration after a partial failure left the target half-written", true},
		{"borderline ok length", "stale lock from a crashed run", true},
	}
	j := DefaultJudge{}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			v, err := j.Evaluate(context.Background(), JudgeRequest{Reason: tc.reason})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if v.Allow != tc.want {
				t.Fatalf("reason %q: allow=%v want=%v (explanation: %s)", tc.reason, v.Allow, tc.want, v.Explanation)
			}
			if !v.Allow && strings.TrimSpace(v.Explanation) == "" {
				t.Fatalf("denied verdict must carry an explanation")
			}
		})
	}
}

func TestRequire_DeniesWithoutReason(t *testing.T) {
	var captured []auditEntry
	g := Guard{
		Tool: "agm send-compact", Flag: "--force",
		Gate: "anti-loop guard", Risk: RiskP0,
		auditSink: func(e auditEntry) { captured = append(captured, e) },
	}
	err := Require(context.Background(), g, "")
	if err == nil {
		t.Fatal("expected refusal when no reason provided")
	}
	var denied *DeniedError
	if !errors.As(err, &denied) {
		t.Fatalf("want *DeniedError, got %T", err)
	}
	if !strings.Contains(err.Error(), "--reason") {
		t.Fatalf("error should guide the caller to --reason, got: %s", err.Error())
	}
	if !strings.Contains(err.Error(), "anti-loop guard") {
		t.Fatalf("error should name the gate, got: %s", err.Error())
	}
	if len(captured) != 1 || captured[0].Allowed {
		t.Fatalf("denial must be audited as not-allowed: %+v", captured)
	}
}

func TestRequire_AllowsRealReason(t *testing.T) {
	// Keep this deterministic and offline: with no API key the P1 default judge
	// collapses to the deterministic floor (audited as "default").
	t.Setenv("ANTHROPIC_API_KEY", "")
	var captured []auditEntry
	g := Guard{
		Tool: "engram migrate", Flag: "--force",
		Gate: "overwrites an existing target workspace", Risk: RiskP1,
		auditSink: func(e auditEntry) { captured = append(captured, e) },
	}
	reason := "previous migration aborted midway; re-running to finish the half-written target"
	if err := Require(context.Background(), g, reason); err != nil {
		t.Fatalf("expected allow, got: %v", err)
	}
	if len(captured) != 1 || !captured[0].Allowed {
		t.Fatalf("allow must be audited as allowed: %+v", captured)
	}
	if captured[0].Reason != reason {
		t.Fatalf("audit must record the reason verbatim, got %q", captured[0].Reason)
	}
	if captured[0].Judge != "default" {
		t.Fatalf("audit must record judge name, got %q", captured[0].Judge)
	}
}

// failingJudge always errors, to prove fail-closed behavior.
type failingJudge struct{}

func (failingJudge) Name() string { return "failing" }
func (failingJudge) Evaluate(context.Context, JudgeRequest) (JudgeVerdict, error) {
	return JudgeVerdict{}, errors.New("classifier unreachable")
}

func TestRequire_FailsClosedOnJudgeError(t *testing.T) {
	var captured []auditEntry
	g := Guard{
		Tool: "agm select-option", Flag: "--force",
		Gate: "safety guard", Risk: RiskP0, Judge: failingJudge{},
		auditSink: func(e auditEntry) { captured = append(captured, e) },
	}
	// Even a perfectly good reason must be refused if the judge cannot decide.
	err := Require(context.Background(), g, "a genuinely detailed and honest justification here")
	if err == nil {
		t.Fatal("expected fail-closed refusal on judge error")
	}
	if len(captured) != 1 || captured[0].Allowed {
		t.Fatalf("judge-error denial must be audited as not-allowed: %+v", captured)
	}
	if !strings.Contains(captured[0].Verdict, "judge error") {
		t.Fatalf("audit should note the judge error, got verdict %q", captured[0].Verdict)
	}
}

// allowAllJudge lets us assert a custom Judge is honored.
type allowAllJudge struct{}

func (allowAllJudge) Name() string { return "allow-all" }
func (allowAllJudge) Evaluate(context.Context, JudgeRequest) (JudgeVerdict, error) {
	return JudgeVerdict{Allow: true, Explanation: "test stub"}, nil
}

func TestRequire_HonorsCustomJudge(t *testing.T) {
	g := Guard{
		Tool: "t", Flag: "--force", Gate: "g", Risk: RiskP0, Judge: allowAllJudge{},
		auditSink: func(auditEntry) {},
	}
	// "x" would be rejected by DefaultJudge but the custom judge allows it.
	if err := Require(context.Background(), g, "x"); err != nil {
		t.Fatalf("custom judge should have allowed, got %v", err)
	}
}

func TestRequireAuditedFailsClosedWhenAuditCannotBePersisted(t *testing.T) {
	wantErr := errors.New("audit storage unavailable")
	var captured auditEntry
	g := Guard{
		Tool: "agm send-compact", Flag: "--force", Gate: "anti-loop guard", Risk: RiskP0,
		Judge: allowAllJudge{},
		durableAuditSink: func(e auditEntry) error {
			captured = e
			return wantErr
		},
	}

	err := RequireAudited(context.Background(), g, "host recovery requires one governed compaction")
	if !errors.Is(err, wantErr) {
		t.Fatalf("RequireAudited() error = %v, want audit failure", err)
	}
	if !captured.Allowed || captured.Judge != "allow-all" {
		t.Fatalf("attempted durable audit = %+v, want allowed judge verdict", captured)
	}
}

func TestRequireAuditedPersistsOwnerOnlyRecord(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "audit")
	t.Setenv("OVERRIDE_AUDIT_DIR", dir)
	g := Guard{
		Tool: "agm send-compact", Flag: "--force", Gate: "anti-loop guard", Risk: RiskP0,
		Judge: allowAllJudge{},
	}
	if err := RequireAudited(context.Background(), g, "host recovery requires one governed compaction"); err != nil {
		t.Fatalf("RequireAudited() error = %v", err)
	}

	dirInfo, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := dirInfo.Mode().Perm(); got != 0o700 {
		t.Fatalf("audit directory mode = %o, want 700", got)
	}
	path := filepath.Join(dir, "override-audit.jsonl")
	fileInfo, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := fileInfo.Mode().Perm(); got != 0o600 {
		t.Fatalf("audit file mode = %o, want 600", got)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var entry auditEntry
	if err := json.Unmarshal(bytes.TrimSpace(data), &entry); err != nil {
		t.Fatalf("decode durable audit: %v", err)
	}
	if !entry.Allowed || entry.Reason != "host recovery requires one governed compaction" || entry.Timestamp == "" {
		t.Fatalf("durable audit entry = %+v", entry)
	}
}

func TestRequireAuditedTightensExistingAuditPermissions(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "audit")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "override-audit.jsonl")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("OVERRIDE_AUDIT_DIR", dir)

	g := Guard{
		Tool: "agm send-compact", Flag: "--force", Gate: "anti-loop guard", Risk: RiskP0,
		Judge: allowAllJudge{},
	}
	if err := RequireAudited(context.Background(), g, "host recovery requires one governed compaction"); err != nil {
		t.Fatalf("RequireAudited() error = %v", err)
	}

	dirInfo, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := dirInfo.Mode().Perm(); got != 0o700 {
		t.Fatalf("audit directory mode = %o, want 700", got)
	}
	fileInfo, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := fileInfo.Mode().Perm(); got != 0o600 {
		t.Fatalf("audit file mode = %o, want 600", got)
	}
}

func TestAppendAudit_WritesJSONL(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("OVERRIDE_AUDIT_DIR", dir)

	g := Guard{Tool: "agm kill", Flag: "--force", Gate: "confirmation prompt", Risk: RiskP2}
	_ = Require(context.Background(), g, "automated reaper has no TTY to confirm at")

	path := filepath.Join(dir, "override-audit.jsonl")
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("audit file not written: %v", err)
	}
	defer func() { _ = f.Close() }()

	sc := bufio.NewScanner(f)
	if !sc.Scan() {
		t.Fatal("audit file empty")
	}
	var e auditEntry
	if err := json.Unmarshal(sc.Bytes(), &e); err != nil {
		t.Fatalf("audit line is not valid JSON: %v", err)
	}
	if e.Tool != "agm kill" || e.Flag != "--force" || !e.Allowed {
		t.Fatalf("unexpected audit entry: %+v", e)
	}
	if e.Timestamp == "" || e.PID == 0 || e.Caller == "" {
		t.Fatalf("audit entry missing provenance fields: %+v", e)
	}
}

func TestRisk_NeedsIndependentJudge(t *testing.T) {
	for r, want := range map[Risk]bool{
		RiskP0: true, RiskP1: true, RiskP2: false, RiskP3: false,
	} {
		if got := r.NeedsIndependentJudge(); got != want {
			t.Fatalf("risk %s: NeedsIndependentJudge=%v want %v", r, got, want)
		}
	}
}
