package daemon

import (
	"strings"
	"testing"
)

func TestRestartSessionRejectsNilTmuxClient(t *testing.T) {
	err := restartSession("worker", nil)
	if err == nil || !strings.Contains(err.Error(), "tmux client is nil") {
		t.Fatalf("restartSession() error = %v, want nil-client error", err)
	}
}

func TestSendRejectionMessageRejectsNilTmuxClient(t *testing.T) {
	err := SendRejectionMessage("worker", "blocked", nil, nil)
	if err == nil || !strings.Contains(err.Error(), "tmux client is nil") {
		t.Fatalf("SendRejectionMessage() error = %v, want nil-client error", err)
	}
}
