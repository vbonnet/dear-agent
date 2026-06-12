package codegen

import (
	"testing"
)

func TestCategoryValid(t *testing.T) {
	t.Parallel()
	valid := []Category{CategoryRead, CategoryMutation, CategoryMeta}
	for _, c := range valid {
		if !c.Valid() {
			t.Errorf("Category(%q).Valid() = false, want true", c)
		}
	}
	if Category("").Valid() {
		t.Error("Category(\"\").Valid() = true, want false")
	}
	if Category("unknown").Valid() {
		t.Error("Category(\"unknown\").Valid() = true, want false")
	}
}

func TestParseFieldTag_Minimal(t *testing.T) {
	t.Parallel()
	ft, err := ParseFieldTag("myfield")
	if err != nil {
		t.Fatalf("ParseFieldTag: %v", err)
	}
	if ft.Name != "myfield" {
		t.Errorf("Name = %q, want myfield", ft.Name)
	}
	if ft.Pos != -1 {
		t.Errorf("Pos = %d, want -1", ft.Pos)
	}
	if ft.Required || ft.Flatten || ft.Hidden {
		t.Error("unexpected boolean flags set on minimal tag")
	}
}

func TestParseFieldTag_AllOptions(t *testing.T) {
	t.Parallel()
	ft, err := ParseFieldTag("cmd,pos=0,short=c,default=val,enum=a|b,required,flatten,hidden,omit=cli|mcp")
	if err != nil {
		t.Fatalf("ParseFieldTag: %v", err)
	}
	if ft.Name != "cmd" {
		t.Errorf("Name = %q, want cmd", ft.Name)
	}
	if ft.Pos != 0 {
		t.Errorf("Pos = %d, want 0", ft.Pos)
	}
	if ft.Short != "c" {
		t.Errorf("Short = %q, want c", ft.Short)
	}
	if ft.Default != "val" {
		t.Errorf("Default = %q, want val", ft.Default)
	}
	if len(ft.Enum) != 2 || ft.Enum[0] != "a" || ft.Enum[1] != "b" {
		t.Errorf("Enum = %v, want [a b]", ft.Enum)
	}
	if !ft.Required || !ft.Flatten || !ft.Hidden {
		t.Error("Required/Flatten/Hidden not set")
	}
	if !ft.Omit["cli"] || !ft.Omit["mcp"] {
		t.Errorf("Omit = %v, want {cli:true mcp:true}", ft.Omit)
	}
}

func TestParseFieldTag_Errors(t *testing.T) {
	t.Parallel()
	cases := []struct{ tag, desc string }{
		{"", "empty tag"},
		{",noname", "empty field name"},
		{"f,pos", "pos without value"},
		{"f,pos=abc", "pos non-integer"},
		{"f,short", "short without value"},
		{"f,short=ab", "short more than one char"},
		{"f,default", "default without value"},
		{"f,enum", "enum without value"},
		{"f,omit", "omit without value"},
		{"f,badopt=x", "unknown option"},
	}
	for _, tc := range cases {
		_, err := ParseFieldTag(tc.tag)
		if err == nil {
			t.Errorf("%s: expected error, got nil (tag=%q)", tc.desc, tc.tag)
		}
	}
}

func TestParseDescTag(t *testing.T) {
	t.Parallel()
	if got := ParseDescTag("a description"); got != "a description" {
		t.Errorf("ParseDescTag = %q, want %q", got, "a description")
	}
	if got := ParseDescTag(""); got != "" {
		t.Errorf("ParseDescTag(\"\") = %q, want empty", got)
	}
}
