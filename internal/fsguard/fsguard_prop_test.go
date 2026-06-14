package fsguard

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
)

// Properties of Guard.Classify:
//
//  1. Any path under ~/worktrees/ is always allowed.
//  2. Any path under ~/src/ is always blocked.
//  3. Any dotfile path (~/.foo) is always blocked.
//  4. Allowed carve-outs (/dev, /tmp, /var/tmp) are always allowed.
//  5. Blocked paths always carry a non-empty guidance message.
//  6. Allowed paths always return an empty guidance message.
func TestFsguardProperties(t *testing.T) {
	properties := gopter.NewProperties(nil)
	// Use a fixed fake home that is NOT under /tmp or /var/folders (both are
	// writable carve-outs in DefaultPolicy, which would defeat "src blocked").
	home := "/home/testuser"
	g := &Guard{Home: home}

	worktreesDir := filepath.Join(home, "worktrees")
	srcDir := filepath.Join(home, "src")

	// Property 1: any sub-path under ~/worktrees/ is allowed.
	properties.Property("worktrees paths always allowed", prop.ForAll(
		func(suffix string) bool {
			path := filepath.Join(worktreesDir, "repo", suffix)
			allowed, _ := g.Classify(path, "")
			return allowed
		},
		gen.Identifier(),
	))

	// Property 2: any sub-path under ~/src/ is blocked.
	properties.Property("src paths always blocked", prop.ForAll(
		func(suffix string) bool {
			path := filepath.Join(srcDir, "repo", suffix)
			allowed, _ := g.Classify(path, "")
			return !allowed
		},
		gen.Identifier(),
	))

	// Property 3: dotfile paths are blocked.
	properties.Property("dotfile paths always blocked", prop.ForAll(
		func(name string) bool {
			if name == "" {
				return true // skip empty
			}
			path := filepath.Join(home, "."+name)
			allowed, _ := g.Classify(path, "")
			return !allowed
		},
		gen.Identifier(),
	))

	// Property 4: blocked paths carry a guidance message.
	properties.Property("blocked paths have non-empty message", prop.ForAll(
		func(suffix string) bool {
			path := filepath.Join(srcDir, suffix)
			allowed, msg := g.Classify(path, "")
			if allowed {
				return true // not blocked, skip
			}
			return msg != ""
		},
		gen.Identifier(),
	))

	// Property 5: allowed paths return empty message.
	properties.Property("allowed paths have empty message", prop.ForAll(
		func(suffix string) bool {
			path := filepath.Join(worktreesDir, "repo", suffix)
			allowed, msg := g.Classify(path, "")
			if !allowed {
				return true // not allowed, skip
			}
			return msg == ""
		},
		gen.Identifier(),
	))

	// Property 6: under(x, base) iff x == base || HasPrefix(x, base+"/").
	properties.Property("under invariant holds", prop.ForAll(
		func(a, b string) bool {
			if a == "" || b == "" {
				return true
			}
			result := under(a, b)
			expected := a == b || strings.HasPrefix(a, b+string(filepath.Separator))
			return result == expected
		},
		gen.AlphaString(),
		gen.AlphaString(),
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}
