package main

import (
	"bytes"
	"context"
	"os/exec"
	"strings"
	"time"
)

// cmdResult is the captured outcome of an external command.
type cmdResult struct {
	stdout string
	stderr string
	err    error // non-nil if the command could not start or exited non-zero
}

// ok reports whether the command ran and exited 0.
func (r cmdResult) ok() bool { return r.err == nil }

// run executes name+args inside dir with a hard timeout, capturing output.
// It never panics: a missing binary or non-zero exit is returned as err so
// every collector can degrade gracefully instead of aborting the scan.
func run(dir string, timeout time.Duration, name string, args ...string) cmdResult {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	err := cmd.Run()
	return cmdResult{
		stdout: strings.TrimRight(out.String(), "\n"),
		stderr: strings.TrimRight(errb.String(), "\n"),
		err:    err,
	}
}

// haveBinary reports whether name resolves on PATH.
func haveBinary(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}
