//go:build !linux && !darwin

package supervisor

import (
	"context"
	"errors"
	"fmt"
	"runtime"
)

var errSysResourceProbeUnsupported = errors.New("system resource probe is unsupported")

// Snapshot refuses to publish incomplete resource evidence on unsupported platforms.
func (p *SysResourceProbe) Snapshot(_ context.Context) (ResourceSnapshot, error) {
	return ResourceSnapshot{}, fmt.Errorf("%w on %s", errSysResourceProbeUnsupported, runtime.GOOS)
}
