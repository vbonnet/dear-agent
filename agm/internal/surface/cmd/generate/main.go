// Command generate produces CLI, MCP, and Skill surface code from AGM
// operation definitions. Run with:
//
//	go run ./internal/surface/cmd/generate/
package main

import (
	"log"
	"reflect"

	"github.com/vbonnet/dear-agent/agm/internal/surface"
	"github.com/vbonnet/dear-agent/pkg/codegen"
)

func main() {
	if err := codegen.Generate(codegen.GenerateConfig{
		Ops: surface.Registry,
		RequestTypes: map[string]reflect.Type{
			"ListSessionsRequest":   reflect.TypeOf(surface.ListSessionsRequest{}),
			"GetSessionRequest":     reflect.TypeOf(surface.GetSessionRequest{}),
			"SearchSessionsRequest": reflect.TypeOf(surface.SearchSessionsRequest{}),
			"GetStatusRequest":      reflect.TypeOf(surface.GetStatusRequest{}),
			"ArchiveSessionRequest": reflect.TypeOf(surface.ArchiveSessionRequest{}),
			"KillSessionRequest":    reflect.TypeOf(surface.KillSessionRequest{}),
			"ListOpsRequest":        reflect.TypeOf(surface.ListOpsRequest{}),
		},
		OutDir:      "./internal/surface",
		Package:     "surface",
		CLIBinary:   "agm",
		BuildIgnore: true,
	}); err != nil {
		log.Fatal(err)
	}
}
