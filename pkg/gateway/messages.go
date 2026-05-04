package gateway

// CommandType enumerates the user intents the gateway knows how to
// dispatch. Adapters parse their wire format into one of these values;
// handlers are registered against them.
//
// Adding a new type means: declare it here, register a Handler in the
// HandlerSet, and document the Args/Body contract in the constants block
// below.
type CommandType string

const (
	// CmdRun starts a new workflow. Args: "file" (string, required),
	// "inputs" (map[string]string, optional). Body: "run_id" (string),
	// "pid" (int).
	CmdRun CommandType = "run"

	// CmdStatus fetches a single run's full state. Args: "run_id"
	// (string, required). Body: "run" (*workflow.RunStatus).
	CmdStatus CommandType = "status"

	// CmdList returns a page of run summaries. Args: "state"
	// (string, optional), "limit" (int, optional). Body: "runs"
	// ([]workflow.RunSummary).
	CmdList CommandType = "list"

	// CmdLogs returns audit events for one run. Args: "run_id"
	// (string, required), "limit" (int, optional). Body: "events"
	// ([]workflow.AuditEvent).
	CmdLogs CommandType = "logs"

	// CmdGates lists pending HITL approvals. Args: none. Body: "gates"
	// ([]workflow.HITLRequest).
	CmdGates CommandType = "gates"

	// CmdApprove records an HITL approval. Args: "approval_id"
	// (string, required), "role" (string, optional), "reason" (string,
	// optional). Body: "approval_id", "decision":"approve".
	CmdApprove CommandType = "approve"

	// CmdReject records an HITL rejection. Same args as CmdApprove.
	CmdReject CommandType = "reject"

	// CmdCancel cancels an in-flight run. Args: "run_id" (string,
	// required), "reason" (string, optional). Body: "run_id",
	// "cancelled":true.
	CmdCancel CommandType = "cancel"
)

// EventType enumerates the asynchronous notifications the gateway can
// broadcast. Adapters that subscribe choose which to forward to their
// end users.
type EventType string

const (
	// EventRunFinished fires after a run reaches a terminal state
	// (succeeded, failed, cancelled). Subject is the run_id.
	EventRunFinished EventType = "run_finished"

	// EventHITLOpened fires when a workflow node enters an HITL gate
	// and a request row is created. Subject is the approval_id.
	EventHITLOpened EventType = "hitl_opened"

	// EventHITLResolved fires after a gate is approved or rejected.
	// Subject is the approval_id.
	EventHITLResolved EventType = "hitl_resolved"
)

// Caller identifies the human (or service) that issued a Command. Mirrors
// pkg/api.Caller — duplicated here so pkg/gateway does not import pkg/api
// (the dependency runs the other way: the HTTP adapter imports both).
type Caller struct {
	LoginName string `json:"login_name"`
	Display   string `json:"display,omitempty"`
}

// Command is one user intent. Adapters construct it from their wire
// format; the gateway dispatches it to a handler.
//
// ID is adapter-assigned and only required if the adapter wants to
// correlate the Response. The gateway does not interpret it.
//
// Args is the type-specific payload. The shape is documented per
// CommandType in the constants above. Handlers re-marshal Args into
// whatever pkg/workflow signature they need.
type Command struct {
	ID     string         `json:"id,omitempty"`
	Type   CommandType    `json:"type"`
	Caller Caller         `json:"caller"`
	Args   map[string]any `json:"args,omitempty"`
}

// Response is the synchronous reply to a Command. CommandID echoes
// Command.ID so an adapter can correlate. On error, Err is set and
// Body is nil; on success, Err is nil and Body holds the type-specific
// payload (see the CommandType constants).
type Response struct {
	CommandID string         `json:"command_id,omitempty"`
	Body      map[string]any `json:"body,omitempty"`
	Err       *Error         `json:"error,omitempty"`
}

// Event is an asynchronous notification. Subject is the natural id for
// the event (run_id, approval_id, etc.); Payload carries the rest.
type Event struct {
	Type    EventType      `json:"type"`
	Subject string         `json:"subject,omitempty"`
	Payload map[string]any `json:"payload,omitempty"`
}
