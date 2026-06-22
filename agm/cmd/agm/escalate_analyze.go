package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/pkg/vroom/escalation"
)

// This file adds the read-side of the escalation log: the LLM outcome
// adjudicator pass (`agm escalate adjudicate`) that backfills the
// outcome/misalignment columns on answered events, and `agm escalate analyze`
// for the three analyses the log schema was designed around (ADR-032):
//
//	agm escalate adjudicate            — LLM judge pass: backfill outcome columns
//	agm escalate analyze misaligned    — incorrect/misaligned answers
//	agm escalate analyze frequent      — frequent question types
//	agm escalate analyze duplicates    — many agents asking the same question
//	agm escalate analyze all           — all three
//
// All of these operate directly on the JSONL event log; none needs the live
// engine or session store, so they are cheap to run anywhere the log is.

var (
	escLogPath     string
	escForce       bool
	escDryRun      bool
	escMinCount    int
	escMinDistinct int
)

var escalateAdjudicateCmd = &cobra.Command{
	Use:   "adjudicate",
	Short: "Backfill outcome/misalignment on answered escalations via an LLM judge",
	Long: `Score each answered escalation in the event log with an independent LLM
judge (a separate model from the agents in the chain) and backfill the
outcome/misalignment columns the 'analyze misaligned' report reads.

The judge degrades safely: with no ANTHROPIC_API_KEY it falls back to the
deterministic floor (which only flags non-answers), and a model error never
invents a verdict — the event is left for a later pass. Already-adjudicated
events are skipped unless --force.`,
	Args: cobra.NoArgs,
	RunE: runEscalateAdjudicate,
}

var escalateAnalyzeCmd = &cobra.Command{
	Use:   "analyze",
	Short: "Analyse the escalation log (misaligned / frequent / duplicates)",
	Long: `Run the three escalation analyses over the JSONL event log:

  misaligned  — answers the judge flagged incorrect or misaligned
  frequent    — questions asked many times (group by question_hash)
  duplicates  — one question asked by many distinct agents (missing prompt
                context); group by question_hash, count distinct origins
  all         — all three`,
}

var escalateAnalyzeMisalignedCmd = &cobra.Command{
	Use:   "misaligned",
	Short: "Answers flagged incorrect or misaligned by the adjudicator",
	Args:  cobra.NoArgs,
	RunE:  runAnalyzeMisaligned,
}

var escalateAnalyzeFrequentCmd = &cobra.Command{
	Use:   "frequent",
	Short: "Most-frequently-asked question types (group by question_hash)",
	Args:  cobra.NoArgs,
	RunE:  runAnalyzeFrequent,
}

var escalateAnalyzeDuplicatesCmd = &cobra.Command{
	Use:   "duplicates",
	Short: "One question asked by many distinct agents (missing prompt context)",
	Args:  cobra.NoArgs,
	RunE:  runAnalyzeDuplicates,
}

var escalateAnalyzeAllCmd = &cobra.Command{
	Use:   "all",
	Short: "Run all three analyses",
	Args:  cobra.NoArgs,
	RunE:  runAnalyzeAll,
}

func init() {
	escalateAdjudicateCmd.Flags().StringVar(&escLogPath, "log", "", "event log path (default ~/.agm/escalation/events.jsonl)")
	escalateAdjudicateCmd.Flags().BoolVar(&escForce, "force", false, "re-adjudicate events that already have an outcome")
	escalateAdjudicateCmd.Flags().BoolVar(&escDryRun, "dry-run", false, "score and report counts but do not write the log")

	for _, c := range []*cobra.Command{
		escalateAnalyzeMisalignedCmd, escalateAnalyzeFrequentCmd,
		escalateAnalyzeDuplicatesCmd, escalateAnalyzeAllCmd,
	} {
		c.Flags().StringVar(&escLogPath, "log", "", "event log path (default ~/.agm/escalation/events.jsonl)")
	}
	escalateAnalyzeFrequentCmd.Flags().IntVar(&escMinCount, "min", 2, "minimum times asked to report a question")
	escalateAnalyzeDuplicatesCmd.Flags().IntVar(&escMinDistinct, "min", 2, "minimum distinct agents to report a question")
	escalateAnalyzeAllCmd.Flags().IntVar(&escMinCount, "min-count", 2, "minimum times asked (frequent)")
	escalateAnalyzeAllCmd.Flags().IntVar(&escMinDistinct, "min-distinct", 2, "minimum distinct agents (duplicates)")

	escalateAnalyzeCmd.AddCommand(
		escalateAnalyzeMisalignedCmd, escalateAnalyzeFrequentCmd,
		escalateAnalyzeDuplicatesCmd, escalateAnalyzeAllCmd,
	)
	escalateCmd.AddCommand(escalateAdjudicateCmd, escalateAnalyzeCmd)
}

// escalationLogPath resolves the event log path: the --log flag if set, else
// the canonical ~/.agm/escalation/events.jsonl.
func escalationLogPath() (string, error) {
	if escLogPath != "" {
		return escLogPath, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".agm", "escalation", "events.jsonl"), nil
}

// loadEscalationEvents reads and parses the event log.
func loadEscalationEvents() ([]escalation.EscalationEvent, error) {
	path, err := escalationLogPath()
	if err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, nil // empty log: analyses report "no data"
	}
	if err != nil {
		return nil, fmt.Errorf("open escalation log %q: %w", path, err)
	}
	defer f.Close()
	return escalation.ReadEvents(f)
}

