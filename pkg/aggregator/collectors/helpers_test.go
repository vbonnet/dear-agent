package collectors

import "os"

// writeFile is a tiny test helper used by collectors that accept a
// filesystem input path. Lives in helpers_test.go so it's compiled
// only with the test binary.
func writeFile(path string, data []byte) error {
	return os.WriteFile(path, data, 0o600)
}
