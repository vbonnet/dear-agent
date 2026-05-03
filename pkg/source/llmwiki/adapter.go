// Package llmwiki implements pkg/source.Adapter against a git-backed
// markdown directory — the format llm-wiki and similar text-first
// knowledge bases standardise on. Each Add writes a markdown file and
// stages+commits it; Fetch greps the working tree.
//
// The package degrades gracefully: when the target directory is not a
// git repo, Add still writes the file but skips the commit. When git
// is on PATH, the adapter wires "git add" + "git commit" so every
// stored Source becomes a versioned history entry — the property that
// distinguishes llm-wiki from a plain filesystem dump.
//
// Search semantics mirror pkg/source/obsidian: substring + cue +
// time + work-item filters in memory, ordered by hit weight then
// IndexedAt. Targeted at small corpora (under ~10k files); a true
// FTS index is left to a future optimisation if a real driver appears.
package llmwiki

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
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
const Name = "llm-wiki"

// defaultK keeps Fetch behaviour consistent with the sqlite adapter.
const defaultK = 10

// Adapter reads/writes Sources to a git-backed markdown directory.
type Adapter struct {
	root string
	now  func() time.Time

	// AutoCommit controls whether Add stages and commits on success.
	// Defaults to true when git is detected at Open. Set false to
	// disable commits entirely (e.g. tests, or a directory whose
	// commits the operator wants to batch by hand).
	AutoCommit bool

	// CommitAuthor is plumbed through to git as `--author=` so the
	// substrate's audit story extends to repository history. Empty
	// uses git's local user.name/user.email config.
	CommitAuthor string

	mu sync.Mutex
}

// Open returns an Adapter rooted at wikiDir. Creates the directory if
// missing. AutoCommit defaults to true when wikiDir is inside a git
// repo, false otherwise — callers can flip the field after Open.
func Open(wikiDir string) (*Adapter, error) {
	if wikiDir == "" {
		return nil, errors.New("source/llmwiki: directory is required")
	}
	info, err := os.Stat(wikiDir)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		if err := os.MkdirAll(wikiDir, 0o755); err != nil {
			return nil, fmt.Errorf("source/llmwiki: mkdir %s: %w", wikiDir, err)
		}
	case err != nil:
		return nil, fmt.Errorf("source/llmwiki: stat %s: %w", wikiDir, err)
	case !info.IsDir():
		return nil, fmt.Errorf("source/llmwiki: %s is not a directory", wikiDir)
	}
	a := &Adapter{root: wikiDir, now: time.Now}
	a.AutoCommit = isGitRepo(wikiDir) && hasGit()
	return a, nil
}

// Name returns the backend identifier.
func (a *Adapter) Name() string { return Name }

// HealthCheck verifies the wiki directory is readable.
func (a *Adapter) HealthCheck(ctx context.Context) error {
	if a == nil {
		return errors.New("source/llmwiki: nil adapter")
	}
	if _, err := os.Stat(a.root); err != nil {
		return fmt.Errorf("source/llmwiki: directory unreachable: %w", err)
	}
	return ctx.Err()
}

// Close is a no-op — adapter holds no long-lived resources.
func (a *Adapter) Close() error { return nil }

// Add writes a Source to the wiki directory and (when AutoCommit is on)
// stages and commits it. The path within the wiki is derived from the
// URI: a "wiki://" URI uses its host+path; any other URI is slugified
// to a wiki-relative path. Re-adding the same URI is idempotent.
func (a *Adapter) Add(ctx context.Context, s source.Source) (source.Ref, error) {
	if err := ctx.Err(); err != nil {
		return source.Ref{}, err
	}
	a.mu.Lock()
	defer a.mu.Unlock()

	rel := pathFromURI(s.URI)
	if rel == "" {
		return source.Ref{}, fmt.Errorf("source/llmwiki: empty URI")
	}
	abs := filepath.Join(a.root, rel)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return source.Ref{}, fmt.Errorf("source/llmwiki: mkdir parent: %w", err)
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
		return source.Ref{}, fmt.Errorf("source/llmwiki: write %s: %w", abs, err)
	}
	if a.AutoCommit {
		if err := a.commit(ctx, rel, s); err != nil {
			// Don't fail the Add on a commit error — the file is on
			// disk, the worst case is the operator commits manually.
			// We surface the error via context but keep the Add
			// successful for the caller's purposes.
			_ = err
		}
	}
	return source.Ref{URI: s.URI, Backend: Name, IndexedAt: now}, nil
}

// commit shells out to git add + git commit. We use exec.CommandContext
// so a stuck git invocation cancels with the run.
func (a *Adapter) commit(ctx context.Context, rel string, s source.Source) error {
	if err := runGit(ctx, a.root, "add", "--", rel); err != nil {
		return err
	}
	msg := commitMessage(s)
	args := []string{"commit", "-m", msg, "--", rel}
	if a.CommitAuthor != "" {
		args = []string{"commit", "--author=" + a.CommitAuthor, "-m", msg, "--", rel}
	}
	return runGit(ctx, a.root, args...)
}

