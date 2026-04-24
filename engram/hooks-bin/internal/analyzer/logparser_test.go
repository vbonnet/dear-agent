package analyzer

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeTestLog(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "test.log")
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return p
}

const sampleLogDenialAndApproval = `[2026-03-09 05:22:31] === Hook invoked ===
[2026-03-09 05:22:31] Raw input: {"session_id":"sess-001","transcript_path":"/tmp/t1.jsonl","cwd":"/home/user","tool_input":{"command":"cd /tmp"},"tool_use_id":"tu-001"}

[2026-03-09 05:22:31] Parsed command: cd /tmp
[2026-03-09 05:22:31] VALIDATOR: Checking 29 patterns against command
[2026-03-09 05:22:31] VALIDATOR: Pattern #1 MATCHED: cd command (pattern: \bcd\s+)
[2026-03-09 05:22:31] Validation result: ok=false, pattern="cd command", remediation="Use absolute paths instead"
[2026-03-09 05:22:31] DENIED: cd command - Use absolute paths instead

[2026-03-09 05:23:00] === Hook invoked ===
[2026-03-09 05:23:00] Raw input: {"session_id":"sess-001","transcript_path":"/tmp/t1.jsonl","cwd":"/home/user","tool_input":{"command":"ls -la"},"tool_use_id":"tu-002"}

[2026-03-09 05:23:00] Parsed command: ls -la
[2026-03-09 05:23:00] VALIDATOR: Checking 29 patterns against command
[2026-03-09 05:23:00] VALIDATOR: No patterns matched - command allowed
[2026-03-09 05:23:00] Validation result: ok=true, pattern="", remediation=""
[2026-03-09 05:23:00] APPROVED
`

func TestParseLog_DenialAndApproval(t *testing.T) {
	path := writeTestLog(t, sampleLogDenialAndApproval)

	denials, approvals, stats, err := ParseLog(path, nil)
	if err != nil {
		t.Fatalf("ParseLog error: %v", err)
	}

	if len(denials) != 1 {
		t.Fatalf("expected 1 denial, got %d", len(denials))
	}
	d := denials[0]
	if d.SessionID != "sess-001" {
		t.Errorf("denial SessionID = %q, want %q", d.SessionID, "sess-001")
	}
	if d.Command != "cd /tmp" {
		t.Errorf("denial Command = %q, want %q", d.Command, "cd /tmp")
	}
	if d.PatternName != "cd command" {
		t.Errorf("denial PatternName = %q, want %q", d.PatternName, "cd command")
	}
	if d.PatternIndex != 1 {
		t.Errorf("denial PatternIndex = %d, want 1", d.PatternIndex)
	}
	if d.PatternRegex != `\bcd\s+` {
		t.Errorf("denial PatternRegex = %q, want %q", d.PatternRegex, `\bcd\s+`)
	}
	if d.Remediation != "Use absolute paths instead" {
		t.Errorf("denial Remediation = %q, want %q", d.Remediation, "Use absolute paths instead")
	}
	if d.ToolUseID != "tu-001" {
		t.Errorf("denial ToolUseID = %q, want %q", d.ToolUseID, "tu-001")
	}
	if d.CWD != "/home/user" {
		t.Errorf("denial CWD = %q, want %q", d.CWD, "/home/user")
	}

	if len(approvals) != 1 {
		t.Fatalf("expected 1 approval, got %d", len(approvals))
	}
	a := approvals[0]
	if a.Command != "ls -la" {
		t.Errorf("approval Command = %q, want %q", a.Command, "ls -la")
	}
	if a.SessionID != "sess-001" {
		t.Errorf("approval SessionID = %q, want %q", a.SessionID, "sess-001")
	}
	if a.ToolUseID != "tu-002" {
		t.Errorf("approval ToolUseID = %q, want %q", a.ToolUseID, "tu-002")
	}

	// Stats checks.
	if stats.TotalInvocations != 2 {
		t.Errorf("TotalInvocations = %d, want 2", stats.TotalInvocations)
	}
	if stats.TotalDenials != 1 {
		t.Errorf("TotalDenials = %d, want 1", stats.TotalDenials)
	}
	if stats.TotalApprovals != 1 {
		t.Errorf("TotalApprovals = %d, want 1", stats.TotalApprovals)
	}
	if stats.UniqueSessionIDs != 1 {
		t.Errorf("UniqueSessionIDs = %d, want 1", stats.UniqueSessionIDs)
	}
	if cnt, ok := stats.DenialsByPattern["cd command"]; !ok || cnt != 1 {
		t.Errorf("DenialsByPattern[cd command] = %d, want 1", cnt)
	}

	// TimeRange.
	expectedStart, _ := time.Parse("2006-01-02 15:04:05", "2026-03-09 05:22:31")
	expectedEnd, _ := time.Parse("2006-01-02 15:04:05", "2026-03-09 05:23:00")
	if !stats.TimeRange[0].Equal(expectedStart) {
		t.Errorf("TimeRange[0] = %v, want %v", stats.TimeRange[0], expectedStart)
	}
	if !stats.TimeRange[1].Equal(expectedEnd) {
		t.Errorf("TimeRange[1] = %v, want %v", stats.TimeRange[1], expectedEnd)
	}
}

