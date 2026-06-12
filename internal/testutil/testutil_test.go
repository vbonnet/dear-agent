package testutil

import (
	"os"
	"testing"
)

func TestSetupTempDir_CreatesDir(t *testing.T) {
	dir := SetupTempDir(t)
	if dir == "" {
		t.Fatal("SetupTempDir returned empty string")
	}
	if _, err := os.Stat(dir); err != nil {
		t.Errorf("SetupTempDir dir does not exist: %v", err)
	}
}

func TestNowNano_Positive(t *testing.T) {
	if n := NowNano(); n <= 0 {
		t.Errorf("NowNano() = %d, want positive", n)
	}
}
