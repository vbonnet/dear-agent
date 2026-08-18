// Package commandparity defines the executable harness contract for every AGM
// Cobra command that directly controls or inspects tmux.
package commandparity

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"

	"github.com/vbonnet/dear-agent/agm/internal/agent"
)

// Strategy describes how a command provides its observable behavior.
type Strategy string

const (
	// StrategyNeutral uses the same tmux operation for every harness.
	StrategyNeutral Strategy = "harness-neutral-tmux"
	// StrategyNative uses a harness-specific native control.
	StrategyNative Strategy = "harness-native"
	// StrategyPromptBridge adapts a detected prompt through neutral tmux keys.
	StrategyPromptBridge Strategy = "prompt-detected-tmux-bridge"
	// StrategyRestartFallback reports the required startup configuration.
	StrategyRestartFallback Strategy = "restart-with-mode-fallback"
	// StrategyBestEffort preserves input with an explicitly unverified mapping.
	StrategyBestEffort Strategy = "best-effort-preservation-fallback"
)

// Contract records the harness and model policy for one tmux-facing command.
type Contract struct {
	Source           string
	Command          string
	ModelIndependent bool
	Strategies       map[string]Strategy
}

// Contracts returns the complete command parity registry.
func Contracts() []Contract {
	neutral := func(source, command string) Contract {
		return Contract{Source: source, Command: command, ModelIndependent: true, Strategies: all(StrategyNeutral)}
	}
	native := func(source, command string) Contract {
		return Contract{Source: source, Command: command, ModelIndependent: true, Strategies: all(StrategyNative)}
	}
	bridge := func(source, command string) Contract {
		return Contract{Source: source, Command: command, ModelIndependent: true, Strategies: all(StrategyPromptBridge)}
	}

	return []Contract{
		neutral("admin_reconcile.go", "admin reconcile"),
		neutral("admin_reconcile_codex.go", "admin reconcile-codex"),
		neutral("admin_watchdog.go", "admin install-watchdog/uninstall-watchdog"),
		neutral("associate.go", "session associate"),
		neutral("boot_check.go", "boot-check"),
		neutral("capture.go", "capture"),
		native("create_child.go", "create-child"),
		neutral("doctor.go", "admin doctor"),
		bridge("escape_ui.go", "send escape-ui"),
		neutral("get_history_path.go", "admin get-history-path"),
		neutral("get_uuid.go", "admin get-uuid"),
		neutral("install_tmux_status.go", "admin install-tmux-status"),
		neutral("kill.go", "session kill"),
		neutral("main.go", "agm"),
		native("new.go", "session new"),
		neutral("recover.go", "session recover"),
		native("resume.go", "session resume"),
		neutral("safety_check.go", "safety check"),
		bridge("select_option.go", "send select-option"),
		bridge("send_approve.go", "send approve"),
		neutral("send_clear.go", "send clear"),
		neutral("send_clear_input.go", "send clear-input"),
		bridge("send_compact.go", "send compact"),
		neutral("send_enter.go", "send enter"),
		modeContract(),
		neutral("send_msg.go", "send msg"),
		bridge("send_reject.go", "send reject"),
		modelContract(),
		stashContract("send_stash.go", "send stash"),
		neutral("send_verify.go", "send verify"),
		neutral("send_wake_loop.go", "send wake-loop"),
		neutral("send_work_request.go", "send work-request"),
		bridge("session_compact.go", "session compact"),
		neutral("session_rename.go", "session rename"),
		stashContract("session_unstick.go", "session unstick"),
		neutral("status_line.go", "status-line"),
		neutral("supervisor.go", "supervisor"),
		neutral("sync.go", "sync"),
		neutral("test_capture.go", "test capture"),
		neutral("test_cleanup.go", "test cleanup"),
	}
}

