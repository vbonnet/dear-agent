package daemon

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vbonnet/dear-agent/pkg/enforcement"
)

func TestRecoveryStrategy_String(t *testing.T) {
	tests := []struct {
		strategy RecoveryStrategy
		want     string
	}{
		{RecoveryEscape, "escape"},
		{RecoveryEnter, "enter"},
		{RecoveryCtrlC, "ctrl_c"},
		{RecoveryRestart, "restart"},
		{RecoveryManual, "manual"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.strategy.String())
		})
	}
}

func TestParseStrategy(t *testing.T) {
	tests := []struct {
		input   string
		want    RecoveryStrategy
		wantErr bool
	}{
		{"escape", RecoveryEscape, false},
		{"enter", RecoveryEnter, false},
		{"ctrl_c", RecoveryCtrlC, false},
		{"restart", RecoveryRestart, false},
		{"manual", RecoveryManual, false},
		{"ESCAPE", RecoveryEscape, false}, // Case insensitive
		{"ENTER", RecoveryEnter, false},   // Case insensitive
		{"Ctrl_C", RecoveryCtrlC, false},
		{"invalid", RecoveryManual, true},
		{"", RecoveryManual, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseStrategy(tt.input)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestNewRecoveryHistory(t *testing.T) {
	history := NewRecoveryHistory("test-session", 3, 0)

	assert.Equal(t, "test-session", history.SessionName)
	assert.Equal(t, 0, history.TotalAttempts)
	assert.Equal(t, 3, history.MaxAttempts)
	assert.Empty(t, history.Attempts)
}

func TestRecoveryHistory_CanAttemptRecovery(t *testing.T) {
	history := NewRecoveryHistory("test-session", 3, 0)

	// Initially should allow attempts
	assert.True(t, history.CanAttemptRecovery())

	// After 2 attempts, should still allow
	history.RecordAttempt(RecoveryEscape, true, "stuck_mustering")
	history.RecordAttempt(RecoveryEscape, true, "stuck_waiting")
	assert.True(t, history.CanAttemptRecovery())
	assert.Equal(t, 2, history.TotalAttempts)

	// After 3 attempts (max), should not allow
	history.RecordAttempt(RecoveryEscape, false, "stuck_frozen")
	assert.False(t, history.CanAttemptRecovery())
	assert.Equal(t, 3, history.TotalAttempts)
}

func TestRecoveryHistory_RecordAttempt(t *testing.T) {
	history := NewRecoveryHistory("test-session", 5, 0)

	// Record successful attempt
	beforeTime := time.Now()
	history.RecordAttempt(RecoveryEscape, true, "stuck_mustering")
	afterTime := time.Now()

	assert.Equal(t, 1, len(history.Attempts))
	assert.Equal(t, 1, history.TotalAttempts)

	attempt := history.Attempts[0]
	assert.Equal(t, RecoveryEscape, attempt.Strategy)
	assert.True(t, attempt.Success)
	assert.Equal(t, "stuck_mustering", attempt.Reason)
	assert.True(t, attempt.Timestamp.After(beforeTime))
	assert.True(t, attempt.Timestamp.Before(afterTime))
	assert.True(t, history.LastAttempt.After(beforeTime))

	// Record failed attempt
	history.RecordAttempt(RecoveryCtrlC, false, "stuck_frozen")

	assert.Equal(t, 2, len(history.Attempts))
	assert.Equal(t, 2, history.TotalAttempts)
	assert.False(t, history.Attempts[1].Success)
}

func TestRecoveryHistory_CircuitBreaker(t *testing.T) {
	// Circuit breaker should prevent excessive recovery attempts
	history := NewRecoveryHistory("flood-test", 2, 0)

	// First attempt allowed
	assert.True(t, history.CanAttemptRecovery())
	history.RecordAttempt(RecoveryEscape, false, "test")

	// Second attempt allowed
	assert.True(t, history.CanAttemptRecovery())
	history.RecordAttempt(RecoveryEscape, false, "test")

	// Third attempt blocked (circuit breaker triggered)
	assert.False(t, history.CanAttemptRecovery())
	assert.Equal(t, 2, history.TotalAttempts)
	assert.Equal(t, 2, history.MaxAttempts)
}

func TestFormatRejectionForTmux(t *testing.T) {
	// Create pattern using enforcement.Pattern struct
	pattern := &enforcement.Pattern{
		ID: "cd-chaining",
	}

	message := "Test violation message"
	formatted := formatRejectionForTmux(message, pattern)

	// Verify formatted message contains expected sections
	assert.Contains(t, formatted, "Test violation message")
	assert.Contains(t, formatted, "NEXT STEPS")
	assert.Contains(t, formatted, "File violation: cd-chaining")
	assert.Contains(t, formatted, "RESUME YOUR WORK")
	assert.Contains(t, formatted, "Do NOT stop your task")
}

func TestCursorPosition(t *testing.T) {
	pos1 := CursorPosition{X: 10, Y: 20}
	pos2 := CursorPosition{X: 10, Y: 20}
	pos3 := CursorPosition{X: 15, Y: 20}

	// Test equality
	assert.Equal(t, pos1.X, pos2.X)
	assert.Equal(t, pos1.Y, pos2.Y)
	assert.NotEqual(t, pos1.X, pos3.X)
}

func TestRecoveryResult(t *testing.T) {
	result := &RecoveryResult{
		Success:      true,
		Strategy:     RecoveryEscape,
		DurationMs:   150,
		BeforeCursor: CursorPosition{X: 0, Y: 10},
		AfterCursor:  CursorPosition{X: 5, Y: 10},
	}

	assert.True(t, result.Success)
	assert.Equal(t, RecoveryEscape, result.Strategy)
	assert.Equal(t, int64(150), result.DurationMs)
	assert.NotEqual(t, result.BeforeCursor.X, result.AfterCursor.X)
}

// (Removed mock pattern - using actual enforcement.Pattern struct instead)

func TestRecoveryAttempt(t *testing.T) {
	attempt := RecoveryAttempt{
		Timestamp: time.Now(),
		Strategy:  RecoveryCtrlC,
		Success:   true,
		Reason:    "stuck_permission_prompt",
	}

	assert.Equal(t, RecoveryCtrlC, attempt.Strategy)
	assert.True(t, attempt.Success)
	assert.Equal(t, "stuck_permission_prompt", attempt.Reason)
	assert.False(t, attempt.Timestamp.IsZero())
}

func TestRecoveryHistory_MultipleAttempts(t *testing.T) {
	// Test recording multiple attempts with different outcomes
	history := NewRecoveryHistory("multi-test", 10, 0)

	strategies := []RecoveryStrategy{
		RecoveryEscape,
		RecoveryEscape,
		RecoveryCtrlC,
		RecoveryRestart,
	}
	successes := []bool{false, false, true, true}
	reasons := []string{"stuck_1", "stuck_2", "stuck_3", "stuck_4"}

	for i := 0; i < len(strategies); i++ {
		history.RecordAttempt(strategies[i], successes[i], reasons[i])
	}

	// Verify all attempts recorded
	assert.Equal(t, 4, len(history.Attempts))
	assert.Equal(t, 4, history.TotalAttempts)

	// Verify attempt details
	for i := 0; i < len(strategies); i++ {
		assert.Equal(t, strategies[i], history.Attempts[i].Strategy)
		assert.Equal(t, successes[i], history.Attempts[i].Success)
		assert.Equal(t, reasons[i], history.Attempts[i].Reason)
	}

	// Verify timestamps are in order
	for i := 1; i < len(history.Attempts); i++ {
		assert.True(t, history.Attempts[i].Timestamp.After(history.Attempts[i-1].Timestamp) ||
			history.Attempts[i].Timestamp.Equal(history.Attempts[i-1].Timestamp))
	}
}

func TestStrategyForSymptom_ZeroToken(t *testing.T) {
	// Zero-token waiting should use ESC (least disruptive)
	tests := []struct {
		symptom  string
		expected RecoveryStrategy
	}{
		{"stuck_zero_token_waiting", RecoveryEscape},
		{"stuck_mustering", RecoveryEscape},
	}

	for _, tt := range tests {
		t.Run(tt.symptom, func(t *testing.T) {
			strategy := StrategyForSymptom(tt.symptom)
			assert.Equal(t, tt.expected, strategy)
			assert.Equal(t, "escape", strategy.String())
		})
	}
}

func TestStrategyForSymptom_PermissionPrompt(t *testing.T) {
	// Permission prompts should use Enter (dismiss the prompt)
	strategy := StrategyForSymptom("stuck_permission_prompt")
	assert.Equal(t, RecoveryEnter, strategy)
	assert.Equal(t, "enter", strategy.String())

	// Verify Enter round-trips through ParseStrategy
	parsed, err := ParseStrategy(strategy.String())
	require.NoError(t, err)
	assert.Equal(t, RecoveryEnter, parsed)
}

func TestStrategyForSymptom_FrozenCursor(t *testing.T) {
	// Frozen cursor should use Ctrl+C (interrupt frozen process)
	strategy := StrategyForSymptom("cursor_frozen")
	assert.Equal(t, RecoveryCtrlC, strategy)
	assert.Equal(t, "ctrl_c", strategy.String())
}

func TestStrategyForSymptom_UnknownFallback(t *testing.T) {
	// Unknown symptoms should fall back to ESC (safest default)
	tests := []struct {
		symptom string
	}{
		{"unknown_symptom"},
		{""},
		{"something_new"},
	}

	for _, tt := range tests {
		t.Run(tt.symptom, func(t *testing.T) {
			strategy := StrategyForSymptom(tt.symptom)
			assert.Equal(t, RecoveryEscape, strategy, "unknown symptoms should default to escape")
		})
	}
}

func TestStrategyForSymptom_AllSymptomsCovered(t *testing.T) {
	// Verify all known symptom types map to distinct strategies
	symptomStrategy := map[string]RecoveryStrategy{
		"stuck_zero_token_waiting":         RecoveryEscape,
		"stuck_mustering":                  RecoveryEscape,
		"stuck_permission_prompt":          RecoveryEnter,
		"stuck_permission_prompt_escalate": RecoveryManual,
		"cursor_frozen":                    RecoveryCtrlC,
	}

	for symptom, expectedStrategy := range symptomStrategy {
		t.Run(symptom, func(t *testing.T) {
			got := StrategyForSymptom(symptom)
			assert.Equal(t, expectedStrategy, got,
				"symptom %q should map to %s, got %s", symptom, expectedStrategy, got)
		})
	}
}

func TestRecoveryEnter_StringAndParse(t *testing.T) {
	// RecoveryEnter should round-trip through String/Parse
	assert.Equal(t, "enter", RecoveryEnter.String())

	parsed, err := ParseStrategy("enter")
	require.NoError(t, err)
	assert.Equal(t, RecoveryEnter, parsed)

	// Case insensitive
	parsed, err = ParseStrategy("ENTER")
	require.NoError(t, err)
	assert.Equal(t, RecoveryEnter, parsed)

	parsed, err = ParseStrategy("Enter")
	require.NoError(t, err)
	assert.Equal(t, RecoveryEnter, parsed)
}

func TestRecoveryStrategy_AllStrategiesHaveStringRepresentation(t *testing.T) {
	// Ensure all strategy values have valid string representations
	strategies := []RecoveryStrategy{
		RecoveryEscape,
		RecoveryEnter,
		RecoveryCtrlC,
		RecoveryRestart,
		RecoveryManual,
	}

	for _, strategy := range strategies {
		str := strategy.String()
		assert.NotEmpty(t, str)
		assert.NotEqual(t, "unknown", str)

		// Verify round-trip parsing
		parsed, err := ParseStrategy(str)
		require.NoError(t, err)
		assert.Equal(t, strategy, parsed)
	}
}
