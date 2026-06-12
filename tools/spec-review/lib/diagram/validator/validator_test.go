package validator

import "testing"

func TestNewSyntaxValidator_NotNil(t *testing.T) {
	v := NewSyntaxValidator()
	if v == nil {
		t.Fatal("NewSyntaxValidator() returned nil")
	}
}
