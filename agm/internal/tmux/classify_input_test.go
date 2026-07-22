package tmux

import (
	"testing"
)

func TestClassifyQueuedInput_AGMMessage(t *testing.T) {
	tests := []struct {
		name           string
		input          string
		expectedType   QueuedInputType
		expectedSender string
	}{
		{
			name:           "stuck AGM message with orchestrator sender",
			input:          "[Pasted text #1 +5 lines]\n[From: orchestrator | ID: 1774863250311-orchestr-10093 | Sent: 2026-03-30T09:34:10Z]\n[Priority: urgent] Do something",
			expectedType:   QueuedInputAGM,
			expectedSender: "orchestrator",
		},
		{
			name:           "stuck AGM message with worker sender",
			input:          "some output\n[Pasted text #2 +3 lines]\n[From: worker-42 | ID: 1774871219202-worker-4-10110 | Sent: 2026-03-30T11:46:59Z]\nPlease review",
			expectedType:   QueuedInputAGM,
			expectedSender: "worker-42",
		},
		{
			name:           "stuck AGM message with astrocyte sender",
			input:          "[Pasted text #1 +2 lines]\n[From: astrocyte | ID: 1774872000000-astrocyt-001 | Sent: 2026-03-30T12:00:00Z]",
			expectedType:   QueuedInputAGM,
			expectedSender: "astrocyte",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotType, gotMsg := ClassifyQueuedInput(tt.input)
			if gotType != tt.expectedType {
				t.Errorf("ClassifyQueuedInput() type = %v, want %v", gotType, tt.expectedType)
			}
			if gotType == QueuedInputAGM {
				// Message should mention the sender and clear-input command
				if !containsSubstring(gotMsg, tt.expectedSender) {
					t.Errorf("message %q should contain sender %q", gotMsg, tt.expectedSender)
				}
				if !containsSubstring(gotMsg, "agm send clear-input") {
					t.Errorf("message %q should mention 'agm send clear-input'", gotMsg)
				}
			}
		})
	}
}

func TestClassifyQueuedInput_HumanInput(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "pasted text without AGM header",
			input: "[Pasted text #1 +2 lines]\nplease fix the bug in auth.go",
		},
		{
			name:  "queued messages prompt without AGM header",
			input: "Press up to edit queued messages\nsome random text the user typed",
		},
		{
			name:  "pasted text with partial From but no ID",
			input: "[Pasted text #1 +1 lines]\n[From: someone but no pipe ID field]",
		},
		{
			name:  "opaque Codex pasted-content chip",
			input: "› [Pasted Content 2172 chars]\n  gpt-5.6 xhigh · /repo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotType, gotMsg := ClassifyQueuedInput(tt.input)
			if gotType != QueuedInputHuman {
				t.Errorf("ClassifyQueuedInput() type = %v, want QueuedInputHuman", gotType)
			}
			if !containsSubstring(gotMsg, "human input in progress") {
				t.Errorf("message %q should mention 'human input in progress'", gotMsg)
			}
		})
	}
}

func TestClassifyQueuedInput_NoInput(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "clean pane with prompt",
			input: "Response text\n❯",
		},
		{
			name:  "empty string",
			input: "",
		},
		{
			name:  "normal output no paste indicators",
			input: "Building project...\nTests passed.\n❯",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotType, gotMsg := ClassifyQueuedInput(tt.input)
			if gotType != QueuedInputNone {
				t.Errorf("ClassifyQueuedInput() type = %v, want QueuedInputNone", gotType)
			}
			if gotMsg != "" {
				t.Errorf("ClassifyQueuedInput() msg = %q, want empty", gotMsg)
			}
		})
	}
}

func TestExtractSender(t *testing.T) {
	tests := []struct {
		name     string
		header   string
		expected string
	}{
		{
			name:     "orchestrator sender",
			header:   "[From: orchestrator | ID: 123-orchestr-001 | Sent: 2026-03-30T09:00:00Z]",
			expected: "orchestrator",
		},
		{
			name:     "hyphenated sender name",
			header:   "[From: worker-42 | ID: 456-worker42-002 | Sent: 2026-03-30T10:00:00Z]",
			expected: "worker-42",
		},
		{
			name:     "no From prefix",
			header:   "random text without From prefix",
			expected: "unknown",
		},
		{
			name:     "From prefix but no pipe separator",
			header:   "[From: incomplete",
			expected: "unknown",
		},
		{
			name:     "meta-orchestrator sender",
			header:   "[From: meta-orchestrator | ID: 789-meta-003 | Sent: 2026-03-30T11:00:00Z]",
			expected: "meta-orchestrator",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractSender(tt.header)
			if got != tt.expected {
				t.Errorf("extractSender(%q) = %q, want %q", tt.header, got, tt.expected)
			}
		})
	}
}

