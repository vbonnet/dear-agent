package main

import (
	"bytes"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestContinuationReceiptHMACRoundTripAndTamperRejection(t *testing.T) {
	stateRoot := filepath.Join(t.TempDir(), "state")
	if err := os.Mkdir(stateRoot, 0o700); err != nil {
		t.Fatalf("create explicit XDG state root: %v", err)
	}
	t.Setenv("XDG_STATE_HOME", stateRoot)
	key, err := loadOrCreateContinuationReceiptKey()
	if err != nil {
		t.Fatalf("create signing key: %v", err)
	}
	receipt := securityContinuationReceipt(t, "github.com", "authenticated reply")
	token, err := encodeContinuationReceiptWithKey(receipt, key)
	if err != nil {
		t.Fatalf("encode authenticated receipt: %v", err)
	}
	decoded, err := decodeContinuationReceipt(token)
	if err != nil {
		t.Fatalf("decode authenticated receipt: %v", err)
	}
	if !reflect.DeepEqual(decoded, receipt) {
		t.Fatalf("decoded receipt = %#v, want %#v", decoded, receipt)
	}

	parts := strings.Split(token, ".")
	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		t.Fatalf("decode payload fixture: %v", err)
	}
	tamperedRaw := bytes.Replace(raw, []byte(receipt.ThreadID), []byte("PRRT_tampered"), 1)
	tamperedPayload := base64.RawURLEncoding.EncodeToString(tamperedRaw) + "." + parts[1]
	if _, err := decodeContinuationReceipt(tamperedPayload); err == nil {
		t.Fatal("payload tampering passed HMAC validation")
	}

	mac, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("decode MAC fixture: %v", err)
	}
	mac[0] ^= 0xff
	tamperedMAC := parts[0] + "." + base64.RawURLEncoding.EncodeToString(mac)
	if _, err := decodeContinuationReceipt(tamperedMAC); err == nil {
		t.Fatal("MAC tampering passed HMAC validation")
	}

	wrongKey := bytes.Repeat([]byte{0xa5}, continuationReceiptKeyBytes)
	if bytes.Equal(wrongKey, key) {
		wrongKey[0] ^= 0xff
	}
	wrongKeyToken, err := encodeContinuationReceiptWithKey(receipt, wrongKey)
	if err != nil {
		t.Fatalf("encode wrong-key fixture: %v", err)
	}
	if _, err := decodeContinuationReceipt(wrongKeyToken); err == nil {
		t.Fatal("receipt authenticated by another key was accepted")
	}
}

func TestContinueResolveEnvironmentFailuresRetainReceiptBeforeBodyOrProvider(t *testing.T) {
	const body = "provider access must remain zero"
	tests := []struct {
		name        string
		issuerHost  string
		ambientHost string
		installKey  bool
		wrongKey    bool
	}{
		{name: "missing issuer key", issuerHost: "github.com", ambientHost: "github.com"},
		{name: "different local key", issuerHost: "github.com", ambientHost: "github.com", installKey: true, wrongKey: true},
		{name: "different provider host", issuerHost: "github.example", ambientHost: "github.com", installKey: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			stateRoot := filepath.Join(t.TempDir(), "state")
			if err := os.Mkdir(stateRoot, 0o700); err != nil {
				t.Fatalf("create explicit XDG state root: %v", err)
			}
			t.Setenv("XDG_STATE_HOME", stateRoot)
			t.Setenv("GH_HOST", test.ambientHost)
			issuerKey := bytes.Repeat([]byte{0x42}, continuationReceiptKeyBytes)
			if test.installKey {
				installed, err := loadOrCreateContinuationReceiptKey()
				if err != nil {
					t.Fatalf("install ambient signing key: %v", err)
				}
				if !test.wrongKey {
					issuerKey = installed
				}
			}
			receipt := securityContinuationReceipt(t, test.issuerHost, body)
			token, err := encodeContinuationReceiptWithKey(receipt, issuerKey)
			if err != nil {
				t.Fatalf("encode issuer receipt: %v", err)
			}
			missingBody := filepath.Join(t.TempDir(), "body-was-not-read")
			provider := installSequencedProvider(t)
			code, stdout, diagnostics := provider.capture(func() int {
				return run([]string{"continue-resolve", token, "--body-file", missingBody})
			})
			if code == 0 {
				t.Fatal("continuation environment mismatch succeeded")
			}
			if stdout != "" {
				t.Fatalf("continuation environment mismatch printed success: %q", stdout)
			}
			if got := provider.requestCount(); got != 0 {
				t.Fatalf("continuation environment mismatch made %d provider requests, want zero", got)
			}
			assertContainsAll(t, diagnostics,
				"Retain the continuation receipt",
				"continuation_receipt="+shellQuoteArgument(token),
				"reply_file="+shellQuoteArgument(missingBody),
				"Restore the exact issuing GH_HOST, XDG_STATE_HOME, and signing key",
				"Do not regenerate or overwrite the key",
				"no provider request was made",
			)
			assertContainsNone(t, diagnostics, body, "open reply body", "invalid continuation receipt")
		})
	}
}

