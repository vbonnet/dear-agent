//go:build !unix

package hookparity

import (
	"fmt"
	"os"
)

func fileOwnerUID(os.FileInfo) (uint32, error) {
	return 0, fmt.Errorf("deployed helper ownership inspection requires Unix")
}
