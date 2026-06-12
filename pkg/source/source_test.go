package source

import (
	"errors"
	"testing"
)

func TestErrNotFound_IsSentinel(t *testing.T) {
	if ErrNotFound == nil {
		t.Fatal("ErrNotFound should not be nil")
	}
	if !errors.Is(ErrNotFound, ErrNotFound) {
		t.Error("ErrNotFound should satisfy errors.Is(ErrNotFound, ErrNotFound)")
	}
}