func TestClassifyQueuedInputBindsCompleteHeaderToLatestMarker(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  QueuedInputType
	}{
		{
			name:  "header before marker is unbound",
			input: "[From: orchestrator | ID: 1774863250311-orchestr-10093 | Sent: 2026-03-30T09:34:10Z]\n[Priority: urgent] Fix something\n[Pasted text #1 +5 lines]",
			want:  QueuedInputHuman,
		},
		{
			name:  "complete header immediately after latest marker",
			input: "❯ some command\noutput line\n[Pasted text #3 +8 lines]\n[From: fix-session | ID: 1774872000000-fix-sess-001 | Sent: 2026-03-30T12:00:00Z]\nDo the thing",
			want:  QueuedInputAGM,
		},
		{
			name:  "queued messages with AGM header",
			input: "Press up to edit queued messages\n[From: astrocyte | ID: 1774872000000-astrocyt-002 | Sent: 2026-03-30T08:00:00Z]\nHealth check",
			want:  QueuedInputAGM,
		},
		{
			name:  "complete header with reply-to",
			input: "[Pasted text #1 +2 lines]\n[From: astrocyte | ID: 1774872000000-astrocyt-003 | Sent: 2026-03-30T08:00:00Z | Reply-To: 1774871000000-orchestr-001]\nHealth check",
			want:  QueuedInputAGM,
		},
		{
			name:  "complete header with legacy valid reply-to",
			input: "[Pasted text #1 +2 lines]\n[From: astrocyte | ID: 1774872000000-astrocyt-004 | Sent: 2026-03-30T08:00:00Z | Reply-To: notanumber-sender-001]\nHealth check",
			want:  QueuedInputAGM,
		},
		{
			name:  "historical AGM header cannot own newer human paste",
			input: "[Pasted text #1 +2 lines]\n[From: orchestrator | ID: 1774872000000-orchestr-001 | Sent: 2026-03-30T08:00:00Z]\nrecover\n[Pasted text #2 +1 lines]\nplease preserve my draft",
			want:  QueuedInputHuman,
		},
		{
			name:  "Codex chip with observable complete header",
			input: "› [Pasted Content 2172 chars]\n[From: orchestrator | ID: 1774872000000-orchestr-002 | Sent: 2026-03-30T08:00:00Z]\nrecover",
			want:  QueuedInputAGM,
		},
		{
			name:  "partial header",
			input: "[Pasted text #1 +1 lines]\n[From: orchestrator | ID:",
			want:  QueuedInputHuman,
		},
		{
			name:  "missing sent field",
			input: "[Pasted text #1 +1 lines]\n[From: orchestrator | ID: 1774872000000-orchestr-003]",
			want:  QueuedInputHuman,
		},
		{
			name:  "invalid sent timestamp",
			input: "[Pasted text #1 +1 lines]\n[From: orchestrator | ID: 1774872000000-orchestr-003 | Sent: recently]",
			want:  QueuedInputHuman,
		},
		{
			name:  "missing closing bracket",
			input: "[Pasted text #1 +1 lines]\n[From: orchestrator | ID: 1774872000000-orchestr-003 | Sent: 2026-03-30T08:00:00Z",
			want:  QueuedInputHuman,
		},
		{
			name:  "invalid sender",
			input: "[Pasted text #1 +1 lines]\n[From: orchestrator! | ID: 1774872000000-orchestr-003 | Sent: 2026-03-30T08:00:00Z]",
			want:  QueuedInputHuman,
		},
		{
			name:  "message ID sender mismatch",
			input: "[Pasted text #1 +1 lines]\n[From: orchestrator | ID: 1774872000000-astrocyt-003 | Sent: 2026-03-30T08:00:00Z]",
			want:  QueuedInputHuman,
		},
		{
			name:  "invalid reply-to",
			input: "[Pasted text #1 +1 lines]\n[From: orchestrator | ID: 1774872000000-orchestr-003 | Sent: 2026-03-30T08:00:00Z | Reply-To: notes]",
			want:  QueuedInputHuman,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotType, _ := ClassifyQueuedInput(tt.input)
			if gotType != tt.want {
				t.Errorf("ClassifyQueuedInput() type = %v, want %v", gotType, tt.want)
			}
		})
	}
}

// containsSubstring is a test helper
func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
