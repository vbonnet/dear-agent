//go:build !windows

package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/pkg/override"
)

func TestAppendInputAcceptsOnlyOneCanonicalBoundedRecord(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ledger.jsonl")
	use := override.Use{
		Kind:    override.KindCodexHookTrust,
		Reason:  "sandbox path rotates per spawn so hooks cannot be pre-trusted",
		Actor:   "vroom-dispatch",
		Session: "worker-ce-2ved",
		AtUTC:   time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC),
	}
	line, err := override.EncodeLedgerUse(use)
	if err != nil {
		t.Fatal(err)
	}
	if err := appendInput(bytes.NewReader(line), path, false); err != nil {
		t.Fatalf("append valid record: %v", err)
	}
	if err := appendInput(bytes.NewReader(line), path, false); err != nil {
		t.Fatalf("append second valid record: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if want := string(line) + string(line); string(got) != want {
		t.Fatalf("ledger = %q, want %q", got, want)
	}

	for name, input := range map[string][]byte{
		"oversized":   []byte(strings.Repeat("x", override.MaxLedgerBatchBytes+1)),
		"two records": append(append([]byte(nil), line...), line...),
		"not JSONL":   bytes.TrimSuffix(line, []byte("\n")),
		"unknown key": []byte(`{"kind":"codex-hook-trust","reason":"sandbox path rotates per spawn so hooks cannot be pre-trusted","actor":"test","at_utc":"2026-07-29T12:00:00Z","extra":true}` + "\n"),
	} {
		t.Run(name, func(t *testing.T) {
			if err := appendInput(bytes.NewReader(input), filepath.Join(t.TempDir(), "ledger"), false); err == nil {
				t.Fatal("invalid input was accepted")
			}
		})
	}
}

func TestAppendInputWritesDistinctKindsAsOneTransaction(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ledger.jsonl")
	now := time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC)
	uses := []override.Use{
		{
			Kind:    override.KindAdmissionBrake,
			Reason:  "host recovered and the operator is verifying one guarded spawn",
			Actor:   "vroom-dispatch",
			Session: "vroom-orchestrator",
			AtUTC:   now,
		},
		{
			Kind:    override.KindSupervisorOAuthCheck,
			Reason:  "validating a development supervisor without stored OAuth",
			Actor:   "vroom-dispatch",
			Session: "vroom-orchestrator",
			AtUTC:   now,
		},
	}
	transaction, err := override.EncodeLedgerUses(uses)
	if err != nil {
		t.Fatal(err)
	}
	if err := appendInput(bytes.NewReader(transaction), path, false); err != nil {
		t.Fatalf("append combined transaction: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, transaction) {
		t.Fatalf("ledger = %q, want atomic transaction %q", got, transaction)
	}
}

func TestAppendInputRejectsReplayedAuthorizationID(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ledger.jsonl")
	use := override.Use{
		Kind:            override.KindAdmissionBrake,
		Reason:          "host recovered and the operator is verifying one guarded spawn",
		Actor:           "vroom-dispatch",
		Session:         "vroom-orchestrator",
		AuthorizationID: "0123456789abcdef0123456789abcdef",
		AtUTC:           time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC),
	}
	line, err := override.EncodeLedgerUse(use)
	if err != nil {
		t.Fatal(err)
	}
	if err := appendInput(bytes.NewReader(line), path, false); err != nil {
		t.Fatalf("append first authorization: %v", err)
	}
	if err := appendInput(bytes.NewReader(line), path, false); err == nil ||
		!strings.Contains(err.Error(), "already recorded") {
		t.Fatalf("replayed authorization error = %v, want duplicate rejection", err)
	}
}

func TestAppendInputCapsTheFixedLedger(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ledger.jsonl")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Truncate(path, maxOperatorLedgerBytes); err != nil {
		t.Fatal(err)
	}
	use := override.Use{
		Kind:   override.KindAdmissionBrake,
		Reason: "disk watchdog clobbered the SRE hold, verifying the fix once",
		Actor:  "vroom-dispatch",
		AtUTC:  time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC),
	}
	line, err := override.EncodeLedgerUse(use)
	if err != nil {
		t.Fatal(err)
	}
	if err := appendInput(bytes.NewReader(line), path, false); err == nil {
		t.Fatal("append exceeded the fixed ledger cap")
	}
}

func TestAppendInputRateLimitsEachKindWithAutomaticRecovery(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ledger.jsonl")
	now := time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC)
	use := override.Use{
		Kind:   override.KindAdmissionBrake,
		Reason: "disk watchdog clobbered the SRE hold, verifying the fix once",
		Actor:  "vroom-dispatch",
		AtUTC:  now,
	}
	line, err := override.EncodeLedgerUse(use)
	if err != nil {
		t.Fatal(err)
	}
	for range maxUsesPerKindPerWindow {
		if err := appendInput(bytes.NewReader(line), path, false); err != nil {
			t.Fatalf("append below rate limit: %v", err)
		}
	}
	if err := appendInput(bytes.NewReader(line), path, false); err == nil ||
		!strings.Contains(err.Error(), "retry after") {
		t.Fatalf("rate limit error = %v, want bounded retry guidance", err)
	}

	recovered := use
	recovered.AtUTC = now.Add(privilegedRateWindow + time.Second)
	recoveredLine, err := override.EncodeLedgerUse(recovered)
	if err != nil {
		t.Fatal(err)
	}
	if err := appendInput(bytes.NewReader(recoveredLine), path, false); err != nil {
		t.Fatalf("rate window did not recover automatically: %v", err)
	}
}

func TestAppendInputRejectsNonRootAtProductionBoundary(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("test process is root")
	}
	err := appendInput(bytes.NewReader(nil), filepath.Join(t.TempDir(), "ledger"), true)
	if err == nil || !strings.Contains(err.Error(), "must run as root") {
		t.Fatalf("error = %v, want root requirement", err)
	}
	if errors.Is(err, override.ErrLedgerRecord) {
		t.Fatalf("root boundary was checked after parsing attacker input: %v", err)
	}
}
