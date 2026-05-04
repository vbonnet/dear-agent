// Command code-intel provides CLI access to the language detection registry
// and tiered verification checks.
//
// Usage:
//
//	code-intel detect-languages [path]
//	code-intel check [--tier 0|1|auto] [--changed-files f1,f2] [--format json|text] [path]
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/vbonnet/dear-agent/pkg/codeintel"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "detect-languages":
		dir := "."
		if len(os.Args) > 2 {
			dir = os.Args[2]
		}
		if err := detectLanguages(dir); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	case "check":
		if err := runCheck(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		usage()
		os.Exit(1)
	}
}

func detectLanguages(dir string) error {
	reg, err := codeintel.NewRegistry(dir)
	if err != nil {
		return fmt.Errorf("loading registry: %w", err)
	}

	langs := reg.DetectLanguages(dir)
	if len(langs) == 0 {
		fmt.Println("No languages detected.")
		return nil
	}

	fmt.Printf("Detected %d language(s) in %s:\n\n", len(langs), dir)
	for _, spec := range langs {
		tier := codeintel.DetectAvailableTier(spec)
		fmt.Printf("  %-12s  tier=%d  manifests=%s\n",
			spec.Name, tier, strings.Join(spec.ManifestFiles, ","))
	}
	return nil
}

//nolint:gocyclo // reason: linear CLI driver dispatching to many independent check helpers
func runCheck(args []string) error {
	dir := "."
	format := "text"
	tier := -1 // auto-detect
	var changedFiles []string

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--tier":
			i++
			if i >= len(args) {
				return fmt.Errorf("--tier requires a value (0, 1, or auto)")
			}
			if args[i] == "auto" {
				tier = -1
			} else {
				t, err := strconv.Atoi(args[i])
				if err != nil || t < 0 || t > 1 {
					return fmt.Errorf("--tier must be 0, 1, or auto")
				}
				tier = t
			}
		case "--changed-files":
			i++
			if i >= len(args) {
				return fmt.Errorf("--changed-files requires a value")
			}
			changedFiles = strings.Split(args[i], ",")
		case "--format":
			i++
			if i >= len(args) {
				return fmt.Errorf("--format requires a value (json or text)")
			}
			format = args[i]
			if format != "json" && format != "text" {
				return fmt.Errorf("--format must be json or text")
			}
		default:
			if strings.HasPrefix(args[i], "-") {
				return fmt.Errorf("unknown flag: %s", args[i])
			}
			dir = args[i]
		}
	}

	result, err := codeintel.RunChecks(dir, changedFiles, tier)
	if err != nil {
		return err
	}

	if format == "json" {
		return printJSON(result)
	}
	return printText(result)
}

func printJSON(result *codeintel.Tier1Result) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(result)
}

func printText(result *codeintel.Tier1Result) error {
	tierLabel := fmt.Sprintf("Tier %d", result.Tier)
	if len(result.Languages) > 0 {
		fmt.Printf("Languages: %s\n", strings.Join(result.Languages, ", "))
	}
	fmt.Printf("Running %s checks\n\n", tierLabel)

	allPassed := true
	for _, c := range result.Checks {
		status := "PASS"
		if !c.Passed {
			status = "FAIL"
			allPassed = false
		}
		fmt.Printf("[%s] %s (%s): %s\n", status, c.Check, c.Severity, c.Message)
		for _, d := range c.Details {
			fmt.Printf("       %s\n", d)
		}
	}

	fmt.Println()
	if allPassed {
		fmt.Printf("All %s checks passed.\n", tierLabel)
	} else {
		fmt.Printf("Some %s checks failed.\n", tierLabel)
		os.Exit(2)
	}
	return nil
}

func usage() {
	fmt.Fprintln(os.Stderr, "Usage: code-intel <command> [args]")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Commands:")
	fmt.Fprintln(os.Stderr, "  detect-languages [path]    Detect languages in a project directory")
	fmt.Fprintln(os.Stderr, "  check [flags] [path]       Run verification checks")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Check flags:")
	fmt.Fprintln(os.Stderr, "  --tier 0|1|auto            Verification tier (default: auto-detect)")
	fmt.Fprintln(os.Stderr, "  --changed-files f1,f2      Comma-separated changed files")
	fmt.Fprintln(os.Stderr, "  --format json|text         Output format (default: text)")
}
