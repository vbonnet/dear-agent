package checks

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/vbonnet/dear-agent/pkg/audit"
)

// GovulncheckCheck wraps `govulncheck -json ./...`. Each affected
// vulnerability becomes one Finding fingerprinted by (osv id,
// affected package). Severity is P1 — a CVE in a dependency we
// actually call is a quality regression, not a build break, but it
// should fail an audit run by default.
//
// We parse govulncheck's JSON stream and only emit findings for
// vulnerabilities the *traces* show we actually call ("affected"),
// not vulnerabilities present in the import graph but never reached.
// This matches govulncheck's own stance.
type GovulncheckCheck struct{}

// Meta returns the check's identity.
func (GovulncheckCheck) Meta() audit.CheckMeta {
	return audit.CheckMeta{
		ID:              "vuln.govulncheck",
		Description:     "govulncheck must report no called vulnerabilities",
		Cadence:         audit.CadenceDaily,
		SeverityCeiling: audit.SeverityP1,
		RequiresNetwork: true,
	}
}

// Run streams govulncheck output and emits one finding per called
// vulnerability.
func (GovulncheckCheck) Run(ctx context.Context, env audit.Env) (audit.Result, error) {
	res := runCommand(ctx, env.WorkingDir, "govulncheck", "-json", "./...")
	out := audit.Result{Status: audit.StatusOK, Stdout: res.Stdout, Stderr: res.Stderr}
	if res.Err != nil {
		out.Status = audit.StatusError
		return out, fmt.Errorf("audit/govulncheck: invoke: %w", res.Err)
	}

	vulns, err := parseGovulncheckOutput(res.Stdout)
	if err != nil {
		out.Status = audit.StatusError
		return out, fmt.Errorf("audit/govulncheck: parse: %w", err)
	}

	for _, v := range vulns {
		out.Findings = append(out.Findings, audit.Finding{
			Fingerprint: audit.Fingerprint("vuln.govulncheck", v.OSV, v.Package),
			Severity:    audit.SeverityP1,
			Title:       fmt.Sprintf("%s: %s in %s", v.OSV, v.Summary, v.Package),
			Detail:      v.Summary,
			Path:        v.Package,
			Suggested: audit.Remediation{
				Strategy: audit.StrategyAuto,
				Command:  fmt.Sprintf("go get -u %s && go mod tidy", v.Package),
			},
			Evidence: map[string]any{
				"osv":     v.OSV,
				"package": v.Package,
				"fixed":   v.Fixed,
			},
		})
	}
	return out, nil
}

// govulnFinding is the rolled-up shape per osv+package we emit. Built
// from parsing govulncheck's stream of typed JSON records.
type govulnFinding struct {
	OSV     string
	Package string
	Summary string
	Fixed   string
}

// govulnRecord matches a relevant subset of govulncheck's -json
// schema. The full schema has many more record kinds; we extract
// only what we need for fingerprinting.
type govulnRecord struct {
	OSV *struct {
		ID      string `json:"id"`
		Summary string `json:"summary"`
	} `json:"osv,omitempty"`
	Finding *struct {
		OSV   string `json:"osv"`
		Trace []struct {
			Module   string `json:"module"`
			Package  string `json:"package"`
			Function string `json:"function"`
		} `json:"trace,omitempty"`
		FixedVersion string `json:"fixed_version"`
	} `json:"finding,omitempty"`
}

// parseGovulncheckOutput streams the JSON records and returns one
// rolled-up finding per (osv, package) where the trace contains a
// call (i.e. Function != ""). govulncheck emits OSV records before
// the corresponding Finding records, so we collect summaries first.
func parseGovulncheckOutput(stdout string) ([]govulnFinding, error) {
	dec := json.NewDecoder(strings.NewReader(stdout))
	summaries := map[string]string{}
	type key struct{ osv, pkg string }
	out := map[key]govulnFinding{}

	for dec.More() {
		var rec govulnRecord
		if err := dec.Decode(&rec); err != nil {
			return nil, err
		}
		switch {
		case rec.OSV != nil:
			summaries[rec.OSV.ID] = rec.OSV.Summary
		case rec.Finding != nil && len(rec.Finding.Trace) > 0:
			// Only count if the trace has at least one called function —
			// govulncheck emits "imported" findings without function
			// info that we should not flag.
			called := false
			pkg := rec.Finding.Trace[0].Package
			for _, t := range rec.Finding.Trace {
				if t.Function != "" {
					called = true
					if t.Package != "" {
						pkg = t.Package
					}
					break
				}
			}
			if !called {
				continue
			}
			k := key{osv: rec.Finding.OSV, pkg: pkg}
			out[k] = govulnFinding{
				OSV:     rec.Finding.OSV,
				Package: pkg,
				Summary: summaries[rec.Finding.OSV],
				Fixed:   rec.Finding.FixedVersion,
			}
		}
	}

	res := make([]govulnFinding, 0, len(out))
	for _, v := range out {
		res = append(res, v)
	}
	return res, nil
}

func init() {
	audit.Default.MustRegister(GovulncheckCheck{})
}
