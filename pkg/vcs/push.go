package vcs

import (
	"fmt"
	"log"
	"sync"
	"time"
)

// PushStrategy defines how changes are pushed to remote
type PushStrategy string

const (
	// PushImmediate pushes after every commit (blocking)
	PushImmediate PushStrategy = "immediate"

	// PushAsync pushes after every commit (non-blocking goroutine)
	PushAsync PushStrategy = "async"

	// PushBatched pushes on a timer interval
	PushBatched PushStrategy = "batched"

	// PushManual only pushes when explicitly requested
	PushManual PushStrategy = "manual"
)

// ParsePushStrategy converts a string to PushStrategy
func ParsePushStrategy(s string) PushStrategy {
	switch s {
	case "immediate":
		return PushImmediate
	case "async":
		return PushAsync
	case "batched":
		return PushBatched
	case "manual":
		return PushManual
	default:
		return PushAsync // default
	}
}

// Pusher handles pushing changes to a remote repository
type Pusher struct {
	repo       *Repo
	strategy   PushStrategy
	remoteName string
	branch     string

	// For batched strategy
	interval    time.Duration
	pendingPush bool
	mu          sync.Mutex
	stopCh      chan struct{}
	stopped     bool
}

// NewPusher creates a new Pusher with the given configuration
func NewPusher(repo *Repo, strategy PushStrategy, remoteName, branch string, interval time.Duration) *Pusher {
	p := &Pusher{
		repo:       repo,
		strategy:   strategy,
		remoteName: remoteName,
		branch:     branch,
		interval:   interval,
		stopCh:     make(chan struct{}),
	}

	if strategy == PushBatched && interval > 0 {
		go p.batchLoop()
	}

	return p
}

// TriggerPush triggers a push based on the configured strategy.
// For async, launches a goroutine. For immediate, blocks. For manual, no-op.
func (p *Pusher) TriggerPush() error {
	switch p.strategy {
	case PushImmediate:
		return p.doPush()
	case PushAsync:
		go func() {
			if err := p.doPushWithRetry(3); err != nil {
				log.Printf("vcs: async push failed: %v", err)
			}
		}()
		return nil
	case PushBatched:
		p.mu.Lock()
		p.pendingPush = true
		p.mu.Unlock()
		return nil
	case PushManual:
		return nil
	default:
		return fmt.Errorf("unknown push strategy: %s", p.strategy)
	}
}

// ForcePush pushes immediately regardless of strategy
func (p *Pusher) ForcePush() error {
	return p.doPush()
}

// Close stops the batch loop if running
func (p *Pusher) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.stopped {
		p.stopped = true
		close(p.stopCh)
	}
}

func (p *Pusher) batchLoop() {
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			p.mu.Lock()
			pending := p.pendingPush
			p.pendingPush = false
			p.mu.Unlock()

			if pending {
				if err := p.doPushWithRetry(3); err != nil {
					log.Printf("vcs: batched push failed: %v", err)
				}
			}
		case <-p.stopCh:
			// Final push on close
			p.mu.Lock()
			pending := p.pendingPush
			p.pendingPush = false
			p.mu.Unlock()
			if pending {
				_ = p.doPush()
			}
			return
		}
	}
}

func (p *Pusher) doPush() error {
	return p.repo.Push(p.remoteName, p.branch)
}

func (p *Pusher) doPushWithRetry(maxAttempts int) error {
	var lastErr error
	for i := 0; i < maxAttempts; i++ {
		if err := p.doPush(); err != nil {
			lastErr = err
			// Exponential backoff: 1s, 2s, 4s
			time.Sleep(time.Duration(1<<uint(i)) * time.Second)
			continue
		}
		return nil
	}
	return fmt.Errorf("push failed after %d attempts: %w", maxAttempts, lastErr)
}
