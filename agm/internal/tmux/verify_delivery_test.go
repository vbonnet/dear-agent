package tmux

import (
	"fmt"
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestExtractKeywords tests keyword extraction from prompt text.
func TestExtractKeywords(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		expected []string
	}{
		{
			name:     "extracts long words",
			text:     "You are a worker session. Task: PROMPT-DELIVERY-VERIFICATION",
			expected: []string{"worker", "session", "prompt-delivery-verification"},
		},
		{
			name:     "skips short words",
			text:     "do the thing now and go",
			expected: nil,
		},
		{
			name:     "limits to 3 keywords",
			text:     "implement feature verification testing coverage analysis reporting",
			expected: []string{"implement", "feature", "verification"},
		},
		{
			name:     "skips common words",
			text:     "please should would through between during",
			expected: nil,
		},
		{
			name:     "strips punctuation",
			text:     "**implement** (feature) [verification]",
			expected: []string{"implement", "feature", "verification"},
		},
		{
			name:     "empty text",
			text:     "",
			expected: nil,
		},
		{
			name:     "mixed short and long",
			text:     "fix the authentication module for deployment",
			expected: []string{"authentication", "module", "deployment"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractKeywords(tt.text)
			assert.Equal(t, tt.expected, got)
		})
	}
}

// TestIsCommonWord tests common word detection.
func TestIsCommonWord(t *testing.T) {
	assert.True(t, isCommonWord("please"))
	assert.True(t, isCommonWord("PLEASE")) // case-insensitive
	assert.True(t, isCommonWord("should"))
	assert.True(t, isCommonWord("between"))
	assert.False(t, isCommonWord("implement"))
	assert.False(t, isCommonWord("verify"))
	assert.False(t, isCommonWord("session"))
}

// TestKeywordsFoundInContent tests keyword matching against pane content.
func TestKeywordsFoundInContent(t *testing.T) {
	tests := []struct {
		name     string
		keywords []string
		content  string
		expected bool
	}{
		{
			name:     "keyword present",
			keywords: []string{"verification", "delivery"},
			content:  "Working on prompt verification task...\n⠋ Running",
			expected: true,
		},
		{
			name:     "keyword present case-insensitive",
			keywords: []string{"verification"},
			content:  "VERIFICATION in progress",
			expected: true,
		},
		{
			name:     "no keywords present",
			keywords: []string{"authentication", "module"},
			content:  "❯ \n\nWelcome to Claude Code",
			expected: false,
		},
		{
			name:     "empty keywords",
			keywords: nil,
			content:  "some content",
			expected: false,
		},
		{
			name:     "empty content",
			keywords: []string{"testing"},
			content:  "",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := keywordsFoundInContent(tt.keywords, tt.content)
			assert.Equal(t, tt.expected, got)
		})
	}
}

// TestVerifyPromptDelivery_SuccessOnFirstAttempt tests that verification succeeds
// immediately when the session shows processing indicators (keyword in content).
func TestVerifyPromptDelivery_SuccessOnFirstAttempt(t *testing.T) {
	skipIfNoTmux(t)
	setupTestSocket(t)
	setupTestState(t)

	sessionName := "test-vfy-ok-" + fmt.Sprintf("%d", time.Now().UnixNano()%100000)
	defer killTestSession(sessionName)

	err := NewSession(sessionName, t.TempDir())
	if err != nil {
		t.Skipf("cannot create tmux session: %v", err)
	}
	time.Sleep(200 * time.Millisecond)

	// Send a command containing the keyword so it appears in the pane
	socketPath := GetSocketPath()
	normalizedName := NormalizeTmuxSessionName(sessionName)
	cmd := exec.Command("tmux", "-S", socketPath, "send-keys", "-t", normalizedName, "echo delivery_keyword_xyzzy", "C-m")
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to send echo: %v", err)
	}
	time.Sleep(300 * time.Millisecond)

	sendCalled := 0
	result, err := VerifyPromptDelivery(sessionName, "delivery_keyword_xyzzy is the prompt", func() error {
		sendCalled++
		return nil
	}, 3)

	assert.NoError(t, err)
	assert.True(t, result.Delivered, "should be verified as delivered")
	assert.Equal(t, 1, result.Attempt, "should succeed on first attempt")
	assert.Equal(t, 0, sendCalled, "should not have called sendFunc")
}

// TestVerifyPromptDelivery_SendFuncError tests that sendFunc errors don't
// abort verification — it continues to the next attempt.
func TestVerifyPromptDelivery_SendFuncError(t *testing.T) {
	skipIfNoTmux(t)
	setupTestSocket(t)
	setupTestState(t)

	sessionName := "test-vfy-err-" + fmt.Sprintf("%d", time.Now().UnixNano()%100000)
	defer killTestSession(sessionName)

	err := NewSession(sessionName, t.TempDir())
	if err != nil {
		t.Skipf("cannot create tmux session: %v", err)
	}
	time.Sleep(200 * time.Millisecond)

	socketPath := GetSocketPath()
	normalizedName := NormalizeTmuxSessionName(sessionName)
	sendCalled := 0

	result, err := VerifyPromptDelivery(sessionName, "senderr_keyword_unique_abcdef", func() error {
		sendCalled++
		if sendCalled == 1 {
			return fmt.Errorf("simulated send failure")
		}
		// Second retry succeeds — echo keyword
		retryCmd := exec.Command("tmux", "-S", socketPath, "send-keys", "-t", normalizedName,
			"echo senderr_keyword_unique_abcdef", "C-m")
		return retryCmd.Run()
	}, 3)

	assert.NoError(t, err, "sendFunc errors should not propagate as verification errors")
	t.Logf("Result: delivered=%v, method=%s, attempt=%d, sendCalled=%d",
		result.Delivered, result.Method, result.Attempt, sendCalled)
}

// TestVerifyPromptDelivery_CaptureError tests that capture-pane failures
// return an error (e.g., non-existent session).
func TestVerifyPromptDelivery_CaptureError(t *testing.T) {
	// Non-existent session should cause capture-pane to fail
	result, err := VerifyPromptDelivery("nonexistent-session-xyz-99999", "test prompt text", func() error {
		return nil
	}, 0)

	assert.Error(t, err, "should error when capture-pane fails")
	assert.False(t, result.Delivered)
}
