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
	dir := flag.String("dir", "", "single plugin Markdown directory (deprecated; use positional paths)")
	check := flag.Bool("check", false, "verify hashes without writing")
	flag.Parse()
	targets := flag.Args()
	if *dir != "" {
		if len(targets) != 0 {
			fmt.Fprintln(os.Stderr, "--dir cannot be combined with positional paths")
			os.Exit(2)
		}
		targets = []string{*dir}
	}
	if len(targets) == 0 {
		targets = []string{"agm-plugin/commands", "agm-plugin/skills", "plugins/agm"}
	}
	if err := runTargets(targets, *check); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(dir string, check bool) error {
	return runTargets([]string{dir}, check)
}

func runTargets(targets []string, check bool) error {
	var paths []string
	for _, target := range targets {
		info, err := os.Stat(target)
		if err != nil {
			return fmt.Errorf("stat plugin Markdown target %s: %w", target, err)
		}
		if !info.IsDir() {
			paths = append(paths, target)
			continue
		}
		walkErr := filepath.WalkDir(target, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() || filepath.Ext(path) != ".md" {
				return nil
			}
			paths = append(paths, path)
			return nil
		})
		if walkErr != nil {
			return fmt.Errorf("walk plugin Markdown target %s: %w", target, walkErr)
		}
	}
	sort.Strings(paths)
	stale := 0
	for _, path := range paths {
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return fmt.Errorf("read %s: %w", path, readErr)
		}
		if !bytes.Contains(content, []byte("content-hash:")) {
			if filepath.Base(path) == "SPEC.md" {
				continue
			}
			return fmt.Errorf("plugin Markdown %s has no content-hash field", path)
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
		return fmt.Errorf("%d plugin Markdown hash(es) are stale", stale)
	}
	return nil
}