const sampleLogLegacy = `[2026-03-09 06:00:00] === Hook invoked ===
[2026-03-09 06:00:00] Raw input: {"parameters":{"command":"rm -rf /"}}

[2026-03-09 06:00:00] Parsed command: rm -rf /
[2026-03-09 06:00:00] VALIDATOR: Checking 29 patterns against command
[2026-03-09 06:00:00] VALIDATOR: Pattern #5 MATCHED: destructive rm (pattern: rm\s+-rf)
[2026-03-09 06:00:00] Validation result: ok=false, pattern="destructive rm", remediation="Don't do that"
[2026-03-09 06:00:00] DENIED: destructive rm - Don't do that
`

func TestParseLog_LegacyFormat(t *testing.T) {
	path := writeTestLog(t, sampleLogLegacy)

	denials, _, _, err := ParseLog(path, nil)
	if err != nil {
		t.Fatalf("ParseLog error: %v", err)
	}

	if len(denials) != 1 {
		t.Fatalf("expected 1 denial, got %d", len(denials))
	}
	d := denials[0]
	if d.Command != "rm -rf /" {
		t.Errorf("denial Command = %q, want %q", d.Command, "rm -rf /")
	}
	if d.SessionID != "" {
		t.Errorf("legacy entry should have empty SessionID, got %q", d.SessionID)
	}
	if d.PatternIndex != 5 {
		t.Errorf("denial PatternIndex = %d, want 5", d.PatternIndex)
	}
}

func TestParseLog_SinceFilter(t *testing.T) {
	path := writeTestLog(t, sampleLogDenialAndApproval)

	// Filter: only entries at or after 05:22:45 (between denial and approval).
	since, _ := time.Parse("2006-01-02 15:04:05", "2026-03-09 05:22:45")
	denials, approvals, stats, err := ParseLog(path, &since)
	if err != nil {
		t.Fatalf("ParseLog error: %v", err)
	}

	if len(denials) != 0 {
		t.Errorf("expected 0 denials after filter, got %d", len(denials))
	}
	if len(approvals) != 1 {
		t.Errorf("expected 1 approval after filter, got %d", len(approvals))
	}
	// TotalInvocations counts all entries seen, but filtered ones don't appear in results.
	if stats.TotalDenials != 0 {
		t.Errorf("TotalDenials = %d, want 0", stats.TotalDenials)
	}
	if stats.TotalApprovals != 1 {
		t.Errorf("TotalApprovals = %d, want 1", stats.TotalApprovals)
	}
}

func TestParseLog_EmptyFile(t *testing.T) {
	path := writeTestLog(t, "")

	denials, approvals, stats, err := ParseLog(path, nil)
	if err != nil {
		t.Fatalf("ParseLog error: %v", err)
	}
	if len(denials) != 0 {
		t.Errorf("expected 0 denials, got %d", len(denials))
	}
	if len(approvals) != 0 {
		t.Errorf("expected 0 approvals, got %d", len(approvals))
	}
	if stats.TotalInvocations != 0 {
		t.Errorf("TotalInvocations = %d, want 0", stats.TotalInvocations)
	}
}

