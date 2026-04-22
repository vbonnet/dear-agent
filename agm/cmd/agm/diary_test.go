package main

import (
	"testing"
)

func TestSplitLines(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  int
	}{
		{"empty", "", 0},
		{"one line", "hello", 1},
		{"two lines", "hello\nworld", 2},
		{"trailing newline", "hello\n", 1},
		{"multiple newlines", "a\nb\nc\n", 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitLines([]byte(tt.input))
			if len(got) != tt.want {
				t.Errorf("splitLines(%q) = %d lines, want %d", tt.input, len(got), tt.want)
			}
		})
	}
}

func TestDiaryEventJSON(t *testing.T) {
	event := DiaryEvent{
		Type:    "COMMIT",
		Session: "test-worker",
		Summary: "added tests",
	}
	if event.Type != "COMMIT" {
		t.Errorf("unexpected type: %s", event.Type)
	}
	if event.Session != "test-worker" {
		t.Errorf("unexpected session: %s", event.Session)
	}
}
