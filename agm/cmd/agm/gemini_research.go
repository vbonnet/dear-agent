package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/agent"
	"github.com/vbonnet/dear-agent/agm/internal/cli"
)

// gemini-research is an off-Claude Gemini research channel (ce-ta3tg, ce-px7ai).
// It shells the Antigravity CLI in PRINT mode (`agy --print`), which reuses the
// cached credential and returns a Gemini response non-interactively. It exists
// because AGM's interactive agy harness re-signs-in per spawn and can stall on
// Antigravity's account-eligibility gate, and because the gemini.google.com web
// app silently drops automated sends. Print mode is multimodal: images made
// available via --add-dir and referenced in the prompt (e.g. `@frame.png`) are
// read by Gemini, so video-frame stills can be analysed, not just transcripts.

// maxGeminiResearchStdinBytes bounds a piped prompt so a large or non-terminating
// stream cannot exhaust memory before agy is even started.
const maxGeminiResearchStdinBytes = 1 << 20 // 1 MiB

// geminiResearchScrubbedEnvVars are removed from the child environment so an
// unrelated credential in AGM's own process env is never forwarded to agy.
// agy authenticates from ~/.gemini (not the environment), so scrubbing these is
// safe. Keep in sync with the sensitive vars stripped by the canonical harness
// launch (internal/harnessexec).
var geminiResearchScrubbedEnvVars = map[string]bool{
	"ANTHROPIC_API_KEY":                   true,
	"CLAUDE_CODE_OAUTH_TOKEN":             true,
	"AWS_SECRET_ACCESS_KEY":               true,
	"AWS_SESSION_TOKEN":                   true,
	"CLAUDE_CODE_ENABLE_TELEMETRY":        true,
	"CLAUDE_CODE_ENHANCED_TELEMETRY_BETA": true,
	"OTEL_EXPORTER_OTLP_ENDPOINT":         true,
	"OTEL_EXPORTER_OTLP_HEADERS":          true,
	"OTEL_EXPORTER_OTLP_PROTOCOL":         true,
	"OTEL_TRACES_EXPORTER":                true,
}

// geminiResearchRunner is the process-exec seam so the command is unit-testable
// without a real Antigravity CLI.
type geminiResearchRunner func(ctx context.Context, agyPath string, args, env []string, dir string) ([]byte, error)

func defaultGeminiResearchRunner(ctx context.Context, agyPath string, args, env []string, dir string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, agyPath, args...)
	cmd.Dir = dir
	cmd.Env = env
	return cmd.CombinedOutput()
}

type geminiResearchOptions struct {
	effort  string        // low|medium|high (optional)
	model   string        // Gemini model or AGM agy alias (optional)
	timeout time.Duration // print-timeout and overall bound
	workDir string        // cwd for the run (honors `agm -C`)
	addDirs []string      // extra dirs agy may read (e.g. a frames dir for images)
	agyPath string        // override the resolved `agy` binary (tests)
}

// buildGeminiResearchArgs assembles the `agy --print` argument vector for a
// one-shot research prompt. Slash/skill expansion is disabled so the prompt is
// treated as literal research input, not a command. The model is expected to be
// already resolved to an agy-native name.
func buildGeminiResearchArgs(prompt string, o geminiResearchOptions) []string {
	args := []string{"--print", "--dangerously-skip-permissions", "--disable-slash-commands"}
	if o.timeout > 0 {
		args = append(args, "--print-timeout", o.timeout.String())
	}
	if o.effort != "" {
		args = append(args, "--effort", o.effort)
	}
	if o.model != "" {
		args = append(args, "--model", o.model)
	}
	for _, dir := range o.addDirs {
		args = append(args, "--add-dir", dir)
	}
	return append(args, "-p", prompt)
}

// geminiResearchEnv returns the current environment with sensitive, agy-unneeded
// variables removed.
func geminiResearchEnv() []string {
	parent := os.Environ()
	filtered := make([]string, 0, len(parent))
	for _, entry := range parent {
		name, _, ok := strings.Cut(entry, "=")
		if ok && geminiResearchScrubbedEnvVars[name] {
			continue
		}
		filtered = append(filtered, entry)
	}
	return filtered
}

