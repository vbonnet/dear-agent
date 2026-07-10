package mergeloop

import "strings"

// SpawnRoute selects the AGM harness and optional model for repair agents.
type SpawnRoute struct {
	Harness string
	Model   string
}

// BuildSessionNewArgs builds shell-free AGM arguments for a repair agent.
// AGM performs canonical harness, model-family, and permission validation.
func BuildSessionNewArgs(req AgentRequest, workspace string, route SpawnRoute) []string {
	args := []string{"session", "new", req.SessionName,
		"--workspace", workspace, "--detached", "--prompt", req.Prompt}
	if strings.TrimSpace(route.Harness) != "" {
		args = append(args, "--harness", route.Harness)
	}
	if strings.TrimSpace(route.Model) != "" {
		args = append(args, "--model", route.Model)
	}
	return args
}
