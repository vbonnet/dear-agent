package main

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// TestStructuredOutputGoesThroughCentralPath is a regression guard for the
// ce-s2tn token-efficiency epic (bead ce-s2tn.8).
//
// The central output path — printJSON / printResult / ops.ApplyFieldMask — is
// the only place structured command output should be serialized, because it is
// where the --fields field mask is applied. A command that marshals JSON and
// writes it straight to stdout (fmt.Print/Println/Printf(string(...))) silently
// ignores --fields, defeating the whole epic. session kill/list were the first
// offenders (fixed in ce-s2tn.2); a2a/dashboard/doctor/safety/reserve/test/
// workflow/escalate/metrics and the --list-commands-json discovery path were
// migrated in ce-s2tn.8.
//
// This test scans every non-test .go file in the package and fails if a file
// both marshals JSON and prints a string(...) conversion to stdout, unless the
// file is in centralPathAllowlist with a documented reason.
func TestStructuredOutputGoesThroughCentralPath(t *testing.T) {
	// Files exempt from the guard, each for a documented reason. Keep this map
	// small — every entry is a place --fields does NOT work, so think twice.
	centralPathAllowlist := map[string]string{
		"output.go": "implements the central path (printJSON itself prints string(...))",
		"scan.go": "continuous `agm scan` daemon loop streams a pretty-JSON snapshot " +
			"each cycle — a monitoring stream, not a single masked result",
		"watch_stalled.go": "streams line-delimited NDJSON (one compact JSON object per " +
			"stalled session); a pretty-printed/masked blob would break the stream",
		"results_courier.go": "rides watch_stalled's same NDJSON stream (detected-events and " +
			"delivery-receipt lines); same reason as watch_stalled.go — it's a monitoring " +
			"log feed, not a single --fields-maskable command result",
	}

	// fmt.Print / Println / Printf all write to stdout. Fprint* (e.g. to
	// os.Stderr) is intentionally excluded — human-facing stderr is fine.
	stdoutPrintRe := regexp.MustCompile(`fmt\.Print(ln|f)?\([^)]*string\(`)

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read package dir: %v", err)
	}

	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		text := string(src)

		// Only files that serialize JSON can leak structured data.
		if !strings.Contains(text, "json.Marshal") {
			continue
		}
		if !stdoutPrintRe.MatchString(text) {
			continue
		}

		if reason, ok := centralPathAllowlist[name]; ok {
			if reason == "" {
				t.Errorf("%s: allowlisted with an empty reason; document why it is exempt", name)
			}
			continue
		}

		t.Errorf("%s: marshals JSON and prints string(...) to stdout directly, "+
			"bypassing the central output path. Route structured output through "+
			"printJSON / printResult so -o json and --fields are honored. If this "+
			"is intentional (streaming protocol, wire-payload preview, or spec "+
			"format), add it to centralPathAllowlist with a reason.", name)
	}
}
