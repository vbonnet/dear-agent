package workflow

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// LoadFile reads a workflow YAML from path and returns a validated
// Workflow. Returns an error if the file is missing, malformed, or fails
// Workflow.Validate().
func LoadFile(path string) (*Workflow, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("workflow: read %s: %w", path, err)
	}
	return LoadBytes(data)
}

// LoadBytes parses YAML from an in-memory buffer. Used by tests and
// callers that embed workflows via go:embed.
func LoadBytes(data []byte) (*Workflow, error) {
	var w Workflow
	if err := yaml.Unmarshal(data, &w); err != nil {
		return nil, fmt.Errorf("workflow: parse yaml: %w", err)
	}
	// Default the max_iters for loop nodes — the engine uses this as a
	// safety belt to avoid runaway loops from a broken Until condition.
	applyDefaults(&w)
	if err := w.Validate(); err != nil {
		return nil, err
	}
	return &w, nil
}

func applyDefaults(w *Workflow) {
	// Recurse into loops to default their max_iters.
	var defaultNodes func(nodes []Node)
	defaultNodes = func(nodes []Node) {
		for i := range nodes {
			if nodes[i].Kind == KindLoop && nodes[i].Loop != nil {
				if nodes[i].Loop.MaxIters <= 0 {
					nodes[i].Loop.MaxIters = 100
				}
				defaultNodes(nodes[i].Loop.Nodes)
			}
		}
	}
	defaultNodes(w.Nodes)
}
