package ops

import (
	"testing"
	"time"
)

func TestBeadIDFromSession(t *testing.T) {
	tests := []struct {
		name    string
		session string
		want    string
	}{
		{name: "hyphen separator", session: "worker-ce-11fi", want: "ce-11fi"},
		{name: "underscore separator", session: "worker_ce-z50t", want: "ce-z50t"},
		{name: "worker uppercase", session: "Worker-ce-abc1", want: "ce-abc1"},
		{name: "orchestrator excluded", session: "orchestrator", want: ""},
		{name: "overseer excluded", session: "overseer-main", want: ""},
		{name: "meta prefix excluded", session: "meta-orchestrator", want: ""},
		{name: "meta mixed case excluded", session: "Meta-SomeSession", want: ""},
		{name: "non-worker session", session: "some-random-session", want: ""},
		{name: "empty string", session: "", want: ""},
		{name: "worker prefix only (hyphen)", session: "worker-", want: ""},
		{name: "worker prefix only (underscore)", session: "worker_", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BeadIDFromSession(tt.session)
			if got != tt.want {
				t.Errorf("BeadIDFromSession(%q) = %q, want %q", tt.session, got, tt.want)
			}
		})
	}
}

func TestClassifyGhostWorker(t *testing.T) {
	const threshold = 10 * time.Minute

	tests := []struct {
		name      string
		session   string
		beadState string
		age       time.Duration
		threshold time.Duration
		want      string
	}{
		{
			name:      "closed bead age above threshold GHOST",
			session:   "worker-ce-11fi",
			beadState: "closed",
			age:       20 * time.Minute,
			threshold: threshold,
			want:      "GHOST",
		},
		{
			name:      "closed bead age equal to threshold GHOST",
			session:   "worker-ce-11fi",
			beadState: "closed",
			age:       threshold,
			threshold: threshold,
			want:      "GHOST",
		},
		{
			name:      "closed bead age below threshold ACTIVE grace period",
			session:   "worker-ce-11fi",
			beadState: "closed",
			age:       5 * time.Minute,
			threshold: threshold,
			want:      "ACTIVE",
		},
		{
			name:      "done bead age above threshold GHOST",
			session:   "worker-ce-z50t",
			beadState: "done",
			age:       24 * time.Hour,
			threshold: threshold,
			want:      "GHOST",
		},
		{
			name:      "done bead zero threshold GHOST immediately",
			session:   "worker-ce-z50t",
			beadState: "done",
			age:       0,
			threshold: 0,
			want:      "GHOST",
		},
		{
			name:      "open bead ACTIVE",
			session:   "worker-ce-abc1",
			beadState: "open",
			age:       24 * time.Hour,
			threshold: threshold,
			want:      "ACTIVE",
		},
		{
			name:      "not-found bead UNKNOWN",
			session:   "worker-ce-xyz",
			beadState: "not-found",
			age:       1 * time.Hour,
			threshold: threshold,
			want:      "UNKNOWN",
		},
		{
			name:      "empty bead state UNKNOWN",
			session:   "worker-ce-xyz",
			beadState: "",
			age:       1 * time.Hour,
			threshold: threshold,
			want:      "UNKNOWN",
		},
		{
			name:      "unknown bead state UNKNOWN",
			session:   "worker-ce-xyz",
			beadState: "pending",
			age:       1 * time.Hour,
			threshold: threshold,
			want:      "UNKNOWN",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ClassifyGhostWorker(tt.session, tt.beadState, tt.age, tt.threshold)
			if got != tt.want {
				t.Errorf("ClassifyGhostWorker(%q, %q, %v, %v) = %q, want %q",
					tt.session, tt.beadState, tt.age, tt.threshold, got, tt.want)
			}
		})
	}
}
