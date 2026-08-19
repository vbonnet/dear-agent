package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/internal/hookparity"
)

func TestRunReportsMissingDeployment(t *testing.T) {
	artifact, deployed, body, policy := statusFixture(t)
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := runWithPolicy([]string{"--artifact", artifact, "--deployed", deployed}, &stdout, &stderr, policy)
	if code != 1 {
		t.Fatalf("run() code = %d, want 1; stderr = %q", code, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("run() stderr = %q, want empty", stderr.String())
	}
	var status hookparity.HelperDeploymentStatus
	if err := json.Unmarshal(stdout.Bytes(), &status); err != nil {
		t.Fatalf("decode run() output %q: %v", stdout.String(), err)
	}
	wantDigest := fmt.Sprintf("%x", sha256.Sum256(body))
	wantPinned := deployed + "." + wantDigest
	if status.Status != hookparity.HelperMissing ||
		status.Stable.Status != hookparity.HelperMissing ||
		status.Stable.Artifact != artifact ||
		status.Stable.Deployed != deployed ||
		status.Stable.ExpectedSHA256 != wantDigest ||
		status.Stable.ActualSHA256 != "" ||
		status.Stable.Reason != "deployed helper is missing" ||
		status.ContentAddressed.Status != hookparity.HelperMissing ||
		status.ContentAddressed.Deployed != wantPinned ||
		status.ContentAddressed.ExpectedSHA256 != wantDigest {
		t.Fatalf("run() status = %#v", status)
	}
}

func TestRunRejectsStableCurrentWhenContentAddressedHelperIsMissing(t *testing.T) {
	artifact, deployed, body, policy := statusFixture(t)
	if err := os.WriteFile(deployed, body, 0o755); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := runWithPolicy([]string{"--artifact", artifact, "--deployed", deployed}, &stdout, &stderr, policy)
	if code != 1 || stderr.Len() != 0 {
		t.Fatalf("run() code = %d, stderr = %q; want 1 and no stderr", code, stderr.String())
	}
	var status hookparity.HelperDeploymentStatus
	if err := json.Unmarshal(stdout.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if status.Status != hookparity.HelperMissing ||
		status.Stable.Status != hookparity.HelperCurrent ||
		status.ContentAddressed.Status != hookparity.HelperMissing {
		t.Fatalf("run() status = %#v", status)
	}
}

func TestRunRejectsUntrustedContentAddressedHelper(t *testing.T) {
	artifact, deployed, body, policy := statusFixture(t)
	if err := os.WriteFile(deployed, body, 0o755); err != nil {
		t.Fatal(err)
	}
	digest := fmt.Sprintf("%x", sha256.Sum256(body))
	pinned, err := hookparity.ContentAddressedHelperPath(deployed, digest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pinned, body, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(pinned, 0o777); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := runWithPolicy([]string{"--artifact", artifact, "--deployed", deployed}, &stdout, &stderr, policy)
	if code != 1 || stderr.Len() != 0 {
		t.Fatalf("run() code = %d, stderr = %q; want 1 and no stderr", code, stderr.String())
	}
	var status hookparity.HelperDeploymentStatus
	if err := json.Unmarshal(stdout.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if status.Status != hookparity.HelperUntrusted ||
		status.Stable.Status != hookparity.HelperCurrent ||
		status.ContentAddressed.Status != hookparity.HelperUntrusted ||
		!strings.Contains(status.ContentAddressed.Reason, "writable") {
		t.Fatalf("run() status = %#v", status)
	}
}

func TestRunReportsCurrentOnlyWhenBothHelperIdentitiesAreCurrent(t *testing.T) {
	artifact, deployed, body, policy := statusFixture(t)
	if err := os.WriteFile(deployed, body, 0o755); err != nil {
		t.Fatal(err)
	}
	digest := fmt.Sprintf("%x", sha256.Sum256(body))
	pinned, err := hookparity.ContentAddressedHelperPath(deployed, digest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Link(deployed, pinned); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := runWithPolicy([]string{"--artifact", artifact, "--deployed", deployed}, &stdout, &stderr, policy)
	if code != 0 || stderr.Len() != 0 {
		t.Fatalf("run() code = %d, stderr = %q; want 0 and no stderr", code, stderr.String())
	}
	var status hookparity.HelperDeploymentStatus
	if err := json.Unmarshal(stdout.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if status.Status != hookparity.HelperCurrent ||
		status.Stable.Status != hookparity.HelperCurrent ||
		status.ContentAddressed.Status != hookparity.HelperCurrent {
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
	root := t.TempDir()
	if err := os.Chmod(root, 0o755); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(root, "missing-artifact")
	policy := hookparity.HelperTrustPolicy{OwnerUID: uint32(os.Getuid()), TrustedRoot: root}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := runWithPolicy([]string{"--artifact", missing, "--deployed", filepath.Join(root, "missing-helper")}, &stdout, &stderr, policy)
	if code != 2 {
		t.Fatalf("run() code = %d, want 2", code)
	}
	if stdout.Len() != 0 {
		t.Fatalf("run() stdout = %q, want empty", stdout.String())
	}
	if !strings.Contains(stderr.String(), "helper artifact") {
		t.Fatalf("run() stderr = %q, want inspection diagnostic", stderr.String())
	}
}

func TestRunReportsEncodingFailure(t *testing.T) {
	artifact, deployed, _, policy := statusFixture(t)
	var stderr bytes.Buffer

	code := runWithPolicy(
		[]string{"--artifact", artifact, "--deployed", deployed},
		errorWriter{err: errors.New("write blocked")},
		&stderr,
		policy,
	)
	if code != 2 {
		t.Fatalf("run() code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "encode result: write blocked") {
		t.Fatalf("run() stderr = %q, want encoding diagnostic", stderr.String())
	}
}

func statusFixture(t *testing.T) (artifact, deployed string, body []byte, policy hookparity.HelperTrustPolicy) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("deployed helper ownership inspection requires Unix")
	}
	root := t.TempDir()
	if err := os.Chmod(root, 0o755); err != nil {
		t.Fatal(err)
	}
	body = []byte("reviewed helper\n")
	artifact = filepath.Join(t.TempDir(), "spec-contract-hook")
	if err := os.WriteFile(artifact, body, 0o755); err != nil {
		t.Fatal(err)
	}
	deployed = filepath.Join(root, "spec-contract-hook")
	policy = hookparity.HelperTrustPolicy{OwnerUID: uint32(os.Getuid()), TrustedRoot: root}
	return artifact, deployed, body, policy
}

type errorWriter struct {
	err error
}

func (writer errorWriter) Write([]byte) (int, error) {
	return 0, writer.err
}
