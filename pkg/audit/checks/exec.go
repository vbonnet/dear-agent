// Package checks ships the built-in audit checks. Each check is a
// small file that implements audit.Check, registers itself with
// audit.Default in init(), and ships an offline test under
// testdata/. See ADR-011 §D9 — a check that cannot be replayed
// offline is rejected at code review.
//
// The package intentionally has no exported surface beyond Register
// and a few testing helpers. Callers consume checks via the registry,
// not by importing them directly.
package checks

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"strings"
	"time"
)

// commandResult is the parsed output of one tool invocation. Every
// check that wraps a CLI tool feeds its raw process output through
// this struct so the parsing logic stays uniform across checks.
type commandResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
	Duration time.Duration
	Err      error
}

// runCommand invokes name with args in dir and returns a commandResult.
// Non-zero exit codes are NOT treated as Go errors — checks frequently
// rely on tool exit codes to signal "issues found", which is not the
// same as "the tool itself failed". The returned commandResult.Err is
// non-nil only when the binary could not start (missing binary,
// permission denied, ctx cancelled).
//
// Output is captured separately for stdout and stderr; total bytes are
// capped at 1 MiB per stream to bound memory for runaway tools.
func runCommand(ctx context.Context, dir, name string, args ...string) commandResult {
	const maxOutBytes = 1 << 20

	start := time.Now()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir

	stdout := &cappedBuffer{cap: maxOutBytes}
	stderr := &cappedBuffer{cap: maxOutBytes}
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	err := cmd.Run()
	res := commandResult{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		Duration: time.Since(start),
	}
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			res.ExitCode = exitErr.ExitCode()
			return res
		}
		res.Err = err
		return res
	}
	if state := cmd.ProcessState; state != nil {
		res.ExitCode = state.ExitCode()
	}
	return res
}

// cappedBuffer is a bytes.Buffer that silently drops writes past cap.
// We accept truncation rather than failing on huge outputs because a
// noisy tool (e.g. `go test ./... -v`) is a common case and we still
// want to record what we can.
type cappedBuffer struct {
	buf bytes.Buffer
	cap int
}

func (b *cappedBuffer) Write(p []byte) (int, error) {
	remaining := b.cap - b.buf.Len()
	if remaining <= 0 {
		return len(p), nil
	}
	if len(p) > remaining {
		p = p[:remaining]
	}
	return b.buf.Write(p)
}

func (b *cappedBuffer) String() string { return b.buf.String() }

// firstNonEmptyLine returns the first non-empty trimmed line of s, or
// "" if there is none. Used by checks to extract a one-line title
// from a multi-line tool output.
func firstNonEmptyLine(s string) string {
	for _, ln := range strings.Split(s, "\n") {
		t := strings.TrimSpace(ln)
		if t != "" {
			return t
		}
	}
	return ""
}
