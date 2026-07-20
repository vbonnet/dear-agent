// Command backlog-suggest ranks explicitly supplied Markdown work-item tables.
// It is a read-only CLI surface over pkg/backlog. Beads owns Dear Agent's live
// work and VROOM dispatches directly from Beads.
//
//	backlog-suggest list --files ./snapshot.md                    # every parsed item
//	backlog-suggest suggest --files ./snapshot.md                 # top-N eligible + blocked
//	backlog-suggest suggest --files ./snapshot.md --phase 6 --top 1
//
// Exit codes: 0 success, 1 runtime error, 2 usage error.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/vbonnet/dear-agent/pkg/backlog"
)

func main() {
	os.Exit(run())
}

func run() int {
	if len(os.Args) < 2 {
		usage()
		return 2
	}
	switch os.Args[1] {
	case "list":
		return cmdList(os.Args[2:])
	case "suggest":
		return cmdSuggest(os.Args[2:])
	default:
		usage()
		return 2
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `Usage: %s <subcommand> [flags]

Subcommands:
  list                  list every parsed backlog item
  suggest               rank and suggest the next items to pick up

Run "%s <subcommand> -h" for subcommand flags.
`, os.Args[0], os.Args[0])
}

// parseFiles splits the required comma-separated --files value.
func parseFiles(v string) ([]string, error) {
	if strings.TrimSpace(v) == "" {
		return nil, fmt.Errorf("--files is required; Beads owns Dear Agent's live work")
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("--files must name at least one Markdown source")
	}
	return out, nil
}

func cmdList(args []string) int {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	files := fs.String("files", "", "required comma-separated Markdown files")
	asJSON := fs.Bool("json", false, "emit JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	paths, err := parseFiles(*files)
	if err != nil {
		fmt.Fprintln(os.Stderr, "list:", err)
		return 2
	}

	src := backlog.NewMarkdownSource(paths...)
	items, err := src.Items(context.Background())
	if err != nil {
		fmt.Fprintf(os.Stderr, "load: %v\n", err)
		return 1
	}
	if *asJSON {
		return encodeJSON(items)
	}
	fmt.Printf("# source: %s (%d items)\n", src.Name(), len(items))
	for _, it := range items {
		fmt.Printf("%-9s %-9s P:%-4s %s:%s  %s\n",
			it.Status, it.ID, it.Priority, "size", it.Effort, it.Title)
	}
	return 0
}

func cmdSuggest(args []string) int {
	fs := flag.NewFlagSet("suggest", flag.ContinueOnError)
	files := fs.String("files", "", "required comma-separated Markdown files")
	phase := fs.Int("phase", -1, "restrict to one phase (-1 = any)")
	top := fs.Int("top", backlog.DefaultCapacity, "max suggestions")
	maxEffort := fs.String("max-effort", "", "drop items larger than this size (S|M|L)")
	asJSON := fs.Bool("json", false, "emit JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	paths, err := parseFiles(*files)
	if err != nil {
		fmt.Fprintln(os.Stderr, "suggest:", err)
		return 2
	}

	src := backlog.NewMarkdownSource(paths...)
	res, err := backlog.NewSuggester(src).Suggest(context.Background(), backlog.Context{
		Phase:     *phase,
		Capacity:  *top,
		MaxEffort: parseEffortFlag(*maxEffort),
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "suggest: %v\n", err)
		return 1
	}

	if *asJSON {
		if code := encodeJSON(res); code != 0 {
			return code
		}
	} else {
		printResult(src.Name(), res)
	}

	return 0
}

// parseEffortFlag maps the --max-effort flag to an Effort; an empty or
// unrecognized value means "no cap".
func parseEffortFlag(v string) backlog.Effort {
	switch strings.ToUpper(strings.TrimSpace(v)) {
	case "S":
		return backlog.EffortSmall
	case "M":
		return backlog.EffortMedium
	case "L":
		return backlog.EffortLarge
	}
	return backlog.EffortUnknown
}

func printResult(srcName string, res backlog.Result) {
	fmt.Printf("# source: %s (%d items)\n", srcName, res.Total)
	fmt.Printf("\n## Suggested next (%d)\n", len(res.Suggested))
	for i, s := range res.Suggested {
		fmt.Printf("%d. %-9s [score %.3f] %s\n   %s\n",
			i+1, s.Item.ID, s.Score, s.Item.Title, s.Reason)
	}
	if len(res.Blocked) > 0 {
		fmt.Printf("\n## Blocked (%d)\n", len(res.Blocked))
		for _, s := range res.Blocked {
			fmt.Printf("- %-9s %s\n   blocked by: %s\n",
				s.Item.ID, s.Item.Title, strings.Join(s.Blockers, ", "))
		}
	}
}

func encodeJSON(v any) int {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		fmt.Fprintf(os.Stderr, "encode: %v\n", err)
		return 1
	}
	return 0
}
