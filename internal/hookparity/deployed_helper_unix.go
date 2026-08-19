//go:build unix

package hookparity

import (
	"fmt"
	"os"
	"syscall"
)

func fileOwnerUID(info os.FileInfo) (uint32, error) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, fmt.Errorf("unsupported stat metadata %T", info.Sys())
	}
	return stat.Uid, nil
}