func TestParseLog_MalformedEntries(t *testing.T) {
	// Entry with missing Raw input and truncated content.
	const malformed = `[2026-03-09 07:00:00] === Hook invoked ===
[2026-03-09 07:00:00] Parsed command: echo hello
[2026-03-09 07:00:00] APPROVED

[2026-03-09 07:01:00] === Hook invoked ===
[2026-03-09 07:01:00] Raw input: {not valid json}
[2026-03-09 07:01:00] DENIED: unknown - something

[2026-03-09 07:02:00] === Hook invoked ===
`
	path := writeTestLog(t, malformed)

	denials, approvals, stats, err := ParseLog(path, nil)
	if err != nil {
		t.Fatalf("ParseLog error: %v", err)
	}

	// First entry: approved without raw input — should still work.
	if len(approvals) != 1 {
		t.Errorf("expected 1 approval, got %d", len(approvals))
	}
	if len(approvals) > 0 && approvals[0].Command != "echo hello" {
		t.Errorf("approval Command = %q, want %q", approvals[0].Command, "echo hello")
	}

	// Second entry: denied with invalid JSON — denial still recorded.
	if len(denials) != 1 {
		t.Errorf("expected 1 denial, got %d", len(denials))
	}

	// Third entry: just the invocation header, no resolution — ignored.
	if stats.TotalInvocations != 3 {
		t.Errorf("TotalInvocations = %d, want 3", stats.TotalInvocations)
	}
}

func TestParseLog_FileNotFound(t *testing.T) {
	_, _, _, err := ParseLog("/nonexistent/path/log.log", nil)
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestParseLog_MultipleSessionIDs(t *testing.T) {
	const log = `[2026-03-09 08:00:00] === Hook invoked ===
[2026-03-09 08:00:00] Raw input: {"session_id":"sess-A","transcript_path":"/tmp/a.jsonl","cwd":"/","tool_input":{"command":"ls"},"tool_use_id":"tu-1"}
[2026-03-09 08:00:00] APPROVED

[2026-03-09 08:01:00] === Hook invoked ===
[2026-03-09 08:01:00] Raw input: {"session_id":"sess-B","transcript_path":"/tmp/b.jsonl","cwd":"/","tool_input":{"command":"pwd"},"tool_use_id":"tu-2"}
[2026-03-09 08:01:00] APPROVED

[2026-03-09 08:02:00] === Hook invoked ===
[2026-03-09 08:02:00] Raw input: {"session_id":"sess-A","transcript_path":"/tmp/a.jsonl","cwd":"/","tool_input":{"command":"echo hi"},"tool_use_id":"tu-3"}
[2026-03-09 08:02:00] APPROVED
`
	path := writeTestLog(t, log)
	_, _, stats, err := ParseLog(path, nil)
	if err != nil {
		t.Fatalf("ParseLog error: %v", err)
	}
	if stats.UniqueSessionIDs != 2 {
		t.Errorf("UniqueSessionIDs = %d, want 2", stats.UniqueSessionIDs)
	}
}

func TestParseTimeDelta(t *testing.T) {
	tests := []struct {
		input string
		want  time.Duration
		err   bool
	}{
		{"7d", 7 * 24 * time.Hour, false},
		{"24h", 24 * time.Hour, false},
		{"1w", 7 * 24 * time.Hour, false},
		{"30m", 30 * time.Minute, false},
		{"60s", 60 * time.Second, false},
		{"0h", 0, false},
		// Invalid inputs.
		{"", 0, true},
		{"d", 0, true},
		{"7x", 0, true},
		{"abc", 0, true},
		{"7", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseTimeDelta(tt.input)
			if tt.err {
				if err == nil {
					t.Errorf("ParseTimeDelta(%q) expected error, got %v", tt.input, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseTimeDelta(%q) unexpected error: %v", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("ParseTimeDelta(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}
