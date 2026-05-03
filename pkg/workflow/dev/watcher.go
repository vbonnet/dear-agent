package dev

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
)

// WatchOptions configures HotReload. The zero value is usable —
// debounce defaults to 200ms, which is enough to coalesce the typical
// editor save burst (atomic write + chmod) into one reload.
type WatchOptions struct {
	// Debounce coalesces rapid write events. A common editor save
	// produces multiple fsnotify events (rename + create + chmod);
	// debouncing collapses them into a single reload tick. Zero uses
	// 200ms. Negative is treated as zero.
	Debounce time.Duration
}

// HotReload watches the workflow file and its fixture file for changes
// and invokes onChange on each debounced event. Returns when ctx is
// cancelled or an unrecoverable watcher error occurs.
//
// onChange runs synchronously on the watcher goroutine; long-running
// callbacks should hand off to a worker. The intended use is REPL hot-
// reload: the callback calls Session.Reload and prints a status line.
//
//nolint:gocyclo // event-loop dispatch reads better as one switch than as helpers
func HotReload(ctx context.Context, paths []string, opts WatchOptions, onChange func(path string)) error {
	if len(paths) == 0 {
		return errors.New("dev: HotReload needs at least one path")
	}
	debounce := opts.Debounce
	if debounce <= 0 {
		debounce = 200 * time.Millisecond
	}
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("dev: new watcher: %w", err)
	}
	defer w.Close()

	// Watch the directory rather than the file itself — many editors
	// rename-then-create on save, which destroys file-level watches.
	dirs := uniqueDirs(paths)
	pathSet := stringSet(paths)
	for _, d := range dirs {
		if err := w.Add(d); err != nil {
			return fmt.Errorf("dev: watch %s: %w", d, err)
		}
	}

	pending := make(map[string]bool)
	var timer *time.Timer
	resetTimer := func() {
		if timer == nil {
			timer = time.NewTimer(debounce)
			return
		}
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		timer.Reset(debounce)
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case ev, ok := <-w.Events:
			if !ok {
				return nil
			}
			if !relevantEvent(ev) {
				continue
			}
			if !pathSet[ev.Name] {
				continue
			}
			pending[ev.Name] = true
			resetTimer()
		case err, ok := <-w.Errors:
			if !ok {
				return nil
			}
			return fmt.Errorf("dev: watcher error: %w", err)
		case <-timerC(timer):
			for p := range pending {
				onChange(p)
			}
			pending = map[string]bool{}
		}
	}
}

func relevantEvent(ev fsnotify.Event) bool {
	// Editors typically Write or Create-on-rename. Chmod alone is
	// usually noise. Remove without a follow-up Create means the file
	// is gone — also relevant so the REPL can show the failure.
	if ev.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Remove|fsnotify.Rename) != 0 {
		return true
	}
	return false
}

func uniqueDirs(paths []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		d := filepath.Dir(p)
		if !seen[d] {
			seen[d] = true
			out = append(out, d)
		}
	}
	return out
}

func stringSet(paths []string) map[string]bool {
	m := make(map[string]bool, len(paths))
	for _, p := range paths {
		m[p] = true
	}
	return m
}

// timerC returns the timer's channel or a nil channel when the timer
// is nil — receiving from nil blocks forever, which is what we want
// when no debounce is pending.
func timerC(t *time.Timer) <-chan time.Time {
	if t == nil {
		return nil
	}
	return t.C
}
