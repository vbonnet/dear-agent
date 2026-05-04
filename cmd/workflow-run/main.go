// Command workflow-run executes a YAML workflow file. This is the minimal
// harness that ties pkg/workflow's Runner to pkg/llm/provider's AI
// executor; richer deployment (supervisor integration, channel gates)
// builds on this surface.
//
// Usage:
//
//	workflow-run -file workflows/research-pipeline.yaml -input topics_file=in.jsonl -input output_file=out.jsonl
//	workflow-run -file workflows/signals-collect.yaml -trigger cron -db ./runs.db
//
// Repeatable -input flag sets workflow inputs. Use -dry-run to validate
// without executing. -trigger labels how the run was started ("cli",
// "cron", "mcp", ...) and is recorded on the runs row when -db points
// at a SQLite file.
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
	os.Exit(run(os.Args[1:], os.Stderr))
}

// run is main() factored to be testable: it accepts argv-style flags
// and a stderr destination, and returns a process exit code instead of
// calling os.Exit. Tests in main_test.go use this entrypoint.
func run(args []string, stderr *os.File) int {
	fs := flag.NewFlagSet("workflow-run", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var (
		file    = fs.String("file", "", "path to workflow YAML (required)")
		dryRun  = fs.Bool("dry-run", false, "validate the workflow and exit without executing")
		verbose = fs.Bool("verbose", false, "debug logging")
		cwd     = fs.String("cwd", "", "default working directory for bash nodes")
		roles   = fs.String("roles", "", "path to roles.yaml for the model router (defaults to config/roles.yaml if present)")
		dbPath  = fs.String("db", "runs.db", "path to SQLite runs.db (created if missing); empty disables persistence")
		trigger = fs.String("trigger", "cli", `how this run was started ("cli", "cron", "mcp", ...) — recorded on the runs row`)
		inputs  multiString
	)
	fs.Var(&inputs, "input", "workflow input as name=value (repeatable)")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	if *file == "" {
		fs.Usage()
		fmt.Fprintln(stderr, "\n-file is required")
		return 2
	}

	level := slog.LevelInfo
	if *verbose {
		level = slog.LevelDebug
	}
	logger := slog.New(slog.NewTextHandler(stderr, &slog.HandlerOptions{Level: level}))

	w, err := workflow.LoadFile(*file)
	if err != nil {
		logger.Error("load workflow", "err", err)
		return 1
	}
	logger.Info("workflow loaded", "name", w.Name, "version", w.Version, "nodes", len(w.Nodes))

	if *dryRun {
		logger.Info("dry-run: validation passed; exiting")
		return 0
	}

	inputMap, err := parseInputs(inputs)
	if err != nil {
		logger.Error("bad -input; expect name=value", "err", err)
		return 2
	}

	ai, err := selectAIExecutor(w.Nodes, *roles, logger)
	if err != nil {
		logger.Error("init AI executor", "err", err)
		return 1
	}

	runner := workflow.NewRunner(ai)
	runner.Logger = logger
	runner.DefaultWorkingDir = *cwd
	runner.Trigger = *trigger

	if *dbPath != "" {
		ss, err := workflow.OpenSQLiteState(*dbPath)
		if err != nil {
			logger.Error("open runs.db", "path", *dbPath, "err", err)
			return 1
		}
		defer func() {
			if err := ss.Close(); err != nil {
				logger.Warn("close runs.db", "err", err)
			}
		}()
		runner.UseSQLiteState(ss)
		logger.Info("runs.db wired", "path", *dbPath)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	rep, runErr := runner.Run(ctx, w, inputMap)
	if runErr != nil {
		logger.Error("run failed", "err", runErr)
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

	if runErr != nil {
		return 1
	}
	return 0
}

// parseInputs decodes name=value flag values into a map. The leading
// idx<=0 check rejects both "=foo" (empty key) and "novalue" (no equals).
func parseInputs(inputs multiString) (map[string]string, error) {
	out := make(map[string]string, len(inputs))
	for _, kv := range inputs {
		idx := strings.IndexByte(kv, '=')
		if idx <= 0 {
			return nil, fmt.Errorf("bad -input %q; expect name=value", kv)
		}
		out[kv[:idx]] = kv[idx+1:]
	}
	return out, nil
}

// selectAIExecutor returns a real AIExecutor when the DAG contains an AI
// node and a no-op stub otherwise. Keeping the LLM-provider init lazy
// lets bash-only workflows run in CI without ANTHROPIC_API_KEY set.
func selectAIExecutor(nodes []workflow.Node, rolesPath string, logger *slog.Logger) (workflow.AIExecutor, error) {
	if !hasAINode(nodes) {
		return nopAI{}, nil
	}
	return buildExecutor(rolesPath, logger)
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
