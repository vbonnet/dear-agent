package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// gemini-research is the durable, off-Claude Gemini research channel (ce-ta3tg,
// ce-px7ai). It shells the Antigravity CLI in PRINT mode (`agy --print`), which
// reuses the cached credential and returns a Gemini response in seconds. This
// deliberately avoids two unreliable paths:
//   - AGM's interactive agy harness, which re-signs-in per spawn and stalls on
//     Antigravity's "verifying your account eligibility" gate (0% in testing).
//   - the gemini.google.com web app, which silently drops automated sends
//     (consumer bot-detection).
//
// Measured: `agy --print` was 100% reliable (5/5, 7-11s) on the same account.

// geminiResearchRunner is the process-exec seam so the command is unit-testable
// without a real Antigravity CLI.
type geminiResearchRunner func(ctx context.Context, agyPath string, args []string, dir string) ([]byte, error)

func defaultGeminiResearchRunner(ctx context.Context, agyPath string, args []string, dir string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, agyPath, args...)
	cmd.Dir = dir
	return cmd.CombinedOutput()
}

type geminiResearchOptions struct {
	effort  string        // low|medium|high (optional)
	model   string        // Gemini model override (optional)
	timeout time.Duration // print-timeout and overall bound
	workDir string        // optional cwd for the run
	agyPath string        // override the resolved `agy` binary (tests)
}

// buildGeminiResearchArgs assembles the `agy --print` argument vector for a
// one-shot research prompt. Slash/skill expansion is disabled so the prompt is
// treated as literal research input, not a command.
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
	return append(args, "-p", prompt)
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
	runCtx := ctx
	if o.timeout > 0 {
		var cancel context.CancelFunc
		// Bound the process slightly beyond agy's own --print-timeout so a wedged
		// CLI cannot outlive the caller.
		runCtx, cancel = context.WithTimeout(ctx, o.timeout+30*time.Second)
		defer cancel()
	}
	out, err := run(runCtx, agyPath, buildGeminiResearchArgs(prompt, o), o.workDir)
	text := strings.TrimSpace(string(out))
	if err != nil {
		return text, fmt.Errorf("gemini-research: agy print failed: %w (output: %s)", err, truncateGeminiOutput(text, 400))
	}
	return text, nil
}

func truncateGeminiOutput(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "…"
}

// resolveGeminiResearchPrompt reads the prompt from --prompt-file, then
// positional args, then stdin (in that order).
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
	data, err := io.ReadAll(stdin)
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
)

var geminiResearchCmd = &cobra.Command{
	Use:   "gemini-research [prompt]",
	Short: "Run a one-shot Gemini research prompt via the reliable Antigravity CLI print mode",
	Long: "Run a one-shot Gemini research prompt off-Claude via `agy --print` (the Antigravity CLI\n" +
		"print mode). This is the dependable Gemini research channel: it reuses the cached\n" +
		"credential and answers in seconds, avoiding both AGM's interactive agy sign-in stall\n" +
		"(ce-ta3tg) and the bot-blocked gemini.google.com web app.\n\n" +
		"The prompt is taken from arguments, --prompt-file, or stdin.",
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
		}, defaultGeminiResearchRunner)
		if err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), out)
		return nil
	},
}

func init() {
	geminiResearchCmd.Flags().StringVar(&geminiResearchEffort, "effort", "", "Reasoning effort: low|medium|high")
	geminiResearchCmd.Flags().StringVar(&geminiResearchModel, "model", "", "Gemini model override (default: agy's configured model)")
	geminiResearchCmd.Flags().DurationVar(&geminiResearchTimeout, "timeout", 3*time.Minute, "Max wait for the Gemini response")
	geminiResearchCmd.Flags().StringVar(&geminiResearchPromptFile, "prompt-file", "", "Read the prompt from a file instead of arguments")
	rootCmd.AddCommand(geminiResearchCmd)
}
