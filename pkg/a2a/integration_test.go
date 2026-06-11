// Copyright 2026 dear-agent contributors. See LICENSE.

package a2a_test

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/pkg/a2a"
	"github.com/vbonnet/dear-agent/pkg/a2a/client"
)

// TestInputRequired_RoundTrip is the headline integration test for the
// spike: supervisor sends a task → agent asks a question by pausing the
// task in input-required → supervisor answers → agent completes.
//
// It exercises every load-bearing piece of the spike: Server bind, Agent
// Card discovery, JSON-RPC SendMessage, input-required state, task
// resume via follow-up message, and terminal state propagation.
func TestInputRequired_RoundTrip(t *testing.T) {
	t.Parallel()

	const (
		userPrompt   = "deploy prod"
		agentQuery   = "Confirm deploy of 'deploy prod'? (yes/no)"
		supervisorOK = "yes"
		agentReply   = "ack: yes"
	)

	var asked atomic.Int32
	srv := startTestServer(t, a2a.HandlerFunc(func(ctx context.Context, prompt string, io a2a.SessionIO) error {
		if prompt != userPrompt {
			return fmt.Errorf("handler got unexpected prompt %q", prompt)
		}
		answer, err := io.AskInput(ctx, fmt.Sprintf("Confirm deploy of '%s'? (yes/no)", prompt))
		if err != nil {
			return fmt.Errorf("AskInput failed: %w", err)
		}
		asked.Add(1)
		if answer != supervisorOK {
			return fmt.Errorf("unexpected answer %q", answer)
		}
		return io.Emit(ctx, "ack: "+answer)
	}))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	c, err := client.NewFromEndpoint(ctx, srv.URL())
	if err != nil {
		t.Fatalf("client: %v", err)
	}

	var sawPrompt string
	resp, err := c.Send(ctx, userPrompt, func(ctx context.Context, q string) (string, error) {
		sawPrompt = q
		return supervisorOK, nil
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	if got, want := int(asked.Load()), 1; got != want {
		t.Fatalf("AskInput call count = %d, want %d", got, want)
	}
	if sawPrompt != agentQuery {
		t.Fatalf("supervisor saw prompt %q, want %q", sawPrompt, agentQuery)
	}
	// The terminal status message carries the last working-state Emit
	// when the handler returns nil — guarantee that body actually
	// surfaces through the protocol.
	if !strings.Contains(resp, agentReply) {
		t.Fatalf("response %q does not contain %q", resp, agentReply)
	}
}

// TestInputRequired_TwoPauses checks that the resume loop is re-entrant:
// an agent that asks twice should round-trip twice, and the answers
// arrive in order.
func TestInputRequired_TwoPauses(t *testing.T) {
	t.Parallel()

	srv := startTestServer(t, a2a.HandlerFunc(func(ctx context.Context, prompt string, io a2a.SessionIO) error {
		a, err := io.AskInput(ctx, "one?")
		if err != nil {
			return err
		}
		b, err := io.AskInput(ctx, "two?")
		if err != nil {
			return err
		}
		return io.Emit(ctx, a+"+"+b)
	}))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	c, err := client.NewFromEndpoint(ctx, srv.URL())
	if err != nil {
		t.Fatalf("client: %v", err)
	}

	answers := []string{"alpha", "beta"}
	idx := 0
	resp, err := c.Send(ctx, "go", func(ctx context.Context, q string) (string, error) {
		a := answers[idx]
		idx++
		return a, nil
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if idx != 2 {
		t.Fatalf("expected 2 input-required pauses, got %d", idx)
	}
	if !strings.Contains(resp, "alpha+beta") {
		t.Fatalf("response %q does not contain combined answer", resp)
	}
}

// TestNoQuestion_StillCompletes covers the trivial path: a handler that
// never pauses still drives the task to Completed and the final Emit
// shows up in the response.
func TestNoQuestion_StillCompletes(t *testing.T) {
	t.Parallel()

	srv := startTestServer(t, a2a.HandlerFunc(func(ctx context.Context, prompt string, io a2a.SessionIO) error {
		return io.Emit(ctx, "hello "+prompt)
	}))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	c, err := client.NewFromEndpoint(ctx, srv.URL())
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	resp, err := c.Send(ctx, "world", nil)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if !strings.Contains(resp, "hello world") {
		t.Fatalf("response %q missing expected text", resp)
	}
}

// TestAgentCardDiscovery confirms that the well-known card endpoint is
// reachable and a client built from the card URL works end-to-end.
func TestAgentCardDiscovery(t *testing.T) {
	t.Parallel()

	srv := startTestServer(t, a2a.HandlerFunc(func(ctx context.Context, prompt string, io a2a.SessionIO) error {
		return io.Emit(ctx, "ok")
	}))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	c, err := client.NewFromCardURL(ctx, srv.CardBaseURL())
	if err != nil {
		t.Fatalf("NewFromCardURL: %v", err)
	}
	resp, err := c.Send(ctx, "ping", nil)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if !strings.Contains(resp, "ok") {
		t.Fatalf("response %q missing expected text", resp)
	}
}

// startTestServer spins up a Server on ":0", schedules its Shutdown via
// t.Cleanup, and returns it.
func startTestServer(t *testing.T, h a2a.SessionHandler) *a2a.Server {
	t.Helper()
	card := a2a.SessionCard{
		SessionID:   "test-" + t.Name(),
		Description: "test session for " + t.Name(),
	}.Build()
	srv, err := a2a.NewServer(context.Background(), a2a.ServerConfig{
		Card:    card,
		Handler: h,
		Addr:    "127.0.0.1:0",
	})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	if err := srv.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	})
	return srv
}
