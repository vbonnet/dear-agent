package collectors

import (
	"context"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/pkg/aggregator"
)

const sampleGovulncheckJSON = `
{"osv":{"id":"GO-2024-0001","summary":"buffer overflow in foo"}}
{"finding":{"osv":"GO-2024-0001","trace":[{"package":"github.com/foo/bar"}]}}
{"finding":{"osv":"GO-2024-0001","trace":[{"package":"github.com/foo/baz"}]}}
{"osv":{"id":"GO-2024-0002","summary":"timing leak in qux"}}
{"finding":{"osv":"GO-2024-0002","trace":[{"package":"github.com/abc/xyz"}]}}
`

func TestParseGovulncheck(t *testing.T) {
	t.Parallel()
	sigs, err := parseGovulncheck([]byte(sampleGovulncheckJSON))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(sigs) != 2 {
		t.Fatalf("got %d signals, want 2 (one per OSV ID)", len(sigs))
	}
	bySubject := map[string]aggregator.Signal{}
	for _, s := range sigs {
		bySubject[s.Subject] = s
	}
	g1, ok := bySubject["GO-2024-0001"]
	if !ok {
		t.Fatal("missing GO-2024-0001")
	}
	if !strings.Contains(g1.Metadata, "buffer overflow") {
		t.Errorf("metadata missing summary: %s", g1.Metadata)
	}
	if !strings.Contains(g1.Metadata, "github.com/foo/bar") ||
		!strings.Contains(g1.Metadata, "github.com/foo/baz") {
		t.Errorf("metadata missing affected packages: %s", g1.Metadata)
	}
}

func TestParseGovulncheckEmpty(t *testing.T) {
	t.Parallel()
	sigs, err := parseGovulncheck(nil)
	if err != nil {
		t.Fatalf("parse(nil): %v", err)
	}
	if len(sigs) != 0 {
		t.Errorf("parse(nil) returned %d signals, want 0", len(sigs))
	}
}

func TestSecurityAlertsInputFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := dir + "/vuln.json"
	if err := writeFile(path, []byte(sampleGovulncheckJSON)); err != nil {
		t.Fatal(err)
	}
	c := &SecurityAlerts{InputFile: path}
	sigs, err := c.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if len(sigs) != 2 {
		t.Errorf("got %d signals, want 2", len(sigs))
	}
}

func TestSecurityAlertsToolMissing(t *testing.T) {
	t.Parallel()
	c := &SecurityAlerts{Repo: "/r", LookPathFn: missingLookPath}
	_, err := c.Collect(context.Background())
	if !aggregator.IsToolMissing(err) {
		t.Errorf("expected ErrToolMissing, got %v", err)
	}
}

func TestSecurityAlertsNoRepoNoFile(t *testing.T) {
	t.Parallel()
	c := &SecurityAlerts{}
	if _, err := c.Collect(context.Background()); err == nil {
		t.Error("missing both Repo and InputFile should fail")
	}
}
