package main

import (
	"strings"
	"testing"
)

// TestRenderWatchStalledPlist verifies the embedded template renders into a
// valid launchd daemon plist: placeholders are substituted, alerts route to the
// chosen orchestrator, and the job is shaped as a KeepAlive daemon rather than
// an interval one-shot.
func TestRenderWatchStalledPlist(t *testing.T) {
	const (
		home = "/Users/test"
		bin  = "/Users/test/go/bin/agm"
		orch = "vroom-orchestrator"
		ws   = "oss"
	)

	out, err := renderWatchStalledPlist(home, bin, orch, ws)
	if err != nil {
		t.Fatalf("renderWatchStalledPlist: %v", err)
	}

	// No placeholder may survive substitution.
	for _, ph := range []string{"__USER_HOME__", "__AGM_BINARY__", "__ORCHESTRATOR__", "__WORKSPACE__"} {
		if strings.Contains(out, ph) {
			t.Errorf("placeholder %s not substituted", ph)
		}
	}

	// The binary runs the watch-stalled command and routes to the orchestrator.
	for _, want := range []string{
		"<string>" + bin + "</string>",
		"<string>watch-stalled</string>",
		"<string>--orchestrator</string>",
		"<string>" + orch + "</string>",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered plist missing %q", want)
		}
	}

	// Daemon shape: KeepAlive + RunAtLoad, NOT a StartInterval one-shot.
	if !strings.Contains(out, "<key>KeepAlive</key>") {
		t.Error("rendered plist missing KeepAlive (watch-stalled is a daemon)")
	}
	if !strings.Contains(out, "<key>RunAtLoad</key>") {
		t.Error("rendered plist missing RunAtLoad")
	}
	if strings.Contains(out, "<key>StartInterval</key>") {
		t.Error("rendered plist has StartInterval; watch-stalled is a persistent daemon, not interval one-shot")
	}

	// Log path is under the dear-agent logs dir with the substituted home.
	if !strings.Contains(out, home+"/Library/Logs/dear-agent/watch-stalled.log") {
		t.Error("rendered plist missing expected log path")
	}

	// WORKSPACE must be injected: launchd's minimal env otherwise picks the
	// default Dolt database and the watcher exits with "database not found".
	if !strings.Contains(out, "<key>WORKSPACE</key>") || !strings.Contains(out, "<string>"+ws+"</string>") {
		t.Error("rendered plist missing WORKSPACE environment variable")
	}
}

// TestRenderWatchStalledPlistCustomOrchestrator confirms a non-default
// orchestrator name flows through to the routing argument.
func TestRenderWatchStalledPlistCustomOrchestrator(t *testing.T) {
	out, err := renderWatchStalledPlist("/h", "/h/agm", "custom-overseer", "oss")
	if err != nil {
		t.Fatalf("renderWatchStalledPlist: %v", err)
	}
	if !strings.Contains(out, "<string>custom-overseer</string>") {
		t.Error("custom orchestrator name not rendered into plist")
	}
	if strings.Contains(out, "vroom-orchestrator") {
		t.Error("default orchestrator leaked when a custom one was supplied")
	}
}
