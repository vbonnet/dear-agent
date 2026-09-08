package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunRejectsInvalidRepositoryAsUsageError(t *testing.T) {
	var stdout, stderr bytes.Buffer
	status := run([]string{"-repo", "../repository"}, func(string) string { return "" }, &stdout, &stderr)
	if status != 2 {
		t.Fatalf("status = %d, want 2", status)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	if !strings.Contains(stderr.String(), "invalid path segment") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestRunRejectsUnexpectedArgumentsBeforeProviderCall(t *testing.T) {
	var stdout, stderr bytes.Buffer
	status := run([]string{"-repo", "owner/repository", "unexpected"}, func(string) string { return "" }, &stdout, &stderr)
	if status != 2 {
		t.Fatalf("status = %d, want 2", status)
	}
	if !strings.Contains(stderr.String(), "Usage: security-audit-issues") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}
