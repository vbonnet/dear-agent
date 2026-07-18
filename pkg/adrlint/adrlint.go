// Package adrlint validates the declared, Git-tracked architecture decision
// corpus as one repository contract.
package adrlint

import "sort"

// Violation is one deterministic ADR contract defect.
type Violation struct {
	Path   string
	Reason string
}

// Report summarizes governed records and content violations.
type Report struct {
	Records    int
	Violations []Violation
}

// Blocking reports whether ADR integrity failed.
func (r Report) Blocking() bool { return len(r.Violations) > 0 }

// CheckRepository validates the complete declared ADR corpus below root.
func CheckRepository(root string) (Report, error) {
	report, err := checkRepository(root)
	if err == nil {
		sort.Slice(report.Violations, func(i, j int) bool {
			if report.Violations[i].Path != report.Violations[j].Path {
				return report.Violations[i].Path < report.Violations[j].Path
			}
			return report.Violations[i].Reason < report.Violations[j].Reason
		})
	}
	return report, err
}
