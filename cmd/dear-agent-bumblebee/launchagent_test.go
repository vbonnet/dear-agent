package main

import (
	"strings"
	"testing"
)

func TestRenderPlist_SubstitutesBothPlaceholders(t *testing.T) {
	const tmpl = `<plist>
  <home>__USER_HOME__</home>
  <bin>__BUMBLEBEE_BINARY__</bin>
  <log>__USER_HOME__/Library/Logs/dear-agent/bumblebee.log</log>
</plist>`
	got := renderPlist(tmpl, "/Users/alice", "/Users/alice/.local/bin/dear-agent-bumblebee")
	for _, want := range []string{
		"<home>/Users/alice</home>",
		"<bin>/Users/alice/.local/bin/dear-agent-bumblebee</bin>",
		"<log>/Users/alice/Library/Logs/dear-agent/bumblebee.log</log>",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("renderPlist output missing %q\nrendered:\n%s", want, got)
		}
	}
	if strings.Contains(got, "__USER_HOME__") || strings.Contains(got, "__BUMBLEBEE_BINARY__") {
		t.Fatalf("renderPlist left a placeholder unsubstituted:\n%s", got)
	}
}

func TestPlistTemplate_HasExpectedShape(t *testing.T) {
	// Embedded template smoke test: catch accidental edits that drop the
	// substitution tokens or the Label this binary expects to bootstrap.
	for _, want := range []string{
		"__USER_HOME__",
		"__BUMBLEBEE_BINARY__",
		"com.dear-agent.bumblebee",
		"<key>StartCalendarInterval</key>",
		"<key>RunAtLoad</key>",
	} {
		if !strings.Contains(plistTemplate, want) {
			t.Fatalf("embedded plist template missing %q", want)
		}
	}
}
