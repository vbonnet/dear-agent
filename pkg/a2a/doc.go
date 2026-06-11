// Package a2a wraps the upstream A2A (Agent-to-Agent) protocol SDK so
// dear-agent components can expose Claude Code sessions as A2A endpoints
// and supervisors can drive them through tasks.
//
// The spike's goal is to replace the in-process AskUserQuestion blocking
// call with the protocol-native input-required task state: an agent that
// needs supervisor input pauses its task with status=input-required, the
// supervisor sends a follow-up message with the answer, and the task
// resumes. See ADR-024.
//
// Public surface (server side):
//
//	Server          — bundles an http.Server, an Agent Card endpoint, and
//	                  the A2A JSON-RPC handler over a SessionHandler.
//	SessionHandler  — application hook that processes one task. May call
//	                  SessionIO.AskInput to pause for input.
//	SessionIO       — runtime methods a SessionHandler uses while a task
//	                  is in flight (Emit, AskInput).
//	SessionCard     — convenience builder for the Agent Card every Server
//	                  serves under /.well-known/agent-card.json.
//
// Public surface (client side) lives in pkg/a2a/client.
//
// Horizontal scaling: each Server is independent. A supervisor talks to
// any number of agents on any number of hosts via their Agent Card URLs;
// there is no central queue and no per-process cap. A2A's task model
// fans out by definition — one TaskID per delegated unit of work.
package a2a
