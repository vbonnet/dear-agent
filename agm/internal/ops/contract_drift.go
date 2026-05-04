package ops

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/contracts"
)

// DriftSeverity indicates the severity of a drift finding.
type DriftSeverity string

// Drift severity values.
const (
	DriftPass DriftSeverity = "PASS"
	DriftWarn DriftSeverity = "WARN"
	DriftFail DriftSeverity = "FAIL"
)

// DriftFinding represents a single contract drift detection result.
type DriftFinding struct {
	SPECFile string        `json:"spec_file"`
	Section  string        `json:"section"`  // "SLO" or "Invariant"
	Metric   string        `json:"metric"`
	Expected string        `json:"expected"` // value from SPEC
	Actual   string        `json:"actual"`   // value from contracts
	Severity DriftSeverity `json:"severity"`
	Detail   string        `json:"detail"`
}

// ContractDriftRequest is the input for the contract drift check.
type ContractDriftRequest struct {
	SpecsDir      string `json:"specs_dir"`
	ContractsFile string `json:"contracts_file,omitempty"` // empty = use default
}

// ContractDriftResult is the output of the contract drift check.
type ContractDriftResult struct {
	Findings      []DriftFinding `json:"findings"`
	TotalSpecs    int            `json:"total_specs"`
	PassCount     int            `json:"pass_count"`
	WarnCount     int            `json:"warn_count"`
	FailCount     int            `json:"fail_count"`
	OverallStatus DriftSeverity  `json:"overall_status"`
}

// MarshalJSON implements json.Marshaler so the result serialises cleanly.
func (r *ContractDriftResult) MarshalJSON() ([]byte, error) {
	type Alias ContractDriftResult
	return json.Marshal((*Alias)(r))
}

// specSLORow is a parsed row from a SPEC's SLOs table.
type specSLORow struct {
	Metric string
	Target string
	Source string
}

// specSection maps a SPEC filename to its contract section name.
var specSectionMap = map[string]string{
	"SPEC-session-lifecycle.md": "session_lifecycle",
	"SPEC-trust-protocol.md":   "trust_protocol",
	"SPEC-scan-loop.md":        "scan_loop",
	"SPEC-stall-detection.md":  "stall_detection",
	"SPEC-audit-trail.md":      "audit_trail",
}

// sloFieldMap maps (section, normalised metric keyword) → contract field getter.
// The getter returns the contract value as a string for comparison.
type contractGetter func(c *contracts.SLOContracts) string

