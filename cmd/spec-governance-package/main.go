// Command spec-governance-package stages and validates the fixed portable
// SPEC governance distribution. It does not build, install, or activate it.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/vbonnet/dear-agent/internal/specpackage"
)

const operationTimeout = 2 * time.Minute

func main() {
	os.Exit(realMain())
}

func realMain() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, operationTimeout)
	defer cancel()
	return run(ctx, os.Args[1:], os.Stdout, os.Stderr)
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		writeUsage(stderr)
		return 2
	}
	switch args[0] {
	case "stage":
		return runStage(ctx, args[1:], stdout, stderr)
	case "validate":
		return runValidate(ctx, args[1:], stdout, stderr)
	case "help", "-h", "--help":
		writeUsage(stdout)
		return 0
	default:
		fmt.Fprintf(stderr, "spec-governance-package: unknown subcommand %q\n", args[0])
		writeUsage(stderr)
		return 2
	}
}

func runStage(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("spec-governance-package stage", flag.ContinueOnError)
	flags.SetOutput(stderr)
	source := flags.String("source", "", "clean absolute dear-agent source root")
	artifact := flags.String("artifact", "", "clean absolute prebuilt specaudit executable")
	parent := flags.String("staging-parent", "", "clean absolute caller-owned staging parent outside the source tree")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 || *source == "" || *artifact == "" || *parent == "" {
		fmt.Fprintln(stderr, "spec-governance-package stage: -source, -artifact, and -staging-parent are required")
		return 2
	}
	staged, err := specpackage.Stage(ctx, *source, *artifact, *parent)
	if err != nil {
		fmt.Fprintf(stderr, "spec-governance-package stage: %v\n", err)
		return 1
	}
	return writeStagedResult(stdout, stderr, staged)
}

func writeStagedResult(stdout, stderr io.Writer, staged specpackage.StagedPackage) int {
	if err := writeJSON(stdout, staged); err != nil {
		fmt.Fprintf(
			stderr,
			"spec-governance-package stage: write receipt failed; staged root retained at %q: %v\n",
			staged.Root,
			err,
		)
		return 1
	}
	return 0
}

func runValidate(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("spec-governance-package validate", flag.ContinueOnError)
	flags.SetOutput(stderr)
	root := flags.String("root", "", "clean absolute staged distribution root")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 || *root == "" {
		fmt.Fprintln(stderr, "spec-governance-package validate: -root is required")
		return 2
	}
	receipt, err := specpackage.Validate(ctx, *root)
	if err != nil {
		fmt.Fprintf(stderr, "spec-governance-package validate: %v\n", err)
		return 1
	}
	if err := writeJSON(stdout, receipt); err != nil {
		fmt.Fprintf(stderr, "spec-governance-package validate: write receipt failed: %v\n", err)
		return 1
	}
	return 0
}

func writeJSON(destination io.Writer, value any) error {
	var encoded bytes.Buffer
	encoder := json.NewEncoder(&encoded)
	encoder.SetEscapeHTML(true)
	if err := encoder.Encode(value); err != nil {
		return err
	}
	_, err := io.Copy(destination, &encoded)
	return err
}

func writeUsage(destination io.Writer) {
	fmt.Fprintln(destination, "Usage:")
	fmt.Fprintln(destination, "  spec-governance-package stage -source <absolute-root> -artifact <absolute-specaudit> -staging-parent <absolute-directory>")
	fmt.Fprintln(destination, "  spec-governance-package validate -root <absolute-distribution-root>")
}
