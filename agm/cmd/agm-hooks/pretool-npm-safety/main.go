package main

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
)

// Rule defines a blocking rule for npm/node commands.
type Rule struct {
	ID          string
	Regex       *regexp.Regexp
	Description string
	Remediation string
	IsExempt    func(cmd string) bool
}

// NpmSafetyGuard checks bash commands for dangerous npm/node patterns.
type NpmSafetyGuard struct {
	toolName  string
	toolInput string
	debug     bool
	rules     []Rule
}

// knownSafeNpxPackages lists npx packages considered safe to run.
var knownSafeNpxPackages = map[string]bool{
	"tsc":              true,
	"typescript":       true,
	"ts-node":          true,
	"tsx":              true,
	"eslint":           true,
	"prettier":         true,
	"jest":             true,
	"vitest":           true,
	"mocha":            true,
	"playwright":       true,
	"create-react-app": true,
	"create-next-app":  true,
	"create-vite":      true,
	"@anthropic-ai/sdk": true,
	"serve":            true,
	"http-server":      true,
	"nodemon":          true,
	"concurrently":     true,
	"rimraf":           true,
	"mkdirp":           true,
	"license-checker":  true,
	"depcheck":         true,
	"madge":            true,
	"npm-check":        true,
	"npm-check-updates": true,
	"ncu":              true,
	"semver":           true,
}

// npxPackageRegex extracts the package name from an npx command.
// Handles: npx <pkg>, npx -y <pkg>, npx --yes <pkg>, npx -p <pkg> <cmd>
var npxPackageRegex = regexp.MustCompile(`\bnpx\s+(?:--yes\s+|-y\s+|-p\s+)?(@?[a-zA-Z0-9][\w./-]*)`)

// isKnownNpxPackage checks if an npx command uses a known-safe package.
func isKnownNpxPackage(cmd string) bool {
	match := npxPackageRegex.FindStringSubmatch(cmd)
	if len(match) < 2 {
		return false
	}
	pkg := match[1]
	// Strip version suffix (e.g., typescript@5.0.0 -> typescript)
	if idx := strings.LastIndex(pkg, "@"); idx > 0 {
		pkg = pkg[:idx]
	}
	return knownSafeNpxPackages[pkg]
}

func buildRules() []Rule {
	return []Rule{
		{
			ID:          "npm-publish",
			Regex:       regexp.MustCompile(`\bnpm\s+publish\b`),
			Description: "npm publish (publishes package to registry)",
			Remediation: "Do not publish packages from automated sessions.\nIf intentional, run manually outside Claude Code.",
		},
		{
			ID:          "npm-config-set",
			Regex:       regexp.MustCompile(`\bnpm\s+config\s+set\b`),
			Description: "npm config set (modifies global npm configuration)",
			Remediation: "Do not modify npm configuration from automated sessions.\nUse project-level .npmrc instead.",
		},
		{
			ID:          "npm-token",
			Regex:       regexp.MustCompile(`\bnpm\s+token\b`),
			Description: "npm token (manages authentication tokens)",
			Remediation: "Do not manage npm tokens from automated sessions.",
		},
		{
			ID:          "npm-owner",
			Regex:       regexp.MustCompile(`\bnpm\s+owner\b`),
			Description: "npm owner (manages package ownership)",
			Remediation: "Do not modify package ownership from automated sessions.",
		},
		{
			ID:          "npm-deprecate",
			Regex:       regexp.MustCompile(`\bnpm\s+deprecate\b`),
			Description: "npm deprecate (deprecates a package version)",
			Remediation: "Do not deprecate packages from automated sessions.",
		},
		{
			ID:          "npm-unpublish",
			Regex:       regexp.MustCompile(`\bnpm\s+unpublish\b`),
			Description: "npm unpublish (removes package from registry)",
			Remediation: "Do not unpublish packages from automated sessions.",
		},
		{
			ID:          "npm-adduser",
			Regex:       regexp.MustCompile(`\bnpm\s+(adduser|login)\b`),
			Description: "npm adduser/login (authenticates to registry)",
			Remediation: "Do not authenticate to npm from automated sessions.",
		},
		{
			ID:          "npm-access",
			Regex:       regexp.MustCompile(`\bnpm\s+access\b`),
			Description: "npm access (manages package access controls)",
			Remediation: "Do not modify package access from automated sessions.",
		},
		{
			ID:          "npx-unknown",
			Regex:       regexp.MustCompile(`\bnpx\s+`),
			Description: "npx with unknown package (arbitrary code execution risk)",
			Remediation: "Only use npx with known-safe packages.\nKnown safe: tsc, eslint, prettier, jest, vitest, playwright, etc.",
			IsExempt: isKnownNpxPackage,
		},
		{
			ID:          "node-inspect-brk",
			Regex:       regexp.MustCompile(`\bnode\s+--inspect-brk\b`),
			Description: "node --inspect-brk (opens debug port with break)",
			Remediation: "Do not open Node.js debug ports from automated sessions.",
		},
		{
			ID:          "node-inspect",
			Regex:       regexp.MustCompile(`\bnode\s+--inspect\b`),
			Description: "node --inspect (opens debug port, potential security risk)",
			Remediation: "Do not open Node.js debug ports from automated sessions.\nDebug ports allow arbitrary code execution from network connections.",
		},
		{
			ID:          "node-eval",
			Regex:       regexp.MustCompile(`\bnode\s+(-e|--eval)\b`),
			Description: "node -e/--eval (executes arbitrary JavaScript)",
			Remediation: "Write JavaScript to a file and run it instead of using eval.",
		},
	}
}

