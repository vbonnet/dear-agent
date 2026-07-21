package cmd

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func TestRootCommandUsesCanonicalSessionSurface(t *testing.T) {
	var names []string
	for _, command := range rootCmd.Commands() {
		names = append(names, command.Name())
	}
	if !slices.Contains(names, "session") {
		t.Fatalf("root commands = %v, want session", names)
	}
	for _, retired := range []string{"start", "autopilot", "features", "abort"} {
		if slices.Contains(names, retired) {
			t.Errorf("root commands expose retired legacy executor %q: %v", retired, names)
		}
	}
}

func TestSessionCommandRegistersCanonicalOperations(t *testing.T) {
	var names []string
	for _, command := range sessionCmd.Commands() {
		names = append(names, command.Name())
	}
	for _, want := range []string{"start", "status", "next-phase", "start-phase", "complete-phase", "end", "task", "rewind-to", "set-lifecycle-state", "coord"} {
		if !slices.Contains(names, want) {
			t.Errorf("session commands = %v, missing %q", names, want)
		}
	}
}

func TestDocumentedWayfinderExamplesMatchCobra(t *testing.T) {
	root := filepath.Join("..", "..", "..")
	checked := 0
	for _, filename := range []string{"README.md", "SKILL.md"} {
		commands := documentedShellCommands(t, filepath.Join(root, filename))
		for _, invocation := range commands {
			if err := validateDocumentedInvocation(rootCmd, invocation); err != nil {
				t.Errorf("%s: %q: %v", filename, invocation, err)
			}
			checked++
		}
	}
	if checked < 10 {
		t.Fatalf("checked %d documented Wayfinder commands; expected at least 10", checked)
	}
}

func TestDocumentedInvocationRejectsUnknownSessionSubcommand(t *testing.T) {
	err := validateDocumentedInvocation(rootCmd, "wayfinder session migrate")
	if err == nil || !strings.Contains(err.Error(), "unknown subcommand") {
		t.Fatalf("validateDocumentedInvocation() error = %v, want unknown subcommand", err)
	}
}

func TestClaudePluginExposesCanonicalSkill(t *testing.T) {
	root := filepath.Join("..", "..", "..")
	manifest, err := os.ReadFile(filepath.Join(root, ".claude-plugin", "plugin.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(manifest), `"skills": ["./skills/"]`) {
		t.Fatal("Claude plugin manifest does not register its skill directory")
	}
	link := filepath.Join(root, "skills", "wayfinder", "SKILL.md")
	target, err := os.Readlink(link)
	if err != nil {
		t.Fatalf("canonical Claude skill link: %v", err)
	}
	if target != "../../SKILL.md" {
		t.Fatalf("Claude skill link = %q, want ../../SKILL.md", target)
	}
}

func documentedShellCommands(t *testing.T, path string) []string {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer file.Close()

	var commands []string
	var pending string
	inShellFence := false
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if language, found := strings.CutPrefix(line, "```"); found {
			if inShellFence {
				inShellFence = false
			} else {
				inShellFence = language == "sh" || language == "bash"
			}
			continue
		}
		if !inShellFence || line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		continued := strings.HasSuffix(line, "\\")
		line = strings.TrimSpace(strings.TrimSuffix(line, "\\"))
		pending = strings.TrimSpace(pending + " " + line)
		if continued {
			continue
		}
		if strings.HasPrefix(pending, "wayfinder ") {
			commands = append(commands, pending)
		}
		pending = ""
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan %s: %v", path, err)
	}
	return commands
}