func TestEffectiveContinuationProviderHostDefaultsAndCanonicalizes(t *testing.T) {
	t.Setenv("GH_HOST", "")
	if host, err := effectiveContinuationProviderHost(); err != nil || host != "github.com" {
		t.Fatalf("default provider host = %q, %v; want github.com", host, err)
	}
	t.Setenv("GH_HOST", " GitHub.Example. ")
	if host, err := effectiveContinuationProviderHost(); err != nil || host != "github.example" {
		t.Fatalf("canonical provider host = %q, %v; want github.example", host, err)
	}
}

func TestReplyResolvePreflightsSigningKeyBeforeReplyMutation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX key-mode rejection is required")
	}
	const (
		threadID = "PRRT_signing_preflight"
		body     = "Do not post until signing state is durable."
	)
	stateRoot := filepath.Join(t.TempDir(), "state")
	t.Setenv("XDG_STATE_HOME", stateRoot)
	keyPath, err := continuationReceiptKeyPath()
	if err != nil {
		t.Fatalf("resolve continuation key path: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(keyPath), 0o700); err != nil {
		t.Fatalf("create key directory: %v", err)
	}
	if err := os.Chmod(filepath.Dir(filepath.Dir(keyPath)), 0o700); err != nil {
		t.Fatalf("protect dear-agent state directory: %v", err)
	}
	if err := os.Chmod(filepath.Dir(keyPath), 0o700); err != nil {
		t.Fatalf("protect command state directory: %v", err)
	}
	if err := os.WriteFile(keyPath, bytes.Repeat([]byte{'k'}, continuationReceiptKeyBytes), 0o644); err != nil {
		t.Fatalf("write unsafe key fixture: %v", err)
	}
	if err := os.Chmod(keyPath, 0o644); err != nil {
		t.Fatalf("set unsafe key mode: %v", err)
	}
	bodyFile := writeContinuationBody(t, body)
	opening := providerComment{id: "PRRC_preflight_opening", login: "reviewer", body: "P1: preflight before posting."}
	provider := installSequencedProvider(t,
		providerStep{stdout: threadResponse(threadID, false, opening)},
		providerStep{stdout: historyResponse(threadID, opening)},
		providerStep{stdout: threadResponse(threadID, false, opening)},
	)
	code, stdout, diagnostics := provider.capture(func() int {
		return run([]string{"reply-resolve", threadID, "--body-file", bodyFile})
	})
	if code == 0 {
		t.Fatal("reply-resolve posted with unsafe signing state")
	}
	provider.assertExhausted()
	assertQueryKinds(t, provider, "thread", "history", "thread")
	assertNoProviderMutation(t, provider)
	if stdout != "" {
		t.Fatalf("signing-state preflight printed success: %q", stdout)
	}
	assertContainsAll(t, diagnostics, "signing state could not be established", "nothing was posted")
	assertContainsNone(t, diagnostics, body)
}

