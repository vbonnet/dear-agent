package main

//go:generate go test . -run ^TestGeneratedPluginSkillsUpToDate$ -update-plugin-skills

import (
	"fmt"
	"slices"
	"strings"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/pluginhash"
)

type pluginCommandContract struct {
	Path  string
	Flags []string
}

type generatedPluginSkill struct {
	Title          string
	Description    string
	ArgumentFlags  []string
	FixedArgs      []string
	OutputColumns  []string
	ExtensionNote  string
	Fallback       *pluginCommandContract
	FallbackReason string
}

type pluginCommandOwner struct {
	Filename  string
	Generated *generatedPluginSkill
	Commands  []pluginCommandContract
}

var pluginCommandInventory = []pluginCommandOwner{
	{
		Filename: "agm-list.md",
		Generated: &generatedPluginSkill{
			Title:         "List AGM sessions",
			Description:   "List AGM sessions. Use when the user needs current or archived session names, states, harnesses, workspaces, tags, or trust information.",
			ArgumentFlags: []string{"all", "tag", "filter", "trust"},
			FixedArgs:     []string{"--output", "json"},
			OutputColumns: []string{"Name", "Status", "Harness", "Workspace", "Updated"},
		},
		Commands: []pluginCommandContract{{Path: "session list", Flags: []string{"all", "tag", "filter", "trust", "output"}}},
	},
	{
		Filename: "agm-search.md",
		Generated: &generatedPluginSkill{
			Title:         "Search archived AGM sessions",
			Description:   "Search archived Claude conversation history semantically. Use for a remembered topic when the Claude and Vertex AI extension is available; otherwise use the harness-neutral list fallback.",
			ArgumentFlags: []string{"max-results"},
			ExtensionNote: "`agm session search` is a Claude-history and Vertex AI extension. It may prompt before restoring a result.",
			Fallback: &pluginCommandContract{
				Path:  "session list",
				Flags: []string{"all", "output"},
			},
			FallbackReason: "For other harnesses, missing Vertex credentials, or a non-interactive lookup, run the fallback and filter session names, tags, and projects in memory.",
		},
		Commands: []pluginCommandContract{
			{Path: "session search", Flags: []string{"max-results"}},
			{Path: "session list", Flags: []string{"all", "output"}},
		},
	},
	{
		Filename: "agm-status.md",
		Generated: &generatedPluginSkill{
			Title:         "Show aggregate AGM status",
			Description:   "Show aggregate live status for AGM sessions. Use when the user needs session state, branch, worktree, workspace, or uncommitted-change information.",
			ArgumentFlags: []string{"workspace"},
			FixedArgs:     []string{"--format", "json"},
			OutputColumns: []string{"sessions[].name", "sessions[].state", "sessions[].branch", "sessions[].workspace", "sessions[].worktree_path", "sessions[].uncommitted"},
		},
		Commands: []pluginCommandContract{{Path: "session status", Flags: []string{"workspace", "format"}}},
	},
	{Filename: "agm-assoc.md", Commands: []pluginCommandContract{
		{Path: "get-session-name"},
		{Path: "session associate", Flags: []string{"create", "harness", "output"}},
	}},
	{Filename: "agm-exit.md", Commands: []pluginCommandContract{
		{Path: "get-session-name"},
		{Path: "session get", Flags: []string{"output"}},
		{Path: "session archive", Flags: []string{"async", "cleanup-worktrees"}},
	}},
	{Filename: "agm-new.md", Commands: []pluginCommandContract{{Path: "session new", Flags: []string{"harness", "workspace", "output"}}}},
	{Filename: "agm-resume.md", Commands: []pluginCommandContract{
		{Path: "session resume", Flags: []string{"delete-prompt-file", "detached", "prompt-file", "output"}},
		{Path: "session list", Flags: []string{"all", "output"}},
	}},
	{Filename: "agm-send.md", Commands: []pluginCommandContract{
		{Path: "send msg", Flags: []string{"prompt-file", "priority", "sender", "output"}},
		{Path: "session list", Flags: []string{"output"}},
	}},
	{Filename: "audit-completion.md", Commands: []pluginCommandContract{
		{Path: "session get", Flags: []string{"output"}},
		{Path: "acceptance show", Flags: []string{"directory", "output"}},
	}},
	{Filename: "wiki-ingest.md", Commands: []pluginCommandContract{{Path: "wiki ingest", Flags: []string{"page", "no-index", "kb"}}}},
	{Filename: "wiki-lint.md", Commands: []pluginCommandContract{{Path: "wiki lint", Flags: []string{"json", "no-append", "kb"}}}},
	{Filename: "wiki-query-save.md", Commands: []pluginCommandContract{{Path: "wiki query-save", Flags: []string{"query-file", "answer-file", "category", "output"}}}},
}

