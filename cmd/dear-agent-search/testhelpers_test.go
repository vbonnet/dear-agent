package main

import (
	"io"
	"os"
	"testing"
)

// captureFile returns two os.File pointers backed by temp files so the
// search CLI can write into them and tests can read what came out.
// run() insists on *os.File so it can hand the arg straight to flag.*
// and json.Encoder; capturing into a bytes.Buffer would force a wider
// signature change.
func captureFile(t *testing.T) (stdout, stderr *os.File) {
	t.Helper()
	out, err := os.CreateTemp(t.TempDir(), "stdout-*")
	if err != nil {
		t.Fatalf("create stdout temp: %v", err)
	}
	er, err := os.CreateTemp(t.TempDir(), "stderr-*")
	if err != nil {
		t.Fatalf("create stderr temp: %v", err)
	}
	return out, er
}

func cleanup(f *os.File) {
	if f == nil {
		return
	}
	_ = f.Close()
	_ = os.Remove(f.Name())
}

func readAll(f *os.File) string {
	if f == nil {
		return ""
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return ""
	}
	b, _ := io.ReadAll(f)
	return string(b)
}