// runGeminiResearch runs one research prompt through the Antigravity CLI print
// mode and returns the trimmed response text.
func runGeminiResearch(ctx context.Context, prompt string, o geminiResearchOptions, run geminiResearchRunner) (string, error) {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return "", errors.New("gemini-research: empty prompt")
	}
	agyPath := o.agyPath
	if agyPath == "" {
		resolved, err := exec.LookPath("agy")
		if err != nil {
			return "", fmt.Errorf("gemini-research: Antigravity CLI 'agy' not found on PATH: %w", err)
		}
		agyPath = resolved
	}
	// Cross AGM's agy model aliases (e.g. 3.5-flash-low) to the agy-native name.
	if o.model != "" {
		o.model = agent.ResolveModelFullName("agy", o.model)
	}
	runCtx := ctx
	if o.timeout > 0 {
		var cancel context.CancelFunc
		// Bound the process slightly beyond agy's own --print-timeout so a wedged
		// CLI cannot outlive the caller.
		runCtx, cancel = context.WithTimeout(ctx, o.timeout+30*time.Second)
		defer cancel()
	}
	out, err := run(runCtx, agyPath, buildGeminiResearchArgs(prompt, o), geminiResearchEnv(), o.workDir)
	text := strings.TrimSpace(string(out))
	if err != nil {
		return text, fmt.Errorf("gemini-research: agy print failed: %w (output: %s)", err, truncateGeminiOutput(text, 400))
	}
	return text, nil
}

// truncateGeminiOutput truncates on a rune boundary so multi-byte UTF-8 is never
// split.
func truncateGeminiOutput(s string, maxRunes int) string {
	if utf8.RuneCountInString(s) <= maxRunes {
		return s
	}
	runes := []rune(s)
	return string(runes[:maxRunes]) + "…"
}

// resolveGeminiResearchPrompt reads the prompt from --prompt-file, then
// positional args, then stdin (bounded).
func resolveGeminiResearchPrompt(args []string, file string, stdin io.Reader) (string, error) {
	if file != "" {
		data, err := os.ReadFile(file)
		if err != nil {
			return "", fmt.Errorf("gemini-research: read prompt file: %w", err)
		}
		return string(data), nil
	}
	if len(args) > 0 {
		return strings.Join(args, " "), nil
	}
	data, err := io.ReadAll(io.LimitReader(stdin, maxGeminiResearchStdinBytes))
	if err != nil {
		return "", fmt.Errorf("gemini-research: read prompt from stdin: %w", err)
	}
	return string(data), nil
}

var (
	geminiResearchEffort     string
	geminiResearchModel      string
	geminiResearchTimeout    time.Duration
	geminiResearchPromptFile string
	geminiResearchAddDirs    []string
)

var geminiResearchCmd = &cobra.Command{
	Use:   "gemini-research [prompt]",
	Short: "Run a one-shot Gemini research prompt via the Antigravity CLI print mode",
	Long: "Run a one-shot Gemini research prompt off-Claude via `agy --print` (the Antigravity CLI\n" +
		"print mode). It reuses the cached Antigravity credential and answers non-interactively,\n" +
		"avoiding both AGM's interactive agy sign-in stall (ce-ta3tg) and the bot-blocked\n" +
		"gemini.google.com web app.\n\n" +
		"The prompt is taken from arguments, --prompt-file, or stdin. Print mode is multimodal:\n" +
		"grant a directory with --add-dir and reference an image in the prompt (e.g. @frame.png)\n" +
		"to have Gemini read it — useful for occasional video-frame analysis.",
	Args:         cobra.ArbitraryArgs,
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		prompt, err := resolveGeminiResearchPrompt(args, geminiResearchPromptFile, cmd.InOrStdin())
		if err != nil {
			return err
		}
		out, err := runGeminiResearch(cmd.Context(), prompt, geminiResearchOptions{
			effort:  geminiResearchEffort,
			model:   geminiResearchModel,
			timeout: geminiResearchTimeout,
			workDir: cli.GetProjectDirectory(), // honor `agm -C <dir>`; "" inherits cwd
			addDirs: geminiResearchAddDirs,
		}, defaultGeminiResearchRunner)
		if err != nil {
			return err
		}
		if outputFormat == "json" {
			payload, mErr := json.Marshal(map[string]string{"response": out})
			if mErr != nil {
				return mErr
			}
			fmt.Fprintln(cmd.OutOrStdout(), string(payload))
			return nil
		}
		fmt.Fprintln(cmd.OutOrStdout(), out)
		return nil
	},
}

func init() {
	geminiResearchCmd.Flags().StringVar(&geminiResearchEffort, "effort", "", "Reasoning effort: low|medium|high")
	geminiResearchCmd.Flags().StringVar(&geminiResearchModel, "model", "", "Gemini model or AGM agy alias (default: agy's configured model)")
	geminiResearchCmd.Flags().DurationVar(&geminiResearchTimeout, "timeout", 3*time.Minute, "Max wait for the Gemini response")
	geminiResearchCmd.Flags().StringVar(&geminiResearchPromptFile, "prompt-file", "", "Read the prompt from a file instead of arguments")
	geminiResearchCmd.Flags().StringArrayVar(&geminiResearchAddDirs, "add-dir", nil, "Grant agy read access to a directory (repeatable); reference files in the prompt, e.g. @frame.png for images")
	rootCmd.AddCommand(geminiResearchCmd)
}
