package safegit

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// newTestConfig returns a BreakGlassConfig with every external seam stubbed out
// so a test never touches the terminal, GitHub, the beads DB, or the real
// audit dir. Callers override individual fields as needed.
func newTestConfig(t *testing.T, stdin string) (*BreakGlassConfig, *bytes.Buffer, *bytes.Buffer, *[]ghCall) {
	t.Helper()
	dir := t.TempDir()
	var out, errb bytes.Buffer
	var calls []ghCall

	cfg := &BreakGlassConfig{
		PRArg:    "42",
		Repo:     "vbonnet/dear-agent",
		In:       strings.NewReader(stdin),
		Out:      &out,
		Err:      &errb,
		IsTTY:    func() bool { return true },
		Now:      func() time.Time { return time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC) },
		Operator: func() string { return "op@example.com" },
		Protected: func(string) (bool, error) {
			return false, nil
		},
		RunGH: func(args ...string) ([]byte, error) {
			calls = append(calls, ghCall{tool: "gh", args: args})
			return []byte("ok"), nil
		},
		RunBD: func(args ...string) ([]byte, error) {
			calls = append(calls, ghCall{tool: "bd", args: args})
			return []byte(`{"id":"ce-test01"}`), nil
		},
		AuditDir: dir,
	}
	return cfg, &out, &errb, &calls
}

type ghCall struct {
	tool string
	args []string
}

func findCall(calls []ghCall, tool string, sub ...string) *ghCall {
	for i := range calls {
		c := calls[i]
		if c.tool != tool {
			continue
		}
		if len(sub) <= len(c.args) {
			match := true
			for j, s := range sub {
				if c.args[j] != s {
					match = false
					break
				}
			}
			if match {
				return &c
			}
		}
	}
	return nil
}

// --- TTY guard ---

func TestBreakGlass_RefusesWithoutTTY(t *testing.T) {
	cfg, _, errb, calls := newTestConfig(t, "this is a perfectly valid reason\n")
	cfg.IsTTY = func() bool { return false }

	err := BreakGlass(*cfg)
	if err == nil {
		t.Fatal("expected error when not a TTY, got nil")
	}
	if !strings.Contains(errb.String(), "interactive TTY") {
		t.Errorf("stderr should explain the TTY requirement, got: %q", errb.String())
	}
	if len(*calls) != 0 {
		t.Errorf("no gh/bd calls should run without a TTY, got: %+v", *calls)
	}
}

// --- reason validation ---

func TestValidateBreakGlassReason(t *testing.T) {
	if err := validateBreakGlassReason("short"); err == nil {
		t.Error("expected error for short reason")
	}
	if err := validateBreakGlassReason(strings.Repeat("x", MinBreakGlassReason-1)); err == nil {
		t.Error("expected error one char under the minimum")
	}
	if err := validateBreakGlassReason(strings.Repeat("x", MinBreakGlassReason)); err != nil {
		t.Errorf("exactly the minimum should pass, got: %v", err)
	}
}

func TestBreakGlass_RejectsShortReason(t *testing.T) {
	cfg, _, errb, calls := newTestConfig(t, "too short\n")

	err := BreakGlass(*cfg)
	if err == nil {
		t.Fatal("expected error for short reason")
	}
	if !strings.Contains(errb.String(), "too short") {
		t.Errorf("stderr should say reason too short, got: %q", errb.String())
	}
	if findCall(*calls, "gh", "pr", "comment") != nil {
		t.Error("must not post a PR comment when the reason is rejected")
	}
}

// --- happy path: unprotected repo merges immediately ---

