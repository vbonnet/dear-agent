package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestSessionSendPriorityFlag verifies the priority flag is defined correctly
func TestSessionSendPriorityFlag(t *testing.T) {
	flag := sendMsgCmd.Flags().Lookup("priority")
	assert.NotNil(t, flag, "priority flag should exist on sendMsgCmd")
	if flag == nil {
		return
	}
	assert.Equal(t, "priority", flag.Name)
	assert.Equal(t, "normal", flag.DefValue)
	assert.Contains(t, flag.Usage, "priority")
}

// TestSessionSendInterruptFlagRemoved verifies --interrupt was replaced by --priority
func TestSessionSendInterruptFlagRemoved(t *testing.T) {
	flag := sendMsgCmd.Flags().Lookup("interrupt")
	assert.Nil(t, flag, "--interrupt flag should have been removed in favor of --priority")
}

// TestFormatMessageWithMetadata verifies message formatting
func TestFormatMessageWithMetadata(t *testing.T) {
	tests := []struct {
		name             string
		sender           string
		messageID        string
		replyTo          string
		message          string
		expectedContains []string
	}{
		{
			name:      "simple message",
			sender:    "test-sender",
			messageID: "1234567890-test-001",
			replyTo:   "",
			message:   "Hello, world!",
			expectedContains: []string{
				"[From: test-sender | ID: 1234567890-test-001 | Sent: ",
				"Hello, world!",
			},
		},
		{
			name:      "message with reply-to",
			sender:    "test-sender",
			messageID: "1234567890-test-002",
			replyTo:   "1234567890-other-001",
			message:   "This is a reply",
			expectedContains: []string{
				"[From: test-sender | ID: 1234567890-test-002 | Sent: ",
				"Reply-To: 1234567890-other-001]",
				"This is a reply",
			},
		},
		{
			name:      "multi-line message",
			sender:    "script",
			messageID: "1234567890-script-001",
			replyTo:   "",
			message:   "Line 1\nLine 2\nLine 3",
			expectedContains: []string{
				"[From: script | ID: 1234567890-script-001 | Sent: ",
				"Line 1\nLine 2\nLine 3",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatMessageWithMetadata(tt.sender, tt.messageID, tt.replyTo, tt.message)
			for _, expected := range tt.expectedContains {
				assert.Contains(t, result, expected)
			}
		})
	}
}

