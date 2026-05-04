// Command workflow-run executes a YAML workflow file. This is the minimal
// harness that ties pkg/workflow's Runner to pkg/llm/provider's AI
// executor; richer deployment (supervisor integration, channel gates)
// builds on this surface.
//
// Usage:
//
//	workflow-run -file workflows/research-pipeline.yaml -input topics_file=in.jsonl -input output_file=out.jsonl
//
// Repeatable -input flag sets workflow inputs. Use -dry-run to validate
// without executing.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	llmprovider "github.com/vbonnet/dear-agent/pkg/llm/provider"
	"github.com/vbonnet/dear-agent/pkg/llm/router"
	"github.com/vbonnet/dear-agent/pkg/workflow"
)

// multiString is a flag.Value that accumulates repeated -input flags.
type multiString []string

func (m *multiString) String() string     { return strings.Join(*m, ",") }
func (m *multiString) Set(v string) error { *m = append(*m, v); return nil }

func main() {
	var (
		file    = flag.String("file", "", "path to workflow YAML (required)")
		dryRun  = flag.Bool("dry-run", false, "validate the workflow and exit without executing")
		verbose = flag.Bool("verbose", false, "debug logging")
		cwd     = flag.String("cwd", "", "default working directory for bash nodes")
		roles   = flag.String("roles", "", "path to roles.yaml for the model router (defaults to config/roles.yaml if present)")
		inputs  multiString
	)
	flag.Var(&inputs, "input", "workflow input as name=value (repeatable)")
	flag.Parse()

	if *file == "" {
		flag.Usage()
		fmt.Fprintln(os.Stderr, "\n-file is required")
		os.Exit(2)
	}

	level := slog.LevelInfo
	if *verbose {
		level = slog.LevelDebug
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))

	w, err := workflow.LoadFile(*file)
	if err != nil {
		logger.Error("load workflow", "err", err)
		os.Exit(1)
	}
	logger.Info("workflow loaded", "name", w.Name, "version", w.Version, "nodes", len(w.Nodes))

	if *dryRun {
		logger.Info("dry-run: validation passed; exiting")
		return
	}

	inputMap := make(map[string]string, len(inputs))
	for _, kv := range inputs {
		idx := strings.IndexByte(kv, '=')
		if idx <= 0 {
			logger.Error("bad -input; expect name=value", "value", kv)
			os.Exit(2)
		}
		inputMap[kv[:idx]] = kv[idx+1:]
	}

	// Wire the AI executor. A nil executor lets workflows that only use
	// bash/gate/loop still run (useful for CI scripts that want the DAG
	// primitives without an LLM).
	var ai workflow.AIExecutor
	if hasAINode(w.Nodes) {
		exec, err := buildExecutor(*roles, logger)
		if err != nil {
			logger.Error("init AI executor", "err", err)
			os.Exit(1)
		}
		ai = exec
	} else {
		ai = nopAI{}
	}

	runner := workflow.NewRunner(ai)
	runner.Logger = logger
	runner.DefaultWorkingDir = *cwd

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	rep, err := runner.Run(ctx, w, inputMap)
	if err != nil {
		logger.Error("run failed", "err", err)
	}
	logger.Info("run finished",
		"workflow", w.Name,
		"succeeded", rep != nil && rep.Succeeded,
		"nodes_run", func() int {
			if rep == nil {
				return 0
			}
			return len(rep.Results)
		}())

	if err != nil {
		cancel() // explicit since deferred cancel won't run after os.Exit
		os.Exit(1) //nolint:gocritic // cancel() called explicitly above
	}
}

// hasAINode returns true if any node in the (possibly-nested) DAG is an
// AI node. Used to skip LLM provider init when the workflow doesn't need it.
func hasAINode(nodes []workflow.Node) bool {
	for i := range nodes {
		n := &nodes[i]
		if n.Kind == workflow.KindAI {
			return true
		}
		if n.Kind == workflow.KindLoop && n.Loop != nil {
			if hasAINode(n.Loop.Nodes) {
				return true
			}
		}
	}
	return false
}

// buildExecutor returns a workflow.AIExecutor wired to the role-based
// model router when a roles config is available, falling back to the
// historical direct-Anthropic path when no config is found.
//
// Resolution of the roles config path:
//  1. The explicit -roles flag (if non-empty).
//  2. The repo-relative default config/roles.yaml (if it exists in the
//     current working directory).
//  3. Direct-Anthropic fallback so existing single-provider setups keep
//     working without forcing every operator to ship a roles.yaml.
func buildExecutor(rolesPath string, logger *slog.Logger) (workflow.AIExecutor, error) {
	if rolesPath == "" {
		const defaultPath = "config/roles.yaml"
		if _, err := os.Stat(defaultPath); err == nil {
			rolesPath = defaultPath
		}
	}

	if rolesPath != "" {
		cfg, err := router.LoadConfig(rolesPath)
		if err != nil {
			return nil, fmt.Errorf("load roles config: %w", err)
		}
		r, err := router.New(router.Options{Config: cfg})
		if err != nil {
			return nil, fmt.Errorf("init router: %w", err)
		}
		logger.Info("AI executor: router-backed", "roles", rolesPath, "default_role", r.DefaultRole())
		return router.NewAIExecutor(r), nil
	}

	logger.Info("AI executor: direct Anthropic (no roles.yaml found)")
	prov, err := llmprovider.NewAnthropicProvider(llmprovider.AnthropicConfig{})
	if err != nil {
		return nil, fmt.Errorf("init anthropic provider: %w", err)
	}
	return &providerAI{inner: prov}, nil
}

// providerAI adapts pkg/llm/provider to workflow.AIExecutor.
type providerAI struct {
	inner *llmprovider.AnthropicProvider
}

func (p *providerAI) Generate(
	ctx context.Context,
	node *workflow.AINode,
	_ map[string]string,
	_ map[string]string,
) (string, error) {
	req := &llmprovider.GenerateRequest{
		Prompt:       node.Prompt,
		Model:        node.Model,
		SystemPrompt: node.System,
		MaxTokens:    node.MaxTokens,
	}
	resp, err := p.inner.Generate(ctx, req)
	if err != nil {
		return "", err
	}
	return resp.Text, nil
}

// nopAI panics if called — the CLI only installs it when there are no
// AI nodes, so invocation would be a bug.
type nopAI struct{}

func (nopAI) Generate(context.Context, *workflow.AINode, map[string]string, map[string]string) (string, error) {
	return "", fmt.Errorf("workflow-run: AI node encountered but no provider configured")
}
