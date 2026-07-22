package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

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

func TestSessionSendInterruptFlagRemoved(t *testing.T) {
	flag := sendMsgCmd.Flags().Lookup("interrupt")
	assert.Nil(t, flag, "--interrupt flag should remain removed in favor of non-disruptive readiness routing")
}

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

func TestIsAPIBasedAgent(t *testing.T) {
	tests := []struct {
		agentType string
		expected  bool
	}{
		{"codex-cli", false},
		{"claude-code", false},
		{"gemini-cli", false},
		{"opencode-cli", false},
		{"pi-cli", false},
		{"openai", true},
		{"gpt", true},
		{"unknown", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.agentType, func(t *testing.T) {
			assert.Equal(t, tt.expected, isAPIBasedAgent(tt.agentType))
		})
	}
}

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
			assert.Equal(t, tt.expected, senderNameRegex.MatchString(tt.input))
		})
	}
}
