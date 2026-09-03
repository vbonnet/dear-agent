# Problem

dear-agent has built event-like plumbing at least six separate times, independently,
without ever converging on one shape: VROOM's decision topic constants (unwired),
Wayfinder's phase events (emitted into an unsubscribed bus), the trigger registry
(implemented, never instantiated in a production process), the absence-alarm's pulse
model, the GitHub webhook receiver (compiled, never deployed), and `pkg/workflow`'s
own audit trail.

The concrete, felt cost is the PR-queue throughput problem: `internal/mergeloop/driver.go`
correctly detects when open PRs exceed its cap and skips the tick, but that signal
reaches only mergeloop's own audit log. Nothing else in the system, an operator,
another pipeline, a capacity decision, can react to it. The system has the sensor; it
has no way to carry the signal anywhere else.

Full analysis: [docs/architecture/feedback-loop-pipelines.md](docs/architecture/feedback-loop-pipelines.md).
