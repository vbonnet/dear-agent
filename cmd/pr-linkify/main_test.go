package main

import (
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/internal/prlinkify"
)

func TestFirstNonEmpty(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want string
	}{
		{"first wins", []string{"a", "b", "c"}, "a"},
		{"skip empties", []string{"", "", "c"}, "c"},
		{"all empty", []string{"", "", ""}, ""},
		{"none", nil, ""},
		{"single", []string{"only"}, "only"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := firstNonEmpty(tt.in...); got != tt.want {
				t.Errorf("firstNonEmpty(%v) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// captureStdio runs fn with stdin replaced by in and returns whatever fn writes
// to stdout.
func captureStdio(t *testing.T, in string, fn func()) string {
	t.Helper()

	origStdin, origStdout := os.Stdin, os.Stdout
	t.Cleanup(func() { os.Stdin, os.Stdout = origStdin, origStdout })

	inR, inW, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe stdin: %v", err)
	}
	outR, outW, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe stdout: %v", err)
	}
	os.Stdin, os.Stdout = inR, outW

	go func() {
		_, _ = io.WriteString(inW, in)
		_ = inW.Close()
	}()

	fn()
	_ = outW.Close()

	out, err := io.ReadAll(outR)
	if err != nil {
		t.Fatalf("read stdout: %v", err)
	}
	return string(out)
}

func TestRunHookEmitsLinks(t *testing.T) {
	hookIn := `{"tool_input":{"transcript":"merged PR #464 and PR #200, fixed issue #999"}}`
	out := captureStdio(t, hookIn, func() {
		runHook(prlinkify.Config{DefaultRepo: "vbonnet/dear-agent"})
	})

	var got struct {
		HookSpecificOutput struct {
			HookEventName     string `json:"hookEventName"`
			AdditionalContext string `json:"additionalContext"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("hook output is not valid JSON: %v\noutput: %q", err, out)
	}
	if got.HookSpecificOutput.HookEventName != "Stop" {
		t.Errorf("hookEventName = %q, want Stop", got.HookSpecificOutput.HookEventName)
	}
	ctx := got.HookSpecificOutput.AdditionalContext
	for _, want := range []string{"pull/464", "pull/200"} {
		if !strings.Contains(ctx, want) {
			t.Errorf("additionalContext missing %q:\n%s", want, ctx)
		}
	}
	// Issue #999 is not a PR and must not appear.
	if strings.Contains(ctx, "pull/999") {
		t.Errorf("additionalContext wrongly linkified issue #999:\n%s", ctx)
	}
}

func TestRunHookNoRefsEmitsNothing(t *testing.T) {
	hookIn := `{"tool_input":{"message":"nothing relevant here"}}`
	out := captureStdio(t, hookIn, func() {
		runHook(prlinkify.Config{})
	})
	if out != "" {
		t.Errorf("expected no output when there are no PR refs, got %q", out)
	}
}

func TestRunHookInvalidJSONIsIgnored(t *testing.T) {
	out := captureStdio(t, "not json at all", func() {
		runHook(prlinkify.Config{})
	})
	if out != "" {
		t.Errorf("invalid hook input should produce no output, got %q", out)
	}
}

func TestPrintSummary(t *testing.T) {
	out := captureStdio(t, "", func() {
		printSummary("see PR #464 and PR #200", prlinkify.Config{})
	})
	for _, want := range []string{"PR #464", "PR #200", "pull/464", "pull/200"} {
		if !strings.Contains(out, want) {
			t.Errorf("summary missing %q:\n%s", want, out)
		}
	}
}

func TestPrintSummaryNoRefs(t *testing.T) {
	out := captureStdio(t, "", func() {
		printSummary("no refs here", prlinkify.Config{})
	})
	if out != "" {
		t.Errorf("summary with no refs should be empty, got %q", out)
	}
}
