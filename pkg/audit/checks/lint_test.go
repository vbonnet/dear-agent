package checks

import "testing"

// TestParseLintReport pins the golangci-lint parser. The fixture is
// a hand-trimmed version of the v2 JSON output.
func TestParseLintReport(t *testing.T) {
	stdout := `{"Issues":[
		{"FromLinter":"errcheck","Text":"Error return value of x.Close is not checked","Severity":"error",
		 "Pos":{"Filename":"main.go","Line":42,"Column":3}},
		{"FromLinter":"staticcheck","Text":"SA1019: foo is deprecated","Severity":"warning",
		 "Pos":{"Filename":"util.go","Line":7,"Column":1}}
	]}`
	r, err := parseLintReport(stdout)
	if err != nil {
		t.Fatalf("parseLintReport: %v", err)
	}
	if len(r.Issues) != 2 {
		t.Fatalf("issues = %d, want 2", len(r.Issues))
	}
	if r.Issues[0].FromLinter != "errcheck" || r.Issues[0].Pos.Line != 42 {
		t.Errorf("first issue wrong: %+v", r.Issues[0])
	}
}

func TestParseLintReportEmpty(t *testing.T) {
	r, err := parseLintReport("")
	if err != nil {
		t.Errorf("empty input should not error: %v", err)
	}
	if len(r.Issues) != 0 {
		t.Errorf("expected 0 issues, got %d", len(r.Issues))
	}
}

func TestParseLintReportNullIssues(t *testing.T) {
	r, err := parseLintReport(`{"Issues":null}`)
	if err != nil {
		t.Errorf("null Issues should not error: %v", err)
	}
	if len(r.Issues) != 0 {
		t.Errorf("expected 0 issues, got %d", len(r.Issues))
	}
}

func TestParseLintReportTrailingTextSummary(t *testing.T) {
	stdout := `{"Issues":[{"FromLinter":"errcheck","Text":"unchecked","Severity":"warning","Pos":{"Filename":"a.go","Line":1}}],"Report":{"Linters":[]}}
0 issues.
`
	r, err := parseLintReport(stdout)
	if err != nil {
		t.Fatalf("parseLintReport with v2 trailing text: %v", err)
	}
	if len(r.Issues) != 1 {
		t.Errorf("issues = %d, want 1", len(r.Issues))
	}
}
