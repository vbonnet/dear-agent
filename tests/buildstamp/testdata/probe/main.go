package main

import (
	"encoding/json"
	"os"

	"github.com/vbonnet/dear-agent/pkg/version"
)

var extra = "unset"

type buildStamp struct {
	Version       string `json:"version"`
	GitCommit     string `json:"git_commit"`
	BuildDate     string `json:"build_date"`
	BuiltBy       string `json:"built_by"`
	Extra         string `json:"extra"`
	GOFLAGSMarker string `json:"goflags_marker"`
}

func main() {
	_ = json.NewEncoder(os.Stdout).Encode(buildStamp{
		Version:       version.Version,
		GitCommit:     version.GitCommit,
		BuildDate:     version.BuildDate,
		BuiltBy:       version.BuiltBy,
		Extra:         extra,
		GOFLAGSMarker: goflagsMarker,
	})
}
