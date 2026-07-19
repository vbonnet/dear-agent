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

func TestStampNormalizesCRLF(t *testing.T) {
	source := []byte("---\r\ncontent-hash: PLACEHOLDER\r\n---\r\n\r\n# Body\r\n")
	stamped, err := Stamp(source)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(stamped), "\r") {
		t.Fatalf("Stamp retained CRLF bytes: %q", stamped)
	}
	hash, err := Compute(source)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(stamped), "content-hash: "+hash) {
		t.Fatalf("CRLF hash mismatch:\n%s", stamped)
	}
}

func TestStampDoesNotRewriteContentHashTextInBody(t *testing.T) {
	source := []byte("---\ncontent-hash: PLACEHOLDER\n---\n\n# Body\n\ncontent-hash: PLACEHOLDER\n")
	stamped, err := Stamp(source)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(string(stamped), "# Body\n\ncontent-hash: PLACEHOLDER\n") {
		t.Fatalf("Stamp rewrote body content:\n%s", stamped)
	}
}

func TestStampRejectsDuplicateFrontmatterHashFields(t *testing.T) {
	source := []byte("---\ncontent-hash: PLACEHOLDER\ncontent-hash: 0000\n---\n\n# Body\n")
	if _, err := Stamp(source); err == nil || !strings.Contains(err.Error(), "exactly one") {
		t.Fatalf("expected duplicate-field error, got %v", err)
	}
}
