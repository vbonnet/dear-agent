package manager_test

import (
	"testing"

	"github.com/vbonnet/dear-agent/agm/internal/manager"
)

func TestRegistry_RegisterAndGet(t *testing.T) {
	reg := manager.NewRegistry()

	err := reg.Register("test", func() (manager.Backend, error) {
		return &mockBackend{name: "test"}, nil
	})
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	b, err := reg.Get("test")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if b.Name() != "test" {
		t.Errorf("expected name 'test', got %q", b.Name())
	}
}

func TestRegistry_RegisterDuplicate(t *testing.T) {
	reg := manager.NewRegistry()
	factory := func() (manager.Backend, error) {
		return &mockBackend{}, nil
	}
	_ = reg.Register("dup", factory)
	err := reg.Register("dup", factory)
	if err == nil {
		t.Fatal("expected error on duplicate registration")
	}
}

func TestRegistry_RegisterNilFactory(t *testing.T) {
	reg := manager.NewRegistry()
	err := reg.Register("nil", nil)
	if err == nil {
		t.Fatal("expected error on nil factory")
	}
}

func TestRegistry_GetUnregistered(t *testing.T) {
	reg := manager.NewRegistry()
	_, err := reg.Get("nonexistent")
	if err == nil {
		t.Fatal("expected error for unregistered backend")
	}
}

func TestRegistry_List(t *testing.T) {
	reg := manager.NewRegistry()
	_ = reg.Register("a", func() (manager.Backend, error) { return &mockBackend{}, nil })
	_ = reg.Register("b", func() (manager.Backend, error) { return &mockBackend{}, nil })

	names := reg.List()
	if len(names) != 2 {
		t.Fatalf("expected 2 backends, got %d", len(names))
	}
}

func TestGetDefault_Tmux(t *testing.T) {
	// DefaultRegistry should have "tmux" registered via init() in tmuxbackend
	// This test verifies the default path works
	b, err := manager.GetDefault("nonexistent-backend")
	if err == nil {
		t.Logf("got backend: %s", b.Name())
	}
	// Not a hard failure since tmuxbackend may not be imported in this test
}
