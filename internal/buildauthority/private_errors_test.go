package buildauthority

import (
	"errors"
	"fmt"
	"reflect"
	"testing"
)

func TestPrivateCausesCollectsEveryJoinedFailure(t *testing.T) {
	t.Parallel()

	err := errors.Join(
		fail(CauseUnstable, "first raw detail"),
		fmt.Errorf("wrapped: %w", fail(CauseDescriptorClose, "second raw detail")),
		errors.New("unclassified raw detail"),
	)
	want := []CauseCode{CauseUnstable, CauseDescriptorClose}
	if got := privateCauses(err, CauseInternalInvariant); !reflect.DeepEqual(got, want) {
		t.Fatalf("causes = %#v, want %#v", got, want)
	}
}

func TestPrivateCausesKeepsOuterReductionAndFallback(t *testing.T) {
	t.Parallel()

	nested := failWith(fail(CausePermission, "inner"), CauseIdentity, "outer")
	if got := privateCauses(nested, CauseInternalInvariant); !reflect.DeepEqual(got, []CauseCode{CauseIdentity}) {
		t.Fatalf("nested causes = %#v, want identity", got)
	}
	if got := privateCauses(errors.New("raw"), CauseMalformed); !reflect.DeepEqual(got, []CauseCode{CauseMalformed}) {
		t.Fatalf("fallback causes = %#v, want malformed", got)
	}
}
