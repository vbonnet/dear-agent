package agent

import (
	"fmt"
	"sync"
	"testing"
)

func TestConcurrentRegistration(t *testing.T) {
	orig := snapshotRegistryForTest()
	resetRegistryForTest(make(map[string]Agent))
	defer func() { resetRegistryForTest(orig) }()

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			name := fmt.Sprintf("agent-%d", id)
			mock := &registryMockAgent{name: name, version: "1.0"}
			Register(name, mock)
		}(i)
	}
	wg.Wait()

	// Verify all agents were registered
	for i := 0; i < 50; i++ {
		name := fmt.Sprintf("agent-%d", i)
		got, ok := Get(name)
		if !ok {
			t.Errorf("agent %q not found after concurrent registration", name)
			continue
		}
		if got.Name() != name {
			t.Errorf("Get(%q).Name() = %q, want %q", name, got.Name(), name)
		}
	}
}

func TestConcurrentLookup(t *testing.T) {
	orig := snapshotRegistryForTest()
	resetRegistryForTest(make(map[string]Agent))
	defer func() { resetRegistryForTest(orig) }()

	// Pre-populate registry
	for i := 0; i < 10; i++ {
		name := fmt.Sprintf("agent-%d", i)
		Register(name, &registryMockAgent{name: name, version: "1.0"})
	}

	// Concurrent reads and writes
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			// Read existing
			name := fmt.Sprintf("agent-%d", id%10)
			Get(name)
			// Write new
			newName := fmt.Sprintf("new-agent-%d", id)
			Register(newName, &registryMockAgent{name: newName, version: "2.0"})
			// Read back
			Get(newName)
		}(i)
	}
	wg.Wait()
}
