//go:build !darwin

package circuitbreaker

// DefaultLoadReader returns the platform-native load reader. On non-Darwin
// platforms this is the /proc/loadavg reader.
func DefaultLoadReader() LoadReader {
	return ProcLoadReader{}
}
