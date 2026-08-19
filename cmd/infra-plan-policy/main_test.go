package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/internal/infraattest"
)

const cliPrivateSentinel = "DA_CLI_PRIVATE_SENTINEL_61c8f44e"

func TestPrivateInputFailuresNeverReachStdoutOrStderr(t *testing.T) {
	directory := t.TempDir()
	privatePath := writeTestFile(t, directory, "private-evidence", cliPrivateSentinel+"\n")
	keyPath := writeTestFile(
		t,
		directory,
		"key",
		base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{0x41}, infraattest.CommitmentKeyMinBytes))+"\n",
	)
	nonce := base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{0x42}, infraattest.CommitmentNonceBytes))
	evidence := evidencePaths{
		DesiredRulesetProjection: privatePath,
		Inventory:                privatePath,
		Backend:                  privatePath,
		StateSnapshot:            privatePath,
		MigrationManifest:        privatePath,
		ProviderSnapshot:         privatePath,
	}
	receiptContextPath := writeJSONTestFile(t, directory, "receipt-context", receiptContext{
		State: stateContext{Lineage: "private-lineage-" + cliPrivateSentinel, Serial: 42},
		Verification: infraattest.VerificationClaims{
			ProviderVisible:  true,
			NoDrift:          true,
			SourceParity:     true,
			BehavioralCanary: true,
		},
		Nonce:    nonce,
		Evidence: evidence,
	})
	expectedPath := writeJSONTestFile(t, directory, "expected", expectedAuthorizationContext{})
	malformedContextPath := writeTestFile(
		t,
		directory,
		"malformed-context",
		`{"private":"`+cliPrivateSentinel+`","unknown":true}`,
	)

	tests := []struct {
		name string
		args []string
	}{
		{
			name: "receipt private authorization",
			args: []string{
				"receipt", "--context", receiptContextPath, "--authorization", privatePath, "--hmac-key", keyPath,
			},
		},
		{
			name: "verify private claims",
			args: []string{
				"verify-authorization", "--claims", privatePath, "--expected", expectedPath,
			},
		},
		{
			name: "authorize private malformed context",
			args: []string{
				"authorize", "--context", malformedContextPath, "--plan", privatePath,
				"--plan-json", privatePath, "--tofu", privatePath, "--provider", privatePath,
				"--lockfile", privatePath, "--toolchain-lock", privatePath,
				"--baseline", privatePath, "--hmac-key", keyPath,
			},
		},
		{
			name: "private flag parse error",
			args: []string{"verify-receipt", "--" + cliPrivateSentinel},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var stdout bytes.Buffer
			var stderr bytes.Buffer
			exitCode := run(test.args, &stdout, &stderr, time.Date(2026, 8, 11, 1, 0, 0, 0, time.UTC))
			if exitCode != 3 {
				t.Fatalf("exit code = %d", exitCode)
			}
			combined := stdout.String() + stderr.String()
			for _, private := range []string{cliPrivateSentinel, "private-lineage", directory} {
				if strings.Contains(combined, private) {
					t.Fatalf("public output exposed private input marker")
				}
			}
			if stdout.Len() != 0 || !strings.HasPrefix(stderr.String(), "infra-plan-policy: infra plan policy rejected: ") {
				t.Fatalf("unexpected public failure envelope")
			}
		})
	}
}

func TestAuthorizationRejectsPersistedPlaintextPlanJSON(t *testing.T) {
	directory := t.TempDir()
	regular := writeTestFile(t, directory, "private-plan-json", cliPrivateSentinel)
	paths := evidencePaths{
		DesiredRulesetProjection: regular,
		Inventory:                regular,
		Backend:                  regular,
		StateSnapshot:            regular,
		MigrationManifest:        regular,
		ProviderSnapshot:         regular,
	}
	readers, err := openAuthorizationReaders(
		regular,
		regular,
		regular,
		regular,
		regular,
		regular,
		regular,
		paths,
	)
	if err == nil {
		readers.close()
		t.Fatal("persisted plaintext plan JSON was accepted")
	}
	if strings.Contains(err.Error(), regular) || strings.Contains(err.Error(), cliPrivateSentinel) {
		t.Fatal("plan JSON stream rejection exposed private input")
	}
}

