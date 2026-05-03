// Command workflow-codemod transforms workflow YAML files between
// schema generations. Two modes ship today:
//
//	workflow-codemod upgrade [--write] [--add-budget] [--drop-model] <file>...
//	    Upgrade v0.1 workflows to v0.2 (schema_version, role-from-model,
//	    optional default budget). The default is dry-run; --write
//	    overwrites the input file in place.
//
//	workflow-codemod from-wayfinder --out <file> <wayfinder-session.yaml>
//	    Synthesise a workflow YAML from a Wayfinder session. Each
//	    roadmap phase becomes one bash node, preserving phase order
//	    via depends edges.
//
// Exit codes: 0 = ok, 1 = transformation error, 2 = bad usage.
//
// Both modes intentionally avoid magic: they print the per-file change
// list to stdout (in dry-run mode) before any disk write, so the
// operator sees exactly what would change. The codemod is a migration
// aid, not a black box.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/vbonnet/dear-agent/pkg/workflow/codemod"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run is the testable entry point. Returns the exit code rather than
// calling os.Exit so tests can drive the binary in-process.
func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printUsage(stderr)
		return 2
	}
	switch args[0] {
	case "upgrade":
		return runUpgrade(args[1:], stdout, stderr)
	case "from-wayfinder":
		return runFromWayfinder(args[1:], stdout, stderr)
	case "-h", "--help", "help":
		printUsage(stdout)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown subcommand %q\n", args[0])
		printUsage(stderr)
		return 2
	}
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  workflow-codemod upgrade [--write] [--add-budget] [--drop-model] <file>...")
	fmt.Fprintln(w, "  workflow-codemod from-wayfinder --out <file> <wayfinder-session.yaml>")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Run --help on a subcommand for full flag listings.")
}

func runUpgrade(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("upgrade", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var (
		write      = fs.Bool("write", false, "overwrite the input files in place (default: dry-run to stdout)")
		addBudget  = fs.Bool("add-budget", false, "insert a default budget block on AI nodes that lack one")
		dropModel  = fs.Bool("drop-model", false, "remove the model: field once role: is added (default: keep both for back-compat)")
	)
	fs.Usage = func() {
		fmt.Fprintln(stderr, "Usage: workflow-codemod upgrade [--write] [--add-budget] [--drop-model] <file>...")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() == 0 {
		fs.Usage()
		return 2
	}
	opts := codemod.UpgradeOptions{
		AddDefaultBudget:         *addBudget,
		DropModelOnRolePromotion: *dropModel,
	}
	failed := false
	for _, path := range fs.Args() {
		in, err := os.ReadFile(path) //nolint:gosec // path is a CLI arg
		if err != nil {
			fmt.Fprintln(stderr, err)
			failed = true
			continue
		}
		r, err := codemod.UpgradeV01ToV02(in, opts)
		if err != nil {
			fmt.Fprintf(stderr, "%s: %v\n", path, err)
			failed = true
			continue
		}
		r.Path = path
		if err := codemod.WriteResult(stdout, r); err != nil {
			fmt.Fprintln(stderr, err)
			failed = true
			continue
		}
		if r.Changed() && *write {
			// Match the source file's mode so chmod state is preserved
			// across the rewrite. WriteFile alone would default to
			// 0600.
			info, err := os.Stat(path)
			mode := os.FileMode(0o644)
			if err == nil {
				mode = info.Mode().Perm()
			}
			if err := os.WriteFile(path, r.Output, mode); err != nil {
				fmt.Fprintln(stderr, err)
				failed = true
				continue
			}
			fmt.Fprintf(stdout, "  wrote %s\n", path)
		}
	}
	if failed {
		return 1
	}
	return 0
}

func runFromWayfinder(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("from-wayfinder", flag.ContinueOnError)
	fs.SetOutput(stderr)
	out := fs.String("out", "", "output workflow YAML path (required; '-' for stdout)")
	fs.Usage = func() {
		fmt.Fprintln(stderr, "Usage: workflow-codemod from-wayfinder --out <file> <wayfinder-session.yaml>")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 1 || *out == "" {
		fs.Usage()
		return 2
	}
	src := fs.Arg(0)
	in, err := os.ReadFile(src) //nolint:gosec // path is a CLI arg
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	r, err := codemod.FromWayfinder(in, src)
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", src, err)
		return 1
	}
	if *out == "-" {
		if _, err := stdout.Write(r.Output); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
	} else {
		if err := os.WriteFile(*out, r.Output, 0o600); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
	}
	if err := codemod.WriteResult(stdout, r); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}
