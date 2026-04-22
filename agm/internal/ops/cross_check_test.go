package ops

import (
	"testing"
	"time"
)

// --- State detection tests ---

func TestDetectSessionState_Healthy(t *testing.T) {
	output := "Some normal output\nClaude is working...\n❯ "
	state := DetectSessionState(output, false, time.Now(), nil)
	if state != StateHealthy {
		t.Errorf("expected HEALTHY, got %s", state)
	}
}

func TestDetectSessionState_PermissionPromptNotExpired(t *testing.T) {
	output := "Allow this action?\n  1. Allow once\n  2. Deny\n"
	// State updated just now — not yet stuck
	state := DetectSessionState(output, false, time.Now(), nil)
	if state != StateHealthy {
		t.Errorf("expected HEALTHY (prompt not yet timed out), got %s", state)
	}
}

func TestDetectSessionState_Stuck(t *testing.T) {
	output := "Allow this action?\n  1. Allow once\n  2. Deny\n"
	// State updated 10 minutes ago — should be stuck
	stateUpdated := time.Now().Add(-10 * time.Minute)
	state := DetectSessionState(output, false, stateUpdated, nil)
	if state != StateStuck {
		t.Errorf("expected STUCK, got %s", state)
	}
}

func TestDetectSessionState_StuckCustomTimeout(t *testing.T) {
	output := "Do you want to allow this tool?\n"
	cfg := &CrossCheckConfig{
		StuckTimeout: 2 * time.Minute,
	}
	// 3 minutes ago with 2-minute timeout
	stateUpdated := time.Now().Add(-3 * time.Minute)
	state := DetectSessionState(output, false, stateUpdated, cfg)
	if state != StateStuck {
		t.Errorf("expected STUCK with custom timeout, got %s", state)
	}
}

func TestDetectSessionState_EnterBug(t *testing.T) {
	output := "Working on task...\n[From: orchestrator | ID: 123 | Sent: 2026-04-13]\n"
	state := DetectSessionState(output, false, time.Now(), nil)
	if state != StateEnterBug {
		t.Errorf("expected ENTER_BUG, got %s", state)
	}
}

func TestDetectSessionState_NotLoopingScanSession(t *testing.T) {
	output := "Some random output\nno scan markers here\n"
	state := DetectSessionState(output, true, time.Now(), nil)
	if state != StateNotLooping {
		t.Errorf("expected NOT_LOOPING for scan-loop session, got %s", state)
	}
}

func TestDetectSessionState_NotLoopingNonScanSession(t *testing.T) {
	// Non-scan-loop sessions should NOT trigger NOT_LOOPING
	output := "Some random output\nno scan markers here\n"
	state := DetectSessionState(output, false, time.Now(), nil)
	if state != StateHealthy {
		t.Errorf("expected HEALTHY for non-scan-loop session, got %s", state)
	}
}

func TestDetectSessionState_ScanSessionWithScanOutput(t *testing.T) {
	output := "=== AGM Orchestrator Scan — 2026-04-13 15:00:00 ===\nSessions: 5\nnext scan in 5m...\n"
	state := DetectSessionState(output, true, time.Now(), nil)
	if state != StateHealthy {
		t.Errorf("expected HEALTHY for scan session with scan output, got %s", state)
	}
}

func TestDetectSessionState_PermissionTakesPriorityOverEnterBug(t *testing.T) {
	// If both permission prompt and enter bug are present, permission takes priority
	output := "Allow this action?\n[From: orchestrator | ID: 123]\n"
	stateUpdated := time.Now().Add(-10 * time.Minute)
	state := DetectSessionState(output, false, stateUpdated, nil)
	if state != StateStuck {
		t.Errorf("expected STUCK (permission takes priority), got %s", state)
	}
}

// --- Orchestrator exclusion tests ---

func TestDetectSessionState_OrchestratorNotFlaggedAsNotLooping(t *testing.T) {
	// Orchestrator sessions work interactively, NOT via scan loops.
	// They should never get NOT_LOOPING even though they are "supervisory".
	output := "Working on delegating tasks...\nCronCreate scheduled for 5m...\n"
	// isScanLoopSession=false for orchestrators
	state := DetectSessionState(output, false, time.Now(), nil)
	if state != StateHealthy {
		t.Errorf("expected HEALTHY for orchestrator session (not a scan loop), got %s", state)
	}
}

