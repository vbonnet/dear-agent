package safety

import (
	"github.com/vbonnet/dear-agent/agm/internal/tmux"
)

// Check runs all enabled guards for a session and returns the result.
// If all guards pass, CheckResult.Safe is true.
// Callers should check the result and either proceed or abort with the error message.
func Check(sessionName string, opts GuardOptions) *CheckResult {
	socketPath := opts.SocketPath
	if socketPath == "" {
		socketPath = tmux.GetSocketPath()
	}

	result := &CheckResult{Safe: true}

	if !opts.SkipHumanTyping {
		if v := CheckHumanTyping(sessionName, socketPath); v != nil {
			result.Safe = false
			result.Violations = append(result.Violations, *v)
		}
	}

	if !opts.SkipUninitialized {
		if v := CheckSessionUninitialized(sessionName, socketPath); v != nil {
			result.Safe = false
			result.Violations = append(result.Violations, *v)
		}
	}

	if !opts.SkipMidResponse {
		if v := CheckClaudeMidResponse(sessionName, socketPath); v != nil {
			result.Safe = false
			result.Violations = append(result.Violations, *v)
		}
	}

	return result
}
