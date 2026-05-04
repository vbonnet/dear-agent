package collectors

import (
	"context"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/pkg/aggregator"
)

const sampleGoListUM = `{"Path":"github.com/me/mymod","Main":true,"Version":"v0.0.0"}
{"Path":"github.com/foo/bar","Version":"v1.0.0","Update":{"Version":"v1.2.0"}}
{"Path":"github.com/baz/qux","Version":"v2.0.0"}
{"Path":"github.com/abc/xyz","Version":"v0.1.0","Update":{"Version":"v0.2.0"}}
`

func TestParseGoListUM(t *testing.T) {
	t.Parallel()
	sigs, err := parseGoListUM([]byte(sampleGoListUM))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(sigs) != 2 {
		t.Fatalf("got %d signals, want 2 (one per outdated dep)", len(sigs))
	}
	subjects := []string{sigs[0].Subject, sigs[1].Subject}
	wantSubjects := map[string]bool{"github.com/foo/bar": false, "github.com/abc/xyz": false}
	for _, s := range subjects {
		if _, ok := wantSubjects[s]; !ok {
			t.Errorf("unexpected subject %q", s)
		}
		wantSubjects[s] = true
	}
	for s, seen := range wantSubjects {
		if !seen {
			t.Errorf("missing subject %q", s)
		}
	}
	for _, s := range sigs {
		if s.Value != 1 {
			t.Errorf("Value = %v, want 1", s.Value)
		}
		if !strings.Contains(s.Metadata, "current") {
			t.Errorf("Metadata missing version info: %s", s.Metadata)
		}
	}
}

func TestParseGoListUMEmpty(t *testing.T) {
	t.Parallel()
	sigs, err := parseGoListUM(nil)
	if err != nil {
		t.Fatalf("parse(nil): %v", err)
	}
	if len(sigs) != 0 {
		t.Errorf("parse(nil) returned %d signals, want 0", len(sigs))
	}
}

func TestParseGoListUMMalformed(t *testing.T) {
	t.Parallel()
	if _, err := parseGoListUM([]byte("{not json")); err == nil {
		t.Error("malformed JSON should fail")
	}
}

func TestDepFreshnessCollect(t *testing.T) {
	t.Parallel()
	c := &DepFreshness{
		Repo:       "/repo",
		LookPathFn: okLookPath,
		Exec: func(_ context.Context, dir, name string, args ...string) ([]byte, error) {
			if name != "go" || args[0] != "list" {
				t.Errorf("unexpected exec: %s %v", name, args)
			}
			if dir != "/repo" {
				t.Errorf("dir = %q, want /repo", dir)
			}
			return []byte(sampleGoListUM), nil
		},
	}
	sigs, err := c.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if len(sigs) != 2 {
		t.Errorf("got %d signals, want 2", len(sigs))
	}
	for _, s := range sigs {
		if s.Kind != aggregator.KindDepFreshness {
			t.Errorf("Kind = %s, want dep_freshness", s.Kind)
		}
	}
}

func TestDepFreshnessToolMissing(t *testing.T) {
	t.Parallel()
	c := &DepFreshness{Repo: "/r", LookPathFn: missingLookPath}
	_, err := c.Collect(context.Background())
	if !aggregator.IsToolMissing(err) {
		t.Errorf("expected ErrToolMissing, got %v", err)
	}
}

func TestDepFreshnessEmptyRepo(t *testing.T) {
	t.Parallel()
	c := &DepFreshness{}
	if _, err := c.Collect(context.Background()); err == nil {
		t.Error("empty Repo should fail")
	}
}
