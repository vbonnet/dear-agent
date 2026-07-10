// Package vroomprompt defines harness-neutral VROOM worker prompt routing.
package vroomprompt

import "fmt"

// Route selects the AGM harness, model, mode, and workspace for a worker.
type Route struct {
	Harness   string
	Model     string
	Mode      string
	Workspace string
}

// DefaultRoute preserves the historical VROOM worker route.
func DefaultRoute() Route {
	return Route{Harness: "claude-code", Model: "claude-opus-4-8", Mode: "auto", Workspace: "oss"}
}

// WorkerRule renders the route as a prompt rule for AGM dispatch.
func WorkerRule(route Route) string {
	return fmt.Sprintf("Workers MUST use harness=%s, model=%s, --mode=%s, --workspace=%s",
		route.Harness, route.Model, route.Mode, route.Workspace)
}
