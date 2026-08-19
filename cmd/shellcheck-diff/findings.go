package main

import (
	"encoding/json"
	"fmt"
)

// Finding is one ShellCheck comment from a JSON1 document.
type Finding struct {
	File    string `json:"file"`
	Line    int    `json:"line"`
	Column  int    `json:"column"`
	Level   string `json:"level"`
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// json1Document is ShellCheck's -f json1 envelope. The older -f json format is
// a bare array; accepting only the envelope keeps the contract single-valued,
// and parseFindings rejects the bare array with an actionable message rather
// than silently reporting zero findings.
type json1Document struct {
	Comments *[]Finding `json:"comments"`
}

// parseFindings reads a ShellCheck JSON1 document. An absent or malformed
// document is an error, never an empty finding set: a gate that reports "clean"
// because it could not read its own input is worse than no gate.
func parseFindings(raw []byte) ([]Finding, error) {
	var doc json1Document
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("findings are not a ShellCheck JSON1 document (use -f json1): %w", err)
	}
	if doc.Comments == nil {
		return nil, fmt.Errorf("findings document has no \"comments\" array (use -f json1, not -f json)")
	}
	for i, f := range *doc.Comments {
		if f.File == "" {
			return nil, fmt.Errorf("finding %d has no file", i)
		}
		if f.Line < 1 {
			return nil, fmt.Errorf("finding %d in %s has a non-positive line %d", i, f.File, f.Line)
		}
		// Validate the level here rather than when filtering. An
		// unrecognized level is evidence that ShellCheck's output contract
		// changed; dropping such a finding later would silently shrink the
		// gate exactly when it is least understood.
		if _, err := severityRank(f.Level); err != nil {
			return nil, fmt.Errorf("finding %d in %s: %w", i, f.File, err)
		}
	}
	return *doc.Comments, nil
}
