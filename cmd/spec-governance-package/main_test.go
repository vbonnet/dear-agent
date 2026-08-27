package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/internal/specpackage"
)

type failingReceiptWriter struct {
	err error
}

func (writer failingReceiptWriter) Write([]byte) (int, error) {
	return 0, writer.err
}

func TestRunRejectsIncompleteStageWithoutStdout(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := run(context.Background(), []string{"stage", "-source", "/tmp/source"}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("run() exit = %d, want 2", code)
	}
	if stdout.Len() != 0 {
		t.Fatalf("run() stdout = %q, want empty", stdout.String())
	}
	if !strings.Contains(stderr.String(), "are required") {
		t.Fatalf("run() stderr = %q, want required-arguments diagnostic", stderr.String())
	}
}

func TestRunRejectsUnknownSubcommand(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := run(context.Background(), []string{"install"}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("run() exit = %d, want 2", code)
	}
	if stdout.Len() != 0 {
		t.Fatalf("run() stdout = %q, want empty", stdout.String())
	}
	if !strings.Contains(stderr.String(), "unknown subcommand") {
		t.Fatalf("run() stderr = %q, want unknown-subcommand diagnostic", stderr.String())
	}
}

func TestRunHelpDescribesOnlyStageAndValidate(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := run(context.Background(), []string{"help"}, &stdout, &stderr)
	if code != 0 || stderr.Len() != 0 {
		t.Fatalf("run() = (%d, %q), want (0, empty stderr)", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), " stage ") || !strings.Contains(stdout.String(), " validate ") {
		t.Fatalf("help = %q, want stage and validate", stdout.String())
	}
	for _, forbidden := range []string{"build", "install", "activate"} {
		if strings.Contains(stdout.String(), " "+forbidden+" ") {
			t.Fatalf("help unexpectedly advertises %q: %q", forbidden, stdout.String())
		}
	}
}

func TestWriteStagedResultReportsRetainedRootWhenReceiptDeliveryFails(t *testing.T) {
	deliveryError := errors.New("closed output")
	staged := specpackage.StagedPackage{Root: "/private/tmp/staged-root"}
	var stderr bytes.Buffer
	code := writeStagedResult(failingReceiptWriter{err: deliveryError}, &stderr, staged)
	if code != 1 {
		t.Fatalf("writeStagedResult() exit = %d, want 1", code)
	}
	for _, want := range []string{
		"write receipt failed",
		`staged root retained at "/private/tmp/staged-root"`,
		deliveryError.Error(),
	} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("writeStagedResult() stderr = %q, want %q", stderr.String(), want)
		}
	}
}
