package contract

import (
	"testing"

	"github.com/vbonnet/dear-agent/pkg/source"
)

func TestEqualStrings_Equal(t *testing.T) {
	if !equalStrings([]string{"a", "b"}, []string{"a", "b"}) {
		t.Error("equalStrings should return true for equal slices")
	}
}

func TestEqualStrings_DifferentValues(t *testing.T) {
	if equalStrings([]string{"a"}, []string{"b"}) {
		t.Error("equalStrings should return false for different values")
	}
}

func TestEqualStrings_DifferentLen(t *testing.T) {
	if equalStrings([]string{"a", "b"}, []string{"a"}) {
		t.Error("equalStrings should return false for different lengths")
	}
}

func TestEqualStrings_Empty(t *testing.T) {
	if !equalStrings([]string{}, []string{}) {
		t.Error("equalStrings should return true for two empty slices")
	}
}

func TestUriSetEquals_Match(t *testing.T) {
	sources := []source.Source{{URI: "u1"}, {URI: "u2"}}
	if !uriSetEquals(sources, []string{"u1", "u2"}) {
		t.Error("uriSetEquals should match equal sets")
	}
}

func TestUriSetEquals_Mismatch(t *testing.T) {
	sources := []source.Source{{URI: "u1"}}
	if uriSetEquals(sources, []string{"u2"}) {
		t.Error("uriSetEquals should not match different URIs")
	}
}

func TestUriSetEquals_DifferentLen(t *testing.T) {
	sources := []source.Source{{URI: "u1"}, {URI: "u2"}}
	if uriSetEquals(sources, []string{"u1"}) {
		t.Error("uriSetEquals should return false for different lengths")
	}
}

func TestUris_ExtractsURIs(t *testing.T) {
	sources := []source.Source{{URI: "a"}, {URI: "b"}}
	got := uris(sources)
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Errorf("uris() = %v, want [a b]", got)
	}
}
