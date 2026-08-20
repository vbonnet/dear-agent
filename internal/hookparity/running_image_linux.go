//go:build linux

package hookparity

import (
	"fmt"
	"os"
)

// runningImageIdentity returns the filesystem identity of the image this
// process actually executed. Linux keeps that binding in /proc/self/exe, which
// refers to the mapped inode rather than to a pathname, so it survives an
// atomic replacement of the file the process was launched from.
func runningImageIdentity() (os.FileInfo, bool, error) {
	info, err := os.Stat("/proc/self/exe")
	if err != nil {
		return nil, false, fmt.Errorf("inspect running image: %w", err)
	}
	return info, true, nil
}
