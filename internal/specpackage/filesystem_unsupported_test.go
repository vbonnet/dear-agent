//go:build !darwin && !linux

package specpackage

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestUnsupportedPlatformFailsClosed(t *testing.T) {
	absolute, err := filepath.Abs("specpackage")
	if err != nil {
		t.Fatalf("resolve absolute test path: %v", err)
	}
	if _, err := Stage(context.Background(), absolute, absolute, absolute); err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("Stage error = %v, want unsupported-platform rejection", err)
	}
	if _, err := Validate(context.Background(), absolute); err == nil || !strings.Contains(err.Error(), "requires handle-anchored filesystem support") {
		t.Fatalf("Validate error = %v, want unsupported-platform rejection", err)
	}
}
