package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/agent"
)

var updatePluginSkills = flag.Bool("update-plugin-skills", false, "rewrite generated AGM plugin skills")

func TestGeneratedPluginSkillsUpToDate(t *testing.T) {
	files, err := renderPluginSkills(rootCmd, pluginCommandInventory)
	if err != nil {
		t.Fatalf("renderPluginSkills: %v", err)
	}
	commandsDir := filepath.Join("..", "..", "agm-plugin", "commands")
	for name, expected := range files {
		path := filepath.Join(commandsDir, name)
		if *updatePluginSkills {
			if err := os.WriteFile(path, expected, 0o600); err != nil {
				t.Fatalf("write %s: %v", path, err)
			}
			continue
		}
		actual, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if !slices.Equal(actual, expected) {
			t.Errorf("%s is stale; run `go generate ./agm/cmd/agm`", path)
		}
	}
}

func TestPluginCommandInventoryCoversInstalledMarkdown(t *testing.T) {
	commandsDir := filepath.Join("..", "..", "agm-plugin", "commands")
	matches, err := filepath.Glob(filepath.Join(commandsDir, "*.md"))
	if err != nil {
		t.Fatalf("glob commands: %v", err)
	}
	var installed []string
	for _, path := range matches {
		if filepath.Base(path) != "SPEC.md" {
			installed = append(installed, filepath.Base(path))
		}
	}
	var inventoried []string
	for _, owner := range pluginCommandInventory {
		inventoried = append(inventoried, owner.Filename)
	}
	slices.Sort(installed)
	slices.Sort(inventoried)
	if !slices.Equal(installed, inventoried) {
		t.Fatalf("installed plugin command inventory mismatch\ninstalled:  %v\ninventoried: %v", installed, inventoried)
	}
}

func TestInstalledPluginInvocationsMatchCobraAndInventory(t *testing.T) {
	commandsDir := filepath.Join("..", "..", "agm-plugin", "commands")
	invocationPattern := regexp.MustCompile("`agm ([^`]+)`")
	for _, owner := range pluginCommandInventory {
		content, err := os.ReadFile(filepath.Join(commandsDir, owner.Filename))
		if err != nil {
			t.Fatalf("read %s: %v", owner.Filename, err)
		}
		matches := invocationPattern.FindAllStringSubmatch(string(content), -1)
		if len(matches) == 0 {
			t.Errorf("%s contains no documented agm invocation", owner.Filename)
			continue
		}
		for _, match := range matches {
			path, flags, err := parseDocumentedInvocation(rootCmd, match[1])
			if err != nil {
				t.Errorf("%s: `agm %s`: %v", owner.Filename, match[1], err)
				continue
			}
			contract, ok := ownerContract(owner, path)
			if !ok {
				t.Errorf("%s documents unowned command path agm %s", owner.Filename, path)
				continue
			}
			for _, flagName := range flags {
				if !slices.Contains(contract.Flags, flagName) {
					t.Errorf("%s documents undeclared flag --%s for agm %s", owner.Filename, flagName, path)
				}
			}
		}
	}
}

func TestRootPluginSkillInvocationsMatchCobra(t *testing.T) {
	path := filepath.Join("..", "..", "plugins", "agm", "SKILL.md")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read root plugin skill: %v", err)
	}
	invocationPattern := regexp.MustCompile("`agm ([^`]+)`")
	for _, match := range invocationPattern.FindAllStringSubmatch(string(content), -1) {
		if strings.HasPrefix(match[1], "<command>") {
			continue
		}
		if _, _, err := parseDocumentedInvocation(rootCmd, match[1]); err != nil {
			t.Errorf("root plugin skill: `agm %s`: %v", match[1], err)
		}
	}
}

func TestRegisteredPluginSkillsMatchCobra(t *testing.T) {
	root := filepath.Join("..", "..", "agm-plugin", "skills")
	matches, err := filepath.Glob(filepath.Join(root, "*", "SKILL.md"))
	if err != nil {
		t.Fatalf("glob registered plugin skills: %v", err)
	}
	var relative []string
	for _, path := range matches {
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			t.Fatalf("relative skill path: %v", relErr)
		}
		relative = append(relative, filepath.ToSlash(rel))
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatalf("read %s: %v", path, readErr)
		}
		invocationPattern := regexp.MustCompile("`agm ([^`]+)`")
		invocations := invocationPattern.FindAllStringSubmatch(string(content), -1)
		if len(invocations) == 0 {
			t.Errorf("%s contains no documented agm invocation", rel)
		}
		for _, invocation := range invocations {
			if _, _, err := parseDocumentedInvocation(rootCmd, invocation[1]); err != nil {
				t.Errorf("%s: `agm %s`: %v", rel, invocation[1], err)
			}
		}
	}
	slices.Sort(relative)
	want := []string{"scan-health/SKILL.md"}
	if !slices.Equal(relative, want) {
		t.Fatalf("registered plugin skill inventory mismatch\ngot:  %v\nwant: %v", relative, want)
	}
}

