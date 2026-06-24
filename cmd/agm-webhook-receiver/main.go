// Command agm-webhook-receiver is the push-based replacement for supervisor
// `gh pr list` polling (spike ce-b1tt; third component of the ce-4m85 Option C
// hybrid).
//
// It listens for GitHub `pull_request` webhook deliveries (forwarded to
// localhost by `gh webhook forward` or `smee-client`), validates the webhook
// secret, extracts the PR fields the supervisor cares about, and appends one
// compact JSON line per event to ~/.agm/vroom/pr-events.jsonl. Supervisors read
// that file (tail-since-offset) instead of polling the GitHub API every tick.
//
// The append shape deliberately mirrors the ce-4m85 Option A delta line so the
// producer can be swapped (poll → push) without changing the supervisor-facing
// contract.
//
// Usage:
//
//	agm-webhook-receiver [--addr :9876] [--out ~/.agm/vroom/pr-events.jsonl]
//
// Environment:
//
//	AGM_WEBHOOK_SECRET  shared secret configured on the GitHub webhook; used to
//	                    verify the X-Hub-Signature-256 HMAC. Required.
//
// SPIKE SCAFFOLDING: this is a skeleton for the spike, not a production binary.
// It is syntactically valid Go but is not wired into a build target, has no
// graceful shutdown, and elides ret/error-path polish. See
// docs/spike-github-webhooks-pr-events.md "Next actions" for the real work.
package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

func main() {
	addr := flag.String("addr", ":9876", "listen address")
	out := flag.String("out", defaultOutPath(), "pr-events.jsonl path")
	flag.Parse()

	secret := os.Getenv("AGM_WEBHOOK_SECRET")
	if secret == "" {
		log.Fatal("AGM_WEBHOOK_SECRET is required")
	}

	sink, err := newSink(*out)
	if err != nil {
		log.Fatalf("open sink: %v", err)
	}
	defer sink.Close()

	srv := &receiver{secret: []byte(secret), sink: sink}

	mux := http.NewServeMux()
	mux.HandleFunc("/webhook", srv.handle)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	log.Printf("agm-webhook-receiver listening on %s → %s", *addr, *out)
	if err := http.ListenAndServe(*addr, mux); err != nil { //nolint:gosec // dev tool, localhost-only
		log.Printf("server: %v", err)
	}
}

// defaultOutPath returns ~/.agm/vroom/pr-events.jsonl.
func defaultOutPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".agm/vroom/pr-events.jsonl"
	}
	return filepath.Join(home, ".agm", "vroom", "pr-events.jsonl")
}

// receiver holds the verified secret and the append-only event sink.
type receiver struct {
	secret []byte
	sink   *sink
}

// prEvent is the compact record appended per relevant pull_request action.
// Shape mirrors the ce-4m85 Option A delta line so the supervisor contract is
// unchanged whether the producer polls or receives a push.
type prEvent struct {
	TS       string `json:"ts"`
	Action   string `json:"action"`
	Merged   bool   `json:"merged"`
	PRNumber int    `json:"pr_number"`
	Repo     string `json:"repo"`
	SHA      string `json:"sha"`
	Title    string `json:"title"`
}

// pullRequestPayload is the subset of the GitHub pull_request webhook body we
// extract. The full payload is large; we read only what the supervisor needs.
type pullRequestPayload struct {
	Action      string `json:"action"`
	Number      int    `json:"number"`
	PullRequest struct {
		Merged bool   `json:"merged"`
		Title  string `json:"title"`
		Head   struct {
			SHA string `json:"sha"`
		} `json:"head"`
	} `json:"pull_request"`
	Repository struct {
		FullName string `json:"full_name"`
	} `json:"repository"`
}

func (r *receiver) handle(w http.ResponseWriter, req *http.Request) {
	body, err := io.ReadAll(io.LimitReader(req.Body, 5<<20)) // 5 MiB cap
	if err != nil {
		http.Error(w, "read body", http.StatusBadRequest)
		return
	}

	if !validSignature(r.secret, req.Header.Get("X-Hub-Signature-256"), body) {
		http.Error(w, "invalid signature", http.StatusUnauthorized)
		return
	}

	// Only pull_request events are relevant; ack everything else cheaply.
	if req.Header.Get("X-GitHub-Event") != "pull_request" {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	var p pullRequestPayload
	if err := json.Unmarshal(body, &p); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}

	ev := prEvent{
		TS:       time.Now().UTC().Format(time.RFC3339),
		Action:   p.Action,
		Merged:   p.PullRequest.Merged,
		PRNumber: p.Number,
		Repo:     p.Repository.FullName,
		SHA:      p.PullRequest.Head.SHA,
		Title:    p.PullRequest.Title,
	}

	if err := r.sink.append(ev); err != nil {
		log.Printf("append: %v", err)
		http.Error(w, "append failed", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// validSignature verifies GitHub's X-Hub-Signature-256 header
// ("sha256=<hex>") against an HMAC-SHA256 of the raw body, in constant time.
func validSignature(secret []byte, header string, body []byte) bool {
	const prefix = "sha256="
	if !strings.HasPrefix(header, prefix) {
		return false
	}
	want, err := hex.DecodeString(strings.TrimPrefix(header, prefix))
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, secret)
	mac.Write(body)
	return hmac.Equal(want, mac.Sum(nil))
}

// sink is the append-only JSONL writer. It follows the same discipline as
// pkg/vroom/decisiontrail: one self-contained JSON object per line, O_APPEND,
// writes serialised by a mutex so concurrent deliveries don't interleave and a
// crash loses at most the trailing partial line.
type sink struct {
	mu sync.Mutex
	f  *os.File
}

func newSink(path string) (*sink, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("mkdir: %w", err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	return &sink{f: f}, nil
}

func (s *sink) append(ev prEvent) error {
	line, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err = s.f.Write(append(line, '\n'))
	return err
}

func (s *sink) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.f.Close()
}
