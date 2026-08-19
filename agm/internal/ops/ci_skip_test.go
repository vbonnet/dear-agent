package ops

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestMain(m *testing.M) {
	flag.Parse()
	if testing.Short() {
		fmt.Println("Skipping: requires infrastructure not available in CI")
		os.Exit(0)
	}
	// Point the alert queue at a throwaway file for the whole package.
	//
	// Stall recovery and the alert router default to the host-wide
	// ~/.agm/alerts/queue.jsonl. Without this redirect, running the package
	// tests appends synthetic alerts to the developer's real queue, and
	// worse, tests suppress one another through that persisted state: an
	// earlier test queuing a fingerprint can make a later test's alert come
	// back "suppressed". Redirecting here rather than per-test means no
	// future test can reintroduce the leak by forgetting to isolate itself.
	queueDir, err := os.MkdirTemp("", "agm-ops-alert-queue-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "create temp alert queue dir: %v\n", err)
		os.Exit(1)
	}
	if err := os.Setenv("AGM_ALERT_QUEUE", filepath.Join(queueDir, "queue.jsonl")); err != nil {
		fmt.Fprintf(os.Stderr, "set AGM_ALERT_QUEUE: %v\n", err)
		os.Exit(1)
	}
	code := m.Run()
	_ = os.RemoveAll(queueDir)
	os.Exit(code)
}
