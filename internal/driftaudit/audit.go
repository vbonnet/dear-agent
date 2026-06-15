// Package driftaudit appends deployment-drift findings to a JSONL audit log,
// following the same durable-trail contract as safe-pr and src-recovery: every
// run leaves one line at ~/.local/state/dear-agent/drift-check.log, whether it
// found drift or not. A clean run is itself evidence ("we checked, nothing
// drifted") and the stream feeds the daily ops audit and OTel exporters.
package driftaudit

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// LogName is the audit file written under ~/.local/state/dear-agent/.
const LogName = "drift-check.log"

// DriftTarget is the per-target detail recorded for a drifted artifact. Only
// actionable targets (drift or required-missing) are recorded, keeping the log
// signal-dense.
type DriftTarget struct {
	Name        string `json:"name"`
	Deployed    string `json:"deployed"`
	Source      string `json:"source"`
	Status      string `json:"status"`
	Remediation string `json:"remediation,omitempty"`
}

// Record is one JSONL line: the summary of a drift-check run plus the list of
// artifacts that need attention.
type Record struct {
	Time     string        `json:"time"` // RFC3339
	HasDrift bool          `json:"has_drift"`
	Total    int           `json:"total"`
	OK       int           `json:"ok"`
	Drift    int           `json:"drift"`
	Missing  int           `json:"missing"`
	Skipped  int           `json:"skipped"`
	Errors   int           `json:"errors"`
	Drifted  []DriftTarget `json:"drifted,omitempty"`
	GitRef   string        `json:"git_ref,omitempty"`
	BeadID   string        `json:"bead_id,omitempty"` // set when a drift bead is filed
}

// Append writes rec as one line to ~/.local/state/dear-agent/drift-check.log,
// creating the directory on first use. Audit failures are returned, not fatal:
// the caller decides whether a failed log write should fail the run (it should
// not — a missed audit line must never hide real drift).
func Append(home string, rec Record) (err error) {
	if home == "" {
		home = "/tmp"
	}
	dir := filepath.Join(home, ".local", "state", "dear-agent")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("cannot create audit log dir %s: %w", dir, err)
	}
	f, err := os.OpenFile(filepath.Join(dir, LogName), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("cannot open audit log: %w", err)
	}
	// A writable handle can surface deferred write errors (flush failures, full
	// disk) on Close. Capture that error but never let it mask an earlier write
	// error.
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("cannot close audit log: %w", cerr)
		}
	}()
	line, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("cannot marshal audit record: %w", err)
	}
	if _, err = f.Write(append(line, '\n')); err != nil {
		return err
	}
	return nil
}
