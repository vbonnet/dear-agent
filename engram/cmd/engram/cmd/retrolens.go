package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/vbonnet/dear-agent/engram/internal/reflection"
)

var (
	retroLensDir    string
	retroLensOutput string
)

var retroLensCmd = &cobra.Command{
	Use:   "retro-lens",
	Short: "Aggregate retro artifacts through four analytical lenses",
	Long: `Apply four distinct analytical lenses to a corpus of DEAR retrospective
artifacts and synthesize the findings into one structured knowledge-base report.

Rather than analyzing each retrospective through a single uniform lens, retro-lens
runs four independent lenses over the whole corpus and a supervisor that joins
their findings per pattern:

  1. root-cause     - extracts the underlying cause patterns behind failures
  2. recurrence     - scores how frequently each pattern recurs across the corpus
  3. remediation    - gathers the recommended remediations for each pattern
  4. classification - labels each pattern as systemic or one-off

This is the dogfood realization of the engram pipeline:
retro artifacts -> structured patterns -> persistent KB.

Examples:
  # Synthesize the engram-research retrospectives corpus to stdout
  engram retro-lens --dir ~/src/engram-research/retrospectives

  # Write the synthesis to a persistent KB file
  engram retro-lens --dir ./retrospectives --output ./RETRO_PATTERNS_KB.md`,
	RunE: runRetroLens,
}

func init() {
	rootCmd.AddCommand(retroLensCmd)
	retroLensCmd.Flags().StringVar(&retroLensDir, "dir", "", "directory of DEAR retro markdown artifacts (required)")
	retroLensCmd.Flags().StringVar(&retroLensOutput, "output", "", "write the synthesis report to this file (default: stdout)")
	_ = retroLensCmd.MarkFlagRequired("dir")
}

func runRetroLens(cmd *cobra.Command, args []string) error {
	artifacts, err := reflection.LoadCorpus(retroLensDir)
	if err != nil {
		return fmt.Errorf("load corpus: %w", err)
	}
	if len(artifacts) == 0 {
		return fmt.Errorf("no retro artifacts found in %s", retroLensDir)
	}

	report := reflection.NewSupervisor().Synthesize(artifacts)
	md := report.RenderMarkdown()

	if retroLensOutput == "" {
		fmt.Print(md)
		return nil
	}
	if err := os.WriteFile(retroLensOutput, []byte(md), 0o600); err != nil {
		return fmt.Errorf("write report: %w", err)
	}
	fmt.Fprintf(os.Stderr, "wrote synthesis of %d patterns across %d artifacts to %s\n",
		len(report.Patterns), report.ArtifactCount, retroLensOutput)
	return nil
}
