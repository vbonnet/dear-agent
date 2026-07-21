// Package statusread exposes validated, read-only canonical Wayfinder status
// fields to consumers outside the wayfinder-session command tree.
package statusread

import (
	"time"

	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/status"
)

// Summary contains the validated fields needed by read-only policy consumers.
type Summary struct {
	ProjectName     string
	Status          string
	CurrentWaypoint string
	Beads           []string
	UpdatedAt       time.Time
}

// Parse fully validates canonical status bytes and returns consumer fields.
func Parse(content []byte) (*Summary, error) {
	parsed, err := status.ParseV2Content(content)
	if err != nil {
		return nil, err
	}
	return summary(parsed), nil
}

// ParseFromDir reads and fully validates the canonical status file in dir.
func ParseFromDir(dir string) (*Summary, error) {
	parsed, err := status.ParseV2FromDir(dir)
	if err != nil {
		return nil, err
	}
	return summary(parsed), nil
}

func summary(parsed *status.StatusV2) *Summary {
	return &Summary{
		ProjectName:     parsed.ProjectName,
		Status:          parsed.Status,
		CurrentWaypoint: parsed.CurrentWaypoint,
		Beads:           append([]string(nil), parsed.Beads...),
		UpdatedAt:       parsed.UpdatedAt,
	}
}