func renderPluginSkills(root *cobra.Command, inventory []pluginCommandOwner) (map[string][]byte, error) {
	if err := validateUniqueCommandPaths(root); err != nil {
		return nil, err
	}
	if err := validatePluginInventory(root, inventory); err != nil {
		return nil, err
	}

	files := make(map[string][]byte)
	for _, owner := range inventory {
		if owner.Generated == nil {
			continue
		}
		cmd, err := resolveCommand(root, owner.Commands[0].Path)
		if err != nil {
			return nil, err
		}
		content, err := renderPluginSkill(owner, cmd)
		if err != nil {
			return nil, fmt.Errorf("render %s: %w", owner.Filename, err)
		}
		files[owner.Filename] = content
	}
	return files, nil
}

func validatePluginInventory(root *cobra.Command, inventory []pluginCommandOwner) error {
	seen := make(map[string]bool)
	for _, owner := range inventory {
		if owner.Filename == "" || seen[owner.Filename] {
			return fmt.Errorf("duplicate or empty plugin filename %q", owner.Filename)
		}
		seen[owner.Filename] = true
		if len(owner.Commands) == 0 {
			return fmt.Errorf("plugin command %s has no Cobra contract", owner.Filename)
		}
		for _, contract := range owner.Commands {
			cmd, err := resolveCommand(root, contract.Path)
			if err != nil {
				return fmt.Errorf("plugin command %s: %w", owner.Filename, err)
			}
			for _, flagName := range contract.Flags {
				if _, ok := lookupCommandFlagType(cmd, flagName); !ok {
					return fmt.Errorf("plugin command %s: agm %s has no --%s flag", owner.Filename, contract.Path, flagName)
				}
			}
		}
	}
	return nil
}

func validateUniqueCommandPaths(root *cobra.Command) error {
	var walk func(*cobra.Command) error
	walk = func(parent *cobra.Command) error {
		seen := make(map[string]bool)
		for _, child := range parent.Commands() {
			if !child.IsAvailableCommand() || child.Hidden {
				continue
			}
			if seen[child.Name()] {
				return fmt.Errorf("duplicate active Cobra command path: %s %s", parent.CommandPath(), child.Name())
			}
			seen[child.Name()] = true
			if err := walk(child); err != nil {
				return err
			}
		}
		return nil
	}
	return walk(root)
}

func resolveCommand(root *cobra.Command, path string) (*cobra.Command, error) {
	current := root
	for segment := range strings.FieldsSeq(path) {
		var matches []*cobra.Command
		for _, child := range current.Commands() {
			if child.IsAvailableCommand() && !child.Hidden && child.Name() == segment {
				matches = append(matches, child)
			}
		}
		if len(matches) != 1 {
			return nil, fmt.Errorf("resolve agm %s: segment %q has %d active matches", path, segment, len(matches))
		}
		current = matches[0]
	}
	return current, nil
}

func lookupCommandFlagType(cmd *cobra.Command, name string) (string, bool) {
	if flag := cmd.Flags().Lookup(name); flag != nil {
		return flag.Value.Type(), true
	}
	if flag := cmd.InheritedFlags().Lookup(name); flag != nil {
		return flag.Value.Type(), true
	}
	return "", false
}

