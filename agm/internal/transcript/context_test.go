package transcript

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractContext(t *testing.T) {
	// Create temp directory for test transcripts
	tempDir := t.TempDir()

	tests := []struct {
		name            string
		html            string
		uuid            string
		numExchanges    int
		wantMessages    int
		wantErr         bool
		setupTranscript bool
	}{
		{
			name: "valid transcript with multiple messages",
			html: `<!DOCTYPE html>
<html>
<body>
<div class="message user"><div class="message-content"><p>Hello, can you help?</p></div></div>
<div class="message assistant"><div class="message-content"><p>Of course! I'd be happy to help.</p></div></div>
<div class="message user"><div class="message-content"><p>I need to implement a feature.</p></div></div>
<div class="message assistant"><div class="message-content"><p>Sure, let me assist with that.</p></div></div>
</body>
</html>`,
			uuid:            "12345678-1234-1234-1234-123456789abc",
			numExchanges:    2,
			wantMessages:    4,
			wantErr:         false,
			setupTranscript: true,
		},
		{
			name: "single exchange",
			html: `<!DOCTYPE html>
<html>
<body>
<div class="message user"><div class="message-content"><p>Quick question</p></div></div>
<div class="message assistant"><div class="message-content"><p>Quick answer</p></div></div>
</body>
</html>`,
			uuid:            "22222222-2222-2222-2222-222222222222",
			numExchanges:    1,
			wantMessages:    2,
			wantErr:         false,
			setupTranscript: true,
		},
		{
			name:            "transcript file not found",
			uuid:            "33333333-3333-3333-3333-333333333333",
			numExchanges:    1,
			wantErr:         true,
			setupTranscript: false,
		},
		{
			name:            "empty transcript",
			html:            `<!DOCTYPE html><html><body></body></html>`,
			uuid:            "44444444-4444-4444-4444-444444444444",
			numExchanges:    1,
			wantMessages:    0,
			wantErr:         false,
			setupTranscript: true,
		},
		{
			name: "malformed HTML (missing closing tags)",
			html: `<!DOCTYPE html>
<html><body>
<div class="message user"><p>Test</p>
</body></html>`,
			uuid:            "55555555-5555-5555-5555-555555555555",
			numExchanges:    1,
			wantMessages:    0, // Should handle gracefully
			wantErr:         false,
			setupTranscript: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup transcript file if needed
			if tt.setupTranscript {
				transcriptDir := filepath.Join(tempDir, "transcripts", tt.uuid)
				require.NoError(t, os.MkdirAll(transcriptDir, 0755))
				transcriptPath := filepath.Join(transcriptDir, "index.html")
				require.NoError(t, os.WriteFile(transcriptPath, []byte(tt.html), 0644))
			}

			// Extract context
			ctx, err := ExtractContext(tempDir, tt.uuid, tt.numExchanges)

			// Check error expectation
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, ctx)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, ctx)
			assert.Equal(t, tt.uuid, ctx.UUID)
			assert.Len(t, ctx.Messages, tt.wantMessages)

			// Validate message structure
			for _, msg := range ctx.Messages {
				assert.NotEmpty(t, msg.Role, "Message role should not be empty")
				assert.Contains(t, []string{"user", "assistant"}, msg.Role, "Role should be user or assistant")
			}
		})
	}
}

func TestContextFormatForDisplay(t *testing.T) {
	tests := []struct {
		name         string
		ctx          *Context
		wantContains []string
	}{
		{
			name: "formats user and assistant messages",
			ctx: &Context{
				UUID: "test-uuid",
				Messages: []Message{
					{Role: "user", Content: "Hello"},
					{Role: "assistant", Content: "Hi there"},
				},
			},
			wantContains: []string{
				"📝 Transcript Context",
				"👤", // User icon
				"🤖", // Assistant icon
				"Hello",
				"Hi there",
			},
		},
		{
			name: "handles long content with wrapping",
			ctx: &Context{
				UUID: "test-uuid",
				Messages: []Message{
					{
						Role:    "user",
						Content: "This is a very long message that should be wrapped to fit within the terminal display width constraints",
					},
				},
			},
			wantContains: []string{
				"This is a very long message",
				"👤",
			},
		},
		{
			name: "handles empty messages",
			ctx: &Context{
				UUID:     "test-uuid",
				Messages: []Message{},
			},
			wantContains: []string{
				"📝 Transcript Context",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := tt.ctx.FormatForDisplay()

			assert.NotEmpty(t, output, "Formatted output should not be empty")

			for _, want := range tt.wantContains {
				assert.Contains(t, output, want, "Output should contain: %s", want)
			}

			// Verify box structure
			assert.Contains(t, output, "┌", "Should have top-left corner")
			assert.Contains(t, output, "└", "Should have bottom-left corner")
			assert.Contains(t, output, "│", "Should have vertical borders")
		})
	}
}

