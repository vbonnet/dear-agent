// Command backlog-suggest ranks the declared backlog (BACKLOG.md +
// ROADMAP.md) and suggests what to pick up next. It is the CLI surface of
// pkg/backlog and the executable form of the VROOM Orchestrator's
// deterministic dispatch scan (agm ADR-023). See docs/adr/ADR-022.
//
//	backlog-suggest list                     # every parsed item
//	backlog-suggest suggest                   # top-N eligible + blocked
//	backlog-suggest suggest --phase 6 --top 1 --emit-vroom
//
// Exit codes: 0 success, 1 runtime error, 2 usage error.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/vbonnet/dear-agent/pkg/backlog"
)

// defaultFiles is the standard backlog source set, resolved relative to the
// current working directory.
var defaultFiles = []string{"docs/workflow-engine/BACKLOG.md", "ROADMAP.md"}

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

// parseFiles splits a comma-separated --files value, falling back to the
// default set when empty.
func parseFiles(v string) []string {
	if strings.TrimSpace(v) == "" {
		return defaultFiles
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func cmdList(args []string) int {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	files := fs.String("files", "", "comma-separated markdown files (default: BACKLOG.md,ROADMAP.md)")
	asJSON := fs.Bool("json", false, "emit JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	src := backlog.NewMarkdownSource(parseFiles(*files)...)
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
	files := fs.String("files", "", "comma-separated markdown files (default: BACKLOG.md,ROADMAP.md)")
	phase := fs.Int("phase", -1, "restrict to one phase (-1 = any)")
	top := fs.Int("top", backlog.DefaultCapacity, "max suggestions")
	maxEffort := fs.String("max-effort", "", "drop items larger than this size (S|M|L)")
	asJSON := fs.Bool("json", false, "emit JSON")
	emit := fs.Bool("emit-vroom", false, "publish a vroom.decision.dispatched event for the top pick")
	vroomOut := fs.String("vroom-out", ".dear-agent/vroom-decisions.jsonl", "JSONL file for the VROOM decision event")
	worker := fs.String("worker", "", "worker hint recorded on the dispatch event")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	src := backlog.NewMarkdownSource(parseFiles(*files)...)
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

	if *emit && len(res.Suggested) > 0 {
		if err := emitDispatch(*vroomOut, *worker, res.Suggested[0]); err != nil {
			fmt.Fprintf(os.Stderr, "emit-vroom: %v\n", err)
			return 1
		}
		fmt.Fprintf(os.Stderr, "emitted vroom.decision.dispatched for %s -> %s\n",
			res.Suggested[0].Item.ID, *vroomOut)
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

// emitDispatch writes the dispatch decision through a JSONL-backed VROOM
// publisher so the pickup is on the append-only decision trail.
func emitDispatch(outPath, worker string, s backlog.Suggestion) error {
	pub, err := newJSONLPublisher(outPath)
	if err != nil {
		return err
	}
	defer func() { _ = pub.Close() }()
	return backlog.NewOrchestratorNotifier(pub).WithWorker(worker).Dispatch(s)
}

// jsonlPublisher implements vroom.EventPublisher by appending one JSON
// object per event to a file. It mirrors eventbus.JSONLSink's shape but
// speaks the vroom.EventPublisher (topic, data) contract directly.
type jsonlPublisher struct {
	f *os.File
}

func newJSONLPublisher(path string) (*jsonlPublisher, error) {
	if dir := filepath.Dir(path); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, fmt.Errorf("create dir: %w", err)
		}
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600) //nolint:gosec // operator-supplied CLI flag
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	return &jsonlPublisher{f: f}, nil
}

// Publish implements vroom.EventPublisher.
func (p *jsonlPublisher) Publish(topic string, data map[string]interface{}) error {
	rec := map[string]interface{}{"topic": topic}
	for k, v := range data {
		rec[k] = v
	}
	line, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}
	if _, err := p.f.Write(append(line, '\n')); err != nil {
		return fmt.Errorf("write event: %w", err)
	}
	return nil
}

func (p *jsonlPublisher) Close() error {
	return p.f.Close()
}
