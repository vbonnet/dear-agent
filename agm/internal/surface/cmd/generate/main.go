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
			"ListSessionsRequest":           reflect.TypeFor[surface.ListSessionsRequest](),
			"GetSessionRequest":             reflect.TypeFor[surface.GetSessionRequest](),
			"GetSessionOutputRequest":       reflect.TypeFor[surface.GetSessionOutputRequest](),
			"SearchSessionsRequest":         reflect.TypeFor[surface.SearchSessionsRequest](),
			"GetStatusRequest":              reflect.TypeFor[surface.GetStatusRequest](),
			"ArchiveSessionRequest":         reflect.TypeFor[surface.ArchiveSessionRequest](),
			"KillSessionRequest":            reflect.TypeFor[surface.KillSessionRequest](),
			"ListOpsRequest":                reflect.TypeFor[surface.ListOpsRequest](),
			"GetCompletionRelayTargetInput": reflect.TypeFor[surface.GetCompletionRelayTargetInput](),
			"SetCompletionRelayTargetInput": reflect.TypeFor[surface.SetCompletionRelayTargetInput](),
			"QuotaStatusInput":              reflect.TypeFor[surface.QuotaStatusInput](),
		},
		OutDir:      "./internal/surface",
		Package:     "surface",
		CLIBinary:   "agm",
		BuildIgnore: true,
	}); err != nil {
		log.Fatal(err)
	}
}
