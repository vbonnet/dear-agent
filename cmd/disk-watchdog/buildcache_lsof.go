package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// lsofTimeout bounds the liveness probe. A hung network mount can wedge lsof
// indefinitely, and a watchdog tick that never returns is a watchdog that has
// stopped watching.
const lsofTimeout = 60 * time.Second

// newLsofProber returns an in-use predicate backed by a single lsof snapshot.
//
// One snapshot per pass, not one per candidate: lsof is expensive, and the
// build-cache reaper runs on every five-minute tick. The snapshot is taken
// lazily so a pass that finds no candidates never shells out at all.
//
// A probe failure is returned to the caller rather than swallowed, because
// the caller's contract is to treat "could not prove idle" as busy.
func newLsofProber() func(string) (bool, error) {
	var (
		once  sync.Once
		paths []string
		err   error
	)
	return func(candidate string) (bool, error) {
		once.Do(func() { paths, err = listOpenPaths(context.Background()) })
		if err != nil {
			return false, err
		}
		prefix := filepath.Clean(candidate) + string(filepath.Separator)
		clean := filepath.Clean(candidate)
		for _, p := range paths {
			if p == clean || strings.HasPrefix(p, prefix) {
				return true, nil
			}
		}
		return false, nil
	}
}

// listOpenPaths returns every path any process holds as a cwd or open fd.
//
// lsof exits non-zero when it cannot stat some files, which is routine on a
// busy host, so a non-empty result is accepted despite a non-zero exit. Only
// an empty result alongside an error is treated as a real failure.
func listOpenPaths(parent context.Context) ([]string, error) {
	ctx, cancel := context.WithTimeout(parent, lsofTimeout)
	defer cancel()

	lsofBin := "lsof"
	if _, err := exec.LookPath("lsof"); err != nil {
		for _, fallback := range []string{"/usr/sbin/lsof", "/usr/bin/lsof"} {
			if _, statErr := os.Stat(fallback); statErr == nil {
				lsofBin = fallback
				break
			}
		}
	}

	out, err := exec.CommandContext(ctx, lsofBin, "-n", "-P", "-F", "n").Output()
	if ctxErr := ctx.Err(); ctxErr != nil {
		return nil, fmt.Errorf("lsof: %w", ctxErr)
	}
	if err != nil && len(out) == 0 {
		return nil, fmt.Errorf("lsof: %w", err)
	}

	var paths []string
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "n/") {
			paths = append(paths, filepath.Clean(strings.TrimPrefix(line, "n")))
		}
	}
	if serr := scanner.Err(); serr != nil {
		return nil, fmt.Errorf("reading lsof output: %w", serr)
	}
	return paths, nil
}
