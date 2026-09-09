package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func continuationToken(t *testing.T, threadID, predecessorID, predecessorBody, replyID, body string) string {
	t.Helper()
	issuer, err := prepareContinuationIssuer()
	if err != nil {
		t.Fatalf("prepare continuation issuer: %v", err)
	}
	token, err := issuer.issue(
		threadID,
		predecessorID,
		replyID,
		[]byte(predecessorBody),
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
		t.Fatalf("issue continuation receipt: %v", err)
	}
	return token
}

func emittedContinuationReceiptToken(t *testing.T, diagnostics string) string {
	t.Helper()
	const prefix = "continuation_receipt='"
	var token string
	for line := range strings.SplitSeq(diagnostics, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, prefix) || !strings.HasSuffix(line, "'") {
			continue
		}
		candidate := strings.TrimSuffix(strings.TrimPrefix(line, prefix), "'")
		if token != "" {
			t.Fatalf("diagnostics emitted multiple continuation receipts")
		}
		token = candidate
	}
	if token == "" {
		t.Fatalf("diagnostics did not emit a continuation receipt:\n%s", diagnostics)
	}
	return token
}

func uncheckedContinuationToken(t *testing.T, receipt continuationReceipt) string {
	t.Helper()
	return rawContinuationToken(t, receipt)
}

func rawContinuationToken(t *testing.T, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal unchecked continuation receipt: %v", err)
	}
	return rawContinuationBytesToken(t, raw)
}

func rawContinuationBytesToken(t *testing.T, raw []byte) string {
	t.Helper()
	key, err := loadOrCreateContinuationReceiptKey()
	if err != nil {
		t.Fatalf("load continuation signing key: %v", err)
	}
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(continuationReceiptMACDomain))
	_, _ = mac.Write(raw)
	return base64.RawURLEncoding.EncodeToString(raw) + "." +
		base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// nonCanonicalBase64URLReceiptToken returns a syntactically decodable alias
// whose unused trailing bits differ from the encoder's canonical zero bits.
// It keeps the decoded JSON byte-for-byte identical so the test isolates the
// base64url canonicality boundary rather than relying on a JSON mismatch.
func nonCanonicalBase64URLReceiptToken(t *testing.T, receipt continuationReceipt) string {
	t.Helper()
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"
	for suffix := ""; ; suffix += "A" {
		candidate := receipt
		candidate.ThreadID += suffix
		raw, err := json.Marshal(candidate)
		if err != nil {
			t.Fatalf("marshal non-canonical base64url fixture: %v", err)
		}
		if len(raw)%3 == 0 {
			continue
		}
		canonical := base64.RawURLEncoding.EncodeToString(raw)
		last := strings.IndexByte(alphabet, canonical[len(canonical)-1])
		if last < 0 || last&1 != 0 {
			t.Fatalf("canonical base64url fixture has unexpected final sextet %q", canonical[len(canonical)-1])
		}
		alias := canonical[:len(canonical)-1] + string(alphabet[last|1])
		decoded, err := base64.RawURLEncoding.DecodeString(alias)
		if err != nil {
			t.Fatalf("decode non-canonical base64url fixture: %v", err)
		}
		if string(decoded) != string(raw) {
			t.Fatal("non-canonical base64url fixture changed the decoded receipt")
		}
		key, err := loadOrCreateContinuationReceiptKey()
		if err != nil {
			t.Fatalf("load continuation signing key: %v", err)
		}
		mac := hmac.New(sha256.New, key)
		_, _ = mac.Write([]byte(continuationReceiptMACDomain))
		_, _ = mac.Write(raw)
		return alias + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	}
}

func writeContinuationBody(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "reply body")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write reply body: %v", err)
	}
	return path
}

func runWithContinuationStdin(t *testing.T, body string, runCommand func() int) int {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("open stdin pipe: %v", err)
	}
	if _, err := writer.WriteString(body); err != nil {
		_ = reader.Close()
		_ = writer.Close()
		t.Fatalf("write stdin body: %v", err)
	}
	if err := writer.Close(); err != nil {
		_ = reader.Close()
		t.Fatalf("close stdin writer: %v", err)
	}
	oldStdin := os.Stdin
	os.Stdin = reader
	defer func() {
		os.Stdin = oldStdin
		_ = reader.Close()
	}()
	return runCommand()
}

func assertNoReplyMutation(t *testing.T, provider *sequencedProvider) {
	t.Helper()
	for _, kind := range queryKinds(provider.requests()) {
		if kind == "reply" {
			t.Error("continuation issued a reply mutation")
		}
	}
}

func assertNoProviderMutation(t *testing.T, provider *sequencedProvider) {
	t.Helper()
	for _, kind := range queryKinds(provider.requests()) {
		switch kind {
		case "reply", "resolve", "unresolve":
			t.Errorf("refusal issued forbidden %s mutation", kind)
		}
	}
}

func assertQueryKindsPrefix(t *testing.T, provider *sequencedProvider, want ...string) {
	t.Helper()
	got := queryKinds(provider.requests())
	if len(got) < len(want) {
		t.Errorf("provider query order = %q, want prefix %q", got, want)
		return
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("provider query order = %q, want prefix %q", got, want)
			return
		}
	}
}

func assertReplyMutationBody(t *testing.T, provider *sequencedProvider, want string) {
	t.Helper()
	for _, request := range provider.requests() {
		if !strings.Contains(request.Query, "addPullRequestReviewThreadReply") {
			continue
		}
		var got string
		if err := json.Unmarshal(request.Variables["body"], &got); err != nil {
			t.Fatalf("decode reply body variable: %v", err)
		}
		if got != want {
			t.Errorf("reply mutation body = %q, want %q", got, want)
		}
		return
	}
	t.Error("provider received no reply mutation")
}

func assertResolveMutationRequestsExactComments(t *testing.T, provider *sequencedProvider) {
	t.Helper()
	for _, request := range provider.requests() {
		if !strings.Contains(request.Query, "resolveReviewThread") {
			continue
		}
		for _, want := range []string{"comments(last:2)", "body"} {
			if !strings.Contains(request.Query, want) {
				t.Errorf("resolve mutation does not request %q exact-comment evidence:\n%s", want, request.Query)
			}
		}
		return
	}
	t.Error("provider received no resolve mutation")
}
