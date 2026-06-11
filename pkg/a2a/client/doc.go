// Package client is the supervisor-side counterpart to pkg/a2a.
//
// A Client wraps the upstream a2aclient.Client with a single, opinionated
// loop: Send a prompt, drive the resulting task to a terminal state, and
// call an AnswerFunc for every input-required pause along the way.
//
// The expected supervisor usage is:
//
//	c, _ := client.NewFromEndpoint(ctx, agentURL)
//	resp, err := c.Send(ctx, prompt, func(ctx context.Context, q string) (string, error) {
//	    // Decide locally, escalate to a parent supervisor, or ask the
//	    // human — whatever the policy is at this layer.
//	    return supervisor.Answer(ctx, q)
//	})
//
// Multiple supervisors may be chained: an intermediate Tertiary can
// implement AnswerFunc by relaying the question to a Secondary's
// AskInput call, which in turn relays to the Primary. The protocol
// stays uniform end-to-end.
package client
