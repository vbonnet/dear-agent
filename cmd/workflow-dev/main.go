// Command workflow-dev is the interactive dev shell for workflows.
// Reads a workflow YAML, defaults to a fixture-backed mock executor
// for sub-second iteration, and exposes verbs for running, retrying
// individual nodes, diffing successive runs, and reloading from disk.
//
// Usage:
//
//	workflow-dev [--fixtures path] [--watch] <workflow.yaml>
//
// Flags:
//
//	--fixtures path  override the conventional path (workflow.fixtures.yaml)
//	--watch          hot-reload on workflow or fixtures file change
//	--debounce dur   debounce interval for the watcher (default 200ms)
//
// Verbs (typed at the dev> prompt):
//
//	r [--live]       run the workflow (mock by default)
//	retry <node>     re-run a single node, replaying upstream outputs
//	diff <node>      diff this run's output against the prior run
//	approve <id>     placeholder; real HITL needs workflow-approve + a SQLite db
//	reload           re-read workflow YAML and fixtures from disk
//	fixtures         list fixture node ids
//	nodes            list workflow node ids
//	history          print run history
//	help             show this list
//	exit             leave the shell
//
// Exit codes: 0 = clean shutdown, 1 = startup error, 2 = bad usage.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/vbonnet/dear-agent/pkg/workflow/dev"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, in io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("workflow-dev", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var (
		fixtures = fs.String("fixtures", "", "path to fixtures YAML (default: <workflow>.fixtures.yaml)")
		watch    = fs.Bool("watch", false, "hot-reload on workflow or fixtures file change")
		debounce = fs.Duration("debounce", 200*time.Millisecond, "watcher debounce interval")
	)
	fs.Usage = func() {
		fmt.Fprintln(stderr, "Usage: workflow-dev [--fixtures path] [--watch] <workflow.yaml>")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return 2
	}
	wfPath := fs.Arg(0)

	sess, err := dev.NewSession(wfPath, *fixtures)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if *watch {
		paths := []string{wfPath, sess.FixturesPath}
		go func() {
			err := dev.HotReload(ctx, paths, dev.WatchOptions{Debounce: *debounce}, func(p string) {
				n, f, err := sess.Reload()
				if err != nil {
					fmt.Fprintf(stdout, "[watch] reload failed (%s): %v\n", p, err)
					return
				}
				fmt.Fprintf(stdout, "[watch] reloaded after %s: %d node(s), %d fixture(s)\n", p, n, f)
			})
			if err != nil && ctx.Err() == nil {
				fmt.Fprintf(stderr, "[watch] %v\n", err)
			}
		}()
	}

	if err := dev.REPL(ctx, sess, in, stdout); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}