func TestReplyResolveAcceptsHomeFallbackSharedStateBeforeMutation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX continuation state is not supported on Windows")
	}
	const (
		threadID = "PRRT_home_fallback_shared_state"
		body     = "The normal shared state layout must not block this reply."
	)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_STATE_HOME", "")
	sharedStateDirectory := filepath.Join(home, ".local", "state", "dear-agent")
	if err := os.MkdirAll(sharedStateDirectory, 0o755); err != nil {
		t.Fatalf("create fallback shared state namespace: %v", err)
	}
	if err := os.Chmod(sharedStateDirectory, 0o755); err != nil {
		t.Fatalf("set fallback shared state namespace mode: %v", err)
	}
	before, err := os.Lstat(sharedStateDirectory)
	if err != nil {
		t.Fatalf("inspect fallback shared state before command: %v", err)
	}
	bodyFile := writeContinuationBody(t, body)
	opening := providerComment{id: "PRRC_home_fallback_opening", login: "reviewer", body: "P1: exercise the deployed state layout."}
	reply := providerComment{id: "PRRC_home_fallback_reply", login: "author", body: body}
	provider := installSequencedProvider(t,
		providerStep{stdout: threadResponse(threadID, false, opening)},
		providerStep{stdout: historyResponse(threadID, opening)},
		providerStep{stdout: threadResponse(threadID, false, opening)},
		providerStep{stdout: replyResponse(reply)},
		providerStep{stdout: threadResponse(threadID, false, opening, reply)},
		providerStep{stdout: threadResponse(threadID, false, opening, reply)},
		providerStep{stdout: resolveResponseWithExactComments(threadID, true, opening, reply)},
	)

	code, stdout, diagnostics := provider.capture(func() int {
		return run([]string{"reply-resolve", threadID, "--body-file", bodyFile})
	})
	if code != 0 {
		t.Fatalf("fallback shared state blocked reply with code %d:\nstdout:\n%s\nstderr:\n%s", code, stdout, diagnostics)
	}
	provider.assertExhausted()
	assertQueryKinds(t, provider, "thread", "history", "thread", "reply", "thread", "thread", "resolve")
	assertReplyMutationBody(t, provider, body)
	after, err := os.Lstat(sharedStateDirectory)
	if err != nil {
		t.Fatalf("inspect fallback shared state after command: %v", err)
	}
	if !os.SameFile(before, after) || after.Mode().Perm() != 0o755 {
		t.Fatalf("fallback shared state changed: same=%t mode=%04o", os.SameFile(before, after), after.Mode().Perm())
	}
}

func TestReplyResolveRejectsWritableSharedStateBeforeMutation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX continuation state is not supported on Windows")
	}
	const (
		threadID = "PRRT_writable_shared_state"
		body     = "Do not post through externally writable state."
	)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_STATE_HOME", "")
	sharedStateDirectory := filepath.Join(home, ".local", "state", "dear-agent")
	if err := os.MkdirAll(sharedStateDirectory, 0o700); err != nil {
		t.Fatalf("create fallback shared state namespace: %v", err)
	}
	if err := os.Chmod(sharedStateDirectory, 0o775); err != nil {
		t.Fatalf("make fallback shared state namespace group writable: %v", err)
	}
	bodyFile := writeContinuationBody(t, body)
	opening := providerComment{id: "PRRC_writable_shared_state_opening", login: "reviewer", body: "P1: fail before provider mutation."}
	provider := installSequencedProvider(t,
		providerStep{stdout: threadResponse(threadID, false, opening)},
		providerStep{stdout: historyResponse(threadID, opening)},
		providerStep{stdout: threadResponse(threadID, false, opening)},
	)

	code, stdout, diagnostics := provider.capture(func() int {
		return run([]string{"reply-resolve", threadID, "--body-file", bodyFile})
	})
	if code == 0 {
		t.Fatal("reply-resolve accepted writable shared state")
	}
	provider.assertExhausted()
	assertQueryKinds(t, provider, "thread", "history", "thread")
	assertNoProviderMutation(t, provider)
	if stdout != "" {
		t.Fatalf("writable shared state printed success: %q", stdout)
	}
	assertContainsAll(t, diagnostics, "signing state could not be established", "nothing was posted")
	assertContainsNone(t, diagnostics, body)
	if _, err := os.Lstat(filepath.Join(sharedStateDirectory, "resolve-review-threads")); err == nil {
		t.Fatal("writable shared state created private continuation state")
	}
}

func TestReplyResolveRequiresExistingExplicitXDGStateRootBeforeMutation(t *testing.T) {
	const (
		threadID = "PRRT_missing_xdg_boundary"
		body     = "Do not post without the configured durability boundary."
	)
	stateRoot := filepath.Join(t.TempDir(), "missing-state-root")
	t.Setenv("XDG_STATE_HOME", stateRoot)
	bodyFile := writeContinuationBody(t, body)
	opening := providerComment{
		id:    "PRRC_missing_xdg_opening",
		login: "reviewer",
		body:  "P1: require an existing explicit state boundary.",
	}
	provider := installSequencedProvider(t,
		providerStep{stdout: threadResponse(threadID, false, opening)},
		providerStep{stdout: historyResponse(threadID, opening)},
		providerStep{stdout: threadResponse(threadID, false, opening)},
	)

	code, stdout, diagnostics := provider.capture(func() int {
		return run([]string{"reply-resolve", threadID, "--body-file", bodyFile})
	})
	if code == 0 {
		t.Fatal("reply-resolve posted without an existing explicit XDG state root")
	}
	provider.assertExhausted()
	assertQueryKinds(t, provider, "thread", "history", "thread")
	assertNoProviderMutation(t, provider)
	if stdout != "" {
		t.Fatalf("missing XDG boundary printed success: %q", stdout)
	}
	assertContainsAll(t, diagnostics, "signing state could not be established", "nothing was posted")
	assertContainsNone(t, diagnostics, body)
	if _, err := os.Lstat(stateRoot); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing explicit XDG state root was mutated: %v", err)
	}
}

