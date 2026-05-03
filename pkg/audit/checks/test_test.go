package checks

import "testing"

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
