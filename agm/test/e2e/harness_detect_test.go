package e2e

import (
	"path/filepath"
	"runtime"
	"testing"
)

func TestHarnessDetectionSupportsSystemBash(t *testing.T) {
	_, testFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve harness detection test source")
	}
	helper := filepath.Join(filepath.Dir(testFile), "lib", "harness-detect.sh")
	if err := ValidatePortableHarnessDetection(helper); err != nil {
		t.Fatal(err)
	}
}
