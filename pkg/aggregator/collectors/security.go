package collectors

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/vbonnet/dear-agent/pkg/aggregator"
)

// SecurityAlerts runs `govulncheck -json ./...` (or reads a precomputed
// JSON file) and emits one signal per distinct vulnerability ID, with
// Value=1 and Metadata recording the affected packages.
type SecurityAlerts struct {
	Repo       string         // path to the Go module root
	InputFile  string         // optional precomputed JSON output
	Exec       Exec           // nil → DefaultExec
	LookPathFn func(string) (string, error) // nil → LookPath
}

// Name implements aggregator.Collector.
func (c *SecurityAlerts) Name() string { return "dear-agent.security" }

// Kind implements aggregator.Collector.
func (c *SecurityAlerts) Kind() aggregator.Kind { return aggregator.KindSecurityAlerts }

// Collect parses govulncheck NDJSON. We extract the "finding" message
// kind and group by OSV ID. Govulncheck emits informational lines too;
// those are ignored.
func (c *SecurityAlerts) Collect(ctx context.Context) ([]aggregator.Signal, error) {
	raw, err := c.readJSON(ctx)
	if err != nil {
		return nil, err
	}
	return parseGovulncheck(raw)
}

func (c *SecurityAlerts) readJSON(ctx context.Context) ([]byte, error) {
	if c.InputFile != "" {
		return readFile(c.InputFile)
	}
	if strings.TrimSpace(c.Repo) == "" {
		return nil, fmt.Errorf("collectors.SecurityAlerts: empty Repo and no InputFile")
	}
	lookPath := c.LookPathFn
	if lookPath == nil {
		lookPath = LookPath
	}
	if _, err := lookPath("govulncheck"); err != nil {
		return nil, &aggregator.ErrToolMissing{Collector: c.Name(), Tool: "govulncheck"}
	}
	execFn := c.Exec
	if execFn == nil {
		execFn = DefaultExec
	}
	out, err := execFn(ctx, c.Repo, "govulncheck", "-json", "./...")
	if err != nil {
		// govulncheck exits 3 when it finds vulns; that's expected.
		var ee *exec.ExitError
		if !errors.As(err, &ee) && len(out) == 0 {
			return nil, fmt.Errorf("collectors.SecurityAlerts: %w", err)
		}
	}
	return out, nil
}

// govulncheckEntry models the subset of govulncheck NDJSON we read.
// govulncheck v2 emits OSV documents and "finding" entries; we group
// findings by OSV.id.
type govulncheckEntry struct {
	OSV *struct {
		ID      string `json:"id"`
		Summary string `json:"summary"`
	} `json:"osv,omitempty"`
	Finding *struct {
		OSV   string `json:"osv"`
		Trace []struct {
			Module  string `json:"module"`
			Package string `json:"package"`
		} `json:"trace,omitempty"`
	} `json:"finding,omitempty"`
}

// parseGovulncheck reads a stream of JSON objects (one per line, the
// govulncheck v2 output shape) and returns one signal per OSV ID seen
// in a "finding" entry.
func parseGovulncheck(raw []byte) ([]aggregator.Signal, error) {
	type vulnInfo struct {
		summary  string
		packages map[string]struct{}
	}
	known := map[string]*vulnInfo{}
	osvSummary := map[string]string{}

	dec := json.NewDecoder(strings.NewReader(string(raw)))
	for dec.More() {
		var e govulncheckEntry
		if err := dec.Decode(&e); err != nil {
			return nil, fmt.Errorf("collectors.SecurityAlerts: decode entry: %w", err)
		}
		if e.OSV != nil && e.OSV.ID != "" {
			osvSummary[e.OSV.ID] = e.OSV.Summary
		}
		if e.Finding != nil && e.Finding.OSV != "" {
			info, ok := known[e.Finding.OSV]
			if !ok {
				info = &vulnInfo{packages: map[string]struct{}{}}
				known[e.Finding.OSV] = info
			}
			for _, t := range e.Finding.Trace {
				if t.Package != "" {
					info.packages[t.Package] = struct{}{}
				}
			}
		}
	}

	sigs := make([]aggregator.Signal, 0, len(known))
	for id, info := range known {
		info.summary = osvSummary[id]
		pkgs := make([]string, 0, len(info.packages))
		for p := range info.packages {
			pkgs = append(pkgs, p)
		}
		md := map[string]any{
			"summary":  info.summary,
			"packages": pkgs,
		}
		mdJSON, _ := json.Marshal(md)
		sigs = append(sigs, aggregator.Signal{
			Kind:     aggregator.KindSecurityAlerts,
			Subject:  id,
			Value:    1,
			Metadata: string(mdJSON),
		})
	}
	return sigs, nil
}