var sloFieldMap = map[string]map[string]contractGetter{
	"session_lifecycle": {
		"resume":       func(c *contracts.SLOContracts) string { return fmtDur(c.SessionLifecycle.ResumeReadyTimeout.Duration) },
		"bloat_size":   func(c *contracts.SLOContracts) string { return fmtBytes(c.SessionLifecycle.BloatSizeThresholdBytes) },
		"bloat_prog":   func(c *contracts.SLOContracts) string { return strconv.Itoa(c.SessionLifecycle.BloatProgressEntryThreshold) },
		"scan_limit":   func(c *contracts.SLOContracts) string { return strconv.Itoa(c.SessionLifecycle.SessionScanLimit) },
		"kill_grace":   func(c *contracts.SLOContracts) string { return fmtDur(c.SessionLifecycle.ProcessKillGracePeriod.Duration) },
	},
	"trust_protocol": {
		"base_score": func(c *contracts.SLOContracts) string { return strconv.Itoa(c.TrustProtocol.BaseScore) },
		"min_score":  func(c *contracts.SLOContracts) string { return strconv.Itoa(c.TrustProtocol.MinScore) },
		"max_score":  func(c *contracts.SLOContracts) string { return strconv.Itoa(c.TrustProtocol.MaxScore) },
		"delta":      func(c *contracts.SLOContracts) string { return fmtDeltaRange(c.TrustProtocol.EventDeltas) },
	},
	"scan_loop": {
		"scan_interval":    func(c *contracts.SLOContracts) string { return fmtDur(c.ScanLoop.DefaultScanInterval.Duration) },
		"stuck":            func(c *contracts.SLOContracts) string { return fmtDur(c.ScanLoop.StuckTimeout.Duration) },
		"scan_gap":         func(c *contracts.SLOContracts) string { return fmtDur(c.ScanLoop.ScanGapTimeout.Duration) },
		"commit_lookback":  func(c *contracts.SLOContracts) string { return fmtDur(c.ScanLoop.WorkerCommitLookback.Duration) },
		"metrics_window":   func(c *contracts.SLOContracts) string { return fmtDur(c.ScanLoop.MetricsWindow.Duration) },
		"capture_depth":    func(c *contracts.SLOContracts) string { return strconv.Itoa(c.ScanLoop.TmuxCaptureDepth) },
		"list_limit":       func(c *contracts.SLOContracts) string { return strconv.Itoa(c.ScanLoop.SessionListLimit) },
	},
	"stall_detection": {
		"permission":       func(c *contracts.SLOContracts) string { return fmtDur(c.StallDetection.PermissionTimeout.Duration) },
		"no_commit":        func(c *contracts.SLOContracts) string { return fmtDur(c.StallDetection.NoCommitTimeout.Duration) },
		"error_repeat":     func(c *contracts.SLOContracts) string { return strconv.Itoa(c.StallDetection.ErrorRepeatThreshold) },
		"capture_depth":    func(c *contracts.SLOContracts) string { return strconv.Itoa(c.StallDetection.TmuxCaptureDepth) },
		"error_msg_len":    func(c *contracts.SLOContracts) string { return strconv.Itoa(c.StallDetection.ErrorMessageMaxLength) },
		"scan_limit":       func(c *contracts.SLOContracts) string { return strconv.Itoa(c.StallDetection.SessionScanLimit) },
	},
	"audit_trail": {
		"line_buffer": func(c *contracts.SLOContracts) string { return fmtBytes(int64(c.AuditTrail.MaxLineBufferBytes)) },
		"dir_perm":    func(c *contracts.SLOContracts) string { return fmt.Sprintf("%04o", c.AuditTrail.LogDirectoryPermissions) },
		"file_perm":   func(c *contracts.SLOContracts) string { return fmt.Sprintf("%04o", c.AuditTrail.LogFilePermissions) },
	},
}

// metricKeywordMap maps metric text patterns (from SPEC SLO tables) to the
// lookup keys used in sloFieldMap. Multiple keywords can match the same field.
var metricKeywordMap = map[string][]string{
	"resume":          {"resume", "ready wait", "ready timeout"},
	"bloat_size":      {"bloat", "file size", "size threshold"},
	"bloat_prog":      {"progress entry", "entry threshold"},
	"scan_limit":      {"session scan limit", "gc session scan"},
	"kill_grace":      {"grace period", "kill grace"},
	"base_score":      {"base", "base score", "base trust"},
	"min_score":       {"min score", "min trust", "hard floor"},
	"max_score":       {"max score", "max trust", "hard ceiling"},
	"delta":           {"delta range", "score delta"},
	"scan_interval":   {"scan interval", "default scan"},
	"stuck":           {"stuck timeout", "stuck"},
	"scan_gap":        {"scan gap"},
	"commit_lookback": {"commit lookback", "worker commit"},
	"metrics_window":  {"metrics window"},
	"capture_depth":   {"capture depth", "tmux capture"},
	"list_limit":      {"list limit", "session list"},
	"permission":      {"permission prompt", "permission timeout"},
	"no_commit":       {"no-commit", "no commit", "no_commit"},
	"error_repeat":    {"error repeat", "error threshold", "repeat threshold"},
	"error_msg_len":   {"error message", "max length"},
	"line_buffer":     {"line buffer", "max line"},
	"dir_perm":        {"directory perm"},
	"file_perm":       {"file perm"},
}

