package agenttrace

import "context"

// InstrumentToolCall runs fn as an instrumented tool call. It opens a
// gen_ai.tool.call span for toolName, records the serialised arguments and the
// returned output, and closes the span with fn's error — all in one call.
//
// This is the middleware form of StartToolCall for the common case of a single
// synchronous attempt; reach for StartToolCall directly when you need to record
// retries or a provider-assigned call id across multiple attempts.
//
// The span context is threaded into fn so any nested spans (e.g. a memory read
// the tool performs) parent correctly.
func InstrumentToolCall(ctx context.Context, toolName, args string, fn func(context.Context) (string, error)) (string, error) {
	ctx, span := StartToolCall(ctx, toolName)
	span.SetArguments(args)
	out, err := fn(ctx)
	span.SetOutput(out)
	span.End(err)
	return out, err
}

// InstrumentReasoning runs fn as an instrumented reasoning step. It opens a
// gen_ai.reasoning span labelled step, hands the caller the span so it can fill
// in plan/action/observation/next-decision as the step unfolds, and closes the
// span with fn's error.
func InstrumentReasoning(ctx context.Context, step string, fn func(context.Context, *ReasoningSpan) error) error {
	ctx, span := StartReasoning(ctx, step)
	err := fn(ctx, span)
	span.End(err)
	return err
}

// InstrumentMemoryOp runs fn as an instrumented memory operation against store.
// It opens a gen_ai.memory span, hands the caller the span so it can record the
// query, relevance score, and freshness, and closes the span with fn's error.
func InstrumentMemoryOp(ctx context.Context, op MemoryOp, store string, fn func(context.Context, *MemoryOpSpan) error) error {
	ctx, span := StartMemoryOp(ctx, op, store)
	err := fn(ctx, span)
	span.End(err)
	return err
}
