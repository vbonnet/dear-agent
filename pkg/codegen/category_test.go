package codegen

import "testing"

func TestCategory_Valid_RecognizedValues(t *testing.T) {
	for _, c := range []Category{CategoryRead, CategoryMutation, CategoryMeta} {
		if !c.Valid() {
			t.Errorf("Category(%q).Valid() = false, want true", c)
		}
	}
}

func TestCategory_Valid_UnrecognizedValue(t *testing.T) {
	for _, c := range []Category{"", "unknown", "write"} {
		if c.Valid() {
			t.Errorf("Category(%q).Valid() = true, want false", c)
		}
	}
}

func TestOpIR_Methods(t *testing.T) {
	op := Op{
		Name:        "ListSessions",
		Description: "Lists all sessions",
		Category:    CategoryRead,
		Deprecated:  false,
		Stub:        false,
	}
	ir := OpIR{
		Op:      op,
		VarName: "ListSessions",
	}

	if ir.OpName() != "ListSessions" {
		t.Errorf("OpName() = %q, want %q", ir.OpName(), "ListSessions")
	}
	if ir.OpDescription() != "Lists all sessions" {
		t.Errorf("OpDescription() = %q, want %q", ir.OpDescription(), "Lists all sessions")
	}
	if ir.IsStub() {
		t.Error("IsStub() should be false")
	}
	if ir.IsDeprecated() {
		t.Error("IsDeprecated() should be false")
	}
	if ir.IsManualSkill() {
		t.Error("IsManualSkill() should be false")
	}
}
