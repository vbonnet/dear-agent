package collectors

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/vbonnet/dear-agent/pkg/aggregator"
)

// DepFreshness invokes `go list -u -m -json all` and emits one signal
// per outdated dependency, with Value=1 (presence) and Metadata
// recording the current and latest versions.
//
// We model "outdated" as a binary 1/0 rather than a "version distance"
// because semver distance is not meaningful across modules, and the
// recommendation engine cares about *count* of stale deps per repo.
type DepFreshness struct {
	Repo       string         // path to the Go module root
	Exec       Exec           // nil → DefaultExec
	LookPathFn func(string) (string, error) // nil → LookPath
}

// Name implements aggregator.Collector.
func (c *DepFreshness) Name() string { return "dear-agent.deps" }

// Kind implements aggregator.Collector.
func (c *DepFreshness) Kind() aggregator.Kind { return aggregator.KindDepFreshness }

// Collect runs `go list -u -m -json all`. Modules without an Update
// field are current; modules with one are stale.
func (c *DepFreshness) Collect(ctx context.Context) ([]aggregator.Signal, error) {
	if strings.TrimSpace(c.Repo) == "" {
		return nil, fmt.Errorf("collectors.DepFreshness: empty Repo")
	}
	lookPath := c.LookPathFn
	if lookPath == nil {
		lookPath = LookPath
	}
	if _, err := lookPath("go"); err != nil {
		return nil, &aggregator.ErrToolMissing{Collector: c.Name(), Tool: "go"}
	}
	execFn := c.Exec
	if execFn == nil {
		execFn = DefaultExec
	}
	out, err := execFn(ctx, c.Repo, "go", "list", "-u", "-m", "-json", "all")
	if err != nil {
		return nil, fmt.Errorf("collectors.DepFreshness: go list: %w", err)
	}
	return parseGoListUM(out)
}

type goListModule struct {
	Path    string `json:"Path"`
	Version string `json:"Version"`
	Update  *struct {
		Version string `json:"Version"`
	} `json:"Update,omitempty"`
	Main bool `json:"Main"`
}

// parseGoListUM streams `go list -m -json` output (a sequence of JSON
// objects, NOT a JSON array) and returns one signal per outdated module.
// The main module itself is always skipped — we never recommend
// "upgrade your own module".
func parseGoListUM(raw []byte) ([]aggregator.Signal, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	var sigs []aggregator.Signal
	for dec.More() {
		var m goListModule
		if err := dec.Decode(&m); err != nil {
			return nil, fmt.Errorf("collectors.DepFreshness: decode module: %w", err)
		}
		if m.Main || m.Update == nil || m.Path == "" {
			continue
		}
		md := map[string]any{
			"current": m.Version,
			"latest":  m.Update.Version,
		}
		mdJSON, _ := json.Marshal(md)
		sigs = append(sigs, aggregator.Signal{
			Kind:     aggregator.KindDepFreshness,
			Subject:  m.Path,
			Value:    1,
			Metadata: string(mdJSON),
		})
	}
	return sigs, nil
}
