package main

import (
	"bytes"
	"testing"
)

func TestRunRejectsInvalidAsOf(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer
	if got := run([]string{"-as-of", "not-a-date"}, &stdout, &stderr); got != 2 {
		t.Fatalf("run exit = %d, want 2; stderr=%q", got, stderr.String())
	}
}
