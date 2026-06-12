package main

import "testing"

func TestParseSize(t *testing.T) {
	cases := map[string]int64{
		"100MB": 100 << 20,
		"1GB":   1 << 30,
		"512KB": 512 << 10,
		"2M":    2 << 20,
		"1024":  1024,
		"0":     0,
		"1.5MB": int64(1.5 * (1 << 20)),
	}
	for in, want := range cases {
		got, err := parseSize(in)
		if err != nil {
			t.Errorf("parseSize(%q) error: %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("parseSize(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestParseSize_Invalid(t *testing.T) {
	for _, in := range []string{"", "abc", "-5MB", "MB"} {
		if _, err := parseSize(in); err == nil {
			t.Errorf("parseSize(%q) expected error, got nil", in)
		}
	}
}

func TestRun_DryRunDefault(t *testing.T) {
	dir := t.TempDir()
	// A directory with no logs: run must succeed and write nothing.
	if err := run([]string{dir}); err != nil {
		t.Fatalf("run dry-run: %v", err)
	}
}

func TestRun_BadDir(t *testing.T) {
	if err := run([]string{"/nonexistent/path/for/logrotate/test"}); err == nil {
		t.Fatal("expected error for missing dir")
	}
}
