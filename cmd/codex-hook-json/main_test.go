package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestInputFilters(t *testing.T) {
	input := `{
		"tool_name":"Bash",
		"tool_input":{
			"command":"bd close ce-1",
			"file_path":"script.sh",
			"content":"one",
			"new_string":"two",
			"edits":[{"new_string":"three"}],
			"patch":"patch text"
		},
		"hook_event_name":"Stop",
		"session_id":"session-1",
		"stop_hook_active":true,
		"cwd":"/repo"
	}`
	tests := []struct {
		filter string
		want   string
	}{
		{".tool_name // empty", "Bash\n"},
		{".tool_input.command // empty", "bd close ce-1\n"},
		{".tool_input.file_path // empty", "script.sh\n"},
		{".hook_event_name // empty", "Stop\n"},
		{".session_id // empty", "session-1\n"},
		{".stop_hook_active // false", "true\n"},
		{".cwd // empty", "/repo\n"},
		{`.tool_input as $t |
			[($t.content // empty), ($t.new_string // empty),
			(($t.edits // [])[] | .new_string // empty)] | join("\n")`, "one\ntwo\nthree\n"},
		{`if (.tool_input | type) == "string" then .tool_input
			else (.tool_input.patch // .tool_input.input // .tool_input.content // empty)
			end`, "patch text\n"},
	}
	for _, tt := range tests {
		t.Run(tt.filter, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if code := run([]string{"-r", tt.filter}, strings.NewReader(input), &stdout, &stderr); code != 0 {
				t.Fatalf("run() = %d, stderr=%s", code, stderr.String())
			}
			if got := stdout.String(); got != tt.want {
				t.Fatalf("output = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestOutputFilters(t *testing.T) {
	tests := []struct {
		args []string
		want map[string]any
	}{
		{
			args: []string{"-cn", "--arg", "msg", "blocked",
				`{hookSpecificOutput:{hookEventName:"PreToolUse",permissionDecision:"deny",additionalContext:$msg}}`},
			want: map[string]any{"hookSpecificOutput": map[string]any{
				"hookEventName": "PreToolUse", "permissionDecision": "deny", "additionalContext": "blocked",
			}},
		},
		{
			args: []string{"-cn", "--arg", "m", "blocked",
				`{hookSpecificOutput:{hookEventName:"PreToolUse",additionalContext:$m},permissionDecision:"deny",denialReason:$m}`},
			want: map[string]any{
				"hookSpecificOutput": map[string]any{"hookEventName": "PreToolUse", "additionalContext": "blocked"},
				"permissionDecision": "deny", "denialReason": "blocked",
			},
		},
		{
			args: []string{"-cn", "--arg", "r", "red", `{decision:"block",reason:$r}`},
			want: map[string]any{"decision": "block", "reason": "red"},
		},
		{
			args: []string{"-cn", "--arg", "c", "note", "--arg", "e", "Stop",
				`{hookSpecificOutput:{hookEventName:$e,additionalContext:$c}}`},
			want: map[string]any{"hookSpecificOutput": map[string]any{
				"hookEventName": "Stop", "additionalContext": "note",
			}},
		},
	}
	for _, tt := range tests {
		var stdout, stderr bytes.Buffer
		if code := run(tt.args, strings.NewReader(""), &stdout, &stderr); code != 0 {
			t.Fatalf("run(%q) = %d, stderr=%s", tt.args, code, stderr.String())
		}
		var got map[string]any
		if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if !mapsEqual(got, tt.want) {
			t.Fatalf("output = %#v, want %#v", got, tt.want)
		}
	}
}

func TestRejectsUnsupportedFilter(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"-r", ".secrets"}, strings.NewReader(`{}`), &stdout, &stderr); code != 2 {
		t.Fatalf("run() = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "unsupported input filter") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestCombinedEditTextOmitsMissingValuesLikeJQ(t *testing.T) {
	input := `{"tool_input":{"content":"one","edits":[{"new_string":"three"}]}}`
	filter := `.tool_input as $t |
		[($t.content // empty), ($t.new_string // empty),
		(($t.edits // [])[] | .new_string // empty)] | join("\n")`
	var stdout, stderr bytes.Buffer
	if code := run([]string{"-r", filter}, strings.NewReader(input), &stdout, &stderr); code != 0 {
		t.Fatalf("run() = %d, stderr=%s", code, stderr.String())
	}
	if got, want := stdout.String(), "one\nthree\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func mapsEqual(left, right map[string]any) bool {
	leftJSON, _ := json.Marshal(left)
	rightJSON, _ := json.Marshal(right)
	return bytes.Equal(leftJSON, rightJSON)
}
