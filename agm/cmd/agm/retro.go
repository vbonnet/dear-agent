package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/ops"
)

var (
	retroAnalyzeInput     string
	retroAnalyzeLens      string
	retroAnalyzeAllLenses bool
)

var retroCmd = &cobra.Command{
	Use:   "retro",
	Short: "Analyze retrospectives across multiple analytical lenses",
	Long: `Retro commands mine retrospective markdown files for structured insights.

The analysis is static: it parses an existing retro FILE and extracts insights
per lens. It does NOT re-run the session. The four lenses let a supervisor
aggregate findings across many worker retros.`,
	Args: cobra.ArbitraryArgs,
	RunE: groupRunE,
}

var retroAnalyzeCmd = &cobra.Command{
	Use:   "analyze",
	Short: "Run analytical lenses over a retrospective file",
	Long: `Analyze a retrospective markdown file through one or more analytical lenses.

Lenses:
  • root-cause          — extract the failure chain (what failed, why, contributing factors)
  • recurrence          — score recurrence: 0=novel, 1=seen-once, 2=recurring (+ prior bead IDs)
  • remediation         — collect concrete follow-up actions (bead IDs, PR numbers, process changes)
  • systemic-vs-oneoff  — classify: structural/will-recur vs context-specific/one-off

Output:
  • Agent mode (default for non-TTY) — compact JSON with each lens's findings
  • Human mode (-o markdown)         — markdown sections per lens

Examples:
  agm retro analyze --input RETRO.md --all-lenses
  agm retro analyze --input RETRO.md --lens=root-cause
  agm retro analyze --input RETRO.md --lens=recurrence -o markdown`,
	Args: cobra.NoArgs,
	RunE: runRetroAnalyze,
}

func init() {
	retroAnalyzeCmd.Flags().StringVar(&retroAnalyzeInput, "input", "", "path to the retrospective markdown file (required)")
	retroAnalyzeCmd.Flags().StringVar(&retroAnalyzeLens, "lens", "", "single lens to run: root-cause|recurrence|remediation|systemic-vs-oneoff")
	retroAnalyzeCmd.Flags().BoolVar(&retroAnalyzeAllLenses, "all-lenses", false, "run all four lenses (default when --lens is omitted)")

	retroCmd.AddCommand(retroAnalyzeCmd)
	rootCmd.AddCommand(retroCmd)
}

func runRetroAnalyze(cmd *cobra.Command, args []string) error {
	result, err := ops.AnalyzeRetro(&ops.AnalyzeRetroRequest{
		InputPath: retroAnalyzeInput,
		Lens:      retroAnalyzeLens,
		AllLenses: retroAnalyzeAllLenses,
	})
	if err != nil {
		return handleError(err)
	}

	return printResult(result, func() {
		printRetroAnalyzeMarkdown(result)
	})
}

func printRetroAnalyzeMarkdown(r *ops.AnalyzeRetroResult) {
	fmt.Printf("# Retro Analysis: %s\n\n", r.RetroFile)
	if r.Title != "" {
		fmt.Printf("**Title:** %s\n\n", r.Title)
	}
	printRootCauseMarkdown(r.Lenses.RootCause)
	printRecurrenceMarkdown(r.Lenses.Recurrence)
	printRemediationMarkdown(r.Lenses.Remediation)
	printSystemicMarkdown(r.Lenses.Systemic)
}

func printRootCauseMarkdown(rc *ops.RootCauseLens) {
	if rc == nil {
		return
	}
	fmt.Println("## Root Cause")
	if rc.Summary != "" {
		fmt.Printf("\n%s\n", rc.Summary)
	}
	if len(rc.Chain) > 0 {
		fmt.Println("\n**Chain:**")
		for i, c := range rc.Chain {
			fmt.Printf("%d. %s\n", i+1, c)
		}
	}
	fmt.Println()
}

func printRecurrenceMarkdown(rec *ops.RecurrenceLens) {
	if rec == nil {
		return
	}
	fmt.Println("## Recurrence")
	fmt.Printf("\n**Score:** %d (%s)\n", rec.Score, rec.Label)
	if len(rec.PriorInstances) > 0 {
		fmt.Printf("**Prior instances:** %s\n", strings.Join(rec.PriorInstances, ", "))
	}
	if len(rec.Signals) > 0 {
		fmt.Printf("**Signals:** %s\n", strings.Join(rec.Signals, ", "))
	}
	fmt.Println()
}

func printRemediationMarkdown(rem *ops.RemediationLens) {
	if rem == nil {
		return
	}
	fmt.Println("## Remediation")
	if len(rem.Actions) == 0 {
		fmt.Println("\n_No concrete follow-up actions found._")
	}
	for _, a := range rem.Actions {
		var tags []string
		if a.Bead != "" {
			tags = append(tags, a.Bead)
		}
		if a.PR != "" {
			tags = append(tags, a.PR)
		}
		prefix := ""
		if len(tags) > 0 {
			prefix = "[" + strings.Join(tags, " ") + "] "
		}
		fmt.Printf("- %s%s\n", prefix, a.Description)
	}
	fmt.Println()
}

func printSystemicMarkdown(sys *ops.SystemicLens) {
	if sys == nil {
		return
	}
	fmt.Println("## Systemic vs One-off")
	fmt.Printf("\n**Classification:** %s (systemic=%d, one-off=%d)\n",
		sys.Classification, sys.SystemicScore, sys.OneOffScore)
	if sys.Reason != "" {
		fmt.Printf("**Reason:** %s\n", sys.Reason)
	}
	fmt.Println()
}