func runEscalateAdjudicate(cmd *cobra.Command, _ []string) error {
	path, err := escalationLogPath()
	if err != nil {
		return err
	}
	adj := escalation.NewClaudeAdjudicator()

	if escDryRun {
		events, lerr := loadEscalationEvents()
		if lerr != nil {
			return lerr
		}
		_, res, berr := escalation.Backfill(cmd.Context(), events, adj, escForce)
		if berr != nil {
			return berr
		}
		return reportBackfill(res, adj.Name(), true)
	}

	res, err := escalation.BackfillFile(cmd.Context(), path, adj, escForce)
	if err != nil {
		return err
	}
	return reportBackfill(res, adj.Name(), false)
}

func reportBackfill(res escalation.BackfillResult, judge string, dryRun bool) error {
	if isJSONOutput() {
		return printJSON(map[string]any{
			"judge":      judge,
			"dry_run":    dryRun,
			"total":      res.Total,
			"candidates": res.Candidates,
			"updated":    res.Updated,
			"skipped":    res.Skipped,
			"by_outcome": res.ByOutcome,
		})
	}
	verb := "wrote"
	if dryRun {
		verb = "would write"
	}
	fmt.Printf("adjudicator: %s\n", judge)
	fmt.Printf("log: %d events, %d answered candidates\n", res.Total, res.Candidates)
	fmt.Printf("%s %d outcomes (%d skipped/declined)\n", verb, res.Updated, res.Skipped)
	for _, o := range []escalation.Outcome{
		escalation.OutcomeCorrect, escalation.OutcomeIncorrect,
		escalation.OutcomeMisaligned, escalation.OutcomeUnclear,
	} {
		if n := res.ByOutcome[o]; n > 0 {
			fmt.Printf("  %-10s %d\n", o, n)
		}
	}
	if res.Candidates > 0 && res.Updated == 0 && judge == "default" {
		fmt.Println("note: no model configured (set ANTHROPIC_API_KEY) — only non-answers are scored offline")
	}
	return nil
}

func runAnalyzeMisaligned(cmd *cobra.Command, _ []string) error {
	events, err := loadEscalationEvents()
	if err != nil {
		return err
	}
	rows := escalation.AnalyzeMisaligned(events)
	if isJSONOutput() {
		return printJSON(rows)
	}
	if len(rows) == 0 {
		fmt.Println("no incorrect or misaligned answers (run 'agm escalate adjudicate' first if the log is unscored)")
		return nil
	}
	fmt.Printf("%d incorrect/misaligned answer(s):\n", len(rows))
	for _, r := range rows {
		fmt.Printf("\n%s  [%s]  by=%s\n", r.EscalationID, r.Outcome, r.AnsweredByRole)
		fmt.Printf("  Q: %s\n", truncate(r.Question, 100))
		fmt.Printf("  A: %s\n", truncate(r.Answer, 100))
		if r.Misalignment != "" {
			fmt.Printf("  ↳ %s\n", r.Misalignment)
		}
	}
	return nil
}

func runAnalyzeFrequent(cmd *cobra.Command, _ []string) error {
	events, err := loadEscalationEvents()
	if err != nil {
		return err
	}
	rows := escalation.AnalyzeFrequentQuestions(events, escMinCount)
	if isJSONOutput() {
		return printJSON(rows)
	}
	if len(rows) == 0 {
		fmt.Printf("no question asked at least %d times\n", escMinCount)
		return nil
	}
	fmt.Printf("frequent questions (asked ≥%d times):\n", escMinCount)
	printQuestionGroups(rows)
	return nil
}

func runAnalyzeDuplicates(cmd *cobra.Command, _ []string) error {
	events, err := loadEscalationEvents()
	if err != nil {
		return err
	}
	rows := escalation.AnalyzeManyAgents(events, escMinDistinct)
	if isJSONOutput() {
		return printJSON(rows)
	}
	if len(rows) == 0 {
		fmt.Printf("no question asked by ≥%d distinct agents\n", escMinDistinct)
		return nil
	}
	fmt.Printf("same question across agents (≥%d distinct origins — likely missing prompt context):\n", escMinDistinct)
	printQuestionGroups(rows)
	return nil
}

func runAnalyzeAll(cmd *cobra.Command, _ []string) error {
	events, err := loadEscalationEvents()
	if err != nil {
		return err
	}
	if isJSONOutput() {
		return printJSON(map[string]any{
			"misaligned": escalation.AnalyzeMisaligned(events),
			"frequent":   escalation.AnalyzeFrequentQuestions(events, escMinCount),
			"duplicates": escalation.AnalyzeManyAgents(events, escMinDistinct),
		})
	}
	mis := escalation.AnalyzeMisaligned(events)
	fmt.Printf("== incorrect/misaligned answers: %d ==\n", len(mis))
	for _, r := range mis {
		fmt.Printf("  %s [%s] %s\n", r.EscalationID, r.Outcome, truncate(r.Question, 70))
	}
	freq := escalation.AnalyzeFrequentQuestions(events, escMinCount)
	fmt.Printf("\n== frequent questions (≥%d): %d ==\n", escMinCount, len(freq))
	printQuestionGroups(freq)
	dup := escalation.AnalyzeManyAgents(events, escMinDistinct)
	fmt.Printf("\n== same question, many agents (≥%d distinct): %d ==\n", escMinDistinct, len(dup))
	printQuestionGroups(dup)
	return nil
}

func printQuestionGroups(rows []escalation.QuestionGroup) {
	for _, g := range rows {
		fmt.Printf("  count=%-3d origins=%-3d topic=%-12s %s\n",
			g.Count, g.DistinctOrigins, dashIfEmpty(g.Topic), truncate(g.Question, 70))
	}
}

func dashIfEmpty(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
