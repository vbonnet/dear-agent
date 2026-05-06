package main

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/pkg/intent"
)

// withIntentBoardDir points the CLI's board lookup at a fresh temp
// directory and restores the previous value afterwards.
func withIntentBoardDir(t *testing.T) {
	t.Helper()
	orig := intentBoardDir
	intentBoardDir = t.TempDir()
	t.Cleanup(func() { intentBoardDir = orig })
}

func resetIntentFlags() {
	intentDeclareSession = ""
	intentDeclareFiles = nil
	intentDeclarePackages = nil
	intentDeclareDescription = ""
	intentDeclareTTL = 0
	intentListSession = ""
	intentListIncludeAll = false
	intentListOverlapping = false
	intentOutputForm = ""
}

func TestIntentDeclareRequiresSession(t *testing.T) {
	withIntentBoardDir(t)
	resetIntentFlags()
	t.Setenv("AGM_SESSION_NAME", "")

	intentDeclareFiles = []string{"a.go"}
	if err := runIntentDeclare(nil, nil); err == nil {
		t.Error("missing session should fail")
	}
}

func TestIntentDeclareUsesEnvFallback(t *testing.T) {
	withIntentBoardDir(t)
	resetIntentFlags()
	t.Setenv("AGM_SESSION_NAME", "envsess")

	intentDeclareFiles = []string{"a.go"}
	stdout := captureStdout(t, func() {
		if err := runIntentDeclare(nil, nil); err != nil {
			t.Fatalf("runIntentDeclare: %v", err)
		}
	})
	if !strings.Contains(stdout, "envsess") {
		t.Errorf("declared output missing session: %q", stdout)
	}
}

func TestIntentDeclareRequiresScope(t *testing.T) {
	withIntentBoardDir(t)
	resetIntentFlags()
	intentDeclareSession = "s1"
	if err := runIntentDeclare(nil, nil); err == nil {
		t.Error("missing files+packages should fail")
	}
}

func TestIntentListAndExpireRoundTrip(t *testing.T) {
	withIntentBoardDir(t)
	resetIntentFlags()

	board, err := resolveIntentBoard()
	if err != nil {
		t.Fatalf("resolveIntentBoard: %v", err)
	}
	for _, sess := range []string{"alpha", "beta"} {
		if _, err := board.Declare(intent.DeclareOpts{
			SessionID: sess,
			Files:     []string{sess + ".go"},
			TTL:       time.Hour,
		}); err != nil {
			t.Fatalf("Declare %s: %v", sess, err)
		}
	}

	intentOutputForm = "json"
	stdout := captureStdout(t, func() {
		if err := runIntentList(nil, nil); err != nil {
			t.Fatalf("runIntentList: %v", err)
		}
	})
	var rows []intent.Intent
	if err := json.Unmarshal([]byte(stdout), &rows); err != nil {
		t.Fatalf("parse JSON: %v\n%s", err, stdout)
	}
	if len(rows) != 2 {
		t.Errorf("rows = %d, want 2", len(rows))
	}

	// Filter by session
	intentListSession = "alpha"
	stdout = captureStdout(t, func() {
		if err := runIntentList(nil, nil); err != nil {
			t.Fatalf("runIntentList: %v", err)
		}
	})
	if err := json.Unmarshal([]byte(stdout), &rows); err != nil {
		t.Fatalf("parse JSON: %v\n%s", err, stdout)
	}
	if len(rows) != 1 || rows[0].SessionID != "alpha" {
		t.Errorf("filtered rows = %+v", rows)
	}
}

func TestIntentListOverlapping(t *testing.T) {
	withIntentBoardDir(t)
	resetIntentFlags()

	board, err := resolveIntentBoard()
	if err != nil {
		t.Fatalf("resolveIntentBoard: %v", err)
	}
	if _, err := board.Declare(intent.DeclareOpts{SessionID: "alpha", Files: []string{"shared.go", "a.go"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := board.Declare(intent.DeclareOpts{SessionID: "beta", Files: []string{"shared.go"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := board.Declare(intent.DeclareOpts{SessionID: "gamma", Files: []string{"unrelated.go"}}); err != nil {
		t.Fatal(err)
	}

	intentListOverlapping = true
	intentOutputForm = "json"
	stdout := captureStdout(t, func() {
		if err := runIntentList(nil, nil); err != nil {
			t.Fatalf("runIntentList: %v", err)
		}
	})
	var rows []intent.Intent
	if err := json.Unmarshal([]byte(stdout), &rows); err != nil {
		t.Fatalf("parse JSON: %v", err)
	}
	if len(rows) != 2 {
		t.Errorf("expected 2 overlapping rows (alpha, beta), got %d: %+v", len(rows), rows)
	}
	for _, r := range rows {
		if r.SessionID == "gamma" {
			t.Errorf("gamma should not be in overlapping set")
		}
	}
}

func TestIntentRemove(t *testing.T) {
	withIntentBoardDir(t)
	resetIntentFlags()

	board, err := resolveIntentBoard()
	if err != nil {
		t.Fatalf("resolveIntentBoard: %v", err)
	}
	in, err := board.Declare(intent.DeclareOpts{SessionID: "s1", Files: []string{"x.go"}})
	if err != nil {
		t.Fatal(err)
	}

	if err := runIntentRemove(nil, []string{in.ID}); err != nil {
		t.Fatalf("runIntentRemove: %v", err)
	}
	if err := runIntentRemove(nil, []string{in.ID}); err == nil {
		t.Error("removing twice should fail")
	}
}
