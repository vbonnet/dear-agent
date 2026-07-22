package circuitbreaker

import (
	"github.com/vbonnet/dear-agent/pkg/vroom/admission"
)

// BrakeReader reports the admission brake currently in force, if any. A nil
// brake with a nil error means no brake is engaged; an error means the latch
// could not be read, which the gate treats as engaged.
type BrakeReader interface {
	Brake() (*admission.Brake, error)
}

// brakePathNamer is an optional interface a BrakeReader may implement so the
// refusal message can tell an operator exactly which file to delete. Keeping it
// separate from BrakeReader means test fakes only have to implement one method.
type brakePathNamer interface {
	BrakePath() string
}

// FileBrakeReader reads the admission brake from a file written by the host
// watchdogs (disk-watchdog, vroom-governor).
type FileBrakeReader struct {
	// Path is the brake file. Empty means admission.DefaultPath().
	Path string
}

// BrakePath returns the resolved brake file location.
func (f FileBrakeReader) BrakePath() string {
	if f.Path != "" {
		return f.Path
	}
	return admission.DefaultPath()
}

// Brake reads the live brake, or (nil, nil) when none is in force.
func (f FileBrakeReader) Brake() (*admission.Brake, error) {
	return admission.Read(f.BrakePath())
}

// DefaultBrakeReader returns a FileBrakeReader at the canonical brake path.
func DefaultBrakeReader() BrakeReader {
	return FileBrakeReader{}
}
