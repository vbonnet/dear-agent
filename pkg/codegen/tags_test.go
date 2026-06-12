package codegen_test

import (
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/pkg/codegen"
)

func TestParseFieldTag_MinimalName(t *testing.T) {
	t.Parallel()
	ft, err := codegen.ParseFieldTag("myfield")
	if err != nil {
		t.Fatalf("ParseFieldTag: %v", err)
	}
	if ft.Name != "myfield" {
		t.Errorf("Name = %q, want %q", ft.Name, "myfield")
	}
	if ft.Pos != -1 {
		t.Errorf("Pos = %d, want -1", ft.Pos)
	}
	if ft.Required {
		t.Error("Required should be false by default")
	}
}

func TestParseFieldTag_AllOptions(t *testing.T) {
	t.Parallel()
	ft, err := codegen.ParseFieldTag("cmd,pos=0,short=c,default=go,enum=go|rust,required,omit=mcp")
	if err != nil {
		t.Fatalf("ParseFieldTag: %v", err)
	}
	if ft.Name != "cmd" {
		t.Errorf("Name = %q, want %q", ft.Name, "cmd")
	}
	if ft.Pos != 0 {
		t.Errorf("Pos = %d, want 0", ft.Pos)
	}
	if ft.Short != "c" {
		t.Errorf("Short = %q, want %q", ft.Short, "c")
	}
	if ft.Default != "go" {
		t.Errorf("Default = %q, want %q", ft.Default, "go")
	}
	if len(ft.Enum) != 2 || ft.Enum[0] != "go" || ft.Enum[1] != "rust" {
		t.Errorf("Enum = %v, want [go rust]", ft.Enum)
	}
	if !ft.Required {
		t.Error("Required should be true")
	}
	if !ft.Omit["mcp"] {
		t.Error("Omit should contain mcp")
	}
}

func TestParseFieldTag_FlattenHidden(t *testing.T) {
	t.Parallel()
	ft, err := codegen.ParseFieldTag("opts,flatten,hidden")
	if err != nil {
		t.Fatalf("ParseFieldTag: %v", err)
	}
	if !ft.Flatten {
		t.Error("Flatten should be true")
	}
	if !ft.Hidden {
		t.Error("Hidden should be true")
	}
}

func TestParseFieldTag_OmitMultiple(t *testing.T) {
	t.Parallel()
	ft, err := codegen.ParseFieldTag("x,omit=cli|mcp")
	if err != nil {
		t.Fatalf("ParseFieldTag: %v", err)
	}
	if !ft.Omit["cli"] {
		t.Error("Omit should contain cli")
	}
	if !ft.Omit["mcp"] {
		t.Error("Omit should contain mcp")
	}
}

func TestParseFieldTag_EmptyTagError(t *testing.T) {
	t.Parallel()
	_, err := codegen.ParseFieldTag("")
	if err == nil {
		t.Error("expected error for empty tag")
	}
}

func TestParseFieldTag_EmptyNameError(t *testing.T) {
	t.Parallel()
	_, err := codegen.ParseFieldTag(",required")
	if err == nil {
		t.Error("expected error for empty field name")
	}
}

func TestParseFieldTag_UnknownOptionError(t *testing.T) {
	t.Parallel()
	_, err := codegen.ParseFieldTag("x,notanoption")
	if err == nil {
		t.Error("expected error for unknown option")
	}
	if !strings.Contains(err.Error(), "unknown ef tag option") {
		t.Errorf("error %q should mention unknown ef tag option", err)
	}
}

func TestParseFieldTag_PosInvalidError(t *testing.T) {
	t.Parallel()
	_, err := codegen.ParseFieldTag("x,pos=abc")
	if err == nil {
		t.Error("expected error for non-integer pos")
	}
}

func TestParseFieldTag_ShortTooLongError(t *testing.T) {
	t.Parallel()
	_, err := codegen.ParseFieldTag("x,short=ab")
	if err == nil {
		t.Error("expected error for short value longer than 1 char")
	}
}

func TestParseDescTag(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   string
		want string
	}{
		{"", ""},
		{"a description", "a description"},
		{"multi word desc", "multi word desc"},
	}
	for _, tc := range cases {
		got := codegen.ParseDescTag(tc.in)
		if got != tc.want {
			t.Errorf("ParseDescTag(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
