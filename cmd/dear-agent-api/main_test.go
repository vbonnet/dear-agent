package main

import (
	"bytes"
	"strings"
	"testing"
)

// TestRun_BadFlags verifies argv parsing surfaces a non-zero exit
// rather than panicking. The full HTTP path is covered by pkg/api
// tests; here we just want the wiring not to blow up.
func TestRun_BadFlags(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"--no-such-flag"}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("run returned 0; want non-zero on unknown flag")
	}
	if !strings.Contains(stderr.String(), "no-such-flag") {
		t.Errorf("stderr did not mention the bad flag: %q", stderr.String())
	}
}

// TestStripZone covers the small helper that normalises IPv6 address
// strings before handing them to tsnet's WhoIs.
func TestStripZone(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"127.0.0.1:8080", "127.0.0.1:8080"},
		{"[fe80::1%en0]:443", "[fe80::1]:443"},
		{"[fe80::1]:443", "[fe80::1]:443"},
		{"not-an-addr", "not-an-addr"},
	}
	for _, tc := range cases {
		got := stripZone(tc.in)
		if got != tc.want {
			t.Errorf("stripZone(%q) = %q; want %q", tc.in, got, tc.want)
		}
	}
}
