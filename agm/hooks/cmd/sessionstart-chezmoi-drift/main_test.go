package main

import (
	"testing"
)

func TestCheckDoesNotPanic(t *testing.T) {
	// check() should never panic regardless of chezmoi state
	check()
}

func TestRunDoesNotPanic(t *testing.T) {
	// run() should never panic (never block session start)
	run()
}
