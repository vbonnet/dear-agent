package profile_test

import (
	"fmt"
	"os"

	"github.com/vbonnet/dear-agent/engram/internal/profile"
)

// Example shows the two-tier lifecycle: a static identity fixed at spawn time,
// then a dynamic profile that a worker updates as it runs, and finally a
// handoff load that resumes both tiers in a later session.
func Example() {
	dir, _ := os.MkdirTemp("", "engram-profiles-")
	defer os.RemoveAll(dir)
	store := profile.NewStore(dir) // in production, "" defaults to ~/.engram/profiles

	// --- spawn time: write the immutable identity once ---
	_ = store.SaveStatic(profile.StaticProfile{
		AgentID:      "ce-ywii",
		Name:         "ce-ywii-worker",
		Role:         "go-implementer",
		Capabilities: []string{"edit-go", "run-tests"},
		Constraints:  []string{"no-force-push", "no-writes-to-src"},
		Model:        "claude-opus-4-8",
	})

	// --- during the session: record focus and discoveries ---
	_, _ = store.SetTaskFocus("ce-ywii", "session-1", "add profile package")
	_, _ = store.RecordDiscovery("ce-ywii", "session-1",
		"engram persistence is file-based", "no external DB; see SPEC.md")

	// --- handoff / resumption: load both tiers at once ---
	p, _ := store.LoadProfile("ce-ywii", "session-1")
	fmt.Println(p.Static.Role)
	fmt.Println(p.Dynamic.TaskFocus)
	fmt.Println(len(p.Dynamic.Discoveries))
	// Output:
	// go-implementer
	// add profile package
	// 1
}
