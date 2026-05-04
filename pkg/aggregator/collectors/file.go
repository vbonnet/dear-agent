package collectors

import "os"

// readFile is the default ReadFile implementation; collectors expose
// the indirection via a struct field so tests can replace it without
// touching the filesystem.
func readFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}
