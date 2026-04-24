package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/pkg/engram"
	"github.com/vbonnet/dear-agent/pkg/trigger"
)

// triggerCmd is the parent command for trigger subcommands.
var triggerCmd = &cobra.Command{
	Use:   "trigger",
	Short: "Manage engram triggers",
	Long:  "List, evaluate, and inspect engram trigger configurations.",
}

// triggerListCmd lists all registered triggers.
var triggerListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all registered triggers",
	Long:  "Scan engram directories and list all engrams with trigger definitions.",
	RunE:  runTriggerList,
}

// triggerEvaluateCmd simulates trigger evaluation.
var triggerEvaluateCmd = &cobra.Command{
	Use:   "evaluate <event-type>",
	Short: "Evaluate triggers for a simulated event",
	Long:  "Simulate an event and show which engrams would be triggered.",
	Args:  cobra.ExactArgs(1),
	RunE:  runTriggerEvaluate,
}

// triggerHistoryCmd shows recent injection history.
var triggerHistoryCmd = &cobra.Command{
	Use:   "history",
	Short: "Show recent trigger injection history",
	Long:  "Display the injection history from trigger-state.json.",
	RunE:  runTriggerHistory,
}

func init() {
	rootCmd.AddCommand(triggerCmd)
	triggerCmd.AddCommand(triggerListCmd)
	triggerCmd.AddCommand(triggerEvaluateCmd)
	triggerCmd.AddCommand(triggerHistoryCmd)

	triggerListCmd.Flags().StringP("path", "p", "engrams", "Path to engrams directory")
	triggerEvaluateCmd.Flags().StringP("path", "p", "engrams", "Path to engrams directory")
	triggerEvaluateCmd.Flags().StringSlice("data", nil, "Event data as key=value pairs")
	triggerHistoryCmd.Flags().String("state-file", "", "Path to trigger-state.json (default: .engram/trigger-state.json)")
}

// triggerEngramEntry holds parsed trigger info for display.
type triggerEngramEntry struct {
	path     string
	triggers []engram.TriggerSpec
}

// scanTriggeredEngrams walks the engram directory, parses files, and returns
// all engrams that have trigger definitions.
func scanTriggeredEngrams(engramPath string) ([]triggerEngramEntry, error) {
	parser := engram.NewParser()
	var entries []triggerEngramEntry

	err := filepath.Walk(engramPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip errors
		}
		if info.IsDir() || !strings.HasSuffix(path, ".ai.md") {
			return nil
		}

		eg, err := parser.Parse(path)
		if err != nil {
			return nil // skip unparseable files
		}

		if len(eg.Frontmatter.Triggers) > 0 {
			entries = append(entries, triggerEngramEntry{
				path:     path,
				triggers: eg.Frontmatter.Triggers,
			})
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scanning engrams: %w", err)
	}
	return entries, nil
}

// buildRegistryFromEntries populates a TriggerRegistry from scanned entries.
func buildRegistryFromEntries(entries []triggerEngramEntry) *trigger.TriggerRegistry {
	registry := trigger.NewTriggerRegistry()
	for _, e := range entries {
		registry.Register(e.path, e.triggers)
	}
	return registry
}

func runTriggerList(cmd *cobra.Command, args []string) error {
	engramPath, _ := cmd.Flags().GetString("path")

	entries, err := scanTriggeredEngrams(engramPath)
	if err != nil {
		return err
	}

	if len(entries) == 0 {
		fmt.Println("No triggered engrams found.")
		return nil
	}

	// Collect unique event types and group entries by event type.
	byEvent := make(map[string][]triggerEngramEntry)
	for _, e := range entries {
		for _, t := range e.triggers {
			byEvent[t.On] = append(byEvent[t.On], e)
		}
	}

	// Sort event types for stable output.
	eventTypes := make([]string, 0, len(byEvent))
	for et := range byEvent {
		eventTypes = append(eventTypes, et)
	}
	sort.Strings(eventTypes)

	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintf(w, "EVENT TYPE\tENGRAM PATH\tPRIORITY\tSCOPE\tCOOLDOWN\n")

	for _, et := range eventTypes {
		seen := make(map[string]bool)
		for _, e := range byEvent[et] {
			if seen[e.path] {
				continue
			}
			seen[e.path] = true
			for _, t := range e.triggers {
				if t.On != et {
					continue
				}
				priority := t.Priority
				if priority == 0 {
					priority = 50
				}
				scope := t.Scope
				if scope == "" {
					scope = "global"
				}
				cooldown := t.Cooldown
				if cooldown == "" {
					cooldown = "-"
				}
				fmt.Fprintf(w, "%s\t%s\t%d\t%s\t%s\n", et, e.path, priority, scope, cooldown)
			}
		}
	}
	w.Flush()

	fmt.Fprintf(os.Stderr, "\n%d triggered engram(s) across %d event type(s)\n", len(entries), len(eventTypes))
	return nil
}

func runTriggerEvaluate(cmd *cobra.Command, args []string) error {
	eventType := args[0]
	engramPath, _ := cmd.Flags().GetString("path")
	dataSlice, _ := cmd.Flags().GetStringSlice("data")

	// Parse --data key=value pairs.
	data := make(map[string]interface{})
	for _, kv := range dataSlice {
		parts := strings.SplitN(kv, "=", 2)
		if len(parts) == 2 {
			data[parts[0]] = parts[1]
		}
	}

	entries, err := scanTriggeredEngrams(engramPath)
	if err != nil {
		return err
	}

	registry := buildRegistryFromEntries(entries)
	matcher := trigger.NewTriggerMatcher(registry)

	event := trigger.TriggerEvent{
		Type: eventType,
		Data: data,
	}

	results := matcher.Match(event)

	if len(results) == 0 {
		fmt.Printf("No engrams matched event type %q\n", eventType)
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintf(w, "PRIORITY\tENGRAM PATH\tSCOPE\tCOOLDOWN\n")
	for _, r := range results {
		scope := r.Trigger.Scope
		if scope == "" {
			scope = "global"
		}
		cooldown := r.Trigger.Cooldown
		if cooldown == "" {
			cooldown = "-"
		}
		fmt.Fprintf(w, "%d\t%s\t%s\t%s\n", r.Priority, r.EngramPath, scope, cooldown)
	}
	w.Flush()

	fmt.Fprintf(os.Stderr, "\n%d engram(s) matched event %q\n", len(results), eventType)
	return nil
}

func runTriggerHistory(cmd *cobra.Command, args []string) error {
	stateFile, _ := cmd.Flags().GetString("state-file")
	if stateFile == "" {
		stateFile = filepath.Join(".engram", "trigger-state.json")
	}

	state, err := trigger.LoadTriggerState(stateFile)
	if err != nil {
		return fmt.Errorf("loading trigger state: %w", err)
	}

	if len(state.LastInjected) == 0 {
		fmt.Println("No injection history found.")
		return nil
	}

	// Sort by timestamp descending (most recent first).
	type historyEntry struct {
		path string
		ts   time.Time
	}
	history := make([]historyEntry, 0, len(state.LastInjected))
	for path, ts := range state.LastInjected {
		history = append(history, historyEntry{path: path, ts: ts})
	}
	sort.Slice(history, func(i, j int) bool {
		return history[i].ts.After(history[j].ts)
	})

	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintf(w, "ENGRAM PATH\tLAST INJECTED\n")
	for _, h := range history {
		fmt.Fprintf(w, "%s\t%s\n", h.path, h.ts.Format(time.RFC3339))
	}
	w.Flush()

	fmt.Fprintf(os.Stderr, "\n%d injection(s) recorded\n", len(history))
	return nil
}
