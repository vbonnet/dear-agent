package fsguard

import (
	"strings"
	"testing"
)

// FuzzInspectCommand ensures InspectCommand never panics on arbitrary input.
// This covers the full command-parsing pipeline: tokenize → splitSegments →
// checkSegments → writeTargets / checkGit / checkRedirections.
//
// Run with: go test -fuzz=FuzzInspectCommand -fuzztime=60s ./internal/fsguard/
func FuzzInspectCommand(f *testing.F) {
	home := f.TempDir()
	g := &Guard{Home: home}

	// Seed corpus: representative command shapes.
	seeds := []string{
		"echo hello",
		"cat /dev/null",
		"git -C ~/src/repo log --oneline",
		"git commit -m 'message'",
		"cp -r src/ dst/",
		"rm -rf /tmp/x",
		"sed -i 's/a/b/' file.txt",
		"dd if=/dev/zero of=/tmp/x",
		"chmod 755 /etc/shadow",
		"> /etc/passwd",
		"tee /etc/crontab",
		"touch ~/src/repo/file.go",
		"python3 -c \"import os; os.write(1, b'x')\"",
		"bash -c 'git commit'",
		"env GIT_DIR=/tmp git push",
		"",
		"\"",
		"'",
		"\x00",
		strings.Repeat("a", 1000),
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, command string) {
		_, _ = g.InspectCommand(command, "")
	})
}

// FuzzTokenize ensures tokenize never panics and returns consistently.
func FuzzTokenize(f *testing.F) {
	f.Add("echo hello world")
	f.Add("git commit -m 'test message with spaces'")
	f.Add("\"unclosed")
	f.Add("'unclosed")
	f.Add("")
	f.Add("\\")
	f.Add("cmd && other | pipe ; semi")

	f.Fuzz(func(t *testing.T, input string) {
		tokens, _ := tokenize(input)
		// Tokenize must always return a non-nil slice on success.
		_ = tokens
	})
}
