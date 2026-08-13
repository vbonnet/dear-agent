package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/internal/hookparity"
)

func TestRunReportsMissingDeployment(t *testing.T) {
	artifact := filepath.Join(t.TempDir(), "spec-contract-hook")
	body := []byte("reviewed helper\n")
	if err := os.WriteFile(artifact, body, 0o755); err != nil {
		t.Fatal(err)
	}
	deployed := filepath.Join(t.TempDir(), "missing-helper")
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := run([]string{"--artifact", artifact, "--deployed", deployed}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("run() code = %d, want 1; stderr = %q", code, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("run() stderr = %q, want empty", stderr.String())
	}
	var status hookparity.DeployedHelperStatus
	if err := json.Unmarshal(stdout.Bytes(), &status); err != nil {
		t.Fatalf("decode run() output %q: %v", stdout.String(), err)
	}
	wantDigest := fmt.Sprintf("%x", sha256.Sum256(body))
	if status.Status != hookparity.HelperMissing ||
		status.Artifact != artifact ||
		status.Deployed != deployed ||
		status.ExpectedSHA256 != wantDigest ||
		status.ActualSHA256 != "" ||
		status.Reason != "deployed helper is missing" {
		t.Fatalf("run() status = %#v", status)
	}
}

func TestRunRejectsInvalidInvocation(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	if code := run([]string{"--unknown"}, &stdout, &stderr); code != 2 {
		t.Fatalf("run() code = %d, want 2", code)
	}
	if stdout.Len() != 0 {
		t.Fatalf("run() stdout = %q, want empty", stdout.String())
	}
	if !strings.Contains(stderr.String(), "flag provided but not defined") {
		t.Fatalf("run() stderr = %q, want flag diagnostic", stderr.String())
	}
}

func TestRunReportsInspectionFailure(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing-artifact")
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := run([]string{"--artifact", missing, "--deployed", filepath.Join(t.TempDir(), "missing-helper")}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("run() code = %d, want 2", code)
	}
	if stdout.Len() != 0 {
		t.Fatalf("run() stdout = %q, want empty", stdout.String())
	}
	if !strings.Contains(stderr.String(), "hash helper artifact") {
		t.Fatalf("run() stderr = %q, want inspection diagnostic", stderr.String())
	}
}

func TestRunReportsEncodingFailure(t *testing.T) {
	artifact := filepath.Join(t.TempDir(), "spec-contract-hook")
	if err := os.WriteFile(artifact, []byte("reviewed helper\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer

	code := run(
		[]string{"--artifact", artifact, "--deployed", filepath.Join(t.TempDir(), "missing-helper")},
		errorWriter{err: errors.New("write blocked")},
		&stderr,
	)
	if code != 2 {
		t.Fatalf("run() code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "encode result: write blocked") {
		t.Fatalf("run() stderr = %q, want encoding diagnostic", stderr.String())
	}
}

type errorWriter struct {
	err error
}

func (writer errorWriter) Write([]byte) (int, error) {
	return 0, writer.err
}