// ContractDrift runs contract drift detection across all SPECs.
func ContractDrift(_ *OpContext, req *ContractDriftRequest) (*ContractDriftResult, error) {
	if req.SpecsDir == "" {
		return nil, fmt.Errorf("specs_dir is required")
	}

	// Load contracts
	var slo *contracts.SLOContracts
	var err error
	if req.ContractsFile != "" {
		slo, err = contracts.LoadFromFile(req.ContractsFile)
		if err != nil {
			return nil, fmt.Errorf("load contracts: %w", err)
		}
	} else {
		slo = contracts.Load()
	}

	// Discover SPEC files
	entries, err := os.ReadDir(req.SpecsDir)
	if err != nil {
		return nil, fmt.Errorf("read specs dir: %w", err)
	}

	result := &ContractDriftResult{
		OverallStatus: DriftPass,
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), "SPEC-") || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		result.TotalSpecs++

		data, err := os.ReadFile(filepath.Join(req.SpecsDir, entry.Name()))
		if err != nil {
			result.Findings = append(result.Findings, DriftFinding{
				SPECFile: entry.Name(),
				Section:  "file",
				Metric:   "readable",
				Severity: DriftFail,
				Detail:   fmt.Sprintf("cannot read SPEC: %v", err),
			})
			result.FailCount++
			continue
		}

		content := string(data)
		section, ok := specSectionMap[entry.Name()]
		if !ok {
			result.Findings = append(result.Findings, DriftFinding{
				SPECFile: entry.Name(),
				Section:  "mapping",
				Metric:   "section_mapping",
				Severity: DriftWarn,
				Detail:   "SPEC file has no known contract section mapping",
			})
			result.WarnCount++
			continue
		}

		// Check SLO drift
		sloFindings := checkSLODrift(entry.Name(), section, content, slo)
		result.Findings = append(result.Findings, sloFindings...)

		// Check invariant coverage
		invFindings := checkInvariantCoverage(entry.Name(), content)
		result.Findings = append(result.Findings, invFindings...)
	}

	// Tally
	for _, f := range result.Findings {
		switch f.Severity {
		case DriftPass:
			result.PassCount++
		case DriftWarn:
			result.WarnCount++
		case DriftFail:
			result.FailCount++
		}
	}

	if result.FailCount > 0 {
		result.OverallStatus = DriftFail
	} else if result.WarnCount > 0 {
		result.OverallStatus = DriftWarn
	}

	return result, nil
}

// checkSLODrift parses the SLO table from a SPEC and compares values against contracts.
func checkSLODrift(specFile, section, content string, slo *contracts.SLOContracts) []DriftFinding {
	rows := parseSLOTable(content)
	if len(rows) == 0 {
		return []DriftFinding{{
			SPECFile: specFile,
			Section:  "SLO",
			Metric:   "table_present",
			Severity: DriftWarn,
			Detail:   "no SLO table found in SPEC",
		}}
	}

	fieldGetters, ok := sloFieldMap[section]
	if !ok {
		return nil
	}

	var findings []DriftFinding
	for _, row := range rows {
		fieldKey := matchMetricToField(row.Metric)
		if fieldKey == "" {
			findings = append(findings, DriftFinding{
				SPECFile: specFile,
				Section:  "SLO",
				Metric:   row.Metric,
				Severity: DriftWarn,
				Detail:   "SLO metric not mapped to any contract field",
			})
			continue
		}

		getter, ok := fieldGetters[fieldKey]
		if !ok {
			// Metric maps to a field in a different section — skip
			continue
		}

		contractVal := getter(slo)
		specVal := normalizeValue(row.Target)
		contractNorm := normalizeValue(contractVal)

		if specVal == contractNorm {
			findings = append(findings, DriftFinding{
				SPECFile: specFile,
				Section:  "SLO",
				Metric:   row.Metric,
				Expected: row.Target,
				Actual:   contractVal,
				Severity: DriftPass,
				Detail:   "SPEC matches contract",
			})
		} else {
			findings = append(findings, DriftFinding{
				SPECFile: specFile,
				Section:  "SLO",
				Metric:   row.Metric,
				Expected: row.Target,
				Actual:   contractVal,
				Severity: DriftFail,
				Detail:   fmt.Sprintf("SPEC says %q but contract has %q", row.Target, contractVal),
			})
		}
	}

	return findings
}

