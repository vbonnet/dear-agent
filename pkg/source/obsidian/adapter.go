// Package obsidian implements pkg/source.Adapter against an Obsidian
// vault on disk. The vault is a directory of .md files with optional
// YAML frontmatter; each Source maps to one file. URIs use the
// "obsidian://" scheme so a Source produced by this adapter survives a
// round-trip through Add → Fetch.
//
// Why a real adapter rather than a stub: Obsidian doesn't have a
// daemon — the vault is just a directory. A filesystem adapter is the
// integration. The actual Obsidian app sees the same files via its
// vault loader, so a workflow that writes via this adapter is visible
// inside Obsidian without any plugin running.
//
// Search semantics are intentionally simple: substring + cue match
// over title, snippet, content, and frontmatter cues. No FTS index;
// the dataset is bounded by the vault size and substring scan over a
// few hundred to a few thousand small markdown files is fast enough
// for the dev/single-user use case Phase 5.1 targets.
package obsidian

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/vbonnet/dear-agent/pkg/source"
)

// Name is the backend identifier the MCP layer matches against
// FetchQuery.Filters.Backend.
const Name = "obsidian"

// defaultK matches the sqlite adapter's default — keep search verb
// behaviour consistent across backends.
const defaultK = 10

// Adapter writes/reads Sources to/from an Obsidian vault directory.
// One Source per file. The vault is a plain directory tree of .md
// files; Open creates it if missing.
type Adapter struct {
	root string
	now  func() time.Time
	mu   sync.Mutex
}

// Open returns an Adapter rooted at vaultDir. The directory is created
// if missing (with the conventional 0o755). Returns an error if the
// path exists and is not a directory.
func Open(vaultDir string) (*Adapter, error) {
	if vaultDir == "" {
		return nil, errors.New("source/obsidian: vault directory is required")
	}
	info, err := os.Stat(vaultDir)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		if err := os.MkdirAll(vaultDir, 0o755); err != nil {
			return nil, fmt.Errorf("source/obsidian: mkdir %s: %w", vaultDir, err)
		}
	case err != nil:
		return nil, fmt.Errorf("source/obsidian: stat %s: %w", vaultDir, err)
	case !info.IsDir():
		return nil, fmt.Errorf("source/obsidian: %s is not a directory", vaultDir)
	}
	return &Adapter{root: vaultDir, now: time.Now}, nil
}

// Name returns the backend identifier.
func (a *Adapter) Name() string { return Name }

// HealthCheck verifies the vault directory is readable.
func (a *Adapter) HealthCheck(ctx context.Context) error {
	if a == nil {
		return errors.New("source/obsidian: nil adapter")
	}
	if _, err := os.Stat(a.root); err != nil {
		return fmt.Errorf("source/obsidian: vault unreachable: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}

// Close is a no-op — there are no resources to release.
func (a *Adapter) Close() error { return nil }

// Add writes s to the vault. The file path is derived from s.URI: an
// "obsidian://" URI uses its host+path; any other URI is slugified to
// a vault-relative path. Re-adding the same URI overwrites the file
// in place (idempotent on URI, matching the contract).
func (a *Adapter) Add(ctx context.Context, s source.Source) (source.Ref, error) {
	if err := ctx.Err(); err != nil {
		return source.Ref{}, err
	}
	a.mu.Lock()
	defer a.mu.Unlock()

	rel, err := pathFromURI(s.URI)
	if err != nil {
		return source.Ref{}, err
	}
	abs := filepath.Join(a.root, rel)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return source.Ref{}, fmt.Errorf("source/obsidian: mkdir parent: %w", err)
	}
	now := s.IndexedAt
	if now.IsZero() {
		now = a.now().UTC()
	}
	body, err := encode(s, now)
	if err != nil {
		return source.Ref{}, err
	}
	if err := os.WriteFile(abs, body, 0o600); err != nil {
		return source.Ref{}, fmt.Errorf("source/obsidian: write %s: %w", abs, err)
	}
	// Preserve the caller's URI verbatim — the contract requires
	// Add → Fetch round-trip to keep URI stable. The vault path is an
	// implementation detail; the URI is data the caller owns.
	return source.Ref{URI: s.URI, Backend: Name, IndexedAt: now}, nil
}

// Fetch walks the vault, decoding each file, then applies the query +
// filters in memory. A vault is small enough that this is the right
// trade-off — adding an FTS index would mean a sidecar database and
// cache invalidation for vaults edited outside this adapter (which is
// the dual-write use case Phase 5.1 targets).
func (a *Adapter) Fetch(ctx context.Context, q source.FetchQuery) ([]source.Source, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	all, err := a.scan(ctx)
	if err != nil {
		return nil, err
	}
	matches := make([]source.Source, 0, len(all))
	queryLower := strings.ToLower(strings.TrimSpace(q.Query))
	for _, s := range all {
		if !matchFilters(s, q.Filters) {
			continue
		}
		if queryLower != "" && !matchesQuery(s, queryLower) {
			continue
		}
		s.Score = scoreFor(s, queryLower)
		matches = append(matches, s)
	}
	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].Score != matches[j].Score {
			return matches[i].Score > matches[j].Score
		}
		return matches[i].IndexedAt.After(matches[j].IndexedAt)
	})
	k := q.K
	if k <= 0 {
		k = defaultK
	}
	if len(matches) > k {
		matches = matches[:k]
	}
	return matches, nil
}

