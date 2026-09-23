package deploy

import (
	"os"
	"path/filepath"
	"strconv"
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

// TestMergeloopPlistPinsBackpressureCap asserts the mergeloop launchd template
// passes an explicit --cap, and that the value stays inside the tick budget.
//
// Without this, a later template cleanup could drop the argument pair while
// every test still passed, silently restoring DefaultCap (50). That default sat
// just above the live open-PR count, and a tick ABOVE the cap is skipped in
// full, so the loop would go quiet exactly when the backlog most needed
// draining.
//
// The upper bound matters as much as the lower one. ListOpen projects required
// checks per PR sequentially at roughly 3.4s per PR, so the 600s StartInterval
// fits about 175. A cap far above that does not remove the silence, it only
// changes its shape: one tick would run past its own interval instead of being
// skipped. Raising it further needs bounded or incremental projection first.
func TestMergeloopPlistPinsBackpressureCap(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "deploy", "launchd", "com.dear-agent.mergeloop.plist"))
	if err != nil {
		t.Fatalf("read mergeloop template: %v", err)
	}
	args := programArguments(t, string(raw))

	idx := -1
	for i, a := range args {
		if a == "--cap" {
			idx = i
			break
		}
	}
	if idx < 0 {
		t.Fatalf("template does not pass --cap, so the loop inherits DefaultCap and a "+
			"queue above it silences every tick; ProgramArguments = %v", args)
	}
	if idx+1 >= len(args) {
		t.Fatal("--cap is the last argument, so no value follows it")
	}
	cap, err := strconv.Atoi(args[idx+1])
	if err != nil {
		t.Fatalf("--cap value %q is not a number", args[idx+1])
	}
	if cap <= 50 {
		t.Errorf("--cap = %d, which is not clear of the default (50) or the live queue; "+
			"a burst of new PRs would skip every tick", cap)
	}
	if cap > 175 {
		t.Errorf("--cap = %d exceeds the measured 10-minute tick budget (~175 PRs at ~3.4s "+
			"per PR); a tick would run past its own interval", cap)
	}
}

// programArguments extracts the ProgramArguments string values from a launchd
// plist template without pulling in a plist parser.
func programArguments(t *testing.T, content string) []string {
	t.Helper()
	start := strings.Index(content, "<key>ProgramArguments</key>")
	if start < 0 {
		t.Fatal("template has no ProgramArguments")
	}
	open := strings.Index(content[start:], "<array>")
	closeIdx := strings.Index(content[start:], "</array>")
	if open < 0 || closeIdx < 0 || closeIdx < open {
		t.Fatal("template has a malformed ProgramArguments array")
	}
	block := content[start+open : start+closeIdx]

	var out []string
	for rest := block; ; {
		i := strings.Index(rest, "<string>")
		if i < 0 {
			break
		}
		rest = rest[i+len("<string>"):]
		j := strings.Index(rest, "</string>")
		if j < 0 {
			break
		}
		out = append(out, rest[:j])
		rest = rest[j:]
	}
	return out
}
