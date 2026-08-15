package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"

	"github.com/vbonnet/dear-agent/internal/specguard"
)

const maxGuardJSONOutputBytes = 8 * 1024 * 1024

const (
	maxGuardCLIArgs     = 16
	maxGuardCLIArgBytes = 8192
)

// runGuard is a thin CLI adapter; internal/specguard owns the complete policy.
func runGuard(args []string, stdout, _ io.Writer) int {
	flags := flag.NewFlagSet("specaudit guard", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	repository := flags.String("repo", ".", "Git repository root or descendant")
	staged := flags.Bool("staged", false, "validate the staged Git index against HEAD")
	base := flags.String("base", "", "validate the committed base..HEAD range")
	argumentBytes := 0
	for _, argument := range args {
		argumentBytes += len(argument)
	}
	parseErr := flag.ErrHelp
	if len(args) <= maxGuardCLIArgs && argumentBytes <= maxGuardCLIArgBytes {
		parseErr = flags.Parse(args)
	}

	mode := specguard.Mode("")
	if parseErr == nil && flags.NArg() == 0 {
		switch {
		case *staged && *base == "":
			mode = specguard.ModeStaged
		case !*staged && *base != "":
			mode = specguard.ModeCommitted
		}
	}
	result := specguard.Evaluate(context.Background(), specguard.Request{
		Repository: *repository,
		Mode:       mode,
		Base:       *base,
	})
	encoded, emitted, err := encodeGuardResult(result, maxGuardJSONOutputBytes)
	if err != nil {
		return 1
	}
	if err := writeGuardJSON(stdout, encoded); err != nil {
		return 1
	}
	if emitted.Decision == specguard.DecisionBlock {
		return 1
	}
	return 0
}

func encodeGuardResult(result specguard.Result, outputLimit int) ([]byte, specguard.Result, error) {
	if outputLimit <= 0 {
		return nil, specguard.Result{}, fmt.Errorf("guard JSON output limit must be positive")
	}
	encoded, err := json.MarshalIndent(result, "", "  ")
	if err == nil && len(encoded)+1 <= outputLimit {
		return append(encoded, '\n'), result, nil
	}

	fallback := specguard.Result{
		SchemaVersion: specguard.SchemaVersion,
		Scope:         specguard.GuardScope,
		Decision:      specguard.DecisionBlock,
		Mode:          "",
		Source:        "",
		Changed:       []string{},
		Findings: []specguard.Finding{{
			Code:    "json-output-limit",
			Message: "guard result could not be emitted within its safety limit",
		}},
		EvidenceClaim: specguard.EvidenceClaim,
		TrustBoundary: specguard.TrustBoundary,
	}
	encoded, err = json.Marshal(fallback)
	if err != nil {
		return nil, fallback, fmt.Errorf("encode fail-closed guard result: %w", err)
	}
	if len(encoded)+1 > outputLimit {
		return nil, fallback, fmt.Errorf("fail-closed guard result exceeds its JSON output limit")
	}
	return append(encoded, '\n'), fallback, nil
}

func writeGuardJSON(writer io.Writer, encoded []byte) error {
	written, err := writer.Write(encoded)
	if err != nil {
		return err
	}
	if written != len(encoded) {
		return io.ErrShortWrite
	}
	return nil
}
