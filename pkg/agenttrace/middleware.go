package agenttrace

import (
	"context"
	"fmt"
)

// InstrumentToolCall runs fn as an instrumented tool call. It opens a
// gen_ai.tool.call span for toolName, records the serialised arguments and the
// returned output, and closes the span with fn's error — all in one call.
//
// This is the middleware form of StartToolCall for the common case of a single
// synchronous attempt; reach for StartToolCall directly when you need to record
// retries or a provider-assigned call id across multiple attempts.
//
// The span context is threaded into fn so any nested spans (e.g. a memory read
// the tool performs) parent correctly. If fn panics the span is still closed
// (tagged with the panic) and the panic is re-raised, so instrumentation never
// swallows a panic or leaks a span.
func InstrumentToolCall(ctx context.Context, toolName, args string, fn func(context.Context) (string, error)) (out string, err error) {
	ctx, span := StartToolCall(ctx, toolName)
	span.SetArguments(args)
	defer func() {
		span.SetOutput(out)
		if r := recover(); r != nil {
			span.End(fmt.Errorf("panic in tool %q: %v", toolName, r))
			panic(r)
		}
		span.End(err)
	}()
	out, err = fn(ctx)
	return out, err
}

// InstrumentReasoning runs fn as an instrumented reasoning step. It opens a
// gen_ai.reasoning span labelled step, hands the caller the span so it can fill
// in plan/action/observation/next-decision as the step unfolds, and closes the
// span with fn's error. A panic in fn closes the span and is re-raised.
func InstrumentReasoning(ctx context.Context, step string, fn func(context.Context, *ReasoningSpan) error) (err error) {
	ctx, span := StartReasoning(ctx, step)
	defer func() {
		if r := recover(); r != nil {
			span.End(fmt.Errorf("panic in reasoning %q: %v", step, r))
			panic(r)
		}
		span.End(err)
	}()
	err = fn(ctx, span)
	return err
}

// InstrumentMemoryOp runs fn as an instrumented memory operation against store.
// It opens a gen_ai.memory span, hands the caller the span so it can record the
// query, relevance score, and freshness, and closes the span with fn's error.
// A panic in fn closes the span and is re-raised.
func InstrumentMemoryOp(ctx context.Context, op MemoryOp, store string, fn func(context.Context, *MemoryOpSpan) error) (err error) {
	ctx, span := StartMemoryOp(ctx, op, store)
	defer func() {
		if r := recover(); r != nil {
			span.End(fmt.Errorf("panic in memory op %q on %q: %v", op, store, r))
			panic(r)
		}
		span.End(err)
	}()
	err = fn(ctx, span)
	return err
}
