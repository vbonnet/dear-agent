package ecphory

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/vbonnet/dear-agent/pkg/engram"
)

// MaxEngrams prevents unbounded memory growth (DoS protection)
const MaxEngrams = 100000

// MaxSymlinkDepth prevents infinite symlink loops
const MaxSymlinkDepth = 5

// Index represents a frontmatter index for fast filtering
type Index struct {
	mu sync.RWMutex // Protects all maps and slices below

	// Map of tag -> engram paths
	byTag map[string][]string

	// Map of type -> engram paths
	byType map[string][]string

	// Map of agent -> engram paths
	byAgent map[string][]string

	// Map of trigger event type -> engram paths
	byTriggerEvent map[string][]string

	// Agent-agnostic engram paths (cached for performance)
	agentAgnostic []string

	// All engram paths
	all []string

	parser *engram.Parser

	// P0-4 FIX: Track visited symlinks to detect cycles
	visitedSymlinks map[string]bool
	symlinkDepth    int
}

// NewIndex creates a new frontmatter index
func NewIndex() *Index {
	return &Index{
		byTag:           make(map[string][]string),
		byType:          make(map[string][]string),
		byAgent:         make(map[string][]string),
		byTriggerEvent:  make(map[string][]string),
		parser:          engram.NewParser(),
		visitedSymlinks: make(map[string]bool),
		symlinkDepth:    0,
	}
}

// Build builds the index by scanning an engram directory
//
//nolint:gocyclo // reason: linear index-build pipeline; each step has its own small concerns.
func (idx *Index) Build(engramPath string) error {
	// Walk directory tree looking for .ai.md files
	return filepath.Walk(engramPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// P0-4 FIX: Handle symlinks with cycle detection
		if info.Mode()&os.ModeSymlink != 0 {
			// Check if we've already visited this symlink (cycle detection)
			absPath, err := filepath.Abs(path)
			if err != nil {
				return nil
			}

			idx.mu.Lock()
			if idx.visitedSymlinks[absPath] {
				idx.mu.Unlock()
				// Cycle detected, skip this path
				log.Printf("ecphory: symlink cycle detected at %s, skipping", path)
				return nil
			}

			// Check symlink depth limit
			if idx.symlinkDepth >= MaxSymlinkDepth {
				idx.mu.Unlock()
				log.Printf("ecphory: max symlink depth (%d) exceeded at %s, skipping", MaxSymlinkDepth, path)
				return nil
			}

			// Mark as visited and increment depth
			idx.visitedSymlinks[absPath] = true
			idx.symlinkDepth++
			idx.mu.Unlock()

			// Follow symlink to get actual file info
			actualPath, err := filepath.EvalSymlinks(path)
			if err != nil {
				// Skip broken symlinks
				return nil
			}
			actualInfo, err := os.Stat(actualPath)
			if err != nil {
				return nil
			}
			info = actualInfo

			// Decrement depth when done with this branch
			defer func() {
				idx.mu.Lock()
				idx.symlinkDepth--
				idx.mu.Unlock()
			}()
		}

		// Skip non-.ai.md files
		if info.IsDir() || !strings.HasSuffix(path, ".ai.md") {
			return nil
		}

		// Parse just frontmatter (not full content)
		eg, err := idx.parser.Parse(path)
		if err != nil {
			// P0-4: Log parse errors instead of silently ignoring
			log.Printf("ecphory: failed to parse engram at %s: %v", path, err)
			// TODO: Add telemetry here when telemetry system is available
			return nil
		}

		idx.mu.Lock()
		defer idx.mu.Unlock()

		// P0-3 FIX: Check engram count limit to prevent unbounded memory growth
		if len(idx.all) >= MaxEngrams {
			return fmt.Errorf("engram limit exceeded (%d), possible directory corruption or DoS attack", MaxEngrams)
		}

		// Index by tags
		for _, tag := range eg.Frontmatter.Tags {
			idx.byTag[tag] = append(idx.byTag[tag], path)
		}

		// Index by type
		idx.byType[eg.Frontmatter.Type] = append(idx.byType[eg.Frontmatter.Type], path)

		// Index by agent (if specified)
		for _, agent := range eg.Frontmatter.Agents {
			idx.byAgent[agent] = append(idx.byAgent[agent], path)
		}

		// Index by trigger event type (if triggers are specified)
		for _, trigger := range eg.Frontmatter.Triggers {
			if trigger.On != "" {
				idx.byTriggerEvent[trigger.On] = append(idx.byTriggerEvent[trigger.On], path)
			}
		}

		// P0-5: Cache agent-agnostic engrams during Build()
		if len(eg.Frontmatter.Agents) == 0 {
			idx.agentAgnostic = append(idx.agentAgnostic, path)
		}

		// Add to all
		idx.all = append(idx.all, path)

		return nil
	})
}

// FilterByTags returns engrams matching any of the given tags
func (idx *Index) FilterByTags(tags []string) []string {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	pathSet := make(map[string]bool)

	for _, tag := range tags {
		// Support hierarchical tag matching (e.g., "languages/python" matches "languages")
		for indexedTag, paths := range idx.byTag {
			if strings.HasPrefix(indexedTag, tag) || strings.HasPrefix(tag, indexedTag) {
				for _, path := range paths {
					pathSet[path] = true
				}
			}
		}
	}

	// Convert set to slice
	var result []string
	for path := range pathSet {
		result = append(result, path)
	}

	return result
}

// FilterByType returns engrams of a specific type
func (idx *Index) FilterByType(typ string) []string {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	// Return a copy to prevent external modification
	result := make([]string, len(idx.byType[typ]))
	copy(result, idx.byType[typ])
	return result
}

// FilterByAgent returns engrams for a specific agent (or agent-agnostic)
// P0-5 FIX: Pre-indexed agent-agnostic engrams for O(1) performance
func (idx *Index) FilterByAgent(agent string) []string {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	// Get agent-specific engrams
	agentSpecific := idx.byAgent[agent]

	// Combine with pre-cached agent-agnostic engrams
	result := make([]string, 0, len(agentSpecific)+len(idx.agentAgnostic))
	result = append(result, agentSpecific...)
	result = append(result, idx.agentAgnostic...)

	return result
}

// FilterByTriggerEvent returns engram paths that have a trigger matching the given event type.
func (idx *Index) FilterByTriggerEvent(eventType string) []string {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	// Return a copy to prevent external modification
	paths := idx.byTriggerEvent[eventType]
	result := make([]string, len(paths))
	copy(result, paths)
	return result
}

// All returns all engram paths
func (idx *Index) All() []string {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	// Return a copy to prevent external modification
	result := make([]string, len(idx.all))
	copy(result, idx.all)
	return result
}

// Clear releases all index resources (P0-2 FIX: cleanup method)
func (idx *Index) Clear() {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	// Clear all maps and slices to release memory
	idx.byTag = make(map[string][]string)
	idx.byType = make(map[string][]string)
	idx.byAgent = make(map[string][]string)
	idx.byTriggerEvent = make(map[string][]string)
	idx.agentAgnostic = nil
	idx.all = nil
	idx.visitedSymlinks = make(map[string]bool)
	idx.symlinkDepth = 0
}