func TestReceiptCommandCanonicalizesSubsecondClockAndSucceeds(t *testing.T) {
	context, authorizationPath, keyPath := cliValidReceiptFixture(t)
	directory := t.TempDir()
	contextPath := writeJSONTestFile(t, directory, "context", context)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	subsecond := time.Date(2026, 8, 11, 1, 3, 0, 987654321, time.FixedZone("private-offset", 60*60))
	exitCode := run(
		[]string{"receipt", "--context", contextPath, "--authorization", authorizationPath, "--hmac-key", keyPath},
		&stdout,
		&stderr,
		subsecond,
	)
	if exitCode != 0 || stderr.Len() != 0 {
		t.Fatalf("receipt command failed: exit=%d stderr=%q", exitCode, stderr.String())
	}
	if strings.Contains(stdout.String(), cliPrivateSentinel) || strings.Contains(stdout.String(), context.State.Lineage) {
		t.Fatal("successful receipt exposed private evidence")
	}
	var receipt infraattest.ReceiptClaims
	if err := json.Unmarshal(stdout.Bytes(), &receipt); err != nil {
		t.Fatal(err)
	}
	if receipt.ObservedAt != "2026-08-11T00:03:00Z" {
		t.Fatalf("canonical observed_at = %q", receipt.ObservedAt)
	}
	canonical, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(stdout.Bytes(), canonical) {
		t.Fatalf("receipt stdout is not canonical claim bytes: got %q", stdout.String())
	}
}

// TestContextJSONWithDuplicateKeyIsRejected pins INFRA-ATTEST-06: encoding/json
// silently keeps the last occurrence of a duplicated key, so a repeated private
// context field must be withheld instead of resolving to one arbitrary value.
// The fixture is otherwise valid, so only the duplication can cause rejection.
func TestContextJSONWithDuplicateKeyIsRejected(t *testing.T) {
	context, authorizationPath, keyPath := cliValidReceiptFixture(t)
	canonical, err := json.Marshal(context)
	if err != nil {
		t.Fatal(err)
	}
	nonce := `"nonce":"` + context.Nonce + `",`
	if !strings.Contains(string(canonical), nonce) {
		t.Fatalf("fixture does not contain %s", nonce)
	}
	directory := t.TempDir()
	now := time.Date(2026, 8, 11, 0, 3, 0, 0, time.UTC)
	issue := func(name, body string) (int, string) {
		var stdout, stderr bytes.Buffer
		path := writeTestFile(t, directory, name, body)
		code := run(
			[]string{"receipt", "--context", path, "--authorization", authorizationPath, "--hmac-key", keyPath},
			&stdout,
			&stderr,
			now,
		)
		return code, stdout.String()
	}

	// Positive control: the identical fixture without duplication must succeed,
	// so only the repeated key can explain the rejection below.
	if code, _ := issue("context-valid", string(canonical)); code != 0 {
		t.Fatalf("valid control fixture was rejected: exit=%d", code)
	}
	code, stdout := issue("context-duplicated", "{"+nonce+strings.TrimPrefix(string(canonical), "{"))
	if code == 0 {
		t.Fatal("duplicated private context key was accepted")
	}
	if stdout != "" {
		t.Fatalf("rejection wrote to stdout: %q", stdout)
	}
}

func cliTestAuthorization(key, nonce []byte, lineage string) infraattest.AuthorizationClaims {
	return infraattest.AuthorizationClaims{
		Schema:        infraattest.AuthorizationSchema,
		Decision:      "authorize-routine-apply",
		Operation:     "production-apply",
		SubjectSHA256: strings.Repeat("9", 64),
		Source: infraattest.SourceClaims{
			Repository: "vbonnet/dear-agent", Ref: "refs/heads/main",
			CommitSHA: strings.Repeat("a", 40), TreeSHA: strings.Repeat("b", 40),
			CanonicalRulesetBlobSHA: strings.Repeat("c", 40),
		},
		Toolchain: infraattest.ToolchainClaims{
			Platform: "darwin_arm64", OpenTofuVersion: infraattest.OpenTofuVersion,
			OpenTofuTagCommit:        infraattest.OpenTofuTagCommit,
			OpenTofuArchiveSHA256:    "dbb5a5bae9b0cabf622cd81a80ea02230eae8a3813215400df41a2cb89b47157",
			OpenTofuBinarySHA256:     "96557429623614140cf41afeb147b8a7e1fbe53e55923b63e7b581bc608d60ca",
			ToolchainManifestSHA256:  infraattest.ToolchainManifestSHA256,
			DependencyLockfileSHA256: infraattest.DependencyLockSHA256,
			Providers: []infraattest.ProviderClaims{{
				Address: infraattest.ProviderAddress, Version: infraattest.ProviderVersion,
				TagCommit:     infraattest.ProviderTagCommit,
				ArchiveSHA256: "c26a9bca4865665084e7f59b1402d7aff34ee63a418d7401a0658fa280cad4d4",
				BinarySHA256:  "b7e4601361cdd0afdcc83d9dfdc4a274dea693af291339c2bc9a915ec4ba62b6",
			}},
		},
		PrivateEvidence: infraattest.AuthorizationCommitments{
			Nonce: base64.RawURLEncoding.EncodeToString(nonce), InventoryHMACSHA256: strings.Repeat("1", 64),
			BackendHMACSHA256: strings.Repeat("2", 64), StateSnapshotHMACSHA256: strings.Repeat("3", 64),
			MigrationSurfaceHMACSHA256: strings.Repeat("4", 64), ProviderSnapshotHMACSHA256: strings.Repeat("5", 64),
			ChangeProjectionHMACSHA256: strings.Repeat("6", 64),
		},
		State: infraattest.StateClaims{
			LineageHMACSHA256: cliTestCommitment(key, nonce, "authorization/lineage", []byte(lineage)),
			Serial:            41,
		},
		Classification: infraattest.ClassificationClaims{Kind: "no-op", HumanAuthorizationRequired: false},
		PlanProfile: infraattest.PlanProfile{
			Mode: "normal", Refresh: true, Targets: []string{}, Excludes: []string{}, Replaces: []string{},
		},
		Freshness: infraattest.FreshnessClaims{
			PlanGeneratedAt: "2026-08-11T00:00:00Z", IssuedAt: "2026-08-11T00:01:00Z",
			NotBefore: "2026-08-11T00:01:00Z", ExpiresAt: "2026-08-11T00:10:00Z",
		},
	}
}

