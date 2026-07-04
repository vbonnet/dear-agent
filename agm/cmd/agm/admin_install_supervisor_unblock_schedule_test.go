package main

import (
	"strings"
	"testing"
)

// TestRenderSupervisorUnblockPlist verifies the embedded template renders into a
// valid launchd daemon plist: placeholders are substituted, the binary runs the
// cross-check scan loop, and the job is shaped as a KeepAlive daemon rather than
// an interval one-shot. This is the external permission-prompt watchdog (ce-20p9)
// that survives a total-mesh stall no supervisor could recover from.
func TestRenderSupervisorUnblockPlist(t *testing.T) {
	const (
		home = "/Users/test"
		bin  = "/Users/test/go/bin/agm"
		ws   = "oss"
	)

	out, err := renderSupervisorUnblockPlist(home, bin, ws)
	if err != nil {
		t.Fatalf("renderSupervisorUnblockPlist: %v", err)
	}

	// No placeholder may survive substitution.
	for _, ph := range []string{"__USER_HOME__", "__AGM_BINARY__", "__WORKSPACE__"} {
		if strings.Contains(out, ph) {
			t.Errorf("placeholder %s not substituted", ph)
		}
	}

	// The binary runs the cross-check scan loop — this is the whole point: the
	// watchdog auto-approves stuck supervisors independent of any supervisor.
	for _, want := range []string{
		"<string>" + bin + "</string>",
		"<string>scan</string>",
		"<string>--loop</string>",
		"<string>--cross-check</string>",
		"<string>--interval</string>",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered plist missing %q", want)
		}
	}

	// Daemon shape: KeepAlive + RunAtLoad, NOT a StartInterval one-shot (the
	// scan loop is persistent, like watch-stalled).
	if !strings.Contains(out, "<key>KeepAlive</key>") {
		t.Error("rendered plist missing KeepAlive (the scan loop is a daemon)")
	}
	if !strings.Contains(out, "<key>RunAtLoad</key>") {
		t.Error("rendered plist missing RunAtLoad")
	}
	if strings.Contains(out, "<key>StartInterval</key>") {
		t.Error("rendered plist has StartInterval; the watchdog is a persistent daemon, not interval one-shot")
	}

	// Log path is under the dear-agent logs dir with the substituted home.
	if !strings.Contains(out, home+"/Library/Logs/dear-agent/supervisor-unblock.log") {
		t.Error("rendered plist missing expected log path")
	}

	// WORKSPACE must be injected: launchd's minimal env otherwise picks the
	// default Dolt database and the scan finds no supervisor sessions.
	if !strings.Contains(out, "<key>WORKSPACE</key>") || !strings.Contains(out, "<string>"+ws+"</string>") {
		t.Error("rendered plist missing WORKSPACE environment variable")
	}

	// WorkingDirectory must be the user's home (CLI-09, ce-k414): launchd
	// otherwise starts the daemon with cwd=/.
	if !strings.Contains(out, "<key>WorkingDirectory</key>\n    <string>"+home+"</string>") {
		t.Errorf("rendered plist missing WorkingDirectory pinned to home %q", home)
	}
}

// TestRenderSupervisorUnblockPlistCustomWorkspace confirms a non-default
// workspace flows through to the environment.
func TestRenderSupervisorUnblockPlistCustomWorkspace(t *testing.T) {
	out, err := renderSupervisorUnblockPlist("/h", "/h/agm", "personal")
	if err != nil {
		t.Fatalf("renderSupervisorUnblockPlist: %v", err)
	}
	if !strings.Contains(out, "<string>personal</string>") {
		t.Error("custom workspace not rendered into plist")
	}
}
