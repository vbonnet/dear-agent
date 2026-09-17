package deploy

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/pkg/recoveryloop"
)

// configuredPulseNames returns every pulse name absence-alarm is deployed to watch.
func configuredPulseNames(t *testing.T) map[string]bool {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "deploy", "absence-alarm", "pulses.json"))
	if err != nil {
		t.Fatalf("read deploy/absence-alarm/pulses.json: %v", err)
	}
	var doc struct {
		Pulses []struct {
			Name string `json:"name"`
		} `json:"pulses"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parse pulses.json: %v", err)
	}
	names := make(map[string]bool, len(doc.Pulses))
	for _, p := range doc.Pulses {
		names[p.Name] = true
	}
	if len(names) == 0 {
		t.Fatal("pulses.json declares no pulses")
	}
	return names
}

// TestRecoveryJobPulsesAreEmitted asserts that every pulse named by a recovery
// job resolves to a pulse absence-alarm actually emits.
//
// A recovery job whose pulse nothing emits can never be verified: the loop has
// no evidence either way, forever. Before the verified-recovery change that was
// invisible, because a job with no pulse evidence fell through to the structural
// checks and reported HEALTHY, which is precisely the false-green this whole
// area exists to remove. It now surfaces as "health unverifiable", which is
// honest but still useless as monitoring.
//
// This is the wire-it-or-delete-it rule applied to the monitor layer itself
// (ce-64333, a slice of ce-ja1f): a job may declare a pulse or declare none,
// but it may not name one that does not exist.
func TestRecoveryJobPulsesAreEmitted(t *testing.T) {
	configured := configuredPulseNames(t)

	check := func(t *testing.T, source string, jobs []recoveryloop.Job) {
		t.Helper()
		var orphans []string
		for _, j := range jobs {
			if j.Pulse == "" {
				// A job with no pulse is verified structurally. That is a
				// deliberate, visible choice, not a dangling reference.
				continue
			}
			if !configured[j.Pulse] {
				orphans = append(orphans, j.Name+" -> "+j.Pulse)
			}
		}
		sort.Strings(orphans)
		for _, o := range orphans {
			t.Errorf("%s: recovery job names a pulse that no pulse emits: %s", source, o)
		}
	}

	t.Run("built-in registry", func(t *testing.T) {
		check(t, "pkg/recoveryloop.DefaultJobs", recoveryloop.DefaultJobs())
	})

	t.Run("deployed registry", func(t *testing.T) {
		raw, err := os.ReadFile(filepath.Join("..", "..", "deploy", "recovery-loop", "jobs.json"))
		if err != nil {
			t.Fatalf("read deploy/recovery-loop/jobs.json: %v", err)
		}
		var doc struct {
			Jobs []recoveryloop.Job `json:"jobs"`
		}
		if err := json.Unmarshal(raw, &doc); err != nil {
			t.Fatalf("parse jobs.json: %v", err)
		}
		if len(doc.Jobs) == 0 {
			t.Fatal("deploy/recovery-loop/jobs.json declares no jobs")
		}
		check(t, "deploy/recovery-loop/jobs.json", doc.Jobs)
	})
}

// TestJSONTimestampPulsesAreNotAppendOnlyLogs asserts that no json_timestamp
// pulse points at a .jsonl file.
//
// extractJSONTimestamp decodes a single JSON value from the head of the stream.
// On a JSONL file that is the FIRST line, which in an append-only audit log is
// the OLDEST record. Pointing a freshness probe at it reads an age that only
// ever grows, so the pulse alarms forever no matter how healthy the writer is.
// Measured on this host when the trap was hit: the first record in
// token-refresher-audit.jsonl was dated 2026-06-19 while the last was minutes
// old. Use file_mtime for append-only logs.
func TestJSONTimestampPulsesAreNotAppendOnlyLogs(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "deploy", "absence-alarm", "pulses.json"))
	if err != nil {
		t.Fatalf("read deploy/absence-alarm/pulses.json: %v", err)
	}
	var doc struct {
		Pulses []struct {
			Name string `json:"name"`
			Type string `json:"type"`
			Path string `json:"path"`
		} `json:"pulses"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parse pulses.json: %v", err)
	}
	for _, p := range doc.Pulses {
		if p.Type == "json_timestamp" && filepath.Ext(p.Path) == ".jsonl" {
			t.Errorf("pulse %q uses json_timestamp on the append-only log %s: "+
				"the probe reads the first record, which is the oldest, so this pulse "+
				"would alarm forever. Use file_mtime.", p.Name, p.Path)
		}
	}
}

func TestTokenRefresherPulseUsesCadenceOnlyLaunchdEvidence(t *testing.T) {
	root := filepath.Join("..", "..")
	plistRaw, err := os.ReadFile(filepath.Join(root, "deploy", "launchd", "com.dear-agent.token-refresher.plist"))
	if err != nil {
		t.Fatalf("read token-refresher plist: %v", err)
	}
	args := programArguments(t, string(plistRaw))
	var cadenceAudit string
	for i := 0; i+1 < len(args); i++ {
		if args[i] == "-audit-log" {
			cadenceAudit = strings.Replace(args[i+1], "__HOME__", "~", 1)
			break
		}
	}
	if cadenceAudit == "" {
		t.Fatal("token-refresher LaunchAgent has no explicit cadence audit path")
	}
	if cadenceAudit == "~/.local/state/dear-agent/token-refresher-audit.jsonl" {
		t.Fatal("cadence audit path is still shared with manual token-refresher invocations")
	}

	pulsesRaw, err := os.ReadFile(filepath.Join(root, "deploy", "absence-alarm", "pulses.json"))
	if err != nil {
		t.Fatalf("read pulses.json: %v", err)
	}
	var doc struct {
		Pulses []struct {
			Name string `json:"name"`
			Type string `json:"type"`
			Path string `json:"path"`
		} `json:"pulses"`
	}
	if err := json.Unmarshal(pulsesRaw, &doc); err != nil {
		t.Fatalf("parse pulses.json: %v", err)
	}
	for _, p := range doc.Pulses {
		if p.Name != "token-refresher-tick" {
			continue
		}
		if p.Type != "file_mtime" || p.Path != cadenceAudit {
			t.Fatalf("token-refresher pulse = type %q path %q, want file_mtime on cadence path %q", p.Type, p.Path, cadenceAudit)
		}
		return
	}
	t.Fatal("pulses.json has no token-refresher-tick pulse")
}
