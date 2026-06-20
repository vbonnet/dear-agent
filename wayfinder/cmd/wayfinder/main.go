package main

import (
	"context"
	"fmt"
	"os"

	"github.com/vbonnet/dear-agent/pkg/otelsetup"
	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder/cmd"
)

func main() { os.Exit(realMain()) }

// realMain runs the CLI and returns the process exit code. It is split from
// main so the OTel shutdown (span flush) runs via defer before os.Exit — a
// bare os.Exit would skip deferred flushes and drop the last spans.
//
// Tracing is opt-in: a no-op unless OTEL_EXPORTER_OTLP_ENDPOINT is set (run
// `otel-local up` and `eval "$(otel-local env)"` to collect phase-transition
// spans).
func realMain() int {
	shutdown := otelsetup.InitTracer("wayfinder")
	defer func() {
		if err := shutdown(context.Background()); err != nil {
			fmt.Fprintf(os.Stderr, "wayfinder: otel shutdown: %v\n", err)
		}
	}()

	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}
