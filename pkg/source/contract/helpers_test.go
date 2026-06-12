package contract

import (
	"testing"

	"github.com/vbonnet/dear-agent/pkg/source"
)

func TestEqualStrings(t *testing.T) {
	tests := []struct {
		a, b []string
		want bool
	}{
		{nil, nil, true},
		{[]string{}, []string{}, true},
		{[]string{"a"}, []string{"a"}, true},
		{[]string{"a", "b"}, []string{"a", "b"}, true},
		{[]string{"a "}, []string{" a"}, true}, // TrimSpace semantics
		{[]string{"a"}, []string{"b"}, false},
		{[]string{"a"}, []string{"a", "b"}, false},
		{[]string{"a", "b"}, []string{"a"}, false},
	}
	for _, tt := range tests {
		if got := equalStrings(tt.a, tt.b); got != tt.want {
			t.Errorf("equalStrings(%v, %v) = %v, want %v", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestURISetEquals(t *testing.T) {
	tests := []struct {
		sources []source.Source
		want    []string
		equal   bool
	}{
		{nil, nil, true},
		{[]source.Source{}, []string{}, true},
		{
			[]source.Source{{URI: "a"}, {URI: "b"}},
			[]string{"a", "b"},
			true,
		},
		{
			[]source.Source{{URI: "a"}, {URI: "b"}},
			[]string{"b", "a"}, // order-independent
			true,
		},
		{
			[]source.Source{{URI: "a"}},
			[]string{"a", "b"},
			false,
		},
		{
			[]source.Source{{URI: "a"}, {URI: "b"}},
			[]string{"a", "c"},
			false,
		},
	}
	for _, tt := range tests {
		if got := uriSetEquals(tt.sources, tt.want); got != tt.equal {
			t.Errorf("uriSetEquals(%v, %v) = %v, want %v", tt.sources, tt.want, got, tt.equal)
		}
	}
}
