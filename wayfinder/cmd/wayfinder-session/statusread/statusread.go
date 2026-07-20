// Package statusread exposes validated, read-only canonical Wayfinder status
// fields to consumers outside the wayfinder-session command tree.
package statusread

import "github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/status"

// Summary contains the lifecycle fields needed by read-only policy hooks.
type Summary struct {
	Status          string
	CurrentWaypoint string
}

// ParseFromDir reads and fully validates the canonical status file in dir.
func ParseFromDir(dir string) (*Summary, error) {
	parsed, err := status.ParseV2FromDir(dir)
	if err != nil {
		return nil, err
	}
	return &Summary{
		Status:          parsed.Status,
		CurrentWaypoint: parsed.CurrentWaypoint,
	}, nil
}
