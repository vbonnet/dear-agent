package main

import (
	"context"

	"github.com/vbonnet/dear-agent/engram/cmd/engram/cmd"
	"github.com/vbonnet/dear-agent/pkg/otelsetup"
)

func main() {
	shutdown := otelsetup.InitTracer("engram")
	defer shutdown(context.Background()) //nolint:errcheck

	cmd.Execute()
}