// checkInvariantCoverage extracts invariants and checks they are parseable.
func checkInvariantCoverage(specFile, content string) []DriftFinding {
	invariants := parseInvariants(content)
	if len(invariants) == 0 {
		return []DriftFinding{{
			SPECFile: specFile,
			Section:  "Invariant",
			Metric:   "section_present",
			Severity: DriftWarn,
			Detail:   "no Invariants section found in SPEC",
		}}
	}

	var findings []DriftFinding
	for i, inv := range invariants {
		findings = append(findings, DriftFinding{
			SPECFile: specFile,
			Section:  "Invariant",
			Metric:   fmt.Sprintf("invariant_%d", i+1),
			Expected: inv,
			Severity: DriftPass,
			Detail:   "invariant parsed successfully",
		})
	}
	return findings
}

// parseSLOTable extracts rows from a markdown SLO table.
// Expected format: | Metric | Target | Source |
var tableRowRe = regexp.MustCompile(`^\|\s*(.+?)\s*\|\s*(.+?)\s*\|\s*(.+?)\s*\|$`)
var tableSepRe = regexp.MustCompile(`^\|[-\s|]+\|$`)

func parseSLOTable(content string) []specSLORow {
	lines := strings.Split(content, "\n")
	inSLOSection := false
	inTable := false
	headerSeen := false

	var rows []specSLORow

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Detect SLO section
		if strings.HasPrefix(trimmed, "## SLOs") || strings.HasPrefix(trimmed, "## SLO") {
			inSLOSection = true
			inTable = false
			headerSeen = false
			continue
		}

		// End SLO section at next ## heading
		if inSLOSection && strings.HasPrefix(trimmed, "## ") {
			break
		}

		if !inSLOSection {
			continue
		}

		// Skip separator lines
		if tableSepRe.MatchString(trimmed) {
			continue
		}

		// Parse table rows
		m := tableRowRe.FindStringSubmatch(trimmed)
		if m == nil {
			if inTable {
				// Table ended
				break
			}
			continue
		}

		if !headerSeen {
			// First row is the header
			headerSeen = true
			inTable = true
			continue
		}

		rows = append(rows, specSLORow{
			Metric: strings.TrimSpace(m[1]),
			Target: strings.TrimSpace(m[2]),
			Source: strings.TrimSpace(m[3]),
		})
	}

	return rows
}

// parseInvariants extracts numbered invariants from the Invariants section.
var invariantRe = regexp.MustCompile(`^\d+\.\s+\*\*(.+?)\*\*`)

func parseInvariants(content string) []string {
	lines := strings.Split(content, "\n")
	inInvSection := false

	var invariants []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "## Invariants") {
			inInvSection = true
			continue
		}

		if inInvSection && strings.HasPrefix(trimmed, "## ") {
			break
		}

		if !inInvSection {
			continue
		}

		m := invariantRe.FindStringSubmatch(trimmed)
		if m != nil {
			invariants = append(invariants, m[1])
		}
	}

	return invariants
}

// matchMetricToField maps a SPEC SLO metric name to a field key using keyword matching.
func matchMetricToField(metric string) string {
	lower := strings.ToLower(metric)

	// Try each field's keywords; pick the one with the longest matching keyword
	bestKey := ""
	bestLen := 0

	for fieldKey, keywords := range metricKeywordMap {
		for _, kw := range keywords {
			if strings.Contains(lower, kw) && len(kw) > bestLen {
				bestKey = fieldKey
				bestLen = len(kw)
			}
		}
	}

	return bestKey
}