func validateDocumentedInvocation(root *cobra.Command, invocation string) error {
	tokens := strings.Fields(invocation)
	if len(tokens) < 2 || tokens[0] != "wayfinder" {
		return fmt.Errorf("not a wayfinder invocation")
	}

	current := root
	resolved := 0
	helpRequested := false
	var positionals []string
	for index := 1; index < len(tokens); {
		token := strings.Trim(tokens[index], "[](),.;\"")
		if strings.HasPrefix(token, "-") {
			flag, ok := documentedFlag(root, current, token)
			if !ok {
				return fmt.Errorf("%s has no %s flag", current.CommandPath(), token)
			}
			helpRequested = helpRequested || flag.Name == "help"
			index++
			if !strings.Contains(token, "=") && flag.NoOptDefVal == "" && index < len(tokens) {
				index++
			}
			continue
		}

		var child *cobra.Command
		for _, candidate := range current.Commands() {
			if candidate.IsAvailableCommand() && !candidate.Hidden && candidate.Name() == token {
				child = candidate
				break
			}
		}
		if child == nil {
			if current.HasAvailableSubCommands() {
				return fmt.Errorf("%s has unknown subcommand %q", current.CommandPath(), token)
			}
			positionals = append(positionals, token)
			index++
			continue
		}
		current = child
		resolved++
		positionals = nil
		index++
	}
	if resolved == 0 {
		return fmt.Errorf("does not resolve to an active command")
	}
	if current.Args != nil && !helpRequested {
		if err := current.Args(current, positionals); err != nil {
			return fmt.Errorf("%s arguments: %w", current.CommandPath(), err)
		}
	}
	return nil
}

func documentedFlag(root, command *cobra.Command, token string) (*pflag.Flag, bool) {
	command.InitDefaultHelpFlag()
	name := strings.SplitN(token, "=", 2)[0]
	long := strings.HasPrefix(name, "--")
	name = strings.TrimLeft(name, "-")
	sets := []*pflag.FlagSet{
		command.Flags(),
		command.InheritedFlags(),
		command.PersistentFlags(),
		root.PersistentFlags(),
	}
	for _, set := range sets {
		var flag *pflag.Flag
		if long {
			flag = set.Lookup(name)
		} else {
			flag = set.ShorthandLookup(name)
		}
		if flag != nil {
			return flag, true
		}
	}
	return nil, false
}

func writeStatus(t *testing.T, dir string) {
	t.Helper()
	content := "---\nschema_version: \"2.0\"\nproject_name: test\nproject_type: feature\nrisk_level: M\ncurrent_waypoint: CHARTER\nstatus: planning\ncreated_at: 2026-07-17T00:00:00Z\nupdated_at: 2026-07-17T00:00:00Z\nwaypoint_history: []\n---\n"
	if err := os.WriteFile(filepath.Join(dir, "WAYFINDER-STATUS.md"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestDiscoverProjectDir_SingleProject verifies auto-discovery when exactly one
// project exists under wf/.
func TestDiscoverProjectDir_SingleProject(t *testing.T) {
	root := t.TempDir()
	projDir := filepath.Join(root, "wf", "my-project")
	if err := os.MkdirAll(projDir, 0o700); err != nil {
		t.Fatal(err)
	}
	writeStatus(t, projDir)

	got, err := discoverProjectDir(root)
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	if got != projDir {
		t.Errorf("expected %q, got %q", projDir, got)
	}
}

// TestDiscoverProjectDir_MultipleProjects verifies an error when more than one
// project is present (user must use -C).
func TestDiscoverProjectDir_MultipleProjects(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"proj-a", "proj-b"} {
		dir := filepath.Join(root, "wf", name)
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		writeStatus(t, dir)
	}

	_, err := discoverProjectDir(root)
	if err == nil {
		t.Fatal("expected error for multiple projects, got nil")
	}
	if !strings.Contains(err.Error(), "-C") {
		t.Errorf("error should mention -C flag, got: %v", err)
	}
}

// TestDiscoverProjectDir_NoWfDir verifies an error when wf/ does not exist.
func TestDiscoverProjectDir_NoWfDir(t *testing.T) {
	root := t.TempDir() // no wf/ subdirectory

	_, err := discoverProjectDir(root)
	if err == nil {
		t.Fatal("expected error when wf/ is absent, got nil")
	}
}

// TestDiscoverProjectDir_EmptyWfDir verifies an error when wf/ exists but has
// no project directories with STATUS files.
func TestDiscoverProjectDir_EmptyWfDir(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "wf"), 0o700); err != nil {
		t.Fatal(err)
	}

	_, err := discoverProjectDir(root)
	if err == nil {
		t.Fatal("expected error for empty wf/, got nil")
	}
}
