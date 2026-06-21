package document

import "testing"

func TestKindValid(t *testing.T) {
	valid := []Kind{KindSpec, KindArchitecture, KindResearch, KindReference, KindADR}
	for _, k := range valid {
		if !k.Valid() {
			t.Errorf("Kind %q should be valid", k)
		}
	}
	for _, k := range []Kind{"", "memory", "bogus"} {
		if k.Valid() {
			t.Errorf("Kind %q should be invalid", k)
		}
	}
}

func TestHashContentDeterministic(t *testing.T) {
	a := HashContent("hello world")
	b := HashContent("hello world")
	if a != b {
		t.Errorf("HashContent not deterministic: %q != %q", a, b)
	}
	if a == HashContent("hello worle") {
		t.Errorf("HashContent collided on different content")
	}
	if len(a) != 64 {
		t.Errorf("HashContent len = %d, want 64 (hex sha256)", len(a))
	}
}
