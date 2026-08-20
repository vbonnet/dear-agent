//go:build !linux

package hookparity

import "os"

// runningImageIdentity reports that this platform exposes no handle on the
// running image. Darwin resolves os.Executable through KERN_PROC_PATHNAME,
// which is a pathname and not the mapped inode, so there is nothing here that
// an atomic replacement could not defeat. Callers must treat the missing
// binding as a documented residual rather than as proof.
func runningImageIdentity() (os.FileInfo, bool, error) {
	return nil, false, nil
}
