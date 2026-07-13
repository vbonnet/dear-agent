package commandparity

import (
	"path/filepath"
	"runtime"
	"testing"
)

func TestContracts(t *testing.T) {
	if err := ValidateContracts(); err != nil {
		t.Fatal(err)
	}
}

func TestTmuxCobraCommandSourcesHaveContracts(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller could not resolve test path")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "..", ".."))
	if err := ValidateSourceCoverage(repoRoot); err != nil {
		t.Fatal(err)
	}
}
