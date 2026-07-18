package pluginhash

import (
	"strings"
	"testing"
)

func TestStampAndCompute(t *testing.T) {
	source := []byte("---\ncontent-hash: PLACEHOLDER\n---\n\n# Body\n")
	stamped, err := Stamp(source)
	if err != nil {
		t.Fatalf("Stamp: %v", err)
	}
	hash, err := Compute(stamped)
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}
	if !strings.Contains(string(stamped), "content-hash: "+hash) {
		t.Fatalf("stamped hash mismatch:\n%s", stamped)
	}
	stampedAgain, err := Stamp(stamped)
	if err != nil {
		t.Fatalf("second Stamp: %v", err)
	}
	if string(stampedAgain) != string(stamped) {
		t.Fatal("Stamp is not idempotent")
	}
}

func TestComputeRejectsMissingFrontmatter(t *testing.T) {
	if _, err := Compute([]byte("# Body\n")); err == nil {
		t.Fatal("expected missing-frontmatter error")
	}
}