func TestBreakGlass_MergesWhenUnprotected(t *testing.T) {
	reason := "production hotfix, CI flaky on unrelated job"
	cfg, out, _, calls := newTestConfig(t, reason+"\n")

	if err := BreakGlass(*cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	comment := findCall(*calls, "gh", "pr", "comment")
	if comment == nil {
		t.Fatal("expected a gh pr comment call")
	}
	merge := findCall(*calls, "gh", "pr", "merge")
	if merge == nil {
		t.Fatal("expected a gh pr merge call on an unprotected repo")
	}
	// break-glass is immediate, never --auto.
	for _, a := range merge.args {
		if a == "--auto" {
			t.Error("break-glass merge must NOT use --auto")
		}
	}
	if !containsArg(merge.args, "--squash") {
		t.Error("break-glass merge should be --squash")
	}
	if findCall(*calls, "bd", "--db") == nil {
		t.Error("expected a bd create call to file the retro bead")
	}
	if !strings.Contains(out.String(), "DEAR retro required") {
		t.Errorf("expected the DEAR retro notice, got: %q", out.String())
	}
	if !strings.Contains(out.String(), "Bead filed: ce-test01") {
		t.Errorf("expected the filed bead id, got: %q", out.String())
	}
}

// --- ruleset-protected path prints instructions and does NOT merge ---

func TestBreakGlass_RulesetProtectedPrintsInstruction(t *testing.T) {
	reason := "need to land emergency revert before the next deploy window"
	cfg, out, _, calls := newTestConfig(t, reason+"\n")
	cfg.Protected = func(string) (bool, error) { return true, nil }

	if err := BreakGlass(*cfg); err != nil {
		t.Fatalf("ruleset-protected path should exit 0 (nil error), got: %v", err)
	}

	if findCall(*calls, "gh", "pr", "merge") != nil {
		t.Error("must NOT attempt a merge on a ruleset-protected repo")
	}
	if findCall(*calls, "gh", "pr", "comment") == nil {
		t.Error("should still post the reason comment before printing the instruction")
	}
	o := out.String()
	if !strings.Contains(o, "RULESET PROTECTED") {
		t.Errorf("expected the ruleset-protected banner, got: %q", o)
	}
	if !strings.Contains(o, "settings/branches") {
		t.Errorf("expected the Settings link, got: %q", o)
	}
}

// --- fail-safe: protection check error is treated as protected ---

func TestBreakGlass_ProtectionErrorFailsSafe(t *testing.T) {
	reason := "unable to verify protection but must merge revert urgently"
	cfg, out, _, calls := newTestConfig(t, reason+"\n")
	cfg.Protected = func(string) (bool, error) {
		return false, errChecking
	}

	if err := BreakGlass(*cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if findCall(*calls, "gh", "pr", "merge") != nil {
		t.Error("on protection-check error, must NOT merge (fail safe to manual path)")
	}
	if !strings.Contains(out.String(), "RULESET PROTECTED") {
		t.Errorf("expected the manual instruction on fail-safe, got: %q", out.String())
	}
}

// --- audit record format ---

func TestBreakGlass_AuditRecordFormat(t *testing.T) {
	reason := "rollback bad migration; gates blocked by stale required check"
	cfg, _, _, _ := newTestConfig(t, reason+"\n")

	if err := BreakGlass(*cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(cfg.AuditDir, "break-glass-audit.jsonl"))
	if err != nil {
		t.Fatalf("reading audit log: %v", err)
	}
	var entry BreakGlassAuditEntry
	if err := json.Unmarshal(bytes.TrimSpace(data), &entry); err != nil {
		t.Fatalf("audit line is not valid JSON: %v\n%s", err, data)
	}
	if entry.PR != "42" {
		t.Errorf("audit PR = %q, want 42", entry.PR)
	}
	if entry.Repo != "vbonnet/dear-agent" {
		t.Errorf("audit Repo = %q", entry.Repo)
	}
	if entry.Reason != reason {
		t.Errorf("audit Reason = %q, want %q", entry.Reason, reason)
	}
	if entry.Operator != "op@example.com" {
		t.Errorf("audit Operator = %q", entry.Operator)
	}
	if entry.Outcome != "merged" {
		t.Errorf("audit Outcome = %q, want merged", entry.Outcome)
	}
	if entry.Bead != "ce-test01" {
		t.Errorf("audit Bead = %q, want ce-test01", entry.Bead)
	}
	if _, err := time.Parse(time.RFC3339, entry.Timestamp); err != nil {
		t.Errorf("audit Timestamp %q is not RFC3339: %v", entry.Timestamp, err)
	}
}

// --- audit log is append-only (never truncated) ---

func TestBreakGlass_AuditIsAppendOnly(t *testing.T) {
	reason := "first break-glass use with a sufficiently long reason"
	cfg, _, _, _ := newTestConfig(t, reason+"\n")

	if err := BreakGlass(*cfg); err != nil {
		t.Fatalf("first run error: %v", err)
	}
	// Second run reusing the same audit dir but a fresh stdin reader.
	cfg.In = strings.NewReader("second break-glass use, also long enough to pass\n")
	if err := BreakGlass(*cfg); err != nil {
		t.Fatalf("second run error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(cfg.AuditDir, "break-glass-audit.jsonl"))
	if err != nil {
		t.Fatalf("reading audit log: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 append-only audit lines, got %d:\n%s", len(lines), data)
	}
}

// --- comment body format ---

func TestBuildBreakGlassComment(t *testing.T) {
	ts := time.Date(2026, 6, 20, 12, 30, 0, 0, time.UTC)
	got := buildBreakGlassComment("rollback now", "op@example.com", ts)
	want := "BREAK-GLASS: rollback now\nOperator: op@example.com\nTimestamp: 2026-06-20T12:30:00Z"
	if got != want {
		t.Errorf("comment body =\n%q\nwant\n%q", got, want)
	}
}

func TestBreakGlass_CommentIncludesReasonOperatorTimestamp(t *testing.T) {
	reason := "emergency: revert breaking change before release cut"
	cfg, _, _, calls := newTestConfig(t, reason+"\n")

	if err := BreakGlass(*cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	comment := findCall(*calls, "gh", "pr", "comment")
	if comment == nil {
		t.Fatal("expected a gh pr comment call")
	}
	body := bodyArg(comment.args)
	if !strings.Contains(body, "BREAK-GLASS: "+reason) {
		t.Errorf("comment body missing reason: %q", body)
	}
	if !strings.Contains(body, "Operator: op@example.com") {
		t.Errorf("comment body missing operator: %q", body)
	}
	if !strings.Contains(body, "Timestamp: 2026-06-20T12:00:00Z") {
		t.Errorf("comment body missing timestamp: %q", body)
	}
}

// --- ruleset protection parsing ---

func TestParseRulesetProtected(t *testing.T) {
	cases := []struct {
		name string
		json string
		want bool
	}{
		{"enforced", `{"enforce_admins":{"enabled":true}}`, true},
		{"not enforced", `{"enforce_admins":{"enabled":false}}`, false},
		{"absent", `{"required_status_checks":{}}`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseRulesetProtected([]byte(tc.json))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("parseRulesetProtected(%s) = %v, want %v", tc.json, got, tc.want)
			}
		})
	}
}

// --- bead id parsing ---

func TestParseBeadID(t *testing.T) {
	cases := []struct {
		name string
		out  string
		want string
	}{
		{"top-level id", `{"id":"ce-abc12","title":"x"}`, "ce-abc12"},
		{"wrapped issue", `{"issue":{"id":"ce-def34"}}`, "ce-def34"},
		{"no id", `{"title":"x"}`, ""},
		{"garbage", `not json`, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := parseBeadID([]byte(tc.out)); got != tc.want {
				t.Errorf("parseBeadID(%q) = %q, want %q", tc.out, got, tc.want)
			}
		})
	}
}

// --- retro bead title is truncated ---

func TestBreakGlass_BeadTitleTruncatesReason(t *testing.T) {
	reason := strings.Repeat("A", 120)
	cfg, _, _, calls := newTestConfig(t, reason+"\n")

	if err := BreakGlass(*cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	bd := findCall(*calls, "bd", "--db")
	if bd == nil {
		t.Fatal("expected a bd create call")
	}
	title := titleArg(bd.args)
	if strings.Contains(title, strings.Repeat("A", 51)) {
		t.Errorf("bead title should truncate reason to 50 chars, got: %q", title)
	}
	if !strings.Contains(title, "DEAR retro: break-glass used on 42") {
		t.Errorf("unexpected bead title: %q", title)
	}
}

// --- helpers / shared test fixtures ---

var errChecking = &testError{"cannot reach github"}

type testError struct{ msg string }

func (e *testError) Error() string { return e.msg }

func containsArg(args []string, want string) bool {
	return slices.Contains(args, want)
}

// bodyArg returns the value following "--body" in an argv slice.
func bodyArg(args []string) string {
	for i, a := range args {
		if a == "--body" && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

// titleArg returns the bd create positional title (the arg right after "create").
func titleArg(args []string) string {
	for i, a := range args {
		if a == "create" && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

// Sanity: the prompt is actually read from the injected reader.
func TestBreakGlass_ReadsReasonFromStdin(t *testing.T) {
	reason := "verify the reader seam is wired to stdin correctly"
	r := bufio.NewReader(strings.NewReader(reason + "\n"))
	cfg, _, _, calls := newTestConfig(t, "")
	cfg.In = r

	if err := BreakGlass(*cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	comment := findCall(*calls, "gh", "pr", "comment")
	if comment == nil || !strings.Contains(bodyArg(comment.args), reason) {
		t.Errorf("reason from stdin not used in comment")
	}
}
