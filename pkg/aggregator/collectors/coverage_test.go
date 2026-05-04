package collectors

import (
	"context"
	"errors"
	"math"
	"testing"
)

const sampleCoverProfile = `mode: set
github.com/x/y/pkg/foo/bar.go:10.1,12.2 1 1
github.com/x/y/pkg/foo/bar.go:14.1,16.2 2 0
github.com/x/y/pkg/baz/qux.go:20.1,22.2 5 1
github.com/x/y/pkg/baz/qux.go:24.1,26.2 5 1
`

func TestParseCoverProfile(t *testing.T) {
	t.Parallel()
	sigs, err := parseCoverProfile([]byte(sampleCoverProfile))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	bySubject := map[string]float64{}
	for _, s := range sigs {
		bySubject[s.Subject] = s.Value
	}
	// foo: 1 of 3 stmts covered → 33.33%
	if v := bySubject["github.com/x/y/pkg/foo"]; math.Abs(v-(1.0/3.0*100)) > 0.01 {
		t.Errorf("foo coverage = %f, want ~33.33", v)
	}
	// baz: 10 of 10 stmts → 100%
	if v := bySubject["github.com/x/y/pkg/baz"]; math.Abs(v-100) > 0.01 {
		t.Errorf("baz coverage = %f, want 100", v)
	}
}

func TestParseCoverProfileEmpty(t *testing.T) {
	t.Parallel()
	sigs, err := parseCoverProfile(nil)
	if err != nil {
		t.Fatalf("parse(nil): %v", err)
	}
	if len(sigs) != 0 {
		t.Errorf("parse(nil) returned %d signals, want 0", len(sigs))
	}
}

func TestTestCoverageCollect(t *testing.T) {
	t.Parallel()
	c := &TestCoverage{
		ProfilePath: "/fake/cover.out",
		ReadFile: func(_ string) ([]byte, error) {
			return []byte(sampleCoverProfile), nil
		},
	}
	sigs, err := c.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if len(sigs) != 2 {
		t.Errorf("got %d signals, want 2", len(sigs))
	}
}

func TestTestCoverageCollectMissingFile(t *testing.T) {
	t.Parallel()
	c := &TestCoverage{
		ProfilePath: "/fake/missing.out",
		ReadFile:    func(_ string) ([]byte, error) { return nil, errors.New("nope") },
	}
	if _, err := c.Collect(context.Background()); err == nil {
		t.Error("missing file should error")
	}
}

func TestTestCoverageEmptyPath(t *testing.T) {
	t.Parallel()
	c := &TestCoverage{}
	if _, err := c.Collect(context.Background()); err == nil {
		t.Error("empty ProfilePath should error")
	}
}

func TestPkgOfPath(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"a/b/c.go":           "a/b",
		"single.go":          "single.go",
		"github.com/x/y.go":  "github.com/x",
	}
	for in, want := range cases {
		if got := pkgOfPath(in); got != want {
			t.Errorf("pkgOfPath(%q) = %q, want %q", in, got, want)
		}
	}
}
