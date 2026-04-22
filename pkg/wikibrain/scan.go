package wikibrain

import (
	"bufio"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var (
	reH1         = regexp.MustCompile(`^#\s+(.+)`)
	reWikiLink   = regexp.MustCompile(`\[\[([^\]]+)\]\]`)
	reMDLink     = regexp.MustCompile(`\[([^\]]*)\]\(([^)]+)\)`)
	reLastUpdate = regexp.MustCompile(`(?i)(?:\*\*last[ -]updated:\*\*|last[ -]updated:)\s*(.+)`)
	reDateFormats = []string{
		"2006-01-02",
		"January 2, 2006",
		"2006-01",
		"2006/01/02",
	}
)

// ScanKB walks the KB directory and returns all parsed pages, keyed by
// their RelPath. Directories matching skipDirs are skipped.
func ScanKB(kbPath string) ([]*Page, error) {
	var pages []*Page
	skipDirs := map[string]bool{
		".git":      true,
		".obsidian": true,
		".claude":   true,
		".sync":     true,
		"scripts":   true,
		"docs":      true,
		"config":    true,
		"tests":     true,
		"templates": true,
		"00-private": true, // private tier — excluded from analysis
	}

	err := filepath.WalkDir(kbPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".md" {
			return nil
		}
		rel, err := filepath.Rel(kbPath, path)
		if err != nil {
			return err
		}
		p, err := parsePage(path, rel)
		if err != nil {
			// non-fatal: skip unreadable pages
			return nil //nolint:nilerr
		}
		pages = append(pages, p)
		return nil
	})
	return pages, err
}

func parsePage(absPath, relPath string) (*Page, error) {
	f, err := os.Open(absPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	p := &Page{
		AbsPath: absPath,
		RelPath: relPath,
		Title:   titleFromPath(relPath),
	}

	scanner := bufio.NewScanner(f)
	lineNum := 0
	summaryLines := 0
	inFrontmatter := false

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		// YAML frontmatter delimiters
		if lineNum == 1 && line == "---" {
			inFrontmatter = true
			continue
		}
		if inFrontmatter {
			if line == "---" {
				inFrontmatter = false
			}
			continue
		}

		// H1 title
		if p.Title == titleFromPath(relPath) {
			if m := reH1.FindStringSubmatch(line); m != nil {
				p.Title = strings.TrimSpace(m[1])
			}
		}

		// Last-updated
		if !p.HasLastUpdated {
			if m := reLastUpdate.FindStringSubmatch(line); m != nil {
				if t, ok := parseDate(strings.TrimSpace(m[1])); ok {
					p.LastUpdated = t
					p.HasLastUpdated = true
				}
			}
		}

		// Summary: first meaningful non-heading paragraph
		if summaryLines == 0 && !strings.HasPrefix(line, "#") && strings.TrimSpace(line) != "" {
			p.Summary = strings.TrimSpace(line)
			summaryLines++
		}

		// Wiki links [[target]]
		for _, m := range reWikiLink.FindAllStringSubmatch(line, -1) {
			target := strings.TrimSpace(m[1])
			// Strip display alias: [[target|alias]]
			if idx := strings.Index(target, "|"); idx != -1 {
				target = target[:idx]
			}
			p.WikiLinks = append(p.WikiLinks, target)
		}

		// Markdown links [text](target) — internal only (no http)
		for _, m := range reMDLink.FindAllStringSubmatch(line, -1) {
			target := strings.TrimSpace(m[2])
			if !strings.HasPrefix(target, "http://") && !strings.HasPrefix(target, "https://") {
				p.MarkdownLinks = append(p.MarkdownLinks, target)
			}
		}
	}

	return p, scanner.Err()
}

func titleFromPath(relPath string) string {
	base := filepath.Base(relPath)
	name := strings.TrimSuffix(base, ".md")
	// Convert hyphens/underscores to spaces and title-case
	name = strings.ReplaceAll(name, "-", " ")
	name = strings.ReplaceAll(name, "_", " ")
	return name
}

func parseDate(s string) (time.Time, bool) {
	// Strip markdown bold markers
	s = strings.Trim(s, "*")
	s = strings.TrimSpace(s)
	for _, layout := range reDateFormats {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}
