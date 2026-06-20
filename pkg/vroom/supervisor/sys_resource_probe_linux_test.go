//go:build linux

package supervisor

import (
	"os"
	"path/filepath"
	"testing"
)

func TestProcPPID(t *testing.T) {
	t.Parallel()

	statusBody := "Name:\tgopls\n" +
		"Umask:\t0022\n" +
		"State:\tS (sleeping)\n" +
		"Tgid:\t4242\n" +
		"Pid:\t4242\n" +
		"PPid:\t1\n" +
		"TracerPid:\t0\n"

	cases := []struct {
		name string
		body string
		want int
	}{
		{name: "orphan reparented to init", body: statusBody, want: 1},
		{name: "live child of a real parent", body: "Name:\tgopls\nPPid:\t98231\n", want: 98231},
		{name: "missing PPid field", body: "Name:\tgopls\nState:\tS\n", want: 0},
		{name: "malformed PPid value", body: "PPid:\tnotanumber\n", want: 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "status")
			if err := os.WriteFile(path, []byte(tc.body), 0o600); err != nil {
				t.Fatalf("write temp status: %v", err)
			}
			if got := procPPID(path); got != tc.want {
				t.Errorf("procPPID(%q) = %d, want %d", tc.name, got, tc.want)
			}
		})
	}
}

func TestProcPPID_MissingFile(t *testing.T) {
	t.Parallel()
	if got := procPPID(filepath.Join(t.TempDir(), "does-not-exist")); got != 0 {
		t.Errorf("procPPID(missing) = %d, want 0", got)
	}
}