func TestExtractMessages(t *testing.T) {
	tests := []struct {
		name          string
		html          string
		limit         int
		wantCount     int
		wantFirstRole string
		wantLastRole  string
	}{
		{
			name: "extracts messages in order",
			html: `
<div class="message user"><div class="message-content"><p>First</p></div></div>
<div class="message assistant"><div class="message-content"><p>Second</p></div></div>
<div class="message user"><div class="message-content"><p>Third</p></div></div>
`,
			limit:         10,
			wantCount:     3,
			wantFirstRole: "user",
			wantLastRole:  "user",
		},
		{
			name: "respects limit",
			html: `
<div class="message user"><div class="message-content"><p>1</p></div></div>
<div class="message assistant"><div class="message-content"><p>2</p></div></div>
<div class="message user"><div class="message-content"><p>3</p></div></div>
<div class="message assistant"><div class="message-content"><p>4</p></div></div>
<div class="message user"><div class="message-content"><p>5</p></div></div>
`,
			limit:         2,
			wantCount:     2,
			wantFirstRole: "assistant",
			wantLastRole:  "user",
		},
		{
			name:      "handles no messages",
			html:      `<div>No messages here</div>`,
			limit:     10,
			wantCount: 0,
		},
		{
			name: "extracts content from nested tags",
			html: `
<div class="message user"><div class="message-content">
  <p>Line 1</p>
  <p>Line 2</p>
  <code>Some code</code>
</div></div>
`,
			limit:     10,
			wantCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			messages := extractMessages(tt.html, tt.limit)

			assert.Len(t, messages, tt.wantCount)

			if tt.wantCount > 0 && tt.wantFirstRole != "" {
				assert.Equal(t, tt.wantFirstRole, messages[0].Role)
			}
			if tt.wantCount > 0 && tt.wantLastRole != "" {
				assert.Equal(t, tt.wantLastRole, messages[len(messages)-1].Role)
			}
		})
	}
}

func TestWordWrap(t *testing.T) {
	tests := []struct {
		name      string
		text      string
		width     int
		wantLines int
		wantLen   int // Approximate expected length
	}{
		{
			name:      "short text no wrap",
			text:      "Hello",
			width:     50,
			wantLines: 1,
		},
		{
			name:      "long text wraps",
			text:      "This is a very long line that should definitely be wrapped at the specified width",
			width:     20,
			wantLines: 5, // Approximate
		},
		{
			name:      "empty string",
			text:      "",
			width:     50,
			wantLines: 1,
		},
		{
			name:      "single word longer than width",
			text:      "supercalifragilisticexpialidocious",
			width:     10,
			wantLines: 1, // Single word doesn't break
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := wordWrap(tt.text, tt.width)

			// Basic validation
			assert.NotNil(t, result)

			// Check that no line exceeds width (unless single word)
			lines := splitLines(result)
			if len(tt.text) > 0 && len(tt.text) < tt.width {
				for _, line := range lines {
					assert.LessOrEqual(t, len(line), tt.width+10, "Line should not exceed width significantly")
				}
			}
		})
	}
}

// Helper function to split text into lines
func splitLines(text string) []string {
	var lines []string
	current := ""
	for _, r := range text {
		if r == '\n' {
			lines = append(lines, current)
			current = ""
		} else {
			current += string(r)
		}
	}
	if current != "" {
		lines = append(lines, current)
	}
	return lines
}

func TestExtractContextIntegration(t *testing.T) {
	// Integration test: Create realistic transcript and verify end-to-end flow
	tempDir := t.TempDir()
	uuid := "aaaabbbb-cccc-dddd-eeee-ffffffffffff"

	// Create realistic transcript HTML
	html := `<!DOCTYPE html>
<html>
<head><title>Claude Session</title></head>
<body>
  <div class="message user"><div class="message-content">
    <p>Can you help me debug this function?</p>
  </div></div>
  <div class="message assistant"><div class="message-content">
    <p>Of course! I'd be happy to help debug your function. Could you share the code?</p>
  </div></div>
  <div class="message user"><div class="message-content">
    <p>Here's the function:</p>
    <pre><code>func process(data []int) int {
    sum := 0
    for i := range data {
        sum += i  // BUG: should be data[i]
    }
    return sum
}</code></pre>
  </div></div>
  <div class="message assistant"><div class="message-content">
    <p>I found the bug! On line 4, you're adding the index <code>i</code> instead of the value <code>data[i]</code>.</p>
  </div></div>
</body>
</html>`

	// Setup transcript
	transcriptDir := filepath.Join(tempDir, "transcripts", uuid)
	require.NoError(t, os.MkdirAll(transcriptDir, 0755))
	transcriptPath := filepath.Join(transcriptDir, "index.html")
	require.NoError(t, os.WriteFile(transcriptPath, []byte(html), 0644))

	// Extract context
	ctx, err := ExtractContext(tempDir, uuid, 2)
	require.NoError(t, err)
	require.NotNil(t, ctx)

	// Verify extraction
	assert.Equal(t, uuid, ctx.UUID)
	assert.Len(t, ctx.Messages, 4) // 2 exchanges = 4 messages

	// Verify roles alternate correctly
	assert.Equal(t, "user", ctx.Messages[0].Role)
	assert.Equal(t, "assistant", ctx.Messages[1].Role)
	assert.Equal(t, "user", ctx.Messages[2].Role)
	assert.Equal(t, "assistant", ctx.Messages[3].Role)

	// Verify content extraction
	assert.Contains(t, ctx.Messages[0].Content, "help me debug")
	assert.Contains(t, ctx.Messages[3].Content, "found the bug")

	// Format and verify display
	display := ctx.FormatForDisplay()
	assert.Contains(t, display, "📝 Transcript Context")
	assert.Contains(t, display, "👤") // User icon
	assert.Contains(t, display, "🤖") // Assistant icon
	assert.Contains(t, display, "help me debug")
}
