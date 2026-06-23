package safety

import (
	"sync"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/tmux"
)

const cooldownTTL = 500 * time.Millisecond

var cooldownMu sync.Mutex
var cooldownCache = make(map[string]time.Time)

func isHumanTypingCooldownActive(sessionName string) bool {
	cooldownMu.Lock()
	defer cooldownMu.Unlock()
	t, ok := cooldownCache[sessionName]
	return ok && time.Since(t) < cooldownTTL
}

func recordHumanTypingCooldown(sessionName string) {
	cooldownMu.Lock()
	defer cooldownMu.Unlock()
	cooldownCache[sessionName] = time.Now()
}

// ResetCooldownCache clears the cooldown cache (for testing).
func ResetCooldownCache() {
	cooldownMu.Lock()
	defer cooldownMu.Unlock()
	cooldownCache = make(map[string]time.Time)
}

// Check runs all enabled guards for a session and returns the result.
// If all guards pass, CheckResult.Safe is true.
// Callers should check the result and either proceed or abort with the error message.
func Check(sessionName string, opts GuardOptions) *CheckResult {
	socketPath := opts.SocketPath
	if socketPath == "" {
		socketPath = tmux.GetSocketPath()
	}

	result := &CheckResult{Safe: true}

	if !opts.SkipHumanTyping && !opts.AutonomousMode {
		if !isHumanTypingCooldownActive(sessionName) {
			if v := CheckHumanTyping(sessionName, socketPath); v != nil {
				result.Safe = false
				result.Violations = append(result.Violations, *v)
			} else {
				recordHumanTypingCooldown(sessionName)
			}
		}
	}

	if !opts.SkipUninitialized {
		if v := CheckSessionUninitialized(sessionName, socketPath, opts.Harness); v != nil {
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
