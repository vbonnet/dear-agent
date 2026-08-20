package main

import (
	"strings"
	"testing"
)

func TestCountLines(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want int
	}{
		{"empty", "", 0},
		{"only shebang", "#!/bin/bash\n", 0},
		{"shebang and comments", "#!/bin/bash\n# a\n#b\n", 0},
		{"blank and whitespace-only", "\n   \n\t\n", 0},
		{"indented comment is a comment", "   # indented\n\t# tabbed\n", 0},
		{"code counts", "#!/bin/bash\nset -e\necho hi\n", 2},
		{"trailing comment on a code line still counts", "echo hi # trailing\n", 1},
		{"no trailing newline", "echo a\necho b", 2},
		{"code after blanks", "\n\necho a\n\n", 1},
		// A '#' that is not the first non-space character is code, not a comment.
		{"hash inside a string", `echo "a#b"`, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := CountLines(strings.NewReader(tc.in))
			if err != nil {
				t.Fatalf("CountLines: %v", err)
			}
			if got != tc.want {
				t.Errorf("CountLines(%q) = %d, want %d", tc.in, got, tc.want)
			}
		})
	}
}

func TestInScope(t *testing.T) {
	out := []string{
		"vendor/x.sh",
		"third_party/vendor/tool.sh", // segment match, not prefix match
		"a/node_modules/b.sh",
		".archived/old.sh",
		"deep/.worktrees/w/x.sh",
	}
	for _, p := range out {
		if InScope(p) {
			t.Errorf("InScope(%q) = true, want false", p)
		}
	}
	in := []string{
		"scripts/deploy.sh",
		"./scripts/deploy.sh",
		"vendored/x.sh",     // not the excluded segment "vendor"
		"my-vendor-tool.sh", // substring, not a segment
	}
	for _, p := range in {
		if !InScope(p) {
			t.Errorf("InScope(%q) = false, want true", p)
		}
	}
}
