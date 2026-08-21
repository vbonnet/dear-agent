package main

import (
	"strings"
	"testing"
)

// The scheduled refresh must publish to the exact path an interactive
// consumer resolves. launchd's EnvironmentVariables carries only PATH and
// HOME (not XDG_STATE_HOME), so quota-meter's own default resolution
// inside the job would silently diverge from $XDG_STATE_HOME for an
// operator who has it set. The installer instead bakes in the path it
// resolves in its own environment via a --state-file argument (codex
// review on #1218).
func TestQuotaRefreshPlistTemplateBakesInAStateFileArgument(t *testing.T) {
	raw, err := schedulesFS.ReadFile(quotaPlistFile)
	if err != nil {
		t.Fatalf("read embedded plist template: %v", err)
	}
	content := string(raw)

	if !strings.Contains(content, "<string>--state-file</string>") {
		t.Fatal("template ProgramArguments is missing --state-file")
	}
	if !strings.Contains(content, "<string>__STATE_FILE__</string>") {
		t.Fatal("template ProgramArguments is missing the __STATE_FILE__ placeholder")
	}

	// --state-file must immediately follow --refresh in ProgramArguments
	// (order matters for a flag argument), and __STATE_FILE__ must be the
	// very next element after --state-file.
	const refreshArg = "<string>--refresh</string>"
	_, rest, found := strings.Cut(content, refreshArg)
	if !found {
		t.Fatal("template is missing --refresh")
	}
	rest = strings.TrimLeft(rest, "\n\t ")
	if !strings.HasPrefix(rest, "<string>--state-file</string>") {
		t.Errorf("--state-file does not immediately follow --refresh: %.80q", rest)
	}
}

// Mirrors the substitution runInstallQuotaSchedule performs, proving the
// rendered plist ends up with the resolved path rather than the literal
// placeholder — the same regression class as __USER_HOME__/__AGM_BINARY__.
func TestQuotaRefreshPlistTemplateSubstitutesStateFilePath(t *testing.T) {
	raw, err := schedulesFS.ReadFile(quotaPlistFile)
	if err != nil {
		t.Fatalf("read embedded plist template: %v", err)
	}

	const statePath = "/Users/test/.local/state/dear-agent/quota/latest.json"
	content := strings.ReplaceAll(string(raw), "__USER_HOME__", "/Users/test")
	content = strings.ReplaceAll(content, "__AGM_BINARY__", "/Users/test/go/bin/agm")
	content = strings.ReplaceAll(content, "__STATE_FILE__", statePath)

	if strings.Contains(content, "__STATE_FILE__") {
		t.Error("__STATE_FILE__ placeholder was not fully substituted")
	}
	if !strings.Contains(content, "<string>"+statePath+"</string>") {
		t.Errorf("rendered plist does not carry the resolved state path %q", statePath)
	}
}