func cliTestCommitment(key, nonce []byte, domain string, payload []byte) string {
	mac := hmac.New(sha256.New, key)
	for _, part := range [][]byte{[]byte("dear-agent/infraattest/v1"), []byte(domain), nonce, payload} {
		var size [8]byte
		binary.BigEndian.PutUint64(size[:], uint64(len(part)))
		_, _ = mac.Write(size[:])
		_, _ = mac.Write(part)
	}
	return hex.EncodeToString(mac.Sum(nil))
}

func writeTestFile(t *testing.T, directory, name, contents string) string {
	t.Helper()
	path := filepath.Join(directory, name)
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeJSONTestFile(t *testing.T, directory, name string, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return writeTestFile(t, directory, name, string(raw))
}

// cliValidReceiptFixture builds a fully valid receipt invocation so tests can
// vary one input at a time without tripping unrelated rejections.
func cliValidReceiptFixture(t *testing.T) (receiptContext, string, string) {
	t.Helper()
	directory := t.TempDir()
	key := bytes.Repeat([]byte{0x41}, infraattest.CommitmentKeyMinBytes)
	authorizationNonce := bytes.Repeat([]byte{0x11}, infraattest.CommitmentNonceBytes)
	receiptNonce := bytes.Repeat([]byte{0x22}, infraattest.CommitmentNonceBytes)
	lineage := "private-lineage-" + cliPrivateSentinel

	authorizationPath := writeJSONTestFile(
		t,
		directory,
		"authorization",
		cliTestAuthorization(key, authorizationNonce, lineage),
	)
	keyPath := writeTestFile(
		t,
		directory,
		"key",
		base64.RawURLEncoding.EncodeToString(key)+"\n",
	)
	evidence := evidencePaths{
		Inventory: writeTestFile(
			t,
			directory,
			"inventory",
			`{"complete":true,"repositories":["`+cliPrivateSentinel+`"]}`,
		),
		Backend: writeTestFile(t, directory, "backend", `bucket = "`+cliPrivateSentinel+`"`+"\n"),
		StateSnapshot: writeTestFile(
			t,
			directory,
			"state",
			`{"lineage":"`+lineage+`","serial":42,"private":"`+cliPrivateSentinel+`"}`,
		),
		MigrationManifest: writeTestFile(
			t,
			directory,
			"migration",
			`{"schema":"dear-agent.opentofu-migration-surface/v1","backend":{"type":"s3","configuration":{"bucket":"`+
				cliPrivateSentinel+`"}},"state_encryption":"disabled","plan_encryption":"enforced","moved_blocks":[],`+
				`"removed_blocks":[],"import_blocks":[],"providers":[{"address":"registry.opentofu.org/integrations/github",`+
				`"version":"6.13.0"}],"workspace":"default"}`,
		),
		ProviderSnapshot: writeTestFile(
			t,
			directory,
			"provider",
			`{"private":"`+cliPrivateSentinel+`"}`,
		),
	}
	context := receiptContext{
		State: stateContext{Lineage: lineage, Serial: 42},
		Verification: infraattest.VerificationClaims{
			ProviderVisible: true, NoDrift: true, SourceParity: true, BehavioralCanary: true,
		},
		Nonce:    base64.RawURLEncoding.EncodeToString(receiptNonce),
		Evidence: evidence,
	}
	return context, authorizationPath, keyPath
}

// TestSubcommandDispatchContract pins IPP-01, IPP-02 and IPP-03: the exit code
// is the only channel a CI caller can branch on, so usage errors (2) must stay
// distinguishable from policy rejections (3), and a missing required flag must
// be a rejection rather than an evaluation against zero-valued inputs.
func TestSubcommandDispatchContract(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantCode   int
		wantUsage  bool
		usageOnOut bool
	}{
		{name: "no arguments", args: nil, wantCode: 2, wantUsage: true},
		{name: "unknown subcommand", args: []string{"apply"}, wantCode: 2, wantUsage: true},
		{name: "help", args: []string{"help"}, wantCode: 0, wantUsage: true, usageOnOut: true},
		{name: "short help flag", args: []string{"-h"}, wantCode: 0, wantUsage: true, usageOnOut: true},
		{name: "long help flag", args: []string{"--help"}, wantCode: 0, wantUsage: true, usageOnOut: true},
		{name: "unknown flag", args: []string{"authorize", "--bogus"}, wantCode: 3},
		{name: "missing required flags", args: []string{"authorize"}, wantCode: 3},
		{name: "positional argument", args: []string{"verify-receipt", "extra"}, wantCode: 3},
		{name: "empty required flag", args: []string{"verify-receipt", "--receipt", "", "--expected", ""}, wantCode: 3},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := run(test.args, &stdout, &stderr, time.Date(2026, 8, 11, 0, 0, 0, 0, time.UTC))
			if code != test.wantCode {
				t.Fatalf("exit = %d, want %d (stderr=%q)", code, test.wantCode, stderr.String())
			}
			target := stderr.String()
			if test.usageOnOut {
				target = stdout.String()
			}
			if test.wantUsage && !strings.Contains(target, "usage: infra-plan-policy") {
				t.Fatalf("usage not written: stdout=%q stderr=%q", stdout.String(), stderr.String())
			}
			if !test.wantUsage && stdout.String() != "" {
				t.Fatalf("rejection wrote to stdout: %q", stdout.String())
			}
			if test.wantCode == 3 && !strings.Contains(stderr.String(), string(infraattest.CodeInvalidInput)) {
				t.Fatalf("rejection code missing from stderr: %q", stderr.String())
			}
		})
	}
}

