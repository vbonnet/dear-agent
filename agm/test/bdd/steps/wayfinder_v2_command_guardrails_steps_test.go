package steps

import (
	"strings"
	"testing"
)

func TestRetiredWayfinderPatternMatchesPrefixedIdentifiers(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  bool
	}{
		{name: "uppercase identifier", value: "S" + "8BuildVerification", want: true},
		{name: "lowercase key", value: "s" + "9_validation_depth", want: true},
		{name: "retired dotted path", value: "design.security", want: true},
		{name: "canonical phase", value: "BUILD", want: false},
		{name: "ordinary lowercase version", value: "api/v1", want: false},
		{name: "ordinary version", value: "revision-v2", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := retiredWayfinderPattern.MatchString(tt.value); got != tt.want {
				t.Fatalf("retiredWayfinderPattern.MatchString(%q) = %t, want %t", tt.value, got, tt.want)
			}
		})
	}
}

func TestRetiredWayfinderDocToken(t *testing.T) {
	tests := []struct {
		name       string
		line       string
		contextual bool
		want       bool
	}{
		{name: "project framing identifier", line: "write W" + "0 requirements", want: true},
		{name: "labeled phase", line: "Phase S" + "8 completed", want: true},
		{name: "status field", line: `"current_phase": "S` + `6"`, want: true},
		{name: "artifact filename", line: "git history/S" + "9-validation.md", want: true},
		{name: "near Wayfinder context", line: "D" + "4 quality gate", contextual: true, want: true},
		{name: "ADR decision label", line: "D" + "2 Source", want: false},
		{name: "unrelated release phase", line: "Phase 5 V" + "1", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := retiredWayfinderDocToken(tt.line, tt.contextual) != ""
			if got != tt.want {
				t.Fatalf("retiredWayfinderDocToken(%q, %t) match = %t, want %t", tt.line, tt.contextual, got, tt.want)
			}
		})
	}
}

func TestLivingWayfinderDocumentRejectsNoncanonicalPhaseInCodeBlock(t *testing.T) {
	content := "" +
		"```json\n" +
		"{\n" +
		"  \"plugin\": \"wayfinder\",\n" +
		"  \"duration_ms\": 1250,\n" +
		"  \"phase\": \"S2\"\n" +
		"}\n" +
		"```\n"
	if err := validateLivingWayfinderDocument("SPEC.md", content); err == nil {
		t.Fatal("expected a noncanonical Wayfinder phase to be rejected")
	}

	canonical := strings.Replace(content, `"S2"`, `"BUILD"`, 1)
	if err := validateLivingWayfinderDocument("SPEC.md", canonical); err != nil {
		t.Fatalf("canonical phase rejected: %v", err)
	}
}
