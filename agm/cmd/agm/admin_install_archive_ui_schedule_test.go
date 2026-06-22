package main

import (
	"strings"
	"testing"
)

// TestRenderArchiveUIPlist verifies the embedded template renders into a valid
// launchd plist: placeholders are substituted, the job runs the conservative
// idle-archival command, and it is shaped as a daily interval one-shot.
func TestRenderArchiveUIPlist(t *testing.T) {
	const (
		home = "/Users/test"
		bin  = "/Users/test/go/bin/agm"
	)

	out, err := renderArchiveUIPlist(home, bin)
	if err != nil {
		t.Fatalf("renderArchiveUIPlist: %v", err)
	}

	// No placeholder may survive substitution.
	for _, ph := range []string{"__USER_HOME__", "__AGM_BINARY__"} {
		if strings.Contains(out, ph) {
			t.Errorf("placeholder %s not substituted", ph)
		}
	}

	// The binary runs the conservative idle-archival command. The flags are
	// load-bearing safety: without --status idle / --older-than 7d the job
	// would consider live and recently-active sessions.
	for _, want := range []string{
		"<string>" + bin + "</string>",
		"<string>session</string>",
		"<string>archive-ui</string>",
		"<string>--older-than</string>",
		"<string>7d</string>",
		"<string>--status</string>",
		"<string>idle</string>",
		"<string>--apply</string>",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered plist missing %q", want)
		}
	}

	// Interval one-shot shape: a daily StartInterval + RunAtLoad to drain the
	// existing backlog at install time. Not a KeepAlive daemon.
	if !strings.Contains(out, "<key>StartInterval</key>") {
		t.Error("rendered plist missing StartInterval (archive-ui is a daily one-shot)")
	}
	if !strings.Contains(out, "<integer>86400</integer>") {
		t.Error("rendered plist missing the 86400s (daily) interval")
	}
	if !strings.Contains(out, "<key>RunAtLoad</key>") {
		t.Error("rendered plist missing RunAtLoad")
	}
	if strings.Contains(out, "<key>KeepAlive</key>") {
		t.Error("rendered plist has KeepAlive; archive-ui is an interval one-shot, not a persistent daemon")
	}

	// Log path is under the dear-agent logs dir with the substituted home.
	if !strings.Contains(out, home+"/Library/Logs/dear-agent/archive-ui.log") {
		t.Error("rendered plist missing expected log path")
	}

	// Label matches the install/uninstall/kickstart contract.
	if !strings.Contains(out, "<string>"+archiveUIPlistLabel+"</string>") {
		t.Errorf("rendered plist missing label %q", archiveUIPlistLabel)
	}
}
