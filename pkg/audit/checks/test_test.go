package checks

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/vbonnet/dear-agent/pkg/audit"
)

// TestParseTestFailures pins the test2json parser against a small
// canned fragment. Real failures show up as one entry per (package,
// test); successful tests emit no rows.
func TestParseTestFailures(t *testing.T) {
	stdout := `{"Time":"2026-05-03T12:00:00Z","Action":"run","Package":"foo","Test":"TestPasses"}
{"Time":"2026-05-03T12:00:01Z","Action":"output","Package":"foo","Test":"TestPasses","Output":"PASS\n"}
{"Time":"2026-05-03T12:00:01Z","Action":"pass","Package":"foo","Test":"TestPasses"}
{"Time":"2026-05-03T12:00:02Z","Action":"run","Package":"foo","Test":"TestFails"}
{"Time":"2026-05-03T12:00:03Z","Action":"output","Package":"foo","Test":"TestFails","Output":"--- FAIL: TestFails\n"}
{"Time":"2026-05-03T12:00:03Z","Action":"output","Package":"foo","Test":"TestFails","Output":"    main_test.go:42: bad value\n"}
{"Time":"2026-05-03T12:00:03Z","Action":"fail","Package":"foo","Test":"TestFails"}
`
	failures := parseTestFailures(stdout)
	if len(failures) != 1 {
		t.Fatalf("expected 1 failure, got %d: %+v", len(failures), failures)
	}
	if failures[0].Test != "TestFails" || failures[0].Package != "foo" {
		t.Errorf("wrong failure: %+v", failures[0])
	}
	if failures[0].Output == "" {
		t.Error("output should be captured")
	}
}

func TestParseTestFailuresBuildFailure(t *testing.T) {
	stdout := `{"Time":"2026-05-03T12:00:00Z","Action":"output","Package":"foo","Output":"FAIL\tfoo [build failed]\n"}
{"Time":"2026-05-03T12:00:00Z","Action":"fail","Package":"foo"}
`
	failures := parseTestFailures(stdout)
	if len(failures) != 1 {
		t.Fatalf("expected 1 failure, got %d: %+v", len(failures), failures)
	}
	if failures[0].Test != "<build>" {
		t.Errorf("build failure should be Test=<build>; got %q", failures[0].Test)
	}
}

func TestTestCheckEmitsPatchlessPRInvestigation(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"go.mod":       "module example.com/failing\n\ngo 1.25\n",
		"fail_test.go": "package failing\n\nimport \"testing\"\n\nfunc TestFails(t *testing.T) { t.Fatal(\"boom\") }\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	result, err := (TestCheck{}).Run(context.Background(), audit.Env{WorkingDir: dir})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(result.Findings) == 0 {
		t.Fatal("Findings is empty, want a failed-test investigation")
	}
	for _, finding := range result.Findings {
		if finding.Evidence["test"] != "TestFails" {
			continue
		}
		suggestion := finding.Suggested
		if suggestion.Strategy != audit.StrategyPR || suggestion.Patch != "" {
			t.Fatalf("suggestion = %+v, want patchless PR investigation", suggestion)
		}
		if suggestion.Title == "" {
			t.Fatal("patchless PR investigation should retain operator context")
		}
		return
	}
	t.Fatal("Findings does not contain TestFails evidence")
}
