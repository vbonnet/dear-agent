package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/internal/specguard"
)

func TestRunGuardEmitsBoundedJSONForMalformedInput(t *testing.T) {
	t.Parallel()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if code := runGuard([]string{"--unknown"}, &stdout, &stderr); code != 1 {
		t.Fatalf("exit code = %d", code)
	}
	if stdout.Len() > maxGuardJSONOutputBytes {
		t.Fatalf("output size = %d", stdout.Len())
	}
	var result specguard.Result
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode output: %v\n%s", err, stdout.String())
	}
	if result.Decision != specguard.DecisionBlock || result.Scope != specguard.GuardScope {
		t.Fatalf("result = %#v", result)
	}
	if result.EvidenceClaim != specguard.EvidenceClaim {
		t.Fatalf("evidence claim = %q", result.EvidenceClaim)
	}
}

func TestEncodeGuardResultFallbackUsesCanonicalEvidenceClaim(t *testing.T) {
	t.Parallel()
	oversized := specguard.Result{
		SchemaVersion: specguard.SchemaVersion,
		Scope:         specguard.GuardScope,
		Decision:      specguard.DecisionReminder,
		Changed:       []string{},
		Findings:      []specguard.Finding{},
		Reminder:      strings.Repeat("x", 4096),
		EvidenceClaim: specguard.EvidenceClaim,
		TrustBoundary: specguard.TrustBoundary,
	}
	encoded, emitted, err := encodeGuardResult(oversized, 2048)
	if err != nil {
		t.Fatal(err)
	}
	if len(encoded) > 2048 {
		t.Fatalf("fallback size = %d", len(encoded))
	}
	var decoded specguard.Result
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if emitted.Decision != specguard.DecisionBlock || decoded.Decision != specguard.DecisionBlock {
		t.Fatalf("emitted = %#v, decoded = %#v", emitted, decoded)
	}
	if decoded.EvidenceClaim != specguard.EvidenceClaim || decoded.TrustBoundary != specguard.TrustBoundary {
		t.Fatalf("fallback disclosure drifted: %#v", decoded)
	}
	for _, phrase := range []string{"no provider", "installation", "hook registration", "runtime state"} {
		if !strings.Contains(decoded.EvidenceClaim, phrase) {
			t.Errorf("fallback evidence claim %q omits %q", decoded.EvidenceClaim, phrase)
		}
	}
	for _, phrase := range []string{"checkpoint-revalidates", "Git common directory", "their ancestors", "filesystem behavior between checkpoints", "cooperative feedback only", "not tamper-resistant", "mandatory immutable enforcement must come from a separately reviewed changed-SPEC CI and provider rollout", "does not attest that such enforcement is deployed, has run for a change, or is provider-required"} {
		if !strings.Contains(decoded.TrustBoundary, phrase) {
			t.Errorf("fallback trust boundary %q omits %q", decoded.TrustBoundary, phrase)
		}
	}
}

func TestWriteGuardJSONRejectsShortWrite(t *testing.T) {
	t.Parallel()
	if err := writeGuardJSON(shortNilWriter{}, []byte("guard\n")); !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("short-write error = %v", err)
	}
}

type shortNilWriter struct{}

func (shortNilWriter) Write(data []byte) (int, error) {
	return len(data) - 1, nil
}

func TestRunGuardBoundsArgumentInput(t *testing.T) {
	t.Parallel()
	var stdout bytes.Buffer
	if code := runGuard([]string{"--base", string(bytes.Repeat([]byte{'x'}, maxGuardCLIArgBytes+1))}, &stdout, &bytes.Buffer{}); code != 1 {
		t.Fatalf("exit code = %d", code)
	}
	if stdout.Len() > maxGuardJSONOutputBytes {
		t.Fatalf("output size = %d", stdout.Len())
	}
	var result specguard.Result
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Decision != specguard.DecisionBlock {
		t.Fatalf("result = %#v", result)
	}
}