func TestContinuationPlatformBoundaryRejectsNonUnix(t *testing.T) {
	if err := validateContinuationReceiptPlatform("windows", false); err == nil {
		t.Fatal("unsupported Windows continuation state was accepted")
	}
	if err := validateContinuationReceiptPlatform("linux", true); err != nil {
		t.Fatalf("supported Unix continuation state was rejected: %v", err)
	}
}

func TestReplyResolveRejectsUnsupportedContinuationPlatformBeforeMutation(t *testing.T) {
	const (
		threadID = "PRRT_unsupported_platform"
		body     = "Do not post without a private durable signing boundary."
	)
	setContinuationReceiptPlatformSupportForTest(t, false)

	bodyFile := writeContinuationBody(t, body)
	opening := providerComment{
		id:    "PRRC_unsupported_platform_opening",
		login: "reviewer",
		body:  "P1: prove platform key privacy before posting.",
	}
	provider := installSequencedProvider(t,
		providerStep{stdout: threadResponse(threadID, false, opening)},
		providerStep{stdout: historyResponse(threadID, opening)},
		providerStep{stdout: threadResponse(threadID, false, opening)},
	)
	code, stdout, diagnostics := provider.capture(func() int {
		return run([]string{"reply-resolve", threadID, "--body-file", bodyFile})
	})
	if code == 0 {
		t.Fatal("reply-resolve posted without a supported continuation key boundary")
	}
	provider.assertExhausted()
	assertQueryKinds(t, provider, "thread", "history", "thread")
	assertNoProviderMutation(t, provider)
	if stdout != "" {
		t.Fatalf("unsupported platform printed success: %q", stdout)
	}
	assertContainsAll(t, diagnostics, "unsupported on "+runtime.GOOS, "nothing was posted")
	assertContainsNone(t, diagnostics, body)
}

func TestContinueResolveRejectsUnsupportedPlatformBeforeBodyOrProvider(t *testing.T) {
	const body = "Retain this source without reading it on an unsupported platform."
	stateRoot := filepath.Join(t.TempDir(), "state")
	if err := os.Mkdir(stateRoot, 0o700); err != nil {
		t.Fatalf("create explicit XDG state root: %v", err)
	}
	t.Setenv("XDG_STATE_HOME", stateRoot)
	key, err := loadOrCreateContinuationReceiptKey()
	if err != nil {
		t.Fatalf("create supported-platform receipt key: %v", err)
	}
	receipt := securityContinuationReceipt(t, "github.com", body)
	token, err := encodeContinuationReceiptWithKey(receipt, key)
	if err != nil {
		t.Fatalf("encode continuation receipt: %v", err)
	}
	bodyFile := filepath.Join(t.TempDir(), "body-source-must-not-be-read")

	setContinuationReceiptPlatformSupportForTest(t, false)
	provider := installSequencedProvider(t)
	code, stdout, diagnostics := provider.capture(func() int {
		return run([]string{"continue-resolve", token, "--body-file", bodyFile})
	})
	if code == 0 {
		t.Fatal("continue-resolve accepted an unsupported continuation platform")
	}
	provider.assertExhausted()
	if stdout != "" {
		t.Fatalf("unsupported continuation printed success: %q", stdout)
	}
	assertContainsAll(t, diagnostics, "unsupported on "+runtime.GOOS, token, bodyFile)
	assertContainsNone(t, diagnostics, body, "open reply body")
}

func setContinuationReceiptPlatformSupportForTest(t *testing.T, supported bool) {
	t.Helper()
	previousOverride := continuationReceiptPlatformTestOverride
	continuationReceiptPlatformTestOverride = &supported
	t.Cleanup(func() { continuationReceiptPlatformTestOverride = previousOverride })
}

func securityContinuationReceipt(t *testing.T, providerHost, body string) continuationReceipt {
	t.Helper()
	receipt, err := newContinuationReceipt(
		providerHost,
		"PRRT_security",
		"PRRC_security_predecessor",
		"PRRC_security_reply",
		[]byte("P1: security predecessor"),
		[]byte(body),
		providerFixtureUpdatedAt,
		providerFixtureUpdatedAt,
		0,
		0,
		"",
		"",
		"reviewer",
		"author",
	)
	if err != nil {
		t.Fatalf("create security receipt: %v", err)
	}
	return receipt
}
