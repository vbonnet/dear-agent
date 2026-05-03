package wikibrain

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// GenerateIndex produces the markdown content for index.md from the scanned
// pages. The catalog is grouped by top-level directory.
func GenerateIndex(pages []*Page, runAt time.Time) string {
	groups := groupByDir(pages)
	dirOrder := sortedDirs(groups)

	var sb strings.Builder
	sb.WriteString("# engram-kb — Page Index\n\n")
	fmt.Fprintf(&sb, "_Auto-generated %s. Do not edit by hand — run `agm wiki index` to refresh._\n\n",
		runAt.Format("2006-01-02"))
	fmt.Fprintf(&sb, "**Total pages:** %d\n\n", len(pages))
	sb.WriteString("---\n\n")

	for _, dir := range dirOrder {
		group := groups[dir]
		label := dirLabel(dir)
		fmt.Fprintf(&sb, "## %s\n\n", label)

		// Sort pages within group by RelPath for stable output
		sort.Slice(group, func(i, j int) bool {
			return group[i].RelPath < group[j].RelPath
		})

		for _, p := range group {
			link := fmt.Sprintf("[%s](%s)", p.Title, p.RelPath)
			date := ""
			if p.HasLastUpdated {
				date = " · " + p.LastUpdated.Format("2006-01-02")
			}
			summary := p.Summary
			if len(summary) > 120 {
				summary = summary[:117] + "..."
			}
			if summary != "" {
				fmt.Fprintf(&sb, "- %s%s — %s\n", link, date, summary)
			} else {
				fmt.Fprintf(&sb, "- %s%s\n", link, date)
			}
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

func groupByDir(pages []*Page) map[string][]*Page {
	groups := make(map[string][]*Page)
	for _, p := range pages {
		dir := filepath.Dir(p.RelPath)
		if dir == "." {
			dir = "(root)"
		}
		groups[dir] = append(groups[dir], p)
	}
	return groups
}

func sortedDirs(groups map[string][]*Page) []string {
	dirs := make([]string, 0, len(groups))
	for d := range groups {
		dirs = append(dirs, d)
	}
	sort.Strings(dirs)
	return dirs
}

func dirLabel(dir string) string {
	if dir == "(root)" {
		return "Root"
	}
	// "01-decisions" → "01-decisions"
	return dir
}
