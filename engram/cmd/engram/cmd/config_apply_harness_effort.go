package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/engram/internal/harnesseffort"
)

var (
	applyHarnessEffortDryRun      bool
	applyHarnessEffortProvider    string
	applyHarnessEffortHarness     string
	applyHarnessEffortOpenCodeDir string
)

var configApplyHarnessEffortCmd = &cobra.Command{
	Use:   "apply-harness-effort",
	Short: "Generate native effort-tier config for Codex CLI, OpenCode, and Gemini CLI",
	Long: `Generate native effort-tier configuration for AI harnesses from the
canonical harness-effort-defaults.yaml specification.

Writes:
  ~/.codex/config.toml    - Codex CLI effort profiles
  ./opencode.json         - OpenCode agents and commands (use --opencode-dir to change path)
  (stdout)                - Gemini CLI alias suggestions (no file written)

Override defaults with:
  ~/.config/company/engram/harness-effort.yaml  (company-level)
  ~/.config/engram/harness-effort.yaml          (user-level)`,
	RunE: runConfigApplyHarnessEffort,
}

func init() {
	configCmd.AddCommand(configApplyHarnessEffortCmd)
	configApplyHarnessEffortCmd.Flags().BoolVar(&applyHarnessEffortDryRun, "dry-run", false, "Print what would be written without modifying files")
	configApplyHarnessEffortCmd.Flags().StringVar(&applyHarnessEffortProvider, "provider", "", "Limit to one provider: anthropic, openai, google")
	configApplyHarnessEffortCmd.Flags().StringVar(&applyHarnessEffortHarness, "harness", "", "Limit to one harness: codex, opencode, gemini")
	configApplyHarnessEffortCmd.Flags().StringVar(&applyHarnessEffortOpenCodeDir, "opencode-dir", ".", "Directory for opencode.json")
}

func runConfigApplyHarnessEffort(cmd *cobra.Command, args []string) error {
	opts := harnesseffort.GenerateOpts{
		DryRun:      applyHarnessEffortDryRun,
		Provider:    applyHarnessEffortProvider,
		Harness:     applyHarnessEffortHarness,
		OpenCodeDir: applyHarnessEffortOpenCodeDir,
	}

	outputs, geminiSuggestions, err := harnesseffort.Generate(opts)
	if err != nil {
		return fmt.Errorf("generating harness configs: %w", err)
	}

	if opts.DryRun {
		fmt.Fprintln(cmd.OutOrStdout(), "[DRY RUN] No files will be written.")
		fmt.Fprintln(cmd.OutOrStdout())
	}

	for _, out := range outputs {
		if opts.DryRun {
			fmt.Fprintf(cmd.OutOrStdout(), "=== %s ===\n", out.Path)
			fmt.Fprintln(cmd.OutOrStdout(), string(out.Content))
		} else {
			if err := writeOutputFile(out); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "  wrote: %s\n", out.Path)
		}
	}

	if geminiSuggestions != "" {
		if !opts.DryRun {
			fmt.Fprintln(cmd.OutOrStdout())
			fmt.Fprintln(cmd.OutOrStdout(), "Gemini alias suggestions (no file written):")
		}
		fmt.Fprint(cmd.OutOrStdout(), geminiSuggestions)
	}

	return nil
}

func writeOutputFile(out harnesseffort.OutputFile) error {
	dir := filepath.Dir(out.Path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating directory %s: %w", dir, err)
	}
	if err := os.WriteFile(out.Path, out.Content, 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", out.Path, err)
	}
	return nil
}
