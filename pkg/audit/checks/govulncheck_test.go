package checks

import "testing"

// TestParseGovulncheckOutput pins the JSON-stream parser. The fixture
// reproduces govulncheck's "OSV record then findings record" pattern.
// We expect one rolled-up entry per (osv, package) where at least one
// trace step has a function name (i.e. an actual call).
func TestParseGovulncheckOutput(t *testing.T) {
	stream := `{"osv":{"id":"GO-2024-0001","summary":"path traversal in foo"}}
{"finding":{"osv":"GO-2024-0001","trace":[{"module":"foo","package":"foo","function":"Open"}],"fixed_version":"v1.2.3"}}
{"osv":{"id":"GO-2024-0002","summary":"unrelated"}}
{"finding":{"osv":"GO-2024-0002","trace":[{"module":"foo","package":"foo"}]}}
`
	got, err := parseGovulncheckOutput(stream)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 called vuln, got %d: %+v", len(got), got)
	}
	if got[0].OSV != "GO-2024-0001" || got[0].Package != "foo" || got[0].Fixed != "v1.2.3" {
		t.Errorf("rolled-up wrong: %+v", got[0])
	}
	if got[0].Summary != "path traversal in foo" {
		t.Errorf("summary not rolled in: %+v", got[0])
	}
}

func TestParseGovulncheckEmpty(t *testing.T) {
	got, err := parseGovulncheckOutput("")
	if err != nil {
		t.Errorf("empty stream: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected 0, got %d", len(got))
	}
}
