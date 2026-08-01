package deploy

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestDeployLaunchdPlistsSetWorkingDirectory asserts every user-scoped launchd
// template under deploy/launchd/ pins WorkingDirectory to the user's home
// (ce-k414). launchd starts jobs with cwd=/ when the key is absent, so a
// template that omits it ships an agent that runs from the filesystem root.
// The Makefile install targets substitute __HOME__ across the whole template,
// so asserting the placeholder here guarantees the staged plist gets the real
// home directory.
func TestDeployLaunchdPlistsSetWorkingDirectory(t *testing.T) {
	// Root LaunchDaemons that deliberately do not run in a user home.
	// fd-limit only runs sysctl as root, while override-audit runs a root-owned
	// fixed command. Both are installed into the system domain without home
	// placeholder substitution, so a __HOME__ WorkingDirectory would be wrong.
	exempt := map[string]bool{
		"com.dear-agent.fd-limit.plist":       true,
		"com.dear-agent.override-audit.plist": true,
	}

	dir := filepath.Join("..", "..", "deploy", "launchd")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read deploy/launchd: %v", err)
	}

	var checked int
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".plist") || exempt[name] {
			continue
		}
		checked++
		t.Run(name, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join(dir, name))
			if err != nil {
				t.Fatalf("read template: %v", err)
			}
			content := string(raw)

			idx := strings.Index(content, "<key>WorkingDirectory</key>")
			if idx < 0 {
				t.Fatal("template missing WorkingDirectory: launchd would start this job with cwd=/")
			}

			rest := content[idx:]
			open := strings.Index(rest, "<string>")
			closeIdx := strings.Index(rest, "</string>")
			if open < 0 || closeIdx < open {
				t.Fatal("WorkingDirectory key has no <string> value")
			}
			if got := rest[open+len("<string>") : closeIdx]; got != "__HOME__" {
				t.Errorf("WorkingDirectory = %q, want __HOME__", got)
			}
		})
	}
	if checked == 0 {
		t.Fatal("no user-scoped launchd templates found in deploy/launchd")
	}
}
