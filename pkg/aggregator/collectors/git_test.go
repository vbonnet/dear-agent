package collectors

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/pkg/aggregator"
)

func okLookPath(string) (string, error) { return "/usr/bin/found", nil }
func missingLookPath(string) (string, error) {
	return "", errors.New("not found")
}

func TestGitActivityCollect(t *testing.T) {
	t.Parallel()
	gitOutput := []byte("aaa\nbbb\nccc\n")
	c := &GitActivity{
		Repo:         "/repo",
		LookbackDays: 14,
		LookPathFn:   okLookPath,
		Exec: func(_ context.Context, dir, name string, args ...string) ([]byte, error) {
			if dir != "/repo" {
				t.Errorf("Exec dir = %q, want /repo", dir)
			}
			if name != "git" {
				t.Errorf("Exec name = %q, want git", name)
			}
			joined := strings.Join(args, " ")
			if !strings.Contains(joined, "--since=14 days ago") {
				t.Errorf("missing --since flag: %s", joined)
			}
			return gitOutput, nil
		},
	}
	sigs, err := c.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if len(sigs) != 1 {
		t.Fatalf("got %d signals, want 1", len(sigs))
	}
	if sigs[0].Value != 3 {
		t.Errorf("Value = %v, want 3", sigs[0].Value)
	}
	if sigs[0].Kind != aggregator.KindGitActivity {
		t.Errorf("Kind = %s, want git_activity", sigs[0].Kind)
	}
	if sigs[0].Subject != "/repo" {
		t.Errorf("Subject = %q, want /repo", sigs[0].Subject)
	}
	if !strings.Contains(sigs[0].Metadata, "lookbackDays") {
		t.Errorf("Metadata missing lookbackDays: %s", sigs[0].Metadata)
	}
}

func TestGitActivityToolMissing(t *testing.T) {
	t.Parallel()
	c := &GitActivity{Repo: "/repo", LookPathFn: missingLookPath}
	_, err := c.Collect(context.Background())
	if !aggregator.IsToolMissing(err) {
		t.Errorf("expected ErrToolMissing, got %v", err)
	}
}

func TestGitActivityEmptyRepo(t *testing.T) {
	t.Parallel()
	c := &GitActivity{}
	if _, err := c.Collect(context.Background()); err == nil {
		t.Error("empty Repo should fail")
	}
}

func TestGitActivityZeroCommits(t *testing.T) {
	t.Parallel()
	c := &GitActivity{
		Repo:       "/repo",
		LookPathFn: okLookPath,
		Exec: func(_ context.Context, _, _ string, _ ...string) ([]byte, error) {
			return nil, nil
		},
	}
	sigs, err := c.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if len(sigs) != 1 || sigs[0].Value != 0 {
		t.Errorf("expected one signal with Value=0, got %+v", sigs)
	}
}

func TestCountLines(t *testing.T) {
	t.Parallel()
	cases := map[string]int{
		"":           0,
		"a":          1,
		"a\n":        1,
		"a\nb":       2,
		"a\nb\n":     2,
		"a\nb\nc\n":  3,
		"\n":         0,
	}
	for in, want := range cases {
		if got := countLines([]byte(in)); got != want {
			t.Errorf("countLines(%q) = %d, want %d", in, got, want)
		}
	}
}