func TestCheckSingleSession_OrchestratorExcludedFromNotLooping(t *testing.T) {
	// Verify that orchestrator-named sessions compute isScanLoopSession=false.
	// We test the name-based logic directly since checkSingleSession is not exported
	// and requires tmux. This tests the classification logic.
	tests := []struct {
		name             string
		sessionName      string
		wantScanLoop     bool
	}{
		{"supervisor session", "supervisor-main", true},
		{"scan session", "scan-loop-1", true},
		{"orchestrator session", "orchestrator-main", false},
		{"overseer session", "overseer-v2", false},
		{"meta- session", "meta-planner", false},
		{"worker session", "worker-1", false},
		{"regular session", "fix-bug-123", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isOrchestratorRole := IsSupervisorSession(tt.sessionName)
			isScanLoop := !isOrchestratorRole &&
				(containsCI(tt.sessionName, "supervisor") ||
					containsCI(tt.sessionName, "scan"))
			if isScanLoop != tt.wantScanLoop {
				t.Errorf("session %q: isScanLoopSession=%v, want %v",
					tt.sessionName, isScanLoop, tt.wantScanLoop)
			}
		})
	}
}

// containsCI is a case-insensitive contains helper for tests.
func containsCI(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr ||
		len(s) > 0 && len(substr) > 0 &&
			containsLower(s, substr))
}

func containsLower(s, substr string) bool {
	// Simple: just use strings.Contains since session names are lowercase
	return len(s) >= len(substr) &&
		(s == substr || findSubstr(s, substr))
}

func findSubstr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// --- ENTER bug detection tests ---

