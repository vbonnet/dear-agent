// Package monitoring provides sub-agent monitoring and validation infrastructure.
//
// It coordinates file watching, git hook integration, output parsing, and
// validation scoring for sub-agent sessions. Validation uses configurable
// thresholds (file count, line count, commits, test runs, stub keywords)
// to produce a pass/fail score for agent work products.
package monitoring
