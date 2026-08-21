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
	"os/exec"
	"os/signal"
	"strings"
	"syscall"

	"github.com/vbonnet/dear-agent/pkg/llm/auth"
	llmprovider "github.com/vbonnet/dear-agent/pkg/llm/provider"
	"github.com/vbonnet/dear-agent/pkg/llm/quota"
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
		file      = fs.String("file", "", "path to workflow YAML (required)")
		dryRun    = fs.Bool("dry-run", false, "validate the workflow and exit without executing")
		verbose   = fs.Bool("verbose", false, "debug logging")
		cwd       = fs.String("cwd", "", "default working directory for bash nodes")
		roles     = fs.String("roles", "", "path to roles.yaml for the model router (defaults to config/roles.yaml if present)")
		quotaMode = fs.String("quota", "auto", `quota-aware routing: "auto" (on when the codexbar meter is installed), "on", or "off"`)
		dbPath    = fs.String("db", "runs.db", "path to SQLite runs.db (created if missing); empty disables persistence")
		trigger   = fs.String("trigger", "cli", `how this run was started ("cli", "cron", "mcp", ...) — recorded on the runs row`)
		inputs    multiString
	)
	fs.Var(&inputs, "input", "workflow input as name=value (repeatable)")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	if *file == "" {
		fs.Usage()
		_, _ = fmt.Fprintln(stderr, "\n-file is required")
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

	ai, err := selectAIExecutor(w.Nodes, *roles, *quotaMode, logger)
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
func selectAIExecutor(nodes []workflow.Node, rolesPath, quotaMode string, logger *slog.Logger) (workflow.AIExecutor, error) {
	if !hasAINode(nodes) {
		return nopAI{}, nil
	}
	return buildExecutor(rolesPath, quotaMode, logger)
}

// buildQuotaMeter resolves the -quota tri-state into a meter.
//
// "auto" enables metering only when the codexbar CLI is already on PATH,
// so a host without it neither pays for a failed subprocess nor changes
// behaviour. "on" builds the meter regardless, which surfaces a missing
// install as an unreadable meter rather than silently doing nothing;
// routing is unchanged either way, because an unreadable meter yields no
// verdicts.
// validateQuotaMode rejects a -quota value that isn't one of the tri-state's
// three spellings. It is pure and does no I/O on purpose: buildExecutor
// calls it unconditionally, on every path, including the no-roles.yaml
// direct-Anthropic fallback that never calls buildQuotaMeter — otherwise a
// typo like "-quota=onn" is silently accepted there and the workflow runs
// unmetered instead of returning this documented error, which can
// invalidate a quota-routing experiment with no diagnostic at all (codex
// review on #1218, third pass).
func validateQuotaMode(mode string) error {
	switch mode {
	case "", "auto", "on", "off":
		return nil
	default:
		return fmt.Errorf("bad -quota %q; expect auto, on, or off", mode)
	}
}

func buildQuotaMeter(mode string, logger *slog.Logger) (*quota.Meter, error) {
	if err := validateQuotaMode(mode); err != nil {
		return nil, err
	}
	switch mode {
	case "off":
		return nil, nil
	case "on":
	case "", "auto":
		if _, err := exec.LookPath(quota.DefaultCodexBarCommand); err != nil {
			logger.Debug("quota routing: disabled, no meter installed", "command", quota.DefaultCodexBarCommand)
			return nil, nil
		}
	}

	// SkipPace: true — Router.OrderModels and the router metadata this
	// meter feeds consume only quota windows, never Pace. The second
	// CodexBar invocation SkipPace would otherwise trigger costs another
	// network refresh and, unlike the identity-redacted dashboard call,
	// has no redacted mode of its own (pkg/llm/quota/codexbar.go's
	// readPace), so routing startup would pay latency and read more
	// account-bearing data than it needs (codex review on #1218).
	meter := quota.New(quota.Options{Reader: credentialFilteredReader{inner: quota.CodexBarReader{SkipPace: true}}})

	// Warm the cache once so the first AI node routes on real data. The
	// read is slow enough to be worth doing here and never on the
	// routing path; a failure is logged and leaves routing unchanged.
	ctx, cancel := context.WithTimeout(context.Background(), quota.DefaultReadTimeout)
	defer cancel()
	snapshot, err := meter.Refresh(ctx)
	if err != nil {
		logger.Warn("quota routing: no reading, routing as configured", "error", err)
		return meter, nil
	}
	if !quota.MeetsMinCodexBarVersion(snapshot.SourceVersion) {
		// ADR-038 limits the audited dependency to 0.49.0+ (earlier builds
		// carry a recorded SQLite cost-store defect). The installed binary
		// answered the dashboard call, so it exists and is on PATH — this
		// is the only point that actually observes its reported version,
		// which -quota=auto's LookPath-only check never does. Disable
		// quota routing rather than route on an unaudited build; the -quota
		// tri-state's own fail-safe (an unreadable/disabled meter yields no
		// verdicts) applies unchanged.
		//
		// An empty SourceVersion goes through this same check rather than
		// bypassing it: MeetsMinCodexBarVersion already reports "" as
		// below the floor, and a build too old (or broken) to report its
		// own version is exactly the case this floor exists to catch, not
		// an exemption from it (codex review on #1218, second pass).
		installed := snapshot.SourceVersion
		if installed == "" {
			installed = "(none reported)"
		}
		logger.Warn("quota routing: disabled, codexbar version is below the audited floor",
			"installed", installed, "floor", quota.MinAuditedCodexBarVersion)
		return nil, nil
	}
	logger.Info("quota routing: enabled", "source", snapshot.Source, "providers", len(snapshot.Providers))
	return meter, nil
}

// credentialFilteredReader wraps a quota.Reader and drops any provider
// quota entry the router will actually reach through a directly-billed
// credential (an API key or Vertex AI/ADC) rather than a CLI-authenticated
// subscription. CodexBar's dashboard reads CLI/subscription identities; the
// default factory routes OpenAI exclusively through OPENAI_API_KEY and
// Anthropic through an API key or Vertex credentials
// (pkg/llm/provider/factory.go), so an unfiltered reading attaches an
// unrelated billing pool's quota to that family's routing decisions — e.g.
// low Codex OAuth quota demoting an OpenAI API account that still has full
// capacity (codex review on #1218).
type credentialFilteredReader struct {
	inner quota.Reader
}

func (r credentialFilteredReader) Read(ctx context.Context) (*quota.Snapshot, error) {
	snapshot, err := r.inner.Read(ctx)
	if err != nil || snapshot == nil {
		return snapshot, err
	}
	filtered := make([]quota.ProviderQuota, 0, len(snapshot.Providers))
	for _, p := range snapshot.Providers {
		if usesDirectlyBilledCredentials(p.Family) {
			continue
		}
		filtered = append(filtered, p)
	}
	snapshot.Providers = filtered
	return snapshot, nil
}

// usesDirectlyBilledCredentials reports whether family should be excluded
// from quota-aware promotion: either because the router will reach it
// through a credential CodexBar's CLI/subscription reading cannot
// represent (an API key or Vertex AI/ADC), or because it cannot be
// constructed at all with the credentials actually present.
//
// AuthNone is the second case, not a "leave it alone" case: every family
// auth.DetectAuthMethod can report — anthropic, gemini, openrouter,
// openai — falls through to AuthNone only when it found no usable
// credential, and provider.Factory's own auth-hierarchy switch
// unconditionally fails to construct every one of them under AuthNone
// (see pkg/llm/provider/factory.go's newOpenAIProvider et al.). A
// favorable CodexBar reading could otherwise promote a candidate the
// factory is about to refuse anyway, burning one candidate attempt
// before falling through on every such role generation — the same
// failure mode meter.go's unimplementedProviderFamilies already excludes
// gemini for (codex review on #1218, third pass). AuthLocal is the one
// family that genuinely constructs with no credential (ollama) and stays
// excluded from this filter.
func usesDirectlyBilledCredentials(family string) bool {
	switch auth.DetectAuthMethod(family) {
	case auth.AuthAPIKey, auth.AuthVertexAI, auth.AuthNone:
		return true
	case auth.AuthLocal:
		return false
	default:
		return false
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
func buildExecutor(rolesPath, quotaMode string, logger *slog.Logger) (workflow.AIExecutor, error) {
	// Validated here, unconditionally, before branching: the direct-Anthropic
	// fallback below never calls buildQuotaMeter, so without this a bad
	// -quota value is silently accepted whenever no roles.yaml is found.
	if err := validateQuotaMode(quotaMode); err != nil {
		return nil, err
	}
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
		meter, err := buildQuotaMeter(quotaMode, logger)
		if err != nil {
			return nil, err
		}
		r, err := router.New(router.Options{Config: cfg, Quota: meter})
		if err != nil {
			return nil, fmt.Errorf("init router: %w", err)
		}
		logger.Info("AI executor: router-backed",
			"roles", rolesPath, "default_role", r.DefaultRole(), "quota_routing", meter.Enabled())
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
