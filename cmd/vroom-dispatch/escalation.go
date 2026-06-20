package main

import (
	"fmt"
	"maps"
	"os"
	"os/exec"
	"strings"
)

// --- Human escalation channel (ce-mcw2) ---
//
// escalateToHuman is the daemon's "pull a human in" hook. It fires when an
// automated recovery path is exhausted (3× restart failure today; OAuth expiry
// with no auto-refresh and a >30min model outage are future callers) and the
// mesh genuinely needs a person. Two channels are attempted, both best-effort
// and both non-blocking so a wedged notifier can never stall the health tick
// (AC-5.3):
//
//   - osascript desktop notification — the daemon runs on the operator's Mac,
//     so a native notification is the always-available local channel (AC-5.1).
//   - PushNotification via MCP — reaches the operator's phone when Remote
//     Control is connected. The MCP tool only exists *inside* a live Claude
//     Code session; this Go daemon has no MCP client, so (consistent with how
//     ce-hn8n does every other session interaction) it relays the push request
//     through an active supervisor session via `agm send`. With no active
//     session there is nothing to relay through — that is the AC-5.6 fallback
//     (osascript already fired), explicitly not an error (AC-5.2, AC-5.6).
//
// Both channels are reached through package-level seams so the escalation path
// is unit-testable without spawning osascript or an agm session (AC-5.5).

// desktopNotify sends a macOS desktop notification. It MUST be non-blocking
// (the daemon must not wait on a notification UI). Overridable in tests.
var desktopNotify = osascriptNotify

// mcpPush attempts a PushNotification via MCP, relayed through an active
// supervisor session. It returns sent=false (with a nil error) when no session
// is available to relay through — the AC-5.6 fallback, not a failure. It MUST
// be non-blocking. Overridable in tests.
var mcpPush = pushViaActiveSession

// escalateToHuman records a structured escalation event and fires both human
// notification channels. trigger is a short stable token (e.g.
// "restart_exhausted"); message is the human-readable summary; extra carries
// any structured fields worth keeping queryable in the trail (e.g. the
// supervisor name and restart count). It never blocks the caller: the two
// channels are contractually non-blocking, and any channel failure is logged to
// the trail rather than propagated (AC-5.3).
//
// trigger is part of the escalation contract: restart_exhausted is the only
// wired trigger today, but the OAuth-expiry and >30min model-outage callers in
// W3-spec pass their own trigger tokens — hence the unparam suppression.
//
//nolint:unparam // trigger varies across future callers (see doc comment)
func escalateToHuman(home, trigger, message string, extra map[string]any) {
	// AC-5.4: append a structured escalation record. trailRecord already carries
	// kind + timestamp at the top level; trigger/message (and any extra fields)
	// go in the payload so they stay queryable.
	payload := map[string]any{
		"trigger": trigger,
		"message": message,
	}
	maps.Copy(payload, extra)
	writeTrail(home, "dispatch.escalation", payload)

	// AC-5.1: desktop notification.
	if err := desktopNotify(message); err != nil {
		writeTrail(home, "dispatch.escalation.desktop_failed", map[string]any{
			"error": err.Error(),
		})
	}

	// AC-5.2 / AC-5.6: phone push via MCP relay, falling back silently when no
	// session is available.
	sent, err := mcpPush(home, message)
	switch {
	case err != nil:
		writeTrail(home, "dispatch.escalation.mcp_failed", map[string]any{
			"error": err.Error(),
		})
	case !sent:
		writeTrail(home, "dispatch.escalation.mcp_unavailable", nil)
	}
}

// osascriptNotify shows a native macOS notification via osascript. It returns
// immediately: the command is Start()ed (not Run()) and reaped in a background
// goroutine, so a slow or hung notification daemon cannot stall the caller
// (AC-5.3). A missing osascript binary surfaces as a Start error.
func osascriptNotify(message string) error {
	args := osascriptArgs(message)
	cmd := exec.Command("osascript", args...)
	cmd.Env = scrubAPIKey(os.Environ())
	if err := cmd.Start(); err != nil {
		return err
	}
	reapAsync(cmd) // reap to avoid a zombie; ignore notification result
	return nil
}

// reapAsync waits on a spawned notifier process in the background so it neither
// blocks the caller (AC-5.3) nor lingers as a zombie. The recover() guards the
// daemon: a notification must never crash the health loop, even if Wait panics
// (e.g. on an already-released process).
func reapAsync(cmd *exec.Cmd) {
	go func() {
		defer func() { recover() }() //nolint:errcheck // intentional: swallow any panic so a notifier never crashes the daemon
		_ = cmd.Wait()
	}()
}

// osascriptArgs builds the osascript invocation for an escalation message.
// Extracted from osascriptNotify so the script construction and AppleScript
// escaping are unit-testable without exec'ing osascript.
func osascriptArgs(message string) []string {
	script := fmt.Sprintf("display notification %s with title %s",
		appleScriptString("VROOM escalation: "+message),
		appleScriptString("VROOM Dispatch Advisor"))
	return []string{"-e", script}
}

// appleScriptString renders s as an AppleScript double-quoted string literal.
// Backslash and double-quote are escaped; control characters (which would break
// the single-line `-e` script) are stripped, with whitespace control chars
// folded to a space.
func appleScriptString(s string) string {
	s = strings.Map(func(r rune) rune {
		switch {
		case r == '\n' || r == '\r' || r == '\t':
			return ' '
		case r < 0x20:
			return -1 // drop other control chars
		default:
			return r
		}
	}, s)
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return `"` + s + `"`
}

// pushViaActiveSession relays a PushNotification request through the first live
// supervisor session, which (unlike this daemon) has the MCP tool. It returns
// sent=false, nil when no supervisor session is alive — the AC-5.6 fallback.
// Like osascriptNotify it Start()s and reaps asynchronously so it never blocks.
func pushViaActiveSession(home, message string) (bool, error) {
	target := firstActiveSupervisor()
	if target == "" {
		return false, nil // AC-5.6: no session to relay through — not an error.
	}
	prompt := "ESCALATION RELAY (vroom-dispatch): call the PushNotification tool " +
		"once with status \"proactive\" and exactly this message, then stop: " +
		"VROOM escalation: " + message
	cmd := exec.Command("agm", "send", "msg", target,
		"--sender", "vroom-dispatch",
		"--autonomous",
		"--prompt", prompt)
	cmd.Env = scrubAPIKey(os.Environ())
	if err := cmd.Start(); err != nil {
		return false, err
	}
	reapAsync(cmd)
	return true, nil
}

// firstActiveSupervisor returns the name of the first supervisor whose session
// is alive, or "" if none are. Used to pick a relay target for the MCP push.
func firstActiveSupervisor() string {
	for _, sup := range supervisors {
		if isSessionAlive(sup.Name) {
			return sup.Name
		}
	}
	return ""
}