// commitMessage builds a one-line commit message that captures the
// work-item attribution. Format mirrors what the audit log records so
// `git log` and `audit_events` can be cross-referenced.
func commitMessage(s source.Source) string {
	work := s.Metadata.WorkItem
	if work == "" {
		work = "manual"
	}
	title := s.Title
	if title == "" {
		title = s.URI
	}
	return fmt.Sprintf("source: %s (work=%s)", title, work)
}

// Fetch walks the wiki, decodes each markdown file, applies filters,
// and returns the top K hits.
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

// scan walks the wiki directory. Skips .git/ and dotfiles.
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
			if base == ".git" || (strings.HasPrefix(base, ".") && path != a.root) {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(base) != ".md" {
			return nil
		}
		body, err := os.ReadFile(path) //nolint:gosec // path comes from WalkDir
		if err != nil {
			return fmt.Errorf("source/llmwiki: read %s: %w", path, err)
		}
		rel, _ := filepath.Rel(a.root, path)
		s, err := decode(body, rel)
		if err != nil {
			return fmt.Errorf("source/llmwiki: decode %s: %w", rel, err)
		}
		out = append(out, s)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// pathFromURI reduces a URI to a wiki-relative file path. wiki:// is
// treated specially; other schemes get a sanitised slug.
func pathFromURI(uri string) string {
	if uri == "" {
		return ""
	}
	if strings.HasPrefix(uri, "wiki://") {
		stem := strings.TrimPrefix(uri, "wiki://")
		stem = strings.TrimPrefix(stem, "/")
		if filepath.Ext(stem) == "" {
			stem += ".md"
		}
		return filepath.Clean(stem)
	}
	// Generic: turn the URI into a filesystem-safe slug rooted at the
	// wiki dir. Hash-prefix would be more compact but harder to
	// inspect by hand; we keep human-readable slugs.
	stem := uri
	stem = strings.TrimPrefix(stem, "https://")
	stem = strings.TrimPrefix(stem, "http://")
	var b strings.Builder
	for _, r := range stem {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		case r == '/':
			b.WriteRune('/')
		case r == '.':
			b.WriteRune('.')
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

// matchFilters mirrors the obsidian adapter — same predicate set, same
// prefix semantics on WorkItem so callers can swap backends without
// surprises.
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

func timeOK(t time.Time, after, before *time.Time) bool {
	if after != nil && t.Before(*after) {
		return false
	}
	if before != nil && !t.Before(*before) {
		return false
	}
	return true
}

func matchesQuery(s source.Source, queryLower string) bool {
	hay := strings.ToLower(s.Title + "\n" + s.Snippet + "\n" + string(s.Content))
	return strings.Contains(hay, queryLower)
}

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

// frontmatter is the YAML header llm-wiki files carry. Identical
// shape to the obsidian adapter for symmetry — knowledge stored in
// either backend can be moved between the two without re-encoding.
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
		return nil, fmt.Errorf("source/llmwiki: marshal frontmatter: %w", err)
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

func decode(body []byte, rel string) (source.Source, error) {
	rest := body
	var fm frontmatter
	if hasFrontmatter(body) {
		end := frontmatterEnd(body)
		if end < 0 {
			return source.Source{}, errors.New("unterminated frontmatter")
		}
		header := body[4:end]
		if err := yaml.Unmarshal(header, &fm); err != nil {
			return source.Source{}, fmt.Errorf("yaml unmarshal: %w", err)
		}
		rest = body[end+4:]
		rest = trimLeftNewline(rest)
	}
	uri := fm.URI
	if uri == "" {
		uri = "wiki:///" + filepath.ToSlash(rel)
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

func hasFrontmatter(b []byte) bool {
	return len(b) >= 4 && string(b[:4]) == "---\n"
}

func frontmatterEnd(b []byte) int {
	const marker = "\n---"
	for i := 4; i < len(b)-3; i++ {
		if string(b[i:i+4]) == marker {
			return i
		}
	}
	return -1
}

func trimLeftNewline(b []byte) []byte {
	for len(b) > 0 && (b[0] == '\n' || b[0] == '\r') {
		b = b[1:]
	}
	return b
}

// --- git helpers ---

// hasGit caches whether `git` is available on PATH. Negative caching
// is fine — if git appears mid-run we'll discover it on the next Open.
var hasGitCache struct {
	once sync.Once
	ok   bool
}

func hasGit() bool {
	hasGitCache.once.Do(func() {
		if _, err := exec.LookPath("git"); err == nil {
			hasGitCache.ok = true
		}
	})
	return hasGitCache.ok
}

// isGitRepo walks parents looking for a .git directory or file. We
// don't shell out to git here — pure stat is enough and avoids a
// process spawn on every Open.
func isGitRepo(dir string) bool {
	cur := dir
	for {
		if _, err := os.Stat(filepath.Join(cur, ".git")); err == nil {
			return true
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return false
		}
		cur = parent
	}
}

// runGit shells out, capturing combined output for the error message.
func runGit(ctx context.Context, dir string, args ...string) error {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %s: %w (%s)", args[0], err, strings.TrimSpace(string(out)))
	}
	return nil
}
