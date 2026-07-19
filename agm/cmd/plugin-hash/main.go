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
	paths, err := pluginMarkdownPaths(targets)
	if err != nil {
		return err
	}
	stale := 0
	for _, path := range paths {
		isStale, stampErr := stampPluginMarkdown(path, check)
		if stampErr != nil {
			return stampErr
		}
		if isStale {
			stale++
		}
	}
	if stale > 0 {
		return fmt.Errorf("%d plugin Markdown hash(es) are stale", stale)
	}
	return nil
}

func pluginMarkdownPaths(targets []string) ([]string, error) {
	var paths []string
	for _, target := range targets {
		info, err := os.Stat(target)
		if err != nil {
			return nil, fmt.Errorf("stat plugin Markdown target %s: %w", target, err)
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
			return nil, fmt.Errorf("walk plugin Markdown target %s: %w", target, walkErr)
		}
	}
	sort.Strings(paths)
	return paths, nil
}

func stampPluginMarkdown(path string, check bool) (bool, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return false, fmt.Errorf("read %s: %w", path, err)
	}
	if !bytes.Contains(content, []byte("content-hash:")) {
		if filepath.Base(path) == "SPEC.md" {
			return false, nil
		}
		return false, fmt.Errorf("plugin Markdown %s has no content-hash field", path)
	}
	stamped, err := pluginhash.Stamp(content)
	if err != nil {
		return false, fmt.Errorf("hash %s: %w", path, err)
	}
	if bytes.Equal(content, stamped) {
		fmt.Printf("OK %s\n", path)
		return false, nil
	}
	if check {
		fmt.Printf("STALE %s\n", path)
		return true, nil
	}
	// #nosec G703 -- every path is an explicit target or was discovered below one.
	if err := os.WriteFile(path, stamped, 0o600); err != nil {
		return false, fmt.Errorf("write %s: %w", path, err)
	}
	fmt.Printf("UPDATED %s\n", path)
	return false, nil
}