// scan walks the vault directory and returns one Source per .md file.
// Hidden files and Obsidian's own .obsidian/ config dir are skipped.
func (a *Adapter) scan(ctx context.Context) ([]source.Source, error) {
	var out []source.Source
	err := filepath.WalkDir(a.root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		base := filepath.Base(path)
		if d.IsDir() {
			if base == ".obsidian" || strings.HasPrefix(base, ".") && path != a.root {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(base) != ".md" {
			return nil
		}
		body, err := os.ReadFile(path) //nolint:gosec // path comes from WalkDir
		if err != nil {
			return fmt.Errorf("source/obsidian: read %s: %w", path, err)
		}
		rel, _ := filepath.Rel(a.root, path)
		s, err := decode(body, rel)
		if err != nil {
			return fmt.Errorf("source/obsidian: decode %s: %w", rel, err)
		}
		out = append(out, s)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// pathFromURI maps a URI to a vault-relative file path. obsidian://
// scheme is preferred; other schemes get a slugified host+path so
// arbitrary external URIs can be ingested.
func pathFromURI(uri string) (string, error) {
	if uri == "" {
		return "", errors.New("source/obsidian: URI is required")
	}
	u, err := url.Parse(uri)
	if err != nil {
		// Treat unparseable URIs as literal slugs — the caller's URI
		// is opaque to us and we just want a stable file path.
		return slugFile(uri), nil //nolint:nilerr // intentional: treat parse error as "use uri as slug"
	}
	if u.Scheme == "obsidian" {
		// obsidian://Note%20Title or obsidian:///path/to/file.md
		path := strings.TrimPrefix(u.Path, "/")
		if path == "" {
			path = u.Host
		}
		if path == "" {
			return "", fmt.Errorf("source/obsidian: empty path in URI %q", uri)
		}
		// Append .md if the URI didn't include an extension.
		if filepath.Ext(path) == "" {
			path += ".md"
		}
		return filepath.Clean(path), nil
	}
	return slugFile(uri), nil
}

// slugFile returns a safe vault filename derived from an arbitrary
// URI. Strips scheme and special characters; ensures .md extension.
func slugFile(uri string) string {
	u, err := url.Parse(uri)
	stem := uri
	if err == nil {
		stem = u.Host + u.Path
	}
	stem = strings.TrimPrefix(stem, "/")
	if stem == "" {
		stem = "untitled"
	}
	var b strings.Builder
	for _, r := range stem {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		case r == '/':
			b.WriteRune('/')
		case r == ' ':
			b.WriteRune('-')
		default:
			b.WriteRune('-')
		}
	}
	out := b.String()
	if filepath.Ext(out) == "" {
		out += ".md"
	}
	return filepath.Clean(out)
}

// matchFilters applies structured predicates. Backend filtering is
// intentionally a no-op here — the MCP layer enforces backend match
// before forwarding the query.
func matchFilters(s source.Source, f source.Filters) bool {
	if f.WorkItem != "" && !workItemMatch(s.Metadata.WorkItem, f.WorkItem) {
		return false
	}
	if !timeOK(s.IndexedAt, f.After, f.Before) {
		return false
	}
	if len(f.Cues) > 0 {
		have := map[string]struct{}{}
		for _, c := range s.Metadata.Cues {
			have[c] = struct{}{}
		}
		for _, want := range f.Cues {
			if _, ok := have[want]; !ok {
				return false
			}
		}
	}
	return true
}

// workItemMatch implements the prefix semantics required by the
// adapter contract: an exact match returns true ("run-A/n1" matches
// "run-A/n1"), and a prefix-segment match returns true ("run-A/n1"
// matches "run-A"). The prefix must align on a path segment ("/")
// boundary so "run-AA" does not match "run-A".
func workItemMatch(have, want string) bool {
	if have == want {
		return true
	}
	if !strings.HasPrefix(have, want) {
		return false
	}
	if len(have) == len(want) {
		return true
	}
	return have[len(want)] == '/'
}

// timeOK applies the After/Before window: After is inclusive, Before
// is exclusive. Matches the convention documented on Filters.
func timeOK(t time.Time, after, before *time.Time) bool {
	if after != nil && t.Before(*after) {
		return false
	}
	if before != nil && !t.Before(*before) {
		return false
	}
	return true
}

// matchesQuery does case-insensitive substring search over the
// human-readable fields. Adequate for vault sizes the single-user use
// case targets.
func matchesQuery(s source.Source, queryLower string) bool {
	hay := strings.ToLower(s.Title + "\n" + s.Snippet + "\n" + string(s.Content))
	return strings.Contains(hay, queryLower)
}

// scoreFor weights matches in the title higher than matches in the
// body — title hits are typically the most relevant. Untyped queries
// (empty queryLower) score zero and fall back to IndexedAt ordering.
func scoreFor(s source.Source, queryLower string) float64 {
	if queryLower == "" {
		return 0
	}
	score := 0.0
	if strings.Contains(strings.ToLower(s.Title), queryLower) {
		score += 2.0
	}
	if strings.Contains(strings.ToLower(s.Snippet), queryLower) {
		score += 1.0
	}
	if strings.Contains(strings.ToLower(string(s.Content)), queryLower) {
		score += 0.5
	}
	return score
}

// frontmatter is the structured header at the top of an Obsidian
// markdown file. We persist enough to round-trip a Source's metadata.
type frontmatter struct {
	Title      string         `yaml:"title,omitempty"`
	URI        string         `yaml:"uri,omitempty"`
	Snippet    string         `yaml:"snippet,omitempty"`
	IndexedAt  time.Time      `yaml:"indexed_at,omitempty"`
	Cues       []string       `yaml:"cues,omitempty"`
	WorkItem   string         `yaml:"work_item,omitempty"`
	Role       string         `yaml:"role,omitempty"`
	Confidence float64        `yaml:"confidence,omitempty"`
	Source     string         `yaml:"source,omitempty"`
	Custom     map[string]any `yaml:"custom,omitempty"`
}

// encode renders a Source as a markdown file with YAML frontmatter.
// Standard Obsidian convention: --- delimiters, frontmatter at the
// top, then the body. Other Obsidian plugins expect this shape.
func encode(s source.Source, indexedAt time.Time) ([]byte, error) {
	fm := frontmatter{
		Title:      s.Title,
		URI:        s.URI,
		Snippet:    s.Snippet,
		IndexedAt:  indexedAt,
		Cues:       s.Metadata.Cues,
		WorkItem:   s.Metadata.WorkItem,
		Role:       s.Metadata.Role,
		Confidence: s.Metadata.Confidence,
		Source:     s.Metadata.Source,
		Custom:     s.Metadata.Custom,
	}
	header, err := yaml.Marshal(fm)
	if err != nil {
		return nil, fmt.Errorf("source/obsidian: marshal frontmatter: %w", err)
	}
	var b strings.Builder
	b.WriteString("---\n")
	b.Write(header)
	b.WriteString("---\n\n")
	b.Write(s.Content)
	if len(s.Content) > 0 && s.Content[len(s.Content)-1] != '\n' {
		b.WriteByte('\n')
	}
	return []byte(b.String()), nil
}

// decode parses a markdown file with optional frontmatter. Files
// lacking frontmatter are still returned (URI derived from the path)
// so externally-authored notes are also indexable.
func decode(body []byte, rel string) (source.Source, error) {
	rest := body
	var fm frontmatter
	if hasFrontmatter(body) {
		end := frontmatterEnd(body)
		if end < 0 {
			return source.Source{}, errors.New("unterminated frontmatter")
		}
		header := body[4:end] // skip leading "---\n"
		if err := yaml.Unmarshal(header, &fm); err != nil {
			return source.Source{}, fmt.Errorf("yaml unmarshal: %w", err)
		}
		rest = body[end+4:] // skip trailing "\n---"
		rest = trimLeftNewline(rest)
	}
	uri := fm.URI
	if uri == "" {
		uri = "obsidian:///" + filepath.ToSlash(rel)
	}
	title := fm.Title
	if title == "" {
		title = strings.TrimSuffix(filepath.Base(rel), ".md")
	}
	return source.Source{
		URI:       uri,
		Title:     title,
		Snippet:   fm.Snippet,
		Content:   rest,
		IndexedAt: fm.IndexedAt,
		Metadata: source.Metadata{
			Cues:       fm.Cues,
			WorkItem:   fm.WorkItem,
			Role:       fm.Role,
			Confidence: fm.Confidence,
			Source:     fm.Source,
			Custom:     fm.Custom,
		},
	}, nil
}

// hasFrontmatter reports whether the body starts with the canonical
// "---\n" marker. Notes without frontmatter are still ingestible.
func hasFrontmatter(b []byte) bool {
	return len(b) >= 4 && string(b[:4]) == "---\n"
}

// frontmatterEnd returns the offset of the closing "---" line, or -1
// if not found.
func frontmatterEnd(b []byte) int {
	const marker = "\n---"
	for i := 4; i < len(b)-3; i++ {
		if string(b[i:i+4]) == marker {
			return i
		}
	}
	return -1
}

// trimLeftNewline strips leading newlines so the body's first line
// is what the user wrote, not whitespace from the frontmatter
// boundary.
func trimLeftNewline(b []byte) []byte {
	for len(b) > 0 && (b[0] == '\n' || b[0] == '\r') {
		b = b[1:]
	}
	return b
}