func TestHasEnterBug_WithFromHeader(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   bool
	}{
		{
			name:   "standard From header",
			output: "Working...\n[From: orchestrator | ID: 123 | Sent: 2026-04-13]\n",
			want:   true,
		},
		{
			name:   "From header with leading spaces",
			output: "Working...\n   [From: worker-1 | ID: 456]\n",
			want:   true,
		},
		{
			name:   "From header with leading tab",
			output: "Working...\n\t[From: worker-1 | ID: 456]\n",
			want:   true,
		},
		{
			name:   "From header with trailing empty lines",
			output: "Working...\n[From: orchestrator | ID: 123]\n\n\n",
			want:   true,
		},
		{
			name:   "no From header",
			output: "Working...\nSome normal output\n",
			want:   false,
		},
		{
			name:   "human text in input",
			output: "Working...\nPlease fix the bug\n",
			want:   false,
		},
		{
			name:   "empty output",
			output: "",
			want:   false,
		},
		{
			name:   "only whitespace",
			output: "   \n\n  \n",
			want:   false,
		},
		{
			name:   "From in middle but not last line",
			output: "[From: old-msg | ID: 1]\nSome other output\n",
			want:   false,
		},
		{
			name:   "partial From header",
			output: "Working...\n[FromSomethingElse: data]\n",
			want:   false,
		},
		{
			name:   "From without bracket",
			output: "Working...\nFrom: sender\n",
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := HasEnterBug(tt.output)
			if got != tt.want {
				t.Errorf("HasEnterBug() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExtractInputLine(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   string
	}{
		{
			name:   "last line is content",
			output: "line1\nline2\nlast line",
			want:   "last line",
		},
		{
			name:   "trailing empty lines",
			output: "line1\nactual last\n\n\n",
			want:   "actual last",
		},
		{
			name:   "all empty",
			output: "\n\n\n",
			want:   "",
		},
		{
			name:   "empty string",
			output: "",
			want:   "",
		},
		{
			name:   "preserves leading whitespace",
			output: "line1\n  indented last\n",
			want:   "  indented last",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractInputLine(tt.output)
			if got != tt.want {
				t.Errorf("ExtractInputLine() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestIsFromHeader(t *testing.T) {
	tests := []struct {
		line string
		want bool
	}{
		{"[From: orchestrator | ID: 123]", true},
		{"  [From: worker-1]", true},
		{"\t[From: test]", true},
		{"[From:", true},
		{"From: not-a-header", false},
		{"some [From: embedded]", false},
		{"", false},
		{"[To: someone]", false},
		{"[FromSomething: data]", false},
	}

	for _, tt := range tests {
		t.Run(tt.line, func(t *testing.T) {
			got := IsFromHeader(tt.line)
			if got != tt.want {
				t.Errorf("IsFromHeader(%q) = %v, want %v", tt.line, got, tt.want)
			}
		})
	}
}

// --- Stuck cross-check nudge tests ---

func TestHasStuckCrossCheckNudge(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   bool
	}{
		{
			name:   "nudge text stuck in input",
			output: "Working...\n[cross-check] Supervisor appears stalled -- no scan output detected.\n",
			want:   true,
		},
		{
			name:   "From wrapper with cross-check sender",
			output: "Working...\n[From: cross-check | ID: abc | Sent: 2026-04-14]\n",
			want:   true,
		},
		{
			name:   "normal From header not cross-check",
			output: "Working...\n[From: orchestrator | ID: 123]\n",
			want:   false,
		},
		{
			name:   "no stuck message",
			output: "Working normally...\nScan complete.\n",
			want:   false,
		},
		{
			name:   "empty output",
			output: "",
			want:   false,
		},
		{
			name:   "cross-check in middle but not input line",
			output: "[cross-check] old nudge\nSome later output\n",
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := HasStuckCrossCheckNudge(tt.output)
			if got != tt.want {
				t.Errorf("HasStuckCrossCheckNudge() = %v, want %v", got, tt.want)
			}
		})
	}
}

// --- Permission prompt detection tests ---

func TestContainsPermissionPrompt(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   bool
	}{
		{"allow action", "Allow this action?\n1. Yes\n2. No", true},
		{"allow tool", "Allow tool Read to read file?", true},
		{"yes prompt", "(Y)es to continue", true},
		{"no prompt", "Working on task...\nProgress: 50%", false},
		{"empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := containsPermissionPrompt(tt.output)
			if got != tt.want {
				t.Errorf("containsPermissionPrompt() = %v, want %v", got, tt.want)
			}
		})
	}
}

// --- RBAC allowlist tests ---

func TestMatchesRBACAllowlist(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   bool
	}{
		{
			name:   "Read tool allowed",
			output: "Allow this action?\nTool: Read /path/to/file\n1. Yes\n2. No",
			want:   true,
		},
		{
			name:   "Bash git allowed",
			output: "Allow this action?\nTool: Bash(git status)\n1. Yes",
			want:   true,
		},
		{
			name:   "unknown tool not allowed",
			output: "Allow this action?\nTool: DeleteEverything\n1. Yes",
			want:   false,
		},
		{
			name:   "no tool line",
			output: "Allow this action?\n1. Yes\n2. No",
			want:   false,
		},
		{
			name:   "Grep allowed",
			output: "Tool: Grep pattern in files\nAllow?",
			want:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MatchesRBACAllowlist(tt.output, nil)
			if got != tt.want {
				t.Errorf("MatchesRBACAllowlist() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMatchesRBACAllowlist_CustomList(t *testing.T) {
	output := "Tool: CustomTool arg1\nAllow?"
	customList := []string{"CustomTool"}
	got := MatchesRBACAllowlist(output, customList)
	if !got {
		t.Error("expected custom allowlist to match CustomTool")
	}
}

// --- Unmanaged sessions tests ---

func TestCheckUnmanagedSessions(t *testing.T) {
	tests := []struct {
		name     string
		tmux     []string
		managed  []string
		expected []string
	}{
		{
			name:     "no unmanaged",
			tmux:     []string{"worker-1", "worker-2"},
			managed:  []string{"worker-1", "worker-2"},
			expected: nil,
		},
		{
			name:     "one unmanaged",
			tmux:     []string{"worker-1", "rogue-session"},
			managed:  []string{"worker-1"},
			expected: []string{"rogue-session"},
		},
		{
			name:     "well-known excluded",
			tmux:     []string{"worker-1", "main", "default"},
			managed:  []string{"worker-1"},
			expected: nil,
		},
		{
			name:     "multiple unmanaged",
			tmux:     []string{"worker-1", "rogue-1", "rogue-2", "main"},
			managed:  []string{"worker-1"},
			expected: []string{"rogue-1", "rogue-2"},
		},
		{
			name:     "empty tmux",
			tmux:     []string{},
			managed:  []string{"worker-1"},
			expected: nil,
		},
		{
			name:     "empty managed",
			tmux:     []string{"session-1"},
			managed:  []string{},
			expected: []string{"session-1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CheckUnmanagedSessions(tt.tmux, tt.managed)
			if len(got) != len(tt.expected) {
				t.Errorf("CheckUnmanagedSessions() = %v, want %v", got, tt.expected)
				return
			}
			for i := range got {
				if got[i] != tt.expected[i] {
					t.Errorf("CheckUnmanagedSessions()[%d] = %q, want %q", i, got[i], tt.expected[i])
				}
			}
		})
	}
}

// --- CrossCheckState string tests ---

func TestCrossCheckState_String(t *testing.T) {
	tests := []struct {
		state CrossCheckState
		want  string
	}{
		{StateHealthy, "HEALTHY"},
		{StateDown, "DOWN"},
		{StateStuck, "STUCK"},
		{StateNotLooping, "NOT_LOOPING"},
		{StateEnterBug, "ENTER_BUG"},
		{CrossCheckState(99), "UNKNOWN"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := tt.state.String()
			if got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

// --- isNotLooping tests ---

func TestIsNotLooping(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   bool
	}{
		{
			name:   "no scan markers",
			output: "Random output\nNo markers here\n",
			want:   true,
		},
		{
			name:   "has scan header",
			output: "=== AGM Orchestrator Scan — 2026-04-13 15:00:00 ===\n",
			want:   false,
		},
		{
			name:   "has next scan message",
			output: "Some output\nnext scan in 5m0s...\n",
			want:   false,
		},
		{
			name:   "has JSON timestamp",
			output: "{\n  \"timestamp\": \"2026-04-13T15:00:00Z\"\n}\n",
			want:   false,
		},
		{
			name:   "empty output",
			output: "",
			want:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isNotLooping(tt.output)
			if got != tt.want {
				t.Errorf("isNotLooping() = %v, want %v", got, tt.want)
			}
		})
	}
}

// --- DefaultCrossCheckConfig tests ---

func TestDefaultCrossCheckConfig(t *testing.T) {
	cfg := DefaultCrossCheckConfig()
	if cfg.StuckTimeout != 5*time.Minute {
		t.Errorf("StuckTimeout = %v, want 5m", cfg.StuckTimeout)
	}
	if cfg.ScanGapTimeout != 10*time.Minute {
		t.Errorf("ScanGapTimeout = %v, want 10m", cfg.ScanGapTimeout)
	}
	if len(cfg.RBACAllowlist) == 0 {
		t.Error("RBACAllowlist should not be empty")
	}
	if cfg.DryRun {
		t.Error("DryRun should default to false")
	}
}

// --- extractToolFromPrompt tests ---

func TestExtractToolFromPrompt(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   string
	}{
		{
			name:   "Tool prefix",
			output: "Allow?\nTool: Read /path/file.go\nOption 1",
			want:   "Read /path/file.go",
		},
		{
			name:   "Command prefix",
			output: "Command: git status\nAllow?",
			want:   "git status",
		},
		{
			name:   "Action prefix",
			output: "Action: Edit file.go\nAllow?",
			want:   "Edit file.go",
		},
		{
			name:   "no recognized prefix",
			output: "Just some text\nNo tool here",
			want:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractToolFromPrompt(tt.output)
			if got != tt.want {
				t.Errorf("extractToolFromPrompt() = %q, want %q", got, tt.want)
			}
		})
	}
}

// --- FilterCrossCheckTargets tests ---

func TestFilterCrossCheckTargets(t *testing.T) {
	allSessions := []SessionSummary{
		{Name: "orchestrator-main"},
		{Name: "overseer-v2"},
		{Name: "meta-planner"},
		{Name: "worker-1"},
		{Name: "impl-fix-bug"},
		{Name: "supervisor-scan"},
		{Name: "scan-loop-1"},
	}

	tests := []struct {
		name          string
		caller        string
		wantNames     []string
		wantExcluded  []string
	}{
		{
			name:         "only supervisors included, workers excluded",
			caller:       "some-other-session",
			wantNames:    []string{"orchestrator-main", "overseer-v2", "meta-planner"},
			wantExcluded: []string{"worker-1", "impl-fix-bug", "supervisor-scan", "scan-loop-1"},
		},
		{
			name:         "self excluded",
			caller:       "orchestrator-main",
			wantNames:    []string{"overseer-v2", "meta-planner"},
			wantExcluded: []string{"orchestrator-main", "worker-1"},
		},
		{
			name:         "case insensitive self exclusion",
			caller:       "Orchestrator-Main",
			wantNames:    []string{"overseer-v2", "meta-planner"},
			wantExcluded: []string{"orchestrator-main"},
		},
		{
			name:         "empty caller does not exclude any supervisor",
			caller:       "",
			wantNames:    []string{"orchestrator-main", "overseer-v2", "meta-planner"},
			wantExcluded: []string{"worker-1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FilterCrossCheckTargets(allSessions, tt.caller)

			gotNames := make(map[string]bool)
			for _, s := range got {
				gotNames[s.Name] = true
			}

			for _, want := range tt.wantNames {
				if !gotNames[want] {
					t.Errorf("expected %q in targets, but it was missing", want)
				}
			}
			for _, excluded := range tt.wantExcluded {
				if gotNames[excluded] {
					t.Errorf("expected %q to be excluded, but it was present", excluded)
				}
			}
		})
	}
}

func TestFilterCrossCheckTargets_EmptyInput(t *testing.T) {
	got := FilterCrossCheckTargets(nil, "orchestrator-main")
	if len(got) != 0 {
		t.Errorf("expected empty result for nil input, got %d", len(got))
	}
}

func TestFilterCrossCheckTargets_NoSupervisors(t *testing.T) {
	sessions := []SessionSummary{
		{Name: "worker-1"},
		{Name: "impl-feature"},
		{Name: "fix-bug-123"},
	}
	got := FilterCrossCheckTargets(sessions, "orchestrator-main")
	if len(got) != 0 {
		t.Errorf("expected no targets when no supervisors present, got %d", len(got))
	}
}
