//go:build !windows

package main

import (
	"bytes"
	"errors"
	"fmt"
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
	request, err := override.EncodePrivilegedAppendRequest([]override.Use{use}, 4242)
	if err != nil {
		t.Fatal(err)
	}
	if err := appendInput(bytes.NewReader(request), path, false); err != nil {
		t.Fatalf("append valid record: %v", err)
	}
	if err := appendInput(bytes.NewReader(request), path, false); err != nil {
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
	request, err := override.EncodePrivilegedAppendRequest(uses, 4242)
	if err != nil {
		t.Fatal(err)
	}
	if err := appendInput(bytes.NewReader(request), path, false); err != nil {
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
	request, err := override.EncodePrivilegedAppendRequest([]override.Use{use}, 4242)
	if err != nil {
		t.Fatal(err)
	}
	if err := appendInput(bytes.NewReader(request), path, false); err != nil {
		t.Fatalf("append first authorization: %v", err)
	}
	if err := appendInput(bytes.NewReader(request), path, false); err == nil ||
		!strings.Contains(err.Error(), "already recorded") {
		t.Fatalf("replayed authorization error = %v, want duplicate rejection", err)
	}
}

func TestAppendInputRejectsIncompleteOrMalformedLedger(t *testing.T) {
	use := override.Use{
		Kind:    override.KindAdmissionBrake,
		Reason:  "host recovered and the operator is verifying one guarded spawn",
		Actor:   "vroom-dispatch",
		Session: "vroom-orchestrator",
		AtUTC:   time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC),
	}
	line, err := override.EncodeLedgerUse(use)
	if err != nil {
		t.Fatal(err)
	}
	request, err := override.EncodePrivilegedAppendRequest([]override.Use{use}, 4242)
	if err != nil {
		t.Fatal(err)
	}
	for name, existing := range map[string][]byte{
		"partial tail":        []byte(`{"kind":"admission-brake"`),
		"complete no newline": bytes.TrimSuffix(line, []byte("\n")),
		"malformed line":      []byte("{not-json}\n"),
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "ledger.jsonl")
			if err := os.WriteFile(path, existing, 0o600); err != nil {
				t.Fatal(err)
			}
			if err := appendInput(bytes.NewReader(request), path, false); err == nil ||
				!errors.Is(err, override.ErrLedgerRecord) {
				t.Fatalf("appendInput() error = %v, want malformed-ledger rejection", err)
			}
			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, existing) {
				t.Fatalf("ledger changed after rejected append: got %q, want %q", got, existing)
			}
		})
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
	request, err := override.EncodePrivilegedAppendRequest([]override.Use{use}, 4242)
	if err != nil {
		t.Fatal(err)
	}
	if err := appendInput(bytes.NewReader(request), path, false); err == nil {
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
	request, err := override.EncodePrivilegedAppendRequest([]override.Use{use}, 4242)
	if err != nil {
		t.Fatal(err)
	}
	for range maxUsesPerKindPerWindow {
		if err := appendInput(bytes.NewReader(request), path, false); err != nil {
			t.Fatalf("append below rate limit: %v", err)
		}
	}
	if err := appendInput(bytes.NewReader(request), path, false); err == nil ||
		!strings.Contains(err.Error(), "retry after") {
		t.Fatalf("rate limit error = %v, want bounded retry guidance", err)
	}

	recovered := use
	recovered.AtUTC = now.Add(privilegedRateWindow + time.Second)
	recoveredRequest, err := override.EncodePrivilegedAppendRequest([]override.Use{recovered}, 4242)
	if err != nil {
		t.Fatal(err)
	}
	if err := appendInput(bytes.NewReader(recoveredRequest), path, false); err != nil {
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

func TestProcessInputIssuesRootAttestedLaunchCapability(t *testing.T) {
	now := time.Now().UTC()
	capability := override.LaunchCapability{
		Version: override.LaunchCapabilityVersion,
		ID:      strings.Repeat("a", 32),
		LaunchCapabilityClaim: override.LaunchCapabilityClaim{
			Protocol:      "__exec-harness",
			HandoffPath:   "/tmp/agm/private-launch/launch-123.json",
			HandoffDigest: strings.Repeat("b", 64),
			OverrideProofs: []override.AuthorizationProof{{
				Kind:            override.KindAdmissionBrake,
				Reason:          "operator reviewed host recovery before this launch",
				Actor:           "dispatcher-test",
				Session:         "worker-1",
				AuthorizationID: strings.Repeat("c", 32),
			}},
			RecordSpawn: true,
			ExpiresUTC:  now.Add(5 * time.Minute),
		},
		IssuedUTC: now,
	}
	request, err := override.EncodePrivilegedLaunchCapabilityRequest(capability, 4242)
	if err != nil {
		t.Fatalf("encode capability request: %v", err)
	}
	root := t.TempDir()
	capabilityDir := filepath.Join(root, "capabilities")
	if err := processInput(
		bytes.NewReader(request),
		filepath.Join(root, "ledger.jsonl"),
		capabilityDir,
		false,
	); err != nil {
		t.Fatalf("process capability request: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(capabilityDir, capability.ID+".json"))
	if err != nil {
		t.Fatalf("read issued capability: %v", err)
	}
	decoded, err := override.DecodeLaunchCapability(data)
	if err != nil {
		t.Fatalf("decode issued capability: %v", err)
	}
	if decoded.ID != capability.ID ||
		decoded.HandoffDigest != capability.HandoffDigest {
		t.Fatalf("issued capability = %+v", decoded)
	}
	if err := processInput(
		bytes.NewReader(request),
		filepath.Join(root, "ledger.jsonl"),
		capabilityDir,
		false,
	); err == nil {
		t.Fatal("duplicate launch capability issuance was accepted")
	}

	consume, err := override.EncodePrivilegedConsumeLaunchCapabilityRequest(
		capability,
		4242,
	)
	if err != nil {
		t.Fatalf("encode capability consume request: %v", err)
	}
	if err := processInput(
		bytes.NewReader(consume),
		filepath.Join(root, "ledger.jsonl"),
		capabilityDir,
		false,
	); err != nil {
		t.Fatalf("consume capability request: %v", err)
	}
	if _, err := os.Stat(filepath.Join(capabilityDir, capability.ID+".json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("consumed capability still exists: %v", err)
	}
	if err := processInput(
		bytes.NewReader(consume),
		filepath.Join(root, "ledger.jsonl"),
		capabilityDir,
		false,
	); err == nil {
		t.Fatal("replayed launch capability consume was accepted")
	}
}

func TestProcessInputPreservesLedgerAppendProtocol(t *testing.T) {
	now := time.Now().UTC()
	use := override.Use{
		Kind:            override.KindAdmissionBrake,
		Reason:          "operator reviewed host recovery before this launch",
		Actor:           "dispatcher-test",
		Session:         "worker-1",
		AuthorizationID: strings.Repeat("d", 32),
		AtUTC:           now,
	}
	request, err := override.EncodePrivilegedAppendRequest([]override.Use{use}, 4242)
	if err != nil {
		t.Fatalf("encode append request: %v", err)
	}
	root := t.TempDir()
	ledger := filepath.Join(root, "ledger.jsonl")
	if err := processInput(
		bytes.NewReader(request),
		ledger,
		filepath.Join(root, "capabilities"),
		false,
	); err != nil {
		t.Fatalf("process append request: %v", err)
	}
	recorded, err := os.ReadFile(ledger)
	if err != nil {
		t.Fatalf("read appended ledger: %v", err)
	}
	if !bytes.Contains(recorded, []byte(use.AuthorizationID)) {
		t.Fatalf("ledger omitted authorization ID: %s", recorded)
	}
}

func TestIssueCapabilityPrunesExpiredSidecars(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "capabilities")
	oldNow := time.Now().UTC().Add(-20 * time.Minute)
	expired := override.LaunchCapability{
		Version: override.LaunchCapabilityVersion,
		ID:      strings.Repeat("1", 32),
		LaunchCapabilityClaim: override.LaunchCapabilityClaim{
			Protocol:      "__exec-harness",
			HandoffPath:   "/tmp/agm/private-launch/launch-expired.json",
			HandoffDigest: strings.Repeat("2", 64),
			OverrideProofs: []override.AuthorizationProof{{
				Kind:            override.KindAdmissionBrake,
				Reason:          "operator reviewed host recovery before this launch",
				Actor:           "dispatcher-test",
				Session:         "expired-worker",
				AuthorizationID: strings.Repeat("3", 32),
			}},
			ExpiresUTC: oldNow.Add(5 * time.Minute),
		},
		IssuedUTC: oldNow,
	}
	if err := issueCapability(dir, expired, oldNow, false); err != nil {
		t.Fatalf("issue expired fixture at its valid time: %v", err)
	}
	now := time.Now().UTC()
	current := expired
	current.ID = strings.Repeat("4", 32)
	current.HandoffPath = "/tmp/agm/private-launch/launch-current.json"
	current.HandoffDigest = strings.Repeat("5", 64)
	current.OverrideProofs[0].AuthorizationID = strings.Repeat("6", 32)
	current.ExpiresUTC = now.Add(5 * time.Minute)
	current.IssuedUTC = now
	if err := issueCapability(dir, current, now, false); err != nil {
		t.Fatalf("issue current capability: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, expired.ID+".json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expired launch capability was not pruned: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, current.ID+".json")); err != nil {
		t.Fatalf("current launch capability missing after pruning: %v", err)
	}
}

func TestIssueCapabilityCapsOutstandingSidecars(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "capabilities")
	if err := prepareCapabilityDirectory(dir, false); err != nil {
		t.Fatalf("prepare capability directory: %v", err)
	}
	now := time.Now().UTC()
	template := override.LaunchCapability{
		Version: override.LaunchCapabilityVersion,
		LaunchCapabilityClaim: override.LaunchCapabilityClaim{
			Protocol:      "__exec-harness",
			HandoffPath:   "/tmp/agm/private-launch/launch-limit.json",
			HandoffDigest: strings.Repeat("a", 64),
			OverrideProofs: []override.AuthorizationProof{{
				Kind:            override.KindAdmissionBrake,
				Reason:          "operator reviewed host recovery before this launch",
				Actor:           "dispatcher-test",
				Session:         "limited-worker",
				AuthorizationID: strings.Repeat("b", 32),
			}},
			ExpiresUTC: now.Add(5 * time.Minute),
		},
		IssuedUTC: now,
	}
	for i := range maxOutstandingLaunchCapabilities {
		capability := template
		capability.ID = fmt.Sprintf("%032x", i+1)
		capability.HandoffDigest = fmt.Sprintf("%064x", i+1)
		capability.OverrideProofs = append(
			[]override.AuthorizationProof(nil),
			template.OverrideProofs...,
		)
		capability.OverrideProofs[0].AuthorizationID = fmt.Sprintf("%032x", i+1)
		data, err := override.EncodeLaunchCapability(capability)
		if err != nil {
			t.Fatalf("encode capability %d: %v", i, err)
		}
		if err := writeCapabilityFile(
			filepath.Join(dir, capability.ID+".json"),
			data,
		); err != nil {
			t.Fatalf("write capability %d: %v", i, err)
		}
	}
	candidate := template
	candidate.ID = strings.Repeat("f", 32)
	candidate.HandoffDigest = strings.Repeat("e", 64)
	candidate.OverrideProofs = append(
		[]override.AuthorizationProof(nil),
		template.OverrideProofs...,
	)
	candidate.OverrideProofs[0].AuthorizationID = strings.Repeat("d", 32)
	if err := issueCapability(dir, candidate, now, false); err == nil ||
		!strings.Contains(err.Error(), "launch capability limit reached") {
		t.Fatalf("issue beyond capability limit error = %v", err)
	}
}
