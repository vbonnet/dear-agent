package collectors

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/vbonnet/dear-agent/pkg/aggregator"
)

// TestCoverage parses a Go coverage profile (the file produced by
// `go test -coverprofile=cover.out`) and emits one signal per package
// with Value set to the per-package coverage percent.
//
// The collector deliberately does not run `go test` itself: tests are
// already running in CI and on developer machines, so reusing the
// existing profile is faster and avoids inflating run time. Operators
// that want a fresh profile can run go test before this collector.
type TestCoverage struct {
	ProfilePath string // path to coverage profile (required in v1)
	ReadFile    func(string) ([]byte, error) // nil → readFile
}

// Name implements aggregator.Collector.
func (c *TestCoverage) Name() string { return "dear-agent.coverage" }

// Kind implements aggregator.Collector.
func (c *TestCoverage) Kind() aggregator.Kind { return aggregator.KindTestCoverage }

// Collect reads the configured profile and emits per-package signals.
// An empty or missing profile is reported as an error so operators
// notice misconfiguration; an empty profile is *not* "100% coverage".
func (c *TestCoverage) Collect(ctx context.Context) ([]aggregator.Signal, error) {
	if strings.TrimSpace(c.ProfilePath) == "" {
		return nil, fmt.Errorf("collectors.TestCoverage: empty ProfilePath")
	}
	read := c.ReadFile
	if read == nil {
		read = readFile
	}
	raw, err := read(c.ProfilePath)
	if err != nil {
		return nil, fmt.Errorf("collectors.TestCoverage: read %s: %w",
			c.ProfilePath, err)
	}
	return parseCoverProfile(raw)
}

// parseCoverProfile parses Go's coverage profile format:
//
//	mode: set
//	pkg/path/file.go:start.col,end.col numStatements count
//
// Each statement in a package contributes; per-package coverage is
// (covered statements) / (total statements) * 100.
func parseCoverProfile(raw []byte) ([]aggregator.Signal, error) {
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	type counts struct {
		total, covered int
	}
	perPkg := map[string]*counts{}

	first := true
	for scanner.Scan() {
		line := scanner.Text()
		if first {
			first = false
			if strings.HasPrefix(line, "mode:") {
				continue
			}
		}
		if line == "" {
			continue
		}
		// "pkg/path/file.go:start.col,end.col numStatements count"
		spaceA := strings.LastIndexByte(line, ' ')
		if spaceA < 0 {
			continue
		}
		countStr := line[spaceA+1:]
		head := line[:spaceA]
		spaceB := strings.LastIndexByte(head, ' ')
		if spaceB < 0 {
			continue
		}
		stmtStr := head[spaceB+1:]
		head = head[:spaceB]
		colon := strings.IndexByte(head, ':')
		if colon < 0 {
			continue
		}
		fileWithPath := head[:colon]
		pkg := pkgOfPath(fileWithPath)

		stmts, err := strconv.Atoi(stmtStr)
		if err != nil {
			continue
		}
		cnt, err := strconv.Atoi(countStr)
		if err != nil {
			continue
		}
		c, ok := perPkg[pkg]
		if !ok {
			c = &counts{}
			perPkg[pkg] = c
		}
		c.total += stmts
		if cnt > 0 {
			c.covered += stmts
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("collectors.TestCoverage: scan: %w", err)
	}

	sigs := make([]aggregator.Signal, 0, len(perPkg))
	for pkg, c := range perPkg {
		pct := 0.0
		if c.total > 0 {
			pct = float64(c.covered) / float64(c.total) * 100
		}
		sigs = append(sigs, aggregator.Signal{
			Kind:    aggregator.KindTestCoverage,
			Subject: pkg,
			Value:   pct,
		})
	}
	return sigs, nil
}

// pkgOfPath reduces "github.com/x/y/pkg/foo/bar.go" to
// "github.com/x/y/pkg/foo" — the directory containing the file.
func pkgOfPath(filePath string) string {
	if i := strings.LastIndexByte(filePath, '/'); i >= 0 {
		return filePath[:i]
	}
	return filePath
}
