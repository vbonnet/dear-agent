package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/vbonnet/dear-agent/pkg/aggregator"
)

func runReport(ctx context.Context, args []string) int {
	fs := flag.NewFlagSet("report", flag.ContinueOnError)
	dbPath := fs.String("db", "signals.db", "path to signals.db")
	kindFlag := fs.String("kind", "",
		"filter by signal kind (e.g. lint_trend); empty == every kind")
	since := fs.Duration("since", 0,
		"include only signals collected within this duration "+
			"(e.g. 168h); 0 == no time filter")
	limit := fs.Int("limit", 50, "maximum rows per kind")
	asJSON := fs.Bool("json", false, "emit machine-readable JSON")
	score := fs.Bool("score", false,
		"compute weighted priority score across the most recent signals")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	store, err := aggregator.OpenSQLiteStore(*dbPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer func() { _ = store.Close() }()

	kinds, err := selectKinds(ctx, store, aggregator.Kind(*kindFlag))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if len(kinds) == 0 {
		fmt.Fprintln(os.Stderr, "no signals stored yet — run 'dear-agent-signals collect' first")
		return 0
	}

	all := map[aggregator.Kind][]aggregator.Signal{}
	for _, k := range kinds {
		var sigs []aggregator.Signal
		if *since > 0 {
			sigs, err = store.Range(ctx, k, time.Now().Add(-*since))
		} else {
			sigs, err = store.Recent(ctx, k, *limit)
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		if *limit > 0 && len(sigs) > *limit {
			sigs = sigs[:*limit]
		}
		all[k] = sigs
	}

	if *score {
		return renderScore(all, *asJSON)
	}
	return renderRows(all, kinds, *asJSON)
}

// selectKinds returns the kinds to render: either the explicit
// --kind flag (validated), or every distinct kind present in the
// store.
func selectKinds(ctx context.Context, store *aggregator.SQLiteStore, k aggregator.Kind) ([]aggregator.Kind, error) {
	if k != "" {
		if err := k.Validate(); err != nil {
			return nil, err
		}
		return []aggregator.Kind{k}, nil
	}
	return store.Kinds(ctx)
}

func renderRows(
	all map[aggregator.Kind][]aggregator.Signal,
	kinds []aggregator.Kind,
	asJSON bool,
) int {
	if asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(all); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		return 0
	}
	tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "KIND\tSUBJECT\tVALUE\tCOLLECTED")
	for _, k := range kinds {
		for _, s := range all[k] {
			fmt.Fprintf(tw, "%s\t%s\t%v\t%s\n",
				s.Kind, s.Subject, s.Value,
				s.CollectedAt.UTC().Format(time.RFC3339))
		}
	}
	_ = tw.Flush()
	return 0
}

func renderScore(
	all map[aggregator.Kind][]aggregator.Signal,
	asJSON bool,
) int {
	scorer := aggregator.Scorer{}
	flat := make([]aggregator.Signal, 0, 32)
	for _, sigs := range all {
		flat = append(flat, sigs...)
	}
	scores := scorer.Score(flat)
	total := scorer.Total(scores)

	if asJSON {
		out := map[string]any{
			"scores": scores,
			"total":  total,
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(out); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		return 0
	}
	tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "KIND\tSUBJECT\tRAW\tNORM\tWEIGHT\tWEIGHTED")
	for _, s := range scores {
		fmt.Fprintf(tw, "%s\t%s\t%.2f\t%.3f\t%.2f\t%.3f\n",
			s.Kind, s.Subject, s.Raw, s.Norm, s.Weight, s.Weighted)
	}
	_ = tw.Flush()
	fmt.Printf("\ntotal weighted priority: %.3f\n", total)
	return 0
}