func modeContract() Contract {
	return Contract{
		Source: "send_mode.go", Command: "send mode", ModelIndependent: true,
		Strategies: map[string]Strategy{
			"claude-code":  StrategyNative,
			"codex-cli":    StrategyRestartFallback,
			"agy":          StrategyRestartFallback,
			"opencode-cli": StrategyNative,
			"pi-cli":       StrategyNative,
		},
	}
}

func modelContract() Contract {
	return Contract{
		Source: "send_set_model.go", Command: "send set-model", ModelIndependent: false,
		Strategies: all(StrategyNative),
	}
}

func stashContract(source, command string) Contract {
	return Contract{
		Source: source, Command: command, ModelIndependent: true,
		Strategies: map[string]Strategy{
			"claude-code":  StrategyNative,
			"codex-cli":    StrategyBestEffort,
			"agy":          StrategyBestEffort,
			"opencode-cli": StrategyBestEffort,
			"pi-cli":       StrategyBestEffort,
		},
	}
}

func all(strategy Strategy) map[string]Strategy {
	out := make(map[string]Strategy, len(agent.ActiveHarnesses()))
	for _, harness := range agent.ActiveHarnesses() {
		out[harness] = strategy
	}
	return out
}

// ValidateContracts verifies that every active harness has an explicit,
// non-empty strategy for every tmux-facing command.
func ValidateContracts() error {
	seenSource := make(map[string]struct{})
	seenCommand := make(map[string]struct{})
	for _, contract := range Contracts() {
		if contract.Source == "" || contract.Command == "" {
			return fmt.Errorf("command parity contract has an empty source or command: %+v", contract)
		}
		if _, ok := seenSource[contract.Source]; ok {
			return fmt.Errorf("duplicate command parity source %q", contract.Source)
		}
		if _, ok := seenCommand[contract.Command]; ok {
			return fmt.Errorf("duplicate command parity command %q", contract.Command)
		}
		seenSource[contract.Source] = struct{}{}
		seenCommand[contract.Command] = struct{}{}
		for _, harness := range agent.ActiveHarnesses() {
			if contract.Strategies[harness] == "" {
				return fmt.Errorf("command %q has no strategy for active harness %q", contract.Command, harness)
			}
		}
	}
	return nil
}

// ValidateSourceCoverage parses AGM command sources and requires every Cobra
// command file that imports the canonical tmux package to appear in Contracts.
func ValidateSourceCoverage(repoRoot string) error {
	dir := filepath.Join(repoRoot, "agm", "cmd", "agm")
	want := make(map[string]struct{})
	for _, contract := range Contracts() {
		want[contract.Source] = struct{}{}
	}

	var uncovered []string
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path != dir {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		covered, err := tmuxCobraCommandSource(path)
		if err != nil {
			return err
		}
		if covered {
			name := filepath.Base(path)
			if _, ok := want[name]; !ok {
				uncovered = append(uncovered, name)
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("scan AGM command parity sources: %w", err)
	}
	if len(uncovered) > 0 {
		slices.Sort(uncovered)
		return fmt.Errorf("tmux-facing Cobra command sources lack parity contracts: %s", strings.Join(uncovered, ", "))
	}
	return nil
}

func tmuxCobraCommandSource(path string) (bool, error) {
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
	if err != nil {
		return false, err
	}
	importsTmux := false
	for _, imported := range file.Imports {
		if strings.Trim(imported.Path.Value, `"`) == "github.com/vbonnet/dear-agent/agm/internal/tmux" {
			importsTmux = true
			break
		}
	}
	if !importsTmux {
		return false, nil
	}

	full, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		return false, err
	}
	hasCobraCommand := false
	ast.Inspect(full, func(node ast.Node) bool {
		selector, ok := node.(*ast.SelectorExpr)
		if !ok || selector.Sel.Name != "Command" {
			return true
		}
		ident, ok := selector.X.(*ast.Ident)
		if ok && ident.Name == "cobra" {
			hasCobraCommand = true
		}
		return true
	})
	return hasCobraCommand, nil
}