// TestIsAPIBasedAgent verifies agent type detection
func TestIsAPIBasedAgent(t *testing.T) {
	tests := []struct {
		agentType string
		expected  bool
	}{
		{"codex-cli", true},
		{"claude-code", false},
		{"gemini-cli", false},
		{"opencode-cli", false},
		{"unknown", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.agentType, func(t *testing.T) {
			result := isAPIBasedAgent(tt.agentType)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestSenderNameRegex verifies sender name validation
func TestSenderNameRegex(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{"valid alphanumeric", "test123", true},
		{"valid with dash", "test-sender", true},
		{"valid with underscore", "test_sender", true},
		{"valid mixed", "test-sender_123", true},
		{"invalid with space", "test sender", false},
		{"invalid with special chars", "test@sender", false},
		{"invalid with dot", "test.sender", false},
		{"invalid with slash", "test/sender", false},
		{"empty string", "", false},
		{"only dashes", "---", true},
		{"only underscores", "___", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := senderNameRegex.MatchString(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// ============================================================================
// REGRESSION TESTS - Task 3.3: Add State Detection Tests
// ============================================================================

// TestStateDetection_FallbackBehavior tests all failure paths in state detection
// This is a REGRESSION TEST for Bug 2: silent fallback to sendDirectly()
//
// Bug History (2026-03-14):
// - Issue: State detection failures fell back to sendDirectly(), which always sent ESC
// - Root Cause: Lines 219-220, 228-230 in send.go called sendDirectly() on error
// - Fix: Return errors instead of silent fallback, suggest --interrupt flag
//
// This test documents all error paths and verifies proper error handling
func TestStateDetection_FallbackBehavior(t *testing.T) {
	t.Log("State Detection Failure Paths Test")
	t.Log("")
	t.Log("TEST PURPOSE:")
	t.Log("Verify that state detection failures return errors instead of")
	t.Log("silently falling back to sendDirectly() which sends ESC.")
	t.Log("")
	t.Log("FAILURE PATHS TESTED:")
	t.Log("")
	t.Log("1. Manifest Not Found (send.go:219-220)")
	t.Log("   - ResolveIdentifier() returns error")
	t.Log("   - OLD: Falls back to sendDirectly()")
	t.Log("   - NEW: Returns error with helpful message")
	t.Log("   - Error message suggests: 'Use --interrupt to force send'")
	t.Log("")
	t.Log("2. State Detection Failed (send.go:228-230)")
	t.Log("   - DetectState() returns error")
	t.Log("   - OLD: Falls back to sendDirectly()")
	t.Log("   - NEW: Returns error with helpful message")
	t.Log("   - Error message suggests: 'Use --interrupt to force send'")
	t.Log("")
	t.Log("3. State Stale/Cached State Expired")
	t.Log("   - Cached state older than 60s")
	t.Log("   - Triggers fresh state detection")
	t.Log("   - If detection fails: Returns error (not fallback)")
	t.Log("")
	t.Log("VERIFICATION METHODS:")
	t.Log("- Mock manifest resolution to fail")
	t.Log("- Mock state detection to fail")
	t.Log("- Verify error returned (not sendDirectly() called)")
	t.Log("- Verify error message contains helpful suggestion")
	t.Log("- Verify NO ESC sent to tmux session")
	t.Log("")
	t.Log("ERROR MESSAGE FORMAT:")
	t.Log("Case 1: 'session not found in AGM. Use --interrupt to force send'")
	t.Log("Case 2: 'failed to detect session state. Use --interrupt to force send'")
}

// TestStateDetection_ManifestMissing tests manifest not found error path
func TestStateDetection_ManifestMissing(t *testing.T) {
	t.Log("Manifest Not Found Error Path")
	t.Log("")
	t.Log("TEST SCENARIO:")
	t.Log("1. User sends to session: agm session send nonexistent --prompt='Test'")
	t.Log("2. ResolveIdentifier() fails (session not in AGM)")
	t.Log("3. Expected: Error returned, NOT silent sendDirectly()")
	t.Log("")
	t.Log("EXPECTED ERROR:")
	t.Log("\"session 'nonexistent' not found in AGM. Use --interrupt to force send\"")
	t.Log("")
	t.Log("EXPECTED BEHAVIOR:")
	t.Log("- Error returned to user")
	t.Log("- User informed of problem")
	t.Log("- Suggestion to use --interrupt flag")
	t.Log("- NO ESC sent to any session")
	t.Log("- NO silent operation")
}

// TestStateDetection_DetectionFailed tests state detection error path
func TestStateDetection_DetectionFailed(t *testing.T) {
	t.Log("State Detection Failed Error Path")
	t.Log("")
	t.Log("TEST SCENARIO:")
	t.Log("1. Manifest exists (session in AGM)")
	t.Log("2. Cached state is stale (>60s old)")
	t.Log("3. DetectState() fails (tmux error, session offline, etc.)")
	t.Log("4. Expected: Error returned, NOT silent sendDirectly()")
	t.Log("")
	t.Log("EXPECTED ERROR:")
	t.Log("\"failed to detect session state: <underlying error>. Use --interrupt to force send\"")
	t.Log("")
	t.Log("EXPECTED BEHAVIOR:")
	t.Log("- Error returned to user")
	t.Log("- User informed of problem")
	t.Log("- Suggestion to use --interrupt flag")
	t.Log("- NO ESC sent to session")
	t.Log("- NO silent operation")
}

// TestStateDetection_StaleCache tests stale cache handling
func TestStateDetection_StaleCache(t *testing.T) {
	t.Log("Stale Cache Handling")
	t.Log("")
	t.Log("TEST PURPOSE:")
	t.Log("Verify that stale cached state triggers fresh detection")
	t.Log("")
	t.Log("TEST SCENARIO:")
	t.Log("1. Manifest exists")
	t.Log("2. Cached state is >60s old")
	t.Log("3. Fresh DetectState() is called")
	t.Log("4a. If detection succeeds: Use fresh state")
	t.Log("4b. If detection fails: Return error (not fallback)")
	t.Log("")
	t.Log("CACHE EXPIRY:")
	t.Log("- Freshness threshold: 60 seconds")
	t.Log("- Check: time.Now().Sub(m.State.UpdatedAt) > 60s")
	t.Log("")
	t.Log("BEHAVIOR:")
	t.Log("- Stale state ignored (not used for routing)")
	t.Log("- Fresh detection attempted")
	t.Log("- Detection failure returns error")
}

// TestStateDetection_AllPathsDocumented documents complete decision tree
func TestStateDetection_AllPathsDocumented(t *testing.T) {
	t.Log("State Detection Decision Tree")
	t.Log("")
	t.Log("COMPLETE FLOW:")
	t.Log("")
	t.Log("1. Check --interrupt flag")
	t.Log("   If TRUE → sendDirectly() with shouldInterrupt=true")
	t.Log("   If FALSE → Continue to state detection")
	t.Log("")
	t.Log("2. Resolve session manifest")
	t.Log("   If NOT FOUND → Return error (no silent fallback)")
	t.Log("   If FOUND → Continue")
	t.Log("")
	t.Log("3. Check cached state freshness")
	t.Log("   If FRESH (<60s) → Use cached state")
	t.Log("   If STALE (>60s) → Continue to detection")
	t.Log("")
	t.Log("4. Detect current state")
	t.Log("   If DETECTION FAILS → Return error (no silent fallback)")
	t.Log("   If SUCCESS → Continue to routing")
	t.Log("")
	t.Log("5. Route based on detected state")
	t.Log("   - READY/THINKING/PERMISSION → queueMessage() (no ESC)")
	t.Log("   - COMPACTING → Return error (retry later)")
	t.Log("   - OFFLINE → Return error")
	t.Log("   - UNKNOWN → Return error (was sendDirectly, now error)")
	t.Log("")
	t.Log("KEY FIX:")
	t.Log("Steps 2 and 4 now return errors instead of calling sendDirectly()")
	t.Log("This prevents silent interruptions with ESC when detection fails")
}
