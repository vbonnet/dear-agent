//go:build !darwin

package circuitbreaker

// DefaultMemReader returns nil on non-Darwin platforms; the memory gate fails
// open when no reader is available.
func DefaultMemReader() MemReader {
	return nil
}
