package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/vbonnet/dear-agent/agm/internal/pluginhash"
)

func main() {
	dir := flag.String("dir", "agm-plugin/commands", "directory containing plugin Markdown")
	check := flag.Bool("check", false, "verify hashes without writing")
	flag.Parse()
	if err := run(*dir, *check); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(dir string, check bool) error {
	paths, err := filepath.Glob(filepath.Join(dir, "*.md"))
	if err != nil {
		return fmt.Errorf("glob plugin Markdown: %w", err)
	}
	sort.Strings(paths)
	stale := 0
	for _, path := range paths {
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return fmt.Errorf("read %s: %w", path, readErr)
		}
		if !bytes.Contains(content, []byte("content-hash:")) {
			continue
		}
		stamped, stampErr := pluginhash.Stamp(content)
		if stampErr != nil {
			return fmt.Errorf("hash %s: %w", path, stampErr)
		}
		if bytes.Equal(content, stamped) {
			fmt.Printf("OK %s\n", path)
			continue
		}
		if check {
			fmt.Printf("STALE %s\n", path)
			stale++
			continue
		}
		// #nosec G703 -- every path comes from filepath.Glob within the explicit plugin directory.
		if writeErr := os.WriteFile(path, stamped, 0o600); writeErr != nil {
			return fmt.Errorf("write %s: %w", path, writeErr)
		}
		fmt.Printf("UPDATED %s\n", path)
	}
	if stale > 0 {
		return fmt.Errorf("%d plugin command hash(es) are stale", stale)
	}
	return nil
}
