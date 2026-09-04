package readiness

import (
	"testing"
	"time"
)

func TestClaudePromptTimeoutDefaultsTo90s(t *testing.T) {
	t.Setenv(ReadyTimeoutEnvVar, "")
	if got := ClaudePromptTimeout(); got != 90*time.Second {
		t.Errorf("ClaudePromptTimeout() = %v, want 90s", got)
	}
}

// The bug: this wait was hardcoded, so raising the env var to debug a slow
// bring-up changed nothing and the spawn still failed at 90s.
func TestClaudePromptTimeoutHonorsEnvVar(t *testing.T) {
	t.Setenv(ReadyTimeoutEnvVar, "180")
	if got := ClaudePromptTimeout(); got != 180*time.Second {
		t.Errorf("ClaudePromptTimeout() = %v, want 180s", got)
	}
}

func TestClaudePromptTimeoutClamps(t *testing.T) {
	t.Setenv(ReadyTimeoutEnvVar, "1")
	if got := ClaudePromptTimeout(); got != minClaudePromptTimeout {
		t.Errorf("ClaudePromptTimeout() = %v, want %v", got, minClaudePromptTimeout)
	}
	t.Setenv(ReadyTimeoutEnvVar, "99999")
	if got := ClaudePromptTimeout(); got != maxClaudePromptTimeout {
		t.Errorf("ClaudePromptTimeout() = %v, want %v", got, maxClaudePromptTimeout)
	}
}

// An unparseable value must not shorten the wait to something that fails a
// healthy cold start.
func TestClaudePromptTimeoutIgnoresInvalidValues(t *testing.T) {
	for _, raw := range []string{"abc", "-5", "0"} {
		t.Setenv(ReadyTimeoutEnvVar, raw)
		if got := ClaudePromptTimeout(); got != 90*time.Second {
			t.Errorf("ClaudePromptTimeout() with %q = %v, want the 90s default", raw, got)
		}
	}
}

// The two budgets share an env var but not a default: the ready-file wait is
// deliberately short so a failed association fails fast, while the composer wait
// has to survive a cold harness start.
func TestReadyTimeoutAndClaudePromptTimeoutDefaultsDiffer(t *testing.T) {
	t.Setenv(ReadyTimeoutEnvVar, "")
	if ReadyTimeout() == ClaudePromptTimeout() {
		t.Error("ready-file and composer waits collapsed to the same default")
	}
}
