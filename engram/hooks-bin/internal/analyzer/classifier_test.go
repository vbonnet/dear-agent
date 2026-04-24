package analyzer

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// buildTranscriptFile creates a JSONL transcript file from raw JSON lines.
func buildTranscriptFile(t *testing.T, dir string, lines ...string) string {
	t.Helper()
	path := filepath.Join(dir, "transcript.jsonl")
	var data string
	for _, line := range lines {
		data += line + "\n"
	}
	if err := os.WriteFile(path, []byte(data), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

// helper to build a tool_use entry JSON line.
func toolUseLine(uuid, toolUseID, toolName string) string {
	return fmt.Sprintf(
		`{"sessionId":"s1","type":"assistant","uuid":"%s","message":{"role":"assistant","content":[{"type":"tool_use","id":"%s","name":"%s","input":{}}]}}`,
		uuid, toolUseID, toolName,
	)
}

// helper to build a tool_result entry JSON line (error/denial).
func toolResultLine(uuid, toolUseID string) string {
	return fmt.Sprintf(
		`{"sessionId":"s1","type":"assistant","uuid":"%s","message":{"role":"assistant","content":[{"type":"tool_result","tool_use_id":"%s","content":"hook denied this"}]}}`,
		uuid, toolUseID,
	)
}

// helper to build a text entry (no tool use).
func textLine(uuid, text string) string {
	return fmt.Sprintf(
		`{"sessionId":"s1","type":"assistant","uuid":"%s","message":{"role":"assistant","content":[{"type":"text","text":"%s"}]}}`,
		uuid, text,
	)
}

func TestClassifyDenials_RetrySuccess(t *testing.T) {
	dir := t.TempDir()
	// Transcript: tool_use denied, then retry with Bash succeeds (no error result).
	path := buildTranscriptFile(t, dir,
		toolUseLine("e1", "toolu_denied", "Bash"),
		toolResultLine("e2", "toolu_denied"),
		toolUseLine("e3", "toolu_retry", "Bash"),
		// No tool_result for toolu_retry means it succeeded.
	)

	denial := DenialEntry{
		TranscriptPath: path,
		ToolUseID:      "toolu_denied",
		SessionID:      "s1",
	}

	cache := NewTranscriptCache(4)
	results := ClassifyDenials([]DenialEntry{denial}, cache)

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	r := results[0]
	if r.Outcome != OutcomeRetrySuccess {
		t.Errorf("expected OutcomeRetrySuccess, got %s", r.Outcome)
	}
	if !r.IsFalsePositive {
		t.Error("expected IsFalsePositive=true for RetrySuccess")
	}
	if r.Confidence != 0.8 {
		t.Errorf("expected confidence 0.8, got %f", r.Confidence)
	}
	if r.NextToolName != "Bash" {
		t.Errorf("expected next tool Bash, got %s", r.NextToolName)
	}
}

func TestClassifyDenials_SwitchedTool(t *testing.T) {
	dir := t.TempDir()
	path := buildTranscriptFile(t, dir,
		toolUseLine("e1", "toolu_denied", "Bash"),
		toolResultLine("e2", "toolu_denied"),
		toolUseLine("e3", "toolu_read", "Read"),
	)

	denial := DenialEntry{
		TranscriptPath: path,
		ToolUseID:      "toolu_denied",
		SessionID:      "s1",
	}

	cache := NewTranscriptCache(4)
	results := ClassifyDenials([]DenialEntry{denial}, cache)

	r := results[0]
	if r.Outcome != OutcomeSwitchedTool {
		t.Errorf("expected OutcomeSwitchedTool, got %s", r.Outcome)
	}
	if r.IsFalsePositive {
		t.Error("expected IsFalsePositive=false for SwitchedTool")
	}
	if r.Confidence != 0.7 {
		t.Errorf("expected confidence 0.7, got %f", r.Confidence)
	}
	if r.NextToolName != "Read" {
		t.Errorf("expected next tool Read, got %s", r.NextToolName)
	}
}

func TestClassifyDenials_RetryDenied(t *testing.T) {
	dir := t.TempDir()
	path := buildTranscriptFile(t, dir,
		toolUseLine("e1", "toolu_denied", "Bash"),
		toolResultLine("e2", "toolu_denied"),
		toolUseLine("e3", "toolu_retry", "Bash"),
		toolResultLine("e4", "toolu_retry"), // retry also denied
	)

	denial := DenialEntry{
		TranscriptPath: path,
		ToolUseID:      "toolu_denied",
		SessionID:      "s1",
	}

	cache := NewTranscriptCache(4)
	results := ClassifyDenials([]DenialEntry{denial}, cache)

	r := results[0]
	if r.Outcome != OutcomeRetryDenied {
		t.Errorf("expected OutcomeRetryDenied, got %s", r.Outcome)
	}
	if !r.IsFalsePositive {
		t.Error("expected IsFalsePositive=true for RetryDenied")
	}
	if r.Confidence != 0.6 {
		t.Errorf("expected confidence 0.6, got %f", r.Confidence)
	}
}

func TestClassifyDenials_GaveUp(t *testing.T) {
	dir := t.TempDir()
	path := buildTranscriptFile(t, dir,
		toolUseLine("e1", "toolu_denied", "Bash"),
		toolResultLine("e2", "toolu_denied"),
		textLine("e3", "I cannot run that command"),
		textLine("e4", "Let me try something else"),
		textLine("e5", "Actually never mind"),
	)

	denial := DenialEntry{
		TranscriptPath: path,
		ToolUseID:      "toolu_denied",
		SessionID:      "s1",
	}

	cache := NewTranscriptCache(4)
	results := ClassifyDenials([]DenialEntry{denial}, cache)

	r := results[0]
	if r.Outcome != OutcomeGaveUp {
		t.Errorf("expected OutcomeGaveUp, got %s", r.Outcome)
	}
	if r.IsFalsePositive {
		t.Error("expected IsFalsePositive=false for GaveUp")
	}
	if r.Confidence != 0.3 {
		t.Errorf("expected confidence 0.3, got %f", r.Confidence)
	}
}

func TestClassifyDenials_TranscriptMissing(t *testing.T) {
	denial := DenialEntry{
		TranscriptPath: "/nonexistent/transcript.jsonl",
		ToolUseID:      "toolu_xxx",
		SessionID:      "s1",
	}

	cache := NewTranscriptCache(4)
	results := ClassifyDenials([]DenialEntry{denial}, cache)

	r := results[0]
	if r.Outcome != OutcomeTranscriptMissing {
		t.Errorf("expected OutcomeTranscriptMissing, got %s", r.Outcome)
	}
}

func TestClassifyDenials_NoMatchingToolUseID(t *testing.T) {
	dir := t.TempDir()
	path := buildTranscriptFile(t, dir,
		toolUseLine("e1", "toolu_other", "Bash"),
		textLine("e2", "some text"),
	)

	denial := DenialEntry{
		TranscriptPath: path,
		ToolUseID:      "toolu_nonexistent",
		SessionID:      "s1",
	}

	cache := NewTranscriptCache(4)
	results := ClassifyDenials([]DenialEntry{denial}, cache)

	r := results[0]
	if r.Outcome != OutcomeUnknown {
		t.Errorf("expected OutcomeUnknown for missing tool_use_id, got %s", r.Outcome)
	}
}

func TestClassifyDenials_WastedCalls(t *testing.T) {
	dir := t.TempDir()
	// Multiple consecutive Bash retries before resolution.
	path := buildTranscriptFile(t, dir,
		toolUseLine("e1", "toolu_denied", "Bash"),
		toolResultLine("e2", "toolu_denied"),
		toolUseLine("e3", "toolu_r1", "Bash"),
		toolUseLine("e4", "toolu_r2", "Bash"),
		// Neither has error results within the window, so first is success.
	)

	denial := DenialEntry{
		TranscriptPath: path,
		ToolUseID:      "toolu_denied",
		SessionID:      "s1",
	}

	cache := NewTranscriptCache(4)
	results := ClassifyDenials([]DenialEntry{denial}, cache)

	r := results[0]
	if r.Outcome != OutcomeRetrySuccess {
		t.Errorf("expected OutcomeRetrySuccess, got %s", r.Outcome)
	}
	if r.WastedCalls != 2 {
		t.Errorf("expected WastedCalls=2, got %d", r.WastedCalls)
	}
	if r.RetryCount != 2 {
		t.Errorf("expected RetryCount=2, got %d", r.RetryCount)
	}
}

func TestClassifyDenials_SwitchedToEachAlternativeTool(t *testing.T) {
	for _, tool := range []string{"Read", "Grep", "Glob", "Write", "Edit"} {
		t.Run(tool, func(t *testing.T) {
			dir := t.TempDir()
			path := buildTranscriptFile(t, dir,
				toolUseLine("e1", "toolu_denied", "Bash"),
				toolResultLine("e2", "toolu_denied"),
				toolUseLine("e3", "toolu_alt", tool),
			)

			denial := DenialEntry{
				TranscriptPath: path,
				ToolUseID:      "toolu_denied",
				SessionID:      "s1",
			}

			cache := NewTranscriptCache(4)
			results := ClassifyDenials([]DenialEntry{denial}, cache)

			r := results[0]
			if r.Outcome != OutcomeSwitchedTool {
				t.Errorf("expected OutcomeSwitchedTool for %s, got %s", tool, r.Outcome)
			}
			if r.NextToolName != tool {
				t.Errorf("expected next tool %s, got %s", tool, r.NextToolName)
			}
		})
	}
}

func TestClassifyDenials_MultipleDenials(t *testing.T) {
	dir := t.TempDir()
	path := buildTranscriptFile(t, dir,
		toolUseLine("e1", "toolu_d1", "Bash"),
		toolResultLine("e2", "toolu_d1"),
		toolUseLine("e3", "toolu_ok", "Read"),
		toolUseLine("e4", "toolu_d2", "Bash"),
		toolResultLine("e5", "toolu_d2"),
		textLine("e6", "giving up"),
	)

	denials := []DenialEntry{
		{TranscriptPath: path, ToolUseID: "toolu_d1", SessionID: "s1"},
		{TranscriptPath: path, ToolUseID: "toolu_d2", SessionID: "s1"},
	}

	cache := NewTranscriptCache(4)
	results := ClassifyDenials(denials, cache)

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].Outcome != OutcomeSwitchedTool {
		t.Errorf("first denial: expected SwitchedTool, got %s", results[0].Outcome)
	}
	if results[1].Outcome != OutcomeGaveUp {
		t.Errorf("second denial: expected GaveUp, got %s", results[1].Outcome)
	}
}