// normalizeValue extracts the core numeric/duration value from a SPEC target string.
// e.g. "5s max" → "5s", "100MB file size" → "100mb", "1000 entries" → "1000"
func normalizeValue(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ToLower(s)

	// Try parsing as pure integer
	if _, err := strconv.Atoi(s); err == nil {
		return s
	}

	// Try parsing as duration
	if d, err := time.ParseDuration(s); err == nil {
		return fmtDur(d)
	}

	// Extract leading number + optional unit
	re := regexp.MustCompile(`^(\d+(?:\.\d+)?)\s*(ms|mb|kb|gb|min|s|m|h|entries|lines|occurrences|chars|per pass|per scan)?`)
	m := re.FindStringSubmatch(s)
	if m != nil {
		num := m[1]
		unit := strings.TrimSpace(m[2])

		switch unit {
		case "min":
			if d, err := time.ParseDuration(num + "m"); err == nil {
				return fmtDur(d)
			}
		case "s", "ms", "m", "h":
			if d, err := time.ParseDuration(num + unit); err == nil {
				return fmtDur(d)
			}
		case "mb":
			if n, err := strconv.ParseInt(num, 10, 64); err == nil {
				return fmtBytes(n * 1024 * 1024)
			}
		case "kb":
			if n, err := strconv.ParseInt(num, 10, 64); err == nil {
				return fmtBytes(n * 1024)
			}
		case "gb":
			if n, err := strconv.ParseInt(num, 10, 64); err == nil {
				return fmtBytes(n * 1024 * 1024 * 1024)
			}
		default:
			return num
		}
	}

	// Handle permission values like "0755"
	if permRe := regexp.MustCompile(`^0[0-7]{3}$`); permRe.MatchString(s) {
		return s
	}

	// Handle ranges like "-15 to +5"
	if rangeRe := regexp.MustCompile(`^-?\d+\s+to\s+[+-]?\d+$`); rangeRe.MatchString(s) {
		return strings.ReplaceAll(s, " ", "")
	}

	return s
}

// fmtDur formats a duration to its canonical short form.
func fmtDur(d time.Duration) string {
	if d >= time.Hour {
		h := int(d.Hours())
		if time.Duration(h)*time.Hour == d {
			return fmt.Sprintf("%dh", h)
		}
	}
	if d >= time.Minute {
		m := int(d.Minutes())
		if time.Duration(m)*time.Minute == d {
			return fmt.Sprintf("%dm", m)
		}
	}
	if d >= time.Second {
		s := int(d.Seconds())
		if time.Duration(s)*time.Second == d {
			return fmt.Sprintf("%ds", s)
		}
	}
	return d.String()
}

// fmtBytes formats a byte count to a human-readable string.
func fmtBytes(b int64) string {
	switch {
	case b >= 1024*1024*1024 && b%(1024*1024*1024) == 0:
		return fmt.Sprintf("%dgb", b/(1024*1024*1024))
	case b >= 1024*1024 && b%(1024*1024) == 0:
		return fmt.Sprintf("%dmb", b/(1024*1024))
	case b >= 1024 && b%1024 == 0:
		return fmt.Sprintf("%dkb", b/1024)
	default:
		return strconv.FormatInt(b, 10)
	}
}

// fmtDeltaRange computes the min and max event deltas.
func fmtDeltaRange(deltas map[string]int) string {
	if len(deltas) == 0 {
		return "0to0"
	}
	minD, maxD := 0, 0
	first := true
	for _, d := range deltas {
		if first || d < minD {
			minD = d
		}
		if first || d > maxD {
			maxD = d
		}
		first = false
	}
	return fmt.Sprintf("%dto+%d", minD, maxD)
}
