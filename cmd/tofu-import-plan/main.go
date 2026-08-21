// Command tofu-import-plan turns recorded evidence about GitHub and OpenTofu
// state into a deterministic import plan.
//
// It exists so infra/import.sh can stay a script. Every decision that would
// otherwise be Bash — validating the evaluated inventory, resolving which
// ruleset is safe to import, proving an existing state address is bound to the
// object the plan expects, classifying a provider error as "absent" rather
// than "broken" — happens here, under unit test, in internal/tofuimport.
//
// The command touches no network and mutates no state. It reads files the
// script collected and writes a plan the script executes.
//
//	repos     print the active repositories, so the caller knows what to collect
//	plan      print the import plan as tab-separated records
//	classify  decide whether a failed `tofu import` merely means "absent"
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/vbonnet/dear-agent/internal/tofuimport"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "tofu-import-plan: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string, stdout io.Writer) error {
	if len(args) == 0 {
		return errors.New("expected a subcommand: repos, plan or classify")
	}
	switch args[0] {
	case "repos":
		return runRepos(args[1:], stdout)
	case "plan":
		return runPlan(args[1:], stdout)
	case "classify":
		return runClassify(args[1:], stdout)
	default:
		return fmt.Errorf("unknown subcommand %q; expected repos, plan or classify", args[0])
	}
}

// runRepos prints the active repositories, one per line, so the script knows
// which ruleset listings to collect before planning.
func runRepos(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("repos", flag.ContinueOnError)
	inventoryPath := fs.String("inventory", "", "path to the JSON emitted by `tofu console` (required)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	inventory, err := readInventory(*inventoryPath)
	if err != nil {
		return err
	}
	for _, repo := range inventory.Active {
		fmt.Fprintln(stdout, repo)
	}
	return nil
}

// runPlan prints the import plan. Every identity is resolved before the first
// record is written, so a plan that prints at all is a plan that is safe to
// execute end to end.
func runPlan(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("plan", flag.ContinueOnError)
	inventoryPath := fs.String("inventory", "", "path to the JSON emitted by `tofu console` (required)")
	statePath := fs.String("state", "", "path to `tofu show -json` output; an empty or absent file is an empty state")
	canonicalPath := fs.String("canonical-ruleset", "", "path to .github/rulesets/main.json (required)")
	rulesetsDir := fs.String("rulesets-dir", "", "directory holding <repo>.json ruleset listings (required)")
	asJSON := fs.Bool("json", false, "emit JSON instead of tab-separated records")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *canonicalPath == "" || *rulesetsDir == "" {
		return errors.New("--canonical-ruleset and --rulesets-dir are required")
	}

	inventory, err := readInventory(*inventoryPath)
	if err != nil {
		return err
	}

	canonicalRaw, err := os.ReadFile(*canonicalPath)
	if err != nil {
		return fmt.Errorf("read canonical ruleset: %w", err)
	}
	canonicalName, err := tofuimport.CanonicalRulesetName(canonicalRaw)
	if err != nil {
		return err
	}

	stateRaw, err := readOptionalFile(*statePath)
	if err != nil {
		return fmt.Errorf("read state: %w", err)
	}
	state, err := tofuimport.ParseState(stateRaw)
	if err != nil {
		return err
	}

	pages := map[string][]byte{}
	for _, repo := range inventory.Active {
		raw, readErr := os.ReadFile(filepath.Join(*rulesetsDir, repo+".json"))
		if readErr != nil {
			// A missing listing is not evidence of absence; it is evidence
			// that the collection step failed.
			return fmt.Errorf("read ruleset listing for %s: %w", repo, readErr)
		}
		pages[repo] = raw
	}

	steps, err := tofuimport.BuildPlan(tofuimport.Evidence{
		Inventory:            inventory,
		State:                state,
		CanonicalRulesetName: canonicalName,
		RulesetPages:         pages,
	})
	if err != nil {
		return err
	}

	if *asJSON {
		encoded, encodeErr := tofuimport.EncodePlanJSON(steps)
		if encodeErr != nil {
			return encodeErr
		}
		fmt.Fprintln(stdout, string(encoded))
		return nil
	}
	fmt.Fprint(stdout, tofuimport.EncodePlan(steps))
	return nil
}

// runClassify exits 0 when a failed import merely means the remote object does
// not exist yet, and non-zero when it is a real failure the caller must stop
// on.
func runClassify(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("classify", flag.ContinueOnError)
	outputPath := fs.String("provider-output", "", "path to the failed import's combined provider output (required)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *outputPath == "" {
		return errors.New("--provider-output is required")
	}
	raw, err := os.ReadFile(*outputPath)
	if err != nil {
		return fmt.Errorf("read provider output: %w", err)
	}
	if !tofuimport.IsBenignImportFailure(string(raw)) {
		return errors.New("provider failure is not a recognized absent-object message")
	}
	fmt.Fprintln(stdout, "absent")
	return nil
}

func readInventory(path string) (tofuimport.Inventory, error) {
	if path == "" {
		return tofuimport.Inventory{}, errors.New("--inventory is required")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return tofuimport.Inventory{}, fmt.Errorf("read inventory: %w", err)
	}
	return tofuimport.ParseInventory(raw)
}

// readOptionalFile treats an unset path or a missing file as empty, which is
// the normal first-run state, and propagates every other error.
func readOptionalFile(path string) ([]byte, error) {
	if path == "" {
		return nil, nil
	}
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	return raw, err
}
