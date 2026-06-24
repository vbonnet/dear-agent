package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// sign produces the X-Hub-Signature-256 header value GitHub would send for
// the given secret and body, so tests can exercise validSignature both ways.
func sign(secret, body []byte) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

// --- validSignature ---

func TestValidSignature(t *testing.T) {
	secret := []byte("s3cr3t")
	body := []byte(`{"action":"opened"}`)

	if !validSignature(secret, sign(secret, body), body) {
		t.Error("valid signature rejected")
	}
	if validSignature(secret, sign([]byte("wrong"), body), body) {
		t.Error("signature signed with wrong secret accepted")
	}
	if validSignature(secret, sign(secret, []byte("tampered")), body) {
		t.Error("signature over different body accepted")
	}
}

func TestValidSignature_Malformed(t *testing.T) {
	secret := []byte("s3cr3t")
	body := []byte(`{}`)
	cases := []string{
		"",                 // empty
		"deadbeef",         // missing sha256= prefix
		"sha256=nothex",    // bad hex
		"sha1=" + "abcdef", // wrong algo prefix
	}
	for _, h := range cases {
		if validSignature(secret, h, body) {
			t.Errorf("malformed header %q accepted", h)
		}
	}
}

// --- sink append ---

func TestSinkAppend(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "pr-events.jsonl")
	s, err := newSink(path) // also exercises MkdirAll of a missing parent
	if err != nil {
		t.Fatalf("newSink: %v", err)
	}
	defer s.Close()

	want := []prEvent{
		{Action: "closed", Merged: true, PRNumber: 660, Repo: "vbonnet/dear-agent"},
		{Action: "opened", PRNumber: 661, Repo: "vbonnet/dear-agent"},
	}
	for _, ev := range want {
		if err := s.append(ev); err != nil {
			t.Fatalf("append: %v", err)
		}
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) != len(want) {
		t.Fatalf("got %d lines, want %d", len(lines), len(want))
	}
	for i, line := range lines {
		var got prEvent
		if err := json.Unmarshal([]byte(line), &got); err != nil {
			t.Fatalf("line %d not valid JSON: %v", i, err)
		}
		if got.PRNumber != want[i].PRNumber || got.Action != want[i].Action {
			t.Errorf("line %d = %+v, want %+v", i, got, want[i])
		}
	}
}

// --- pull_request payload extraction ---

func TestPullRequestPayloadExtract(t *testing.T) {
	body := []byte(`{
		"action": "closed",
		"number": 660,
		"pull_request": {"merged": true, "title": "fix: thing", "head": {"sha": "abc1234"}},
		"repository": {"full_name": "vbonnet/dear-agent"}
	}`)
	var p pullRequestPayload
	if err := json.Unmarshal(body, &p); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if p.Action != "closed" || p.Number != 660 || !p.PullRequest.Merged {
		t.Errorf("unexpected extract: %+v", p)
	}
	if p.PullRequest.Head.SHA != "abc1234" || p.Repository.FullName != "vbonnet/dear-agent" {
		t.Errorf("unexpected extract: %+v", p)
	}
}
