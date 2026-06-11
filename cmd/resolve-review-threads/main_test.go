package main

import "testing"

func TestCleanBody(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"collapses whitespace", "a\n\tb   c", "a b c"},
		{"trims ends", "  hello world  ", "hello world"},
		{"empty", "", ""},
		{"only whitespace", "  \n\t ", ""},
		{"truncates to preview length", repeat("x", bodyPreviewLen+50), repeat("x", bodyPreviewLen)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := cleanBody(tt.in); got != tt.want {
				t.Errorf("cleanBody(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestCleanBodyRuneSafe(t *testing.T) {
	// A multibyte string longer than the preview must be cut on a rune
	// boundary, never mid-codepoint, and yield exactly bodyPreviewLen runes.
	in := repeat("é", bodyPreviewLen+10)
	got := cleanBody(in)
	if n := len([]rune(got)); n != bodyPreviewLen {
		t.Fatalf("got %d runes, want %d", n, bodyPreviewLen)
	}
}

func TestFilterThreads(t *testing.T) {
	threads := []thread{
		{ID: "1", IsResolved: false, Author: "gemini-code-assist"},
		{ID: "2", IsResolved: true, Author: "gemini-code-assist"},
		{ID: "3", IsResolved: false, Author: "alice"},
		{ID: "4", IsResolved: false, Author: "gemini-code-assist"},
	}

	t.Run("no author keeps all unresolved", func(t *testing.T) {
		got := filterThreads(threads, "")
		if want := []string{"1", "3", "4"}; !idsEqual(got, want) {
			t.Errorf("got %v, want %v", ids(got), want)
		}
	})

	t.Run("author filter never matches resolved", func(t *testing.T) {
		got := filterThreads(threads, "gemini-code-assist")
		if want := []string{"1", "4"}; !idsEqual(got, want) {
			t.Errorf("got %v, want %v", ids(got), want)
		}
	})

	t.Run("author with no unresolved threads yields empty", func(t *testing.T) {
		if got := filterThreads(threads, "nobody"); len(got) != 0 {
			t.Errorf("got %v, want empty", ids(got))
		}
	})
}

func repeat(s string, n int) string {
	out := make([]byte, 0, len(s)*n)
	for range n {
		out = append(out, s...)
	}
	return string(out)
}

func ids(ts []thread) []string {
	out := make([]string, len(ts))
	for i, t := range ts {
		out[i] = t.ID
	}
	return out
}

func idsEqual(ts []thread, want []string) bool {
	got := ids(ts)
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