func renderPluginSkill(owner pluginCommandOwner, cmd *cobra.Command) ([]byte, error) {
	spec := owner.Generated
	argumentHint, err := buildPluginArgumentHint(cmd, spec.ArgumentFlags)
	if err != nil {
		return nil, err
	}
	path := owner.Commands[0].Path
	primaryArgs := []string{"agm", path}
	useParts := strings.Fields(cmd.Use)
	if len(useParts) > 1 {
		primaryArgs = append(primaryArgs, useParts[1:]...)
	}
	primaryArgs = append(primaryArgs, spec.FixedArgs...)
	primary := strings.Join(primaryArgs, " ")

	allowed := []string{"Bash(agm " + path + " *)"}
	var fallback string
	if spec.Fallback != nil {
		allowed = append(allowed, "Bash(agm "+spec.Fallback.Path+" *)")
		fallback = "agm " + spec.Fallback.Path
		if hasString(spec.Fallback.Flags, "all") {
			fallback += " --all"
		}
		if hasString(spec.Fallback.Flags, "output") {
			fallback += " --output json"
		}
	}

	var body strings.Builder
	fmt.Fprintln(&body, "<!-- Code generated from registered Cobra metadata. DO NOT EDIT. -->")
	fmt.Fprintf(&body, "# %s\n\n", spec.Title)
	fmt.Fprintln(&body, "## Run")
	fmt.Fprintln(&body)
	fmt.Fprintln(&body, "- Treat user-provided values as separate argv values. Never build shell syntax with concatenation, command substitution, or unquoted interpolation.")
	fmt.Fprintf(&body, "- Run `%s`.\n", primary)
	if len(spec.ArgumentFlags) > 0 {
		var optional []string
		for _, flagName := range spec.ArgumentFlags {
			optional = append(optional, "`--"+flagName+"`")
		}
		fmt.Fprintf(&body, "- Forward only requested optional flags: %s.\n", strings.Join(optional, ", "))
	}
	if spec.ExtensionNote != "" {
		fmt.Fprintf(&body, "- %s\n", spec.ExtensionNote)
	}
	if fallback != "" {
		fmt.Fprintf(&body, "- %s Run `%s`.\n", spec.FallbackReason, fallback)
	}
	fmt.Fprintln(&body)
	fmt.Fprintln(&body, "## Report")
	fmt.Fprintln(&body)
	if fallback != "" {
		fmt.Fprintf(&body, "- If the primary command exits non-zero because its extension or credentials are unavailable, show its stderr and run the documented fallback `%s`. For any other non-zero exit, show stderr and stop. Do not invent another fallback command.\n", fallback)
	} else {
		fmt.Fprintln(&body, "- If AGM exits non-zero, show its stderr and stop. Do not invent a fallback command.")
	}
	if len(spec.OutputColumns) > 0 {
		fmt.Fprintf(&body, "- Present successful structured output with these useful fields when available: %s.\n", strings.Join(spec.OutputColumns, ", "))
	} else {
		fmt.Fprintln(&body, "- Present AGM's result and any confirmation request without changing its meaning.")
	}
	fmt.Fprintln(&body, "- If no sessions match, say so without treating the empty result as an error.")

	bodyText := body.String()
	hash := pluginhash.SumBody([]byte("\n" + strings.TrimRight(bodyText, "\n")))
	frontmatter := fmt.Sprintf(`---
model: haiku
effort: low
content-hash: %s
description: >-
  %s
argument-hint: %q
allowed-tools: %s
---

`, hash, spec.Description, argumentHint, strings.Join(allowed, ", "))
	return []byte(frontmatter + bodyText), nil
}

func buildPluginArgumentHint(cmd *cobra.Command, flagNames []string) (string, error) {
	parts := strings.Fields(cmd.Use)
	hints := append([]string(nil), parts[1:]...)
	for _, name := range flagNames {
		flagType, ok := lookupCommandFlagType(cmd, name)
		if !ok {
			return "", fmt.Errorf("agm %s has no --%s flag", cmd.CommandPath(), name)
		}
		hint := "--" + name
		switch flagType {
		case "bool":
		case "int", "int64", "float64", "duration":
			hint += " N"
		default:
			hint += " VALUE"
		}
		hints = append(hints, "["+hint+"]")
	}
	return strings.Join(hints, " "), nil
}

func hasString(values []string, target string) bool {
	return slices.Contains(values, target)
}