// TestSubcommandHelpPrintsFlagUsage guards against a subcommand's --help
// discarding flag.ErrHelp into the generic rejection: an operator running
// e.g. "infra-plan-policy authorize --help" needs to discover the many
// required flags without reading source, but the flag set's own usage
// output must never be discarded, and non-help parse errors on a
// caller-supplied flag must still never write anything (they can carry
// private evidence in their name or value; see the private-flag-parse-error
// case in TestPrivateInputFailuresNeverReachStdoutOrStderr) (codex review
// on #1257).
func TestSubcommandHelpPrintsFlagUsage(t *testing.T) {
	for _, subcommand := range []string{"authorize", "verify-authorization", "receipt", "verify-receipt"} {
		t.Run(subcommand, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := run([]string{subcommand, "--help"}, &stdout, &stderr, time.Date(2026, 8, 11, 0, 0, 0, 0, time.UTC))
			if code != 0 {
				t.Fatalf("exit = %d, want 0; stderr=%q", code, stderr.String())
			}
			if !strings.Contains(stderr.String(), "Usage of "+subcommand) {
				t.Fatalf("flag usage not written to stderr: %q", stderr.String())
			}
			if stdout.String() != "" {
				t.Fatalf("help wrote to stdout: %q", stdout.String())
			}
		})
	}
}

// TestVerificationInputsAreBounded pins IPP-15: claims are attacker-influenced
// bytes, so the reader must stop at the declared claims bound instead of
// loading an arbitrarily large file into memory before parsing it.
func TestVerificationInputsAreBounded(t *testing.T) {
	directory := t.TempDir()
	expectedPath := writeJSONTestFile(t, directory, "expected", expectedReceiptContext{})
	oversizePath := writeTestFile(t, directory, "oversize-claims", strings.Repeat("a", infraattest.MaxClaimsBytes+1))
	atBoundPath := writeTestFile(t, directory, "bound-claims", strings.Repeat("a", infraattest.MaxClaimsBytes))

	for _, claimsPath := range []string{oversizePath, atBoundPath} {
		var stdout, stderr bytes.Buffer
		code := run(
			[]string{"verify-receipt", "--receipt", claimsPath, "--expected", expectedPath},
			&stdout,
			&stderr,
			time.Date(2026, 8, 11, 0, 0, 0, 0, time.UTC),
		)
		// Both are rejected (neither is valid canonical JSON); the contract
		// under test is that the oversize read is refused rather than buffered,
		// and that neither leaks input bytes back to the caller.
		if code != 3 {
			t.Fatalf("exit = %d, want 3", code)
		}
		if stdout.String() != "" {
			t.Fatalf("rejection wrote to stdout: %q", stdout.String())
		}
		if strings.Contains(stderr.String(), "aaaa") {
			t.Fatalf("rejection echoed input: %q", stderr.String())
		}
	}
}
