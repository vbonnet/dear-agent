package bus

import (
	"errors"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
)

// recordingDelivery is a test Delivery that captures everything written to it
// and counts Close calls. Safe for concurrent Deliver.
type recordingDelivery struct {
	mu      sync.Mutex
	frames  []*Frame
	closed  atomic.Bool
	deliverErr error
}

func (r *recordingDelivery) Deliver(f *Frame) error {
	if r.deliverErr != nil {
		return r.deliverErr
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.frames = append(r.frames, f)
	return nil
}

func (r *recordingDelivery) Close() error {
	r.closed.Store(true)
	return nil
}

func (r *recordingDelivery) Frames() []*Frame {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]*Frame, len(r.frames))
	copy(out, r.frames)
	return out
}

func TestRegistryRegisterRoute(t *testing.T) {
	r := NewRegistry()
	d := &recordingDelivery{}
	if err := r.Register("s1", d); err != nil {
		t.Fatalf("Register: %v", err)
	}
	got, err := r.Route("s1")
	if err != nil {
		t.Fatalf("Route: %v", err)
	}
	if got != d {
		t.Fatalf("Route returned %v, want %v", got, d)
	}
}

func TestRegistryRouteOffline(t *testing.T) {
	r := NewRegistry()
	if _, err := r.Route("nobody"); !errors.Is(err, ErrTargetOffline) {
		t.Errorf("Route on empty registry: got %v, want ErrTargetOffline", err)
	}
}

func TestRegistryDuplicateRegister(t *testing.T) {
	r := NewRegistry()
	if err := r.Register("s1", &recordingDelivery{}); err != nil {
		t.Fatal(err)
	}
	err := r.Register("s1", &recordingDelivery{})
	if !errors.Is(err, ErrAlreadyRegistered) {
		t.Errorf("duplicate Register: got %v, want ErrAlreadyRegistered", err)
	}
}

func TestRegistryUnregister(t *testing.T) {
	r := NewRegistry()
	d := &recordingDelivery{}
	_ = r.Register("s1", d)

	got := r.Unregister("s1")
	if got != d {
		t.Errorf("Unregister returned %v, want original delivery", got)
	}
	// Now re-registration should succeed.
	if err := r.Register("s1", &recordingDelivery{}); err != nil {
		t.Errorf("re-Register after Unregister: %v", err)
	}
	// Unregister of absent session returns nil.
	if r.Unregister("nobody") != nil {
		t.Error("Unregister absent: expected nil")
	}
}

func TestRegistryRegisterValidation(t *testing.T) {
	r := NewRegistry()
	if err := r.Register("", &recordingDelivery{}); err == nil {
		t.Error("Register empty id should fail")
	}
	if err := r.Register("s1", nil); err == nil {
		t.Error("Register nil delivery should fail")
	}
}

func TestRegistryActive(t *testing.T) {
	r := NewRegistry()
	_ = r.Register("s1", &recordingDelivery{})
	_ = r.Register("s2", &recordingDelivery{})
	_ = r.Register("s3", &recordingDelivery{})

	active := r.Active()
	sort.Strings(active)
	want := []string{"s1", "s2", "s3"}
	if len(active) != len(want) {
		t.Fatalf("Active returned %v, want %v", active, want)
	}
	for i, id := range active {
		if id != want[i] {
			t.Errorf("Active[%d] = %q, want %q", i, id, want[i])
		}
	}
	if r.Len() != 3 {
		t.Errorf("Len = %d, want 3", r.Len())
	}
}

func TestRegistryConcurrent(t *testing.T) {
	r := NewRegistry()
	var wg sync.WaitGroup
	// 10 writers registering, 10 readers routing, simultaneously. This is
	// mainly to let the race detector catch any mishandled shared state.
	for i := 0; i < 10; i++ {
		wg.Add(2)
		go func(id int) {
			defer wg.Done()
			d := &recordingDelivery{}
			sessionID := string(rune('A' + id))
			_ = r.Register(sessionID, d)
		}(i)
		go func() {
			defer wg.Done()
			_, _ = r.Route("A")
		}()
	}
	wg.Wait()

	// Between 1 and 10 registrations depending on scheduling; at minimum A
	// should be registered and Route(A) should succeed.
	if _, err := r.Route("A"); err != nil {
		t.Errorf("expected A to be routable: %v", err)
	}
}
