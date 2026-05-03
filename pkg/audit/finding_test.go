package audit

import (
	"strings"
	"testing"
)

// TestFingerprintStability is the core de-dup contract from ADR-011 §D9:
// the same parts (in the same order) produce the same fingerprint
// across calls. This is the only thing standing between us and "every
// audit run looks like 100 new findings".
func TestFingerprintStability(t *testing.T) {
	a := Fingerprint("build", "/repo", "main.go:42 undefined: foo")
	b := Fingerprint("build", "/repo", "main.go:42 undefined: foo")
	if a != b {
		t.Fatalf("fingerprint should be stable across calls; got %s vs %s", a, b)
	}
	if len(a) != 32 {
		t.Errorf("fingerprint should be 32 hex chars (16 bytes), got %d", len(a))
	}
}

// TestFingerprintEmptyPartsSkipped verifies that empty parts do not
// poison the hash — adding/removing nil-y trailing components must
// not change a fingerprint built from the same significant parts.
func TestFingerprintEmptyPartsSkipped(t *testing.T) {
	a := Fingerprint("build", "/repo", "msg")
	b := Fingerprint("build", "", "/repo", "", "msg")
	if a != b {
		t.Errorf("empty parts should be skipped: %s != %s", a, b)
	}
}

// TestFingerprintSeparatorPreventsAmbiguity verifies that a path
// containing the separator char cannot collide with two adjacent
// parts. RS (U+001E) was chosen because it's not a valid path
// character; this test pins that contract.
func TestFingerprintSeparatorAmbiguity(t *testing.T) {
	a := Fingerprint("a", "b", "c")
	b := Fingerprint("a:b", "c")
	if a == b {
		t.Errorf("ambiguous parts produced same fingerprint: %s", a)
	}
}

func TestFindingValidate(t *testing.T) {
	good := Finding{
		CheckID:     "build",
		Fingerprint: "fp",
		Severity:    SeverityP1,
		Title:       "title",
	}
	if err := good.Validate(); err != nil {
		t.Errorf("good finding failed validate: %v", err)
	}

	cases := []Finding{
		{Fingerprint: "fp", Severity: SeverityP1, Title: "t"},   // missing CheckID
		{CheckID: "x", Severity: SeverityP1, Title: "t"},        // missing Fingerprint
		{CheckID: "x", Fingerprint: "fp", Title: "t"},           // missing Severity
		{CheckID: "x", Fingerprint: "fp", Severity: "P9", Title: "t"},
		{CheckID: "x", Fingerprint: "fp", Severity: SeverityP1}, // missing Title
	}
	for i, c := range cases {
		err := c.Validate()
		if err == nil {
			t.Errorf("case %d should fail validate: %+v", i, c)
			continue
		}
		if !strings.Contains(err.Error(), "audit:") {
			t.Errorf("case %d error missing audit: prefix: %v", i, err)
		}
	}
}

func TestFindingStateLifecycle(t *testing.T) {
	if !FindingResolved.IsTerminal() {
		t.Error("resolved should be terminal")
	}
	if FindingOpen.IsTerminal() {
		t.Error("open should not be terminal")
	}
	if FindingState("garbage").IsValid() {
		t.Error("garbage state should be invalid")
	}
}
