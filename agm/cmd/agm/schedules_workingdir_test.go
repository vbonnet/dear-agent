package main

import (
	"path"
	"strings"
	"testing"
)

// TestScheduleTemplatesSetWorkingDirectory asserts every embedded launchd
// schedule template pins WorkingDirectory to the user's home (CLI-09, ce-k414).
//
// launchd starts jobs with cwd=/ when the key is absent, so any schedule that
// omits it runs agm (and everything agm spawns) from the filesystem root —
// relative paths, repo discovery, and worktree tooling all silently misbehave.
// The installers substitute __USER_HOME__ across the whole template, so
// asserting the placeholder here guarantees the installed plist gets the real
// home directory.
func TestScheduleTemplatesSetWorkingDirectory(t *testing.T) {
	entries, err := schedulesFS.ReadDir("schedules")
	if err != nil {
		t.Fatalf("read embedded schedules dir: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("no embedded schedule templates found")
	}

	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".plist") {
			continue
		}
		t.Run(name, func(t *testing.T) {
			raw, err := schedulesFS.ReadFile(path.Join("schedules", name))
			if err != nil {
				t.Fatalf("read template: %v", err)
			}
			content := string(raw)

			idx := strings.Index(content, "<key>WorkingDirectory</key>")
			if idx < 0 {
				t.Fatal("template missing WorkingDirectory: launchd would start this job with cwd=/")
			}

			// The very next <string> value after the key must be the home
			// placeholder, so the installed job runs from the user's home.
			rest := content[idx:]
			open := strings.Index(rest, "<string>")
			closeIdx := strings.Index(rest, "</string>")
			if open < 0 || closeIdx < open {
				t.Fatal("WorkingDirectory key has no <string> value")
			}
			if got := rest[open+len("<string>") : closeIdx]; got != "__USER_HOME__" {
				t.Errorf("WorkingDirectory = %q, want __USER_HOME__", got)
			}
		})
	}
}
