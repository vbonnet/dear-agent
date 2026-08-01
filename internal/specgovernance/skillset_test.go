package specgovernance

import (
	"slices"
	"testing"
)

func TestFixedExports(t *testing.T) {
	if got, want := Names(), []string{"audit-specs", "write-spec"}; !slices.Equal(got, want) {
		t.Fatalf("Names() = %v, want %v", got, want)
	}
	if got, want := NativeExports(), []string{"./skills/audit-specs", "./skills/write-spec"}; !slices.Equal(got, want) {
		t.Fatalf("NativeExports() = %v, want %v", got, want)
	}
	mutated := Names()
	mutated[0] = "review-spec"
	if got := Names()[0]; got != "audit-specs" {
		t.Fatalf("Names() exposed mutable canonical state: %q", got)
	}
}
