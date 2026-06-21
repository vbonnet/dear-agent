package supervisor

import (
	"context"
	"errors"
	"testing"
)

func TestSupervisedReclaimer_ParsesJSON(t *testing.T) {
	r := &SupervisedReclaimer{
		MaxRSSMB: 1500,
		run: func(_ context.Context, name string, args ...string) ([]byte, error) {
			if name != "agm-aware-reaper" {
				t.Errorf("expected agm-aware-reaper, got %s", name)
			}
			// --json and --max-rss-mb 1500 must be present.
			joined := args
			var sawJSON, sawRSS bool
			for i, a := range joined {
				if a == "--json" {
					sawJSON = true
				}
				if a == "--max-rss-mb" && i+1 < len(joined) && joined[i+1] == "1500" {
					sawRSS = true
				}
			}
			if !sawJSON || !sawRSS {
				t.Errorf("missing expected flags in %v", joined)
			}
			return []byte(`{"dry_run":false,"candidates":5,"protected":2,"reapable":3,"terminated":2,"errors":["boom"]}`), nil
		},
	}
	res, err := r.Reclaim(context.Background())
	if err != nil {
		t.Fatalf("Reclaim: %v", err)
	}
	if res.OrphansFound != 3 || res.OrphansKilled != 2 || res.OrphansFailed != 1 {
		t.Errorf("unexpected mapping: %+v", res)
	}
}

func TestSupervisedReclaimer_UnparseableIsError(t *testing.T) {
	r := &SupervisedReclaimer{
		run: func(context.Context, string, ...string) ([]byte, error) {
			return []byte("not json"), errors.New("exit 1")
		},
	}
	if _, err := r.Reclaim(context.Background()); err == nil {
		t.Fatal("expected an error on unparseable output")
	}
}

// TestSupervisedReclaimer_PartialExitStillParses asserts a non-zero exit with a
// valid JSON summary is treated as progress, not failure.
func TestSupervisedReclaimer_PartialExitStillParses(t *testing.T) {
	r := &SupervisedReclaimer{
		run: func(context.Context, string, ...string) ([]byte, error) {
			return []byte(`{"reapable":1,"terminated":0,"errors":["kill failed"]}`), errors.New("exit 2")
		},
	}
	res, err := r.Reclaim(context.Background())
	if err != nil {
		t.Fatalf("Reclaim should tolerate a parseable non-zero exit: %v", err)
	}
	if res.OrphansFound != 1 || res.OrphansKilled != 0 || res.OrphansFailed != 1 {
		t.Errorf("unexpected mapping: %+v", res)
	}
}
