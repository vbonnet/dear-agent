package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/vbonnet/dear-agent/agm/internal/ops"
)

// isJSONOutput returns true if the user requested JSON output via --output json.
func isJSONOutput() bool {
	return outputFormat == "json"
}

// printJSON marshals the value to JSON and prints it to stdout.
// If --fields is set, applies field mask filtering for token efficiency.
func printJSON(v any) error {
	if len(fieldsFlag) > 0 {
		filtered, err := ops.ApplyFieldMask(v, fieldsFlag)
		if err != nil {
			return fmt.Errorf("field mask error: %w", err)
		}
		fmt.Println(string(filtered))
		return nil
	}

	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("JSON marshal error: %w", err)
	}
	fmt.Println(string(data))
	return nil
}

// printResult outputs the result in the appropriate format (text or JSON).
// For JSON mode, it marshals the result. For text mode, it calls the textFn.
func printResult(result any, textFn func()) error {
	if isJSONOutput() {
		return printJSON(result)
	}
	textFn()
	return nil
}

// printOpError outputs an OpError in the appropriate format.
// JSON mode: structured RFC 7807 error. Text mode: human-readable with suggestions.
func printOpError(err *ops.OpError) {
	if isJSONOutput() {
		fmt.Fprintln(os.Stderr, string(err.JSON()))
		return
	}

	fmt.Fprintf(os.Stderr, "Error [%s]: %s\n", err.Code, err.Title)
	fmt.Fprintf(os.Stderr, "  %s\n", err.Detail)
	if len(err.Suggestions) > 0 {
		fmt.Fprintln(os.Stderr, "\nSuggestions:")
		for _, s := range err.Suggestions {
			fmt.Fprintf(os.Stderr, "  • %s\n", s)
		}
	}
}

// handleError checks if an error is an OpError and formats it appropriately.
// Returns the error for Cobra to handle exit codes.
func handleError(err error) error {
	if err == nil {
		return nil
	}
	var opErr *ops.OpError
	if errors.As(err, &opErr) {
		printOpError(opErr)
		return fmt.Errorf("%s", opErr.Code)
	}
	return err
}

// newOpContext creates an OpContext from the current CLI state.
// This wires global CLI flags into the ops layer.
// DryRun is not set here — individual commands set it from their own --dry-run flag.
func newOpContext() *ops.OpContext {
	return &ops.OpContext{
		Fields:     fieldsFlag,
		OutputMode: outputFormat,
		Tmux:       tmuxClient,
		Manager:    managerBackend,
	}
}

// newOpContextWithStorage creates an OpContext with Dolt storage.
// Returns a cleanup function that must be deferred to close the storage connection.
// DryRun is not set here — individual commands set it from their own --dry-run flag.
func newOpContextWithStorage() (*ops.OpContext, func(), error) {
	adapter, err := getStorage()
	if err != nil {
		return nil, func() {}, err
	}

	cleanup := func() { adapter.Close() }

	return &ops.OpContext{
		Storage:    adapter,
		Fields:     fieldsFlag,
		OutputMode: outputFormat,
		Tmux:       tmuxClient,
		Manager:    managerBackend,
	}, cleanup, nil
}
