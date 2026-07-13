// Package burndownmaint defines harness-neutral AGM worker launch arguments.
package burndownmaint

import "strings"

// Route selects the harness, model, and workspace for a burndown worker.
type Route struct {
	Harness   string
	Model     string
	Workspace string
}

// BuildSessionArgs builds shell-free arguments for `agm session new`.
func BuildSessionArgs(name string, route Route) []string {
	args := []string{"session", "new", name, "--detached"}
	if strings.TrimSpace(route.Harness) != "" {
		args = append(args, "--harness", route.Harness)
	}
	if strings.TrimSpace(route.Model) != "" {
		args = append(args, "--model", route.Model)
	}
	if strings.TrimSpace(route.Workspace) != "" {
		args = append(args, "--workspace", route.Workspace)
	}
	return args
}
