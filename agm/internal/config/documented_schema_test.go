package config

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// agmConfigFence matches a fenced YAML block that the surrounding prose
// introduces as the AGM configuration file. Once Load rejects unknown fields,
// a snippet an operator is told to paste into ~/.config/agm/config.yaml is
// executable guidance: if it does not decode, following the documentation
// breaks every AGM command.
var agmConfigFence = regexp.MustCompile("(?s)```ya?ml\n(.*?)```")

// documentedConfigSources are the normative places an operator copies AGM
// configuration from. Paths are relative to the repository root.
var documentedConfigSources = []string{
	"agm/config.example.yaml",
	"docs/DEPLOYMENT.md",
	"agm/CENTRALIZED-STORAGE.md",
	"agm/docs/CONFIGURATION.md",
	"agm/README.md",
}

func TestDocumentedAgentConfigDecodesUnderStrictSchema(t *testing.T) {
	root := repositoryRoot(t)
	checked := 0
	for _, relative := range documentedConfigSources {
		path := filepath.Join(root, relative)
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", relative, err)
		}
		for _, snippet := range agmConfigSnippets(relative, string(data)) {
			checked++
			t.Run(snippet.name, func(t *testing.T) {
				cfg := defaultWithHome(t.TempDir())
				if err := decodeConfig([]byte(snippet.body), cfg); err != nil {
					t.Fatalf("documented ~/.config/agm/config.yaml snippet does not decode: %v\n%s",
						err, snippet.body)
				}
			})
		}
	}
	if checked == 0 {
		t.Fatal("found no documented AGM configuration snippets; the marker heuristic has drifted")
	}
}

type documentedSnippet struct {
	name string
	body string
}

// agmConfigSnippets returns the fenced YAML blocks that the document presents
// as ~/.config/agm/config.yaml content. A block qualifies when the preceding
// prose names that file, or when the whole file IS the example config.
func agmConfigSnippets(relative, contents string) []documentedSnippet {
	if strings.HasSuffix(relative, ".yaml") {
		return []documentedSnippet{{name: relative, body: contents}}
	}
	var snippets []documentedSnippet
	for _, match := range agmConfigFence.FindAllStringSubmatchIndex(contents, -1) {
		body := contents[match[2]:match[3]]
		line := strings.Count(contents[:match[0]], "\n") + 1
		if !introducedAsAgentConfig(contents[:match[0]]) {
			continue
		}
		snippets = append(snippets, documentedSnippet{
			name: relative + ":" + itoa(line),
			body: body,
		})
	}
	return snippets
}

// introducedAsAgentConfig reports whether the prose immediately before a fence
// names the AGM configuration file. Only the last few lines count: a mention
// far earlier in the document is describing something else by the time the
// reader reaches this block.
func introducedAsAgentConfig(preceding string) bool {
	lines := strings.Split(preceding, "\n")
	const lookback = 4
	if len(lines) > lookback {
		lines = lines[len(lines)-lookback:]
	}
	return strings.Contains(strings.Join(lines, "\n"), "/.config/agm/config.yaml")
}

func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	var digits []byte
	for value > 0 {
		digits = append([]byte{byte('0' + value%10)}, digits...)
		value /= 10
	}
	return string(digits)
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		} else if !errors.Is(err, os.ErrNotExist) {
			t.Fatal(err)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("repository root (go.mod) not found above the package directory")
		}
		dir = parent
	}
}
