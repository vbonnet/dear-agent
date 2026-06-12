package validator

import (
	"context"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/tools/spec-review/lib/diagram/renderer"
)

// newEmpty returns a SyntaxValidator backed by an empty registry (no renderers).
func newEmpty() *SyntaxValidator {
	return &SyntaxValidator{registry: renderer.NewRegistry()}
}

func TestNewSyntaxValidator_NotNil(t *testing.T) {
	t.Parallel()
	v := NewSyntaxValidator()
	if v == nil {
		t.Fatal("NewSyntaxValidator() returned nil")
	}
}

func TestValidate_UnknownFormat(t *testing.T) {
	t.Parallel()
	v := newEmpty()
	err := v.Validate(context.Background(), renderer.Format("nonexistent"), strings.NewReader("source"))
	if err == nil {
		t.Fatal("Validate with unknown format: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "no renderer found for format") {
		t.Errorf("Validate error = %q, want to contain 'no renderer found for format'", err.Error())
	}
}

func TestValidateD2_EmptyRegistry_Error(t *testing.T) {
	t.Parallel()
	v := newEmpty()
	if err := v.ValidateD2(context.Background(), strings.NewReader("x -> y")); err == nil {
		t.Error("ValidateD2 with empty registry: expected error, got nil")
	}
}

func TestValidateMermaid_EmptyRegistry_Error(t *testing.T) {
	t.Parallel()
	v := newEmpty()
	if err := v.ValidateMermaid(context.Background(), strings.NewReader("graph LR")); err == nil {
		t.Error("ValidateMermaid with empty registry: expected error, got nil")
	}
}

func TestValidatePlantUML_EmptyRegistry_Error(t *testing.T) {
	t.Parallel()
	v := newEmpty()
	if err := v.ValidatePlantUML(context.Background(), strings.NewReader("@startuml")); err == nil {
		t.Error("ValidatePlantUML with empty registry: expected error, got nil")
	}
}

func TestValidateStructurizr_EmptyRegistry_Error(t *testing.T) {
	t.Parallel()
	v := newEmpty()
	if err := v.ValidateStructurizr(context.Background(), strings.NewReader("workspace {}")); err == nil {
		t.Error("ValidateStructurizr with empty registry: expected error, got nil")
	}
}

func TestValidateAll_EmptyRegistry_Error(t *testing.T) {
	t.Parallel()
	v := newEmpty()
	_, err := v.ValidateAll(context.Background(), strings.NewReader("some content"))
	if err == nil {
		t.Fatal("ValidateAll with empty registry: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "source is not valid for any supported format") {
		t.Errorf("ValidateAll error = %q, want 'source is not valid for any supported format'", err.Error())
	}
}

func TestReaderAdapter_Read(t *testing.T) {
	t.Parallel()
	data := []byte("hello world")
	ra := &readerAdapter{data: data}

	buf := make([]byte, 5)
	n, err := ra.Read(buf)
	if err != nil || n != 5 || string(buf) != "hello" {
		t.Errorf("first Read: n=%d err=%v buf=%q, want n=5 err=nil buf=\"hello\"", n, err, buf)
	}

	// Second read for remaining bytes.
	buf2 := make([]byte, 10)
	n2, err2 := ra.Read(buf2)
	if err2 != nil || n2 != 6 || string(buf2[:n2]) != " world" {
		t.Errorf("second Read: n=%d err=%v buf=%q, want n=6 err=nil buf=\" world\"", n2, err2, buf2[:n2])
	}
}
