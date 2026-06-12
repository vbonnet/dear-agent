package main

import "testing"

func TestDetectLanguages_EmptyDir_NoError(t *testing.T) {
	dir := t.TempDir()
	if err := detectLanguages(dir); err != nil {
		t.Errorf("detectLanguages(empty dir): %v", err)
	}
}
