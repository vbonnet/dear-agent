package openviking

import (
	"context"
	"errors"
	"testing"

	"github.com/vbonnet/dear-agent/pkg/source"
)

func TestStub_AllMethodsReturnNotImplemented(t *testing.T) {
	a, err := Open(Config{URL: "bolt://localhost:7687"})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer a.Close()
	if a.Name() != Name {
		t.Errorf("Name = %q, want %q", a.Name(), Name)
	}
	if err := a.HealthCheck(context.Background()); !errors.Is(err, ErrNotImplemented) {
		t.Errorf("HealthCheck err = %v, want ErrNotImplemented", err)
	}
	if _, err := a.Fetch(context.Background(), source.FetchQuery{}); !errors.Is(err, ErrNotImplemented) {
		t.Errorf("Fetch err = %v, want ErrNotImplemented", err)
	}
	if _, err := a.Add(context.Background(), source.Source{URI: "x"}); !errors.Is(err, ErrNotImplemented) {
		t.Errorf("Add err = %v, want ErrNotImplemented", err)
	}
	if got := a.Config(); got.URL != "bolt://localhost:7687" {
		t.Errorf("Config().URL = %q", got.URL)
	}
}

func TestStub_SatisfiesInterface(t *testing.T) {
	var _ source.Adapter = (*Adapter)(nil)
}
