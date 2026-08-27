package main

// ConfirmationEnvelope is the agent-mode response for a mutation that would
// otherwise block on an interactive TTY confirmation prompt (e.g. killing an
// active session without --confirmed-stuck). Instead of hanging on stdin, the
// command emits this envelope as JSON and exits 4 (state-conflict /
// confirmation-required) so an unattended agent can parse the required
// confirmation and re-run with the documented bypass flag.
//
// This implements the defer-don't-block pattern: a human at a TTY still gets
// the interactive prompt; an agent gets a parseable response plus the exact
// command to re-run.
type ConfirmationEnvelope struct {
	ExitCode int    `json:"exit_code"` // 4 (ExitStateConflict)
	Error    string `json:"error"`     // "confirmation required"
	Action   string `json:"action"`    // e.g. "session_kill"
	Target   string `json:"target"`    // session name / resource identifier
	RerunCmd string `json:"rerun_cmd"` // exact command to re-run with the bypass flag
	Reason   string `json:"reason"`    // human-readable explanation of what to confirm
}

// emitConfirmationEnvelope marshals env to stdout as JSON and returns an
// exitError carrying ExitStateConflict (4). Returning an error (rather than
// calling os.Exit directly) lets the value flow back through run() →
// exitCodeFromError, which both yields the correct process exit code and keeps
// the caller's deferred cleanup (OTel spans, audit logging) intact.
func emitConfirmationEnvelope(env ConfirmationEnvelope) error {
	if env.ExitCode == 0 {
		env.ExitCode = ExitStateConflict
	}
	if env.Error == "" {
		env.Error = "confirmation required"
	}
	if err := printJSON(env); err != nil {
		return err
	}
	return &exitError{code: env.ExitCode, msg: env.Error, rendered: true}
}
