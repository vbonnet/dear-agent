package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

//go:embed bash-anti-patterns.yaml
var patternsYAML []byte

// PatternFile is the top-level YAML structure.
type PatternFile struct {
	Patterns []PatternDef `yaml:"patterns"`
}

// PatternDef is a single pattern entry from the YAML file.
type PatternDef struct {
	ID               string `yaml:"id"`
	Order            int    `yaml:"order"`
	RE2Regex         string `yaml:"re2_regex"`
	PatternName      string `yaml:"pattern_name"`
	Remediation      string `yaml:"remediation"`
	Relaxed          bool   `yaml:"relaxed"`
	ConsolidatedInto string `yaml:"consolidated_into"`
}

// Rule is a compiled blocking rule.
type Rule struct {
	ID          string
	Order       int
	Regex       *regexp.Regexp
	PatternName string
	Remediation string
	IsExempt    func(cmd string, match string) bool
}

// BashBlocker checks bash commands for dangerous patterns and blocks them.
type BashBlocker struct {
	toolName  string
	toolInput string
	debug     bool
	rules     []Rule
}

// braceExpansionRegex matches shell brace expansion patterns like {a,b} or {1..10}.
var braceExpansionRegex = regexp.MustCompile(`\{[^{}]*,[^{}]*\}`)

// goPathContext matches brace expansion in Go-style directory paths.
var goPathContext = regexp.MustCompile(`/\{[^{}]*,[^{}]*\}(/|\.|\s|$)`)

// isDangerousBraceExpansion returns true if the brace expansion looks dangerous.
func isDangerousBraceExpansion(_ string, match string) bool {
	inner := match[1 : len(match)-1]
	parts := strings.Split(inner, ",")
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if strings.HasPrefix(trimmed, "-") {
			return true
		}
		for _, dangerous := range []string{"rm", "chmod", "chown", "dd", "mkfs", "kill", "shutdown", "reboot"} {
			if trimmed == dangerous {
				return true
			}
		}
	}
	return false
}

// isExemptBraceExpansion checks if a brace expansion match is a safe Go path pattern.
func isExemptBraceExpansion(cmd string, match string) bool {
	idx := strings.Index(cmd, match)
	if idx < 0 {
		return false
	}
	if idx > 0 && cmd[idx-1] == '/' {
		return !isDangerousBraceExpansion(cmd, match)
	}
	if goPathContext.MatchString(cmd) {
		return !isDangerousBraceExpansion(cmd, match)
	}
	return false
}

// loadRules parses the embedded YAML and compiles active rules.
func loadRules() ([]Rule, error) {
	var pf PatternFile
	if err := yaml.Unmarshal(patternsYAML, &pf); err != nil {
		return nil, fmt.Errorf("failed to parse patterns YAML: %w", err)
	}

	var rules []Rule
	for _, p := range pf.Patterns {
		if p.RE2Regex == "" {
			continue
		}
		if p.Relaxed {
			continue
		}
		if p.ConsolidatedInto != "" {
			continue
		}

		re, err := regexp.Compile(p.RE2Regex)
		if err != nil {
			return nil, fmt.Errorf("failed to compile regex for pattern %s: %w", p.ID, err)
		}

		rule := Rule{
			ID:          p.ID,
			Order:       p.Order,
			Regex:       re,
			PatternName: p.PatternName,
			Remediation: p.Remediation,
		}
		rules = append(rules, rule)
	}

	if len(rules) == 0 {
		return nil, fmt.Errorf("no active rules loaded from patterns YAML")
	}

	sort.Slice(rules, func(i, j int) bool {
		return rules[i].Order < rules[j].Order
	})

	// Prepend the brace-expansion rule with custom exemption logic.
	braceRule := Rule{
		ID:          "brace-expansion",
		Order:       -1,
		Regex:       braceExpansionRegex,
		PatternName: "brace expansion ({a,b})",
		Remediation: "Avoid shell brace expansion",
		IsExempt:    isExemptBraceExpansion,
	}
	rules = append([]Rule{braceRule}, rules...)

	return rules, nil
}

// NewBashBlocker creates a new blocker from environment variables.
func NewBashBlocker() (*BashBlocker, error) {
	rules, err := loadRules()
	if err != nil {
		return nil, err
	}
	return &BashBlocker{
		toolName:  os.Getenv("CLAUDE_TOOL_NAME"),
		toolInput: os.Getenv("CLAUDE_TOOL_INPUT"),
		debug:     os.Getenv("AGM_HOOK_DEBUG") == "1",
		rules:     rules,
	}, nil
}

func (b *BashBlocker) log(message string) {
	if b.debug {
		fmt.Fprintf(os.Stderr, "[BashBlocker] INFO: %s\n", message)
	}
}

// extractCommand extracts the bash command string from tool input.
// Claude Code sends JSON: {"command": "..."} or plain text.
func extractCommand(toolInput string) string {
	toolInput = strings.TrimSpace(toolInput)
	if len(toolInput) == 0 {
		return ""
	}
	if toolInput[0] == '{' {
		var obj map[string]interface{}
		if err := json.Unmarshal([]byte(toolInput), &obj); err == nil {
			if cmd, ok := obj["command"]; ok {
				if s, ok := cmd.(string); ok {
					return s
				}
			}
		}
	}
	return toolInput
}

// CheckCommand validates a command against all rules.
func (b *BashBlocker) CheckCommand(cmd string) *Rule {
	for i := range b.rules {
		rule := &b.rules[i]
		match := rule.Regex.FindString(cmd)
		if match == "" {
			continue
		}
		b.log(fmt.Sprintf("Rule %s matched: %q", rule.ID, match))

		if rule.IsExempt != nil && rule.IsExempt(cmd, match) {
			b.log(fmt.Sprintf("Rule %s exempted for match: %q", rule.ID, match))
			continue
		}

		return rule
	}
	return nil
}

// Run executes the main hook logic.
//
// Returns:
//   - 0: allow execution
//   - 2: block execution (Claude Code hook protocol)
func (b *BashBlocker) Run() int {
	b.log(fmt.Sprintf("Hook started for tool: %s", b.toolName))

	if b.toolName != "Bash" {
		b.log(fmt.Sprintf("Skipping non-Bash tool: %s", b.toolName))
		return 0
	}

	cmd := extractCommand(b.toolInput)
	if cmd == "" {
		b.log("Empty command — allowing")
		return 0
	}

	rule := b.CheckCommand(cmd)
	if rule == nil {
		b.log("No violations found — command allowed")
		return 0
	}

	b.log(fmt.Sprintf("BLOCKING: rule %s — %s", rule.ID, rule.PatternName))

	msg := fmt.Sprintf("Blocked by %s: %s", rule.ID, rule.PatternName)
	if rule.Remediation != "" {
		msg += "\n" + rule.Remediation
	}
	fmt.Fprintf(os.Stderr, "%s\n", msg)
	return 2
}

func main() {
	os.Exit(run())
}

func run() (exitCode int) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(os.Stderr, "[BashBlocker] FATAL: hook panic (fail-closed): %v\n", r)
			exitCode = 2
		}
	}()

	blocker, err := NewBashBlocker()
	if err != nil {
		fmt.Fprintf(os.Stderr, "[BashBlocker] FATAL: failed to load rules (fail-closed): %v\n", err)
		return 2
	}

	return blocker.Run()
}
