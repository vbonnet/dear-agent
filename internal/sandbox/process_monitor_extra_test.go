package sandbox

import (
	"context"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestProcessMonitor_CountDescendants(t *testing.T) {
	m := NewProcessMonitor(os.Getpid(), DefaultProcessLimits(), nil)
	count, err := m.CountDescendants()
	if err != nil {
		t.Skipf("cannot count descendants on this platform: %v", err)
	}
	assert.GreaterOrEqual(t, count, 0)
}

func TestProcessMonitor_CustomLimits(t *testing.T) {
	limits := ProcessLimits{
		MaxProcesses:        100,
		MaxProcessSpawnRate: 10,
		PollInterval:        500 * time.Millisecond,
	}
	m := NewProcessMonitor(os.Getpid(), limits, nil)
	assert.Equal(t, 100, m.limits.MaxProcesses)
	assert.Equal(t, 10, m.limits.MaxProcessSpawnRate)
	assert.Equal(t, 500*time.Millisecond, m.limits.PollInterval)
}

func TestProcessMonitor_StopWithoutStart(t *testing.T) {
	m := NewProcessMonitor(os.Getpid(), ProcessLimits{PollInterval: 50 * time.Millisecond}, nil)
	// Should not panic
	m.Stop()
}

func TestProcessMonitor_ContextCancellation(t *testing.T) {
	var alertCount atomic.Int32
	m := NewProcessMonitor(os.Getpid(), ProcessLimits{
		MaxProcesses:        10000,
		MaxProcessSpawnRate: 10000,
		PollInterval:        50 * time.Millisecond,
	}, func(at AlertType, msg string) {
		alertCount.Add(1)
	})

	ctx, cancel := context.WithCancel(context.Background())
	m.Start(ctx)
	time.Sleep(100 * time.Millisecond)
	cancel() // Cancel context should stop the monitor
	time.Sleep(100 * time.Millisecond)

	assert.Equal(t, int32(0), alertCount.Load())
}

func TestGetChildPIDs_InvalidPID(t *testing.T) {
	_, err := getChildPIDs(999999999)
	assert.Error(t, err)
}

func TestGetChildPIDs_CurrentProcess(t *testing.T) {
	children, err := getChildPIDs(os.Getpid())
	if err != nil {
		t.Skipf("cannot read /proc on this platform: %v", err)
	}
	// Current process may or may not have children; just verify no panic
	_ = children
}