// NewNpmSafetyGuard creates a new guard from environment variables.
func NewNpmSafetyGuard() *NpmSafetyGuard {
	return &NpmSafetyGuard{
		toolName:  os.Getenv("CLAUDE_TOOL_NAME"),
		toolInput: os.Getenv("CLAUDE_TOOL_INPUT"),
		debug:     os.Getenv("AGM_HOOK_DEBUG") == "1",
		rules:     buildRules(),
	}
}

func (g *NpmSafetyGuard) log(message string) {
	if g.debug {
		fmt.Fprintf(os.Stderr, "[NpmSafety] INFO: %s\n", message)
	}
}

// extractCommand extracts the bash command string from tool input.
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
// Returns the first matching rule, or nil if the command is allowed.
func (g *NpmSafetyGuard) CheckCommand(cmd string) *Rule {
	for i := range g.rules {
		rule := &g.rules[i]
		if !rule.Regex.MatchString(cmd) {
			continue
		}
		g.log(fmt.Sprintf("Rule %s matched command", rule.ID))

		if rule.IsExempt != nil && rule.IsExempt(cmd) {
			g.log(fmt.Sprintf("Rule %s exempted", rule.ID))
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
func (g *NpmSafetyGuard) Run() int {
	g.log(fmt.Sprintf("Hook started for tool: %s", g.toolName))

	if g.toolName != "Bash" {
		g.log(fmt.Sprintf("Skipping non-Bash tool: %s", g.toolName))
		return 0
	}

	cmd := extractCommand(g.toolInput)
	if cmd == "" {
		g.log("Empty command — allowing")
		return 0
	}

	rule := g.CheckCommand(cmd)
	if rule == nil {
		g.log("No violations found — command allowed")
		return 0
	}

	g.log(fmt.Sprintf("BLOCKING: rule %s — %s", rule.ID, rule.Description))

	msg := fmt.Sprintf("Blocked by %s: %s", rule.ID, rule.Description)
	if rule.Remediation != "" {
		msg += "\n" + rule.Remediation
	}
	fmt.Fprintf(os.Stderr, "%s\n", msg)
	return 2
}

func main() {
	exitCode := run()
	os.Exit(exitCode)
}

func run() (code int) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(os.Stderr, "[NpmSafety] FATAL: hook panic (fail-closed): %v\n", r)
			code = 2
		}
	}()

	guard := NewNpmSafetyGuard()
	return guard.Run()
}
