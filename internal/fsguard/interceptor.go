package fsguard

// Interceptor is the reusable, in-process sandbox middleware. It wraps a Guard
// (the worktree-only policy) and a Recorder (violation logging), exposing two
// pre-write checks — CheckWrite for the file tools and CheckCommand for Bash.
// Both return a Decision and, when a write is blocked, record a Violation for
// later DEAR-retro analysis.
//
// It is the integration point for AGM sessions (and the PreToolUse hooks): a
// caller intercepting a tool invocation asks the Interceptor whether the write
// is allowed and, if not, surfaces Decision.Message (which already carries the
// escalation hint) to the agent. Decision.Enforcement tells the hook binary
// which graduated response to emit (hard block / soft warn / ask / defer).
type Interceptor struct {
	guard       *Guard
	rec         Recorder
	enforcement Enforcement
}

// Decision is the result of an interception check.
type Decision struct {
	// Allowed reports whether the write may proceed.
	Allowed bool
	// Message is the positive-guidance text to surface when blocked; it already
	// includes the escalation hint. Empty when allowed.
	Message string
	// Violation is the recorded violation when blocked; nil when allowed.
	Violation *Violation
	// Enforcement carries the policy enforcement level at the time of the check.
	// For allowed writes it is always EnforceDeny (the zero value). For blocked
	// writes it reflects the configured level so hook binaries can decide
	// whether to hard-block (EnforceDeny), soft-warn (EnforceWarn), ask the
	// user (EnforceAsk), or defer (EnforceDefer).
	Enforcement Enforcement
}

// NewInterceptor builds an Interceptor from the environment-resolved Config
// (see LoadConfig): the default worktree-only policy layered with FSGUARD_*
// overrides, and a FileRecorder at the configured log path (or a NopRecorder
// when logging is disabled).
//
// It never returns an unguarded interceptor: a malformed FSGUARD_CONFIG degrades
// to safe defaults rather than disabling enforcement, and the config error is
// returned for visibility. Callers that must not fail closed (the hooks) can use
// the interceptor regardless of the returned error.
func NewInterceptor() (*Interceptor, error) {
	cfg, err := LoadConfig()
	g := NewWithPolicy(cfg.Policy)
	var rec Recorder = NopRecorder{}
	if cfg.LogPath != "" {
		rec = NewFileRecorder(cfg.LogPath)
	}
	return &Interceptor{guard: g, rec: rec, enforcement: cfg.Policy.Enforcement}, err
}

// NewInterceptorWith builds an Interceptor from an explicit Guard and Recorder.
// Used by tests and callers that want full control over policy and logging.
func NewInterceptorWith(g *Guard, rec Recorder) *Interceptor {
	if rec == nil {
		rec = NopRecorder{}
	}
	enforcement := EnforceDeny
	if g.policy != nil {
		enforcement = g.policy.Enforcement
	}
	return &Interceptor{guard: g, rec: rec, enforcement: enforcement}
}

// CheckWrite classifies a file-tool write to path (anchored at cwd) for the
// named tool (Edit/Write/MultiEdit). On a block it records a Violation and
// returns guidance with the escalation hint appended.
func (i *Interceptor) CheckWrite(tool, path, cwd string) Decision {
	// Resolve once and reuse: ClassifyResolved and the Violation share the same
	// expanded+symlink-resolved target, avoiding a second EvalSymlinks pass.
	resolved := i.guard.Resolve(path, cwd)
	allowed, msg := i.guard.ClassifyResolved(resolved)
	if allowed {
		return Decision{Allowed: true}
	}
	v := Violation{
		Tool:     tool,
		Path:     path,
		CWD:      cwd,
		Resolved: resolved,
		Reason:   msg,
	}
	_ = i.rec.Record(v)
	return Decision{
		Allowed:     false,
		Message:     msg + Escalation,
		Violation:   &v,
		Enforcement: i.enforcement,
	}
}

// CheckCommand inspects a Bash command (anchored at cwd) for filesystem-mutating
// operations that escape the sandbox. On a block it records a Violation and
// returns guidance with the escalation hint appended.
func (i *Interceptor) CheckCommand(command, cwd string) Decision {
	allowed, msg := i.guard.InspectCommand(command, cwd)
	if allowed {
		return Decision{Allowed: true}
	}
	v := Violation{
		Tool:    "Bash",
		Command: command,
		CWD:     cwd,
		Reason:  msg,
	}
	_ = i.rec.Record(v)
	return Decision{
		Allowed:     false,
		Message:     msg + Escalation,
		Violation:   &v,
		Enforcement: i.enforcement,
	}
}
