//go:build !darwin && !linux

package marketplaceparity

import "fmt"

type anchoredTreeEntry struct {
	Path      string
	Directory bool
	Data      []byte
}

func readAnchoredRegular(string, string, int64) ([]byte, error) {
	return nil, fmt.Errorf("marketplace projection validation is unsupported on this platform; requires descriptor-anchored filesystem support on darwin or linux")
}

func readAnchoredTree(string, string, int, int64) ([]anchoredTreeEntry, error) {
	return nil, fmt.Errorf("marketplace projection validation is unsupported on this platform; requires descriptor-anchored filesystem support on darwin or linux")
}