func parseDocumentedInvocation(root *cobra.Command, invocation string) (string, []string, error) {
	tokens := strings.Fields(invocation)
	current := root
	var path []string
	index := 0
	for index < len(tokens) {
		token := strings.Trim(tokens[index], "[](),.;")
		if strings.HasPrefix(token, "-") || strings.HasPrefix(token, "<") {
			break
		}
		var child *cobra.Command
		for _, candidate := range current.Commands() {
			if candidate.IsAvailableCommand() && !candidate.Hidden && candidate.Name() == token {
				child = candidate
				break
			}
		}
		if child == nil {
			break
		}
		path = append(path, token)
		current = child
		index++
	}
	if len(path) == 0 {
		return "", nil, fmt.Errorf("does not resolve to an active Cobra command")
	}

	var flags []string
	for ; index < len(tokens); index++ {
		token := strings.Trim(tokens[index], "[](),.;")
		if !strings.HasPrefix(token, "-") {
			continue
		}
		name := strings.TrimLeft(strings.SplitN(token, "=", 2)[0], "-")
		if name == "" {
			continue
		}
		if len(strings.SplitN(token, "=", 2)[0]) == 2 {
			flag := current.Flags().ShorthandLookup(name)
			if flag == nil {
				flag = current.InheritedFlags().ShorthandLookup(name)
			}
			if flag == nil {
				return "", nil, fmt.Errorf("agm %s has no -%s flag", strings.Join(path, " "), name)
			}
			name = flag.Name
		}
		if _, ok := lookupCommandFlagType(current, name); !ok {
			return "", nil, fmt.Errorf("agm %s has no --%s flag", strings.Join(path, " "), name)
		}
		flags = append(flags, name)
	}
	return strings.Join(path, " "), flags, nil
}

func ownerContract(owner pluginCommandOwner, path string) (pluginCommandContract, bool) {
	for _, contract := range owner.Commands {
		if contract.Path == path {
			return contract, true
		}
	}
	return pluginCommandContract{}, false
}

func TestPluginSkillRendererRejectsDuplicateCommandPaths(t *testing.T) {
	root := &cobra.Command{Use: "agm"}
	run := func(*cobra.Command, []string) {}
	root.AddCommand(&cobra.Command{Use: "list", Run: run}, &cobra.Command{Use: "list", Run: run})
	_, err := renderPluginSkills(root, []pluginCommandOwner{{
		Filename: "list.md",
		Generated: &generatedPluginSkill{
			Title:       "List",
			Description: "List",
		},
		Commands: []pluginCommandContract{{Path: "list"}},
	}})
	if err == nil || !strings.Contains(err.Error(), "duplicate active Cobra command path") {
		t.Fatalf("expected duplicate-path error, got %v", err)
	}
}

func TestPluginSkillRendererRejectsMissingFlag(t *testing.T) {
	root := &cobra.Command{Use: "agm"}
	root.AddCommand(&cobra.Command{Use: "list", Run: func(*cobra.Command, []string) {}})
	_, err := renderPluginSkills(root, []pluginCommandOwner{{
		Filename: "list.md",
		Generated: &generatedPluginSkill{
			Title:       "List",
			Description: "List",
		},
		Commands: []pluginCommandContract{{Path: "list", Flags: []string{"missing"}}},
	}})
	if err == nil || !strings.Contains(err.Error(), "has no --missing flag") {
		t.Fatalf("expected missing-flag error, got %v", err)
	}
}

func TestGeneratedPluginSkillsIncludeBinaryPathAndGovernedPermissions(t *testing.T) {
	files, err := renderPluginSkills(rootCmd, pluginCommandInventory)
	if err != nil {
		t.Fatalf("renderPluginSkills: %v", err)
	}
	for name, content := range files {
		text := string(content)
		if !strings.Contains(text, "Run `agm ") {
			t.Errorf("%s omitted agm binary from invocation", name)
		}
		if strings.Contains(text, ":*)") {
			t.Errorf("%s uses retired colon permission syntax", name)
		}
	}
	search := string(files["agm-search.md"])
	if !strings.Contains(search, "because its extension or credentials are unavailable") ||
		!strings.Contains(search, "run the documented fallback `agm session list --all --output json`") {
		t.Fatal("agm-search.md does not preserve its declared fallback after a credential or extension failure")
	}
}

func TestPluginHarnessGuidanceMatchesRegistry(t *testing.T) {
	commandsDir := filepath.Join("..", "..", "agm-plugin", "commands")
	for _, filename := range []string{"agm-assoc.md", "agm-new.md"} {
		content, err := os.ReadFile(filepath.Join(commandsDir, filename))
		if err != nil {
			t.Fatalf("read %s: %v", filename, err)
		}
		text := string(content)
		for _, harness := range agent.ActiveHarnesses() {
			if !strings.Contains(text, "`"+harness+"`") {
				t.Errorf("%s omits active harness %q", filename, harness)
			}
		}
		for _, harness := range agent.DeprecatedHarnesses() {
			if !strings.Contains(text, "`"+harness+"`") {
				t.Errorf("%s omits deprecated compatibility harness %q", filename, harness)
			}
			if !strings.Contains(text, "deprecated") {
				t.Errorf("%s names deprecated harness %q without a deprecation warning", filename, harness)
			}
		}
	}
}
