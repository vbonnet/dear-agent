package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
	"golang.org/x/net/html"
)

type linkRef struct {
	target string
	line   int
}

type document struct {
	links   []linkRef
	anchors map[string]bool
}

type linkChecker struct {
	root      string
	verbose   bool
	markdown  goldmark.Markdown
	documents map[string]*document
}

func newLinkChecker(root string, verbose bool) *linkChecker {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		absRoot = filepath.Clean(root)
	}
	return &linkChecker{
		root:      absRoot,
		verbose:   verbose,
		markdown:  goldmark.New(goldmark.WithExtensions(extension.GFM)),
		documents: map[string]*document{},
	}
}

func (c *linkChecker) loadDocument(path string) (*document, error) {
	path = filepath.Clean(path)
	if doc, ok := c.documents[path]; ok {
		return doc, nil
	}
	source, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	doc := parseDocument(c.markdown, source)
	c.documents[path] = doc
	return doc, nil
}

func parseDocument(markdown goldmark.Markdown, source []byte) *document {
	root := markdown.Parser().Parse(text.NewReader(source))
	doc := &document{anchors: map[string]bool{}}
	headingAnchors := map[string]bool{}

	_ = ast.Walk(root, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch n := node.(type) {
		case *ast.Heading:
			base := githubSlug(headingText(n, source))
			if base == "" {
				break
			}
			id := base
			for suffix := 1; headingAnchors[id]; suffix++ {
				id = fmt.Sprintf("%s-%d", base, suffix)
			}
			headingAnchors[id] = true
			doc.anchors[id] = true
		case *ast.Link:
			doc.links = append(doc.links, linkRef{target: string(n.Destination), line: nodeLine(source, n)})
		case *ast.Image:
			doc.links = append(doc.links, linkRef{target: string(n.Destination), line: nodeLine(source, n)})
		case *ast.RawHTML:
			collectExplicitAnchors(doc.anchors, n.Segments.Value(source))
		case *ast.HTMLBlock:
			collectExplicitAnchors(doc.anchors, n.Lines().Value(source))
		}
		return ast.WalkContinue, nil
	})
	return doc
}

func headingText(heading *ast.Heading, source []byte) string {
	var content strings.Builder
	_ = ast.Walk(heading, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch n := node.(type) {
		case *ast.Text:
			content.Write(n.Value(source))
			if n.SoftLineBreak() || n.HardLineBreak() {
				content.WriteByte(' ')
			}
		case *ast.String:
			content.Write(n.Value)
		case *ast.AutoLink:
			content.Write(n.Label(source))
		}
		return ast.WalkContinue, nil
	})
	resolved := util.ResolveNumericReferences([]byte(content.String()))
	resolved = util.ResolveEntityNames(resolved)
	return string(resolved)
}

func collectExplicitAnchors(anchors map[string]bool, raw []byte) {
	tokenizer := html.NewTokenizer(bytes.NewReader(raw))
	for {
		switch tokenizer.Next() {
		case html.ErrorToken:
			if tokenizer.Err() == io.EOF {
				return
			}
			return
		case html.StartTagToken, html.SelfClosingTagToken:
			tag, more := tokenizer.TagName()
			legacyNamedAnchor := bytes.EqualFold(tag, []byte("a"))
			for more {
				key, value, next := tokenizer.TagAttr()
				if (bytes.EqualFold(key, []byte("id")) || (legacyNamedAnchor && bytes.EqualFold(key, []byte("name")))) && len(value) > 0 {
					anchors[string(value)] = true
				}
				more = next
			}
		}
	}
}

func nodeLine(source []byte, node ast.Node) int {
	start := -1
	_ = ast.Walk(node, func(child ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering || start >= 0 {
			return ast.WalkContinue, nil
		}
		if segment, ok := child.(*ast.Text); ok {
			start = segment.Segment.Start
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
	for parent := node.Parent(); start < 0 && parent != nil; parent = parent.Parent() {
		if lines := parent.Lines(); lines != nil && lines.Len() > 0 {
			start = lines.At(0).Start
		}
	}
	if start < 0 || start > len(source) {
		return 1
	}
	return bytes.Count(source[:start], []byte{'\n'}) + 1
}

func githubSlug(heading string) string {
	var slug strings.Builder
	for _, r := range strings.TrimSpace(heading) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			slug.WriteRune(unicode.ToLower(r))
		case unicode.IsSpace(r):
			slug.WriteByte('-')
		case r == '-' || r == '_':
			slug.WriteRune(r)
		}
	}
	return slug.String()
}

// findMarkdown returns every tracked Markdown file, including files under
// hidden directories. Git is the repository inventory authority.
func findMarkdown(ctx context.Context, root string) ([]string, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("absolute repository root: %w", err)
	}
	cmd := exec.CommandContext(ctx, "git", "-C", absRoot, "ls-files", "-z", "--")
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("git ls-files: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	var files []string
	for raw := range bytes.SplitSeq(out, []byte{0}) {
		if len(raw) == 0 || !strings.EqualFold(filepath.Ext(string(raw)), ".md") {
			continue
		}
		files = append(files, filepath.Join(absRoot, filepath.FromSlash(string(raw))))
	}
	sort.Strings(files)
	return files, nil
}

func repositoryRoot(ctx context.Context, dir string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "-C", dir, "rev-parse", "--show-toplevel")
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	output, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", fmt.Errorf("git repository root: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return filepath.Clean(strings.TrimSpace(string(output))), nil
}

func isExternalLink(target string) bool {
	parsed, err := url.Parse(strings.TrimSpace(target))
	return err == nil && (parsed.Scheme != "" || parsed.Host != "")
}

func (c *linkChecker) checkFile(mdFile string) ([]finding, error) {
	doc, err := c.loadDocument(mdFile)
	if err != nil {
		return nil, err
	}
	rel, err := filepath.Rel(c.root, mdFile)
	if err != nil {
		return nil, fmt.Errorf("relative source path: %w", err)
	}
	rel = filepath.ToSlash(rel)

	var findings []finding
	for _, link := range doc.links {
		target := strings.TrimSpace(link.target)
		if target == "" || isExternalLink(target) {
			continue
		}
		broken, err := c.brokenLocalLink(mdFile, target)
		if err != nil {
			return nil, fmt.Errorf("%s:%d (%s): %w", rel, link.line, target, err)
		}
		if c.verbose {
			fmt.Printf("  check: %s:%d: %s\n", rel, link.line, target)
		}
		if broken {
			findings = append(findings, finding{file: rel, line: link.line, target: target})
		}
	}
	return findings, nil
}

func (c *linkChecker) brokenLocalLink(sourcePath, target string) (bool, error) {
	parsed, err := url.Parse(target)
	if err != nil {
		return false, fmt.Errorf("parse target: %w", err)
	}
	pathPart, err := url.PathUnescape(parsed.Path)
	if err != nil {
		return false, fmt.Errorf("unescape target path: %w", err)
	}
	fragment, err := url.PathUnescape(parsed.Fragment)
	if err != nil {
		return false, fmt.Errorf("unescape target fragment: %w", err)
	}

	targetPath := sourcePath
	if pathPart != "" {
		if strings.HasPrefix(pathPart, "/") {
			targetPath = filepath.Join(c.root, filepath.FromSlash(pathPart[1:]))
		} else {
			targetPath = filepath.Join(filepath.Dir(sourcePath), filepath.FromSlash(pathPart))
		}
	}
	targetPath = filepath.Clean(targetPath)
	inside, err := pathInside(c.root, targetPath)
	if err != nil {
		return false, fmt.Errorf("resolve target path: %w", err)
	}
	if !inside {
		return true, nil
	}
	info, err := os.Stat(targetPath)
	if err != nil {
		if os.IsNotExist(err) {
			return true, nil
		}
		return false, fmt.Errorf("stat target: %w", err)
	}
	if fragment == "" || info.IsDir() || !strings.EqualFold(filepath.Ext(targetPath), ".md") {
		return false, nil
	}
	targetDoc, err := c.loadDocument(targetPath)
	if err != nil {
		return false, err
	}
	return !targetDoc.anchors[fragment], nil
}

func pathInside(root, target string) (bool, error) {
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return false, err
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)), nil
}

func findingKey(file, target string) string {
	return filepath.ToSlash(filepath.Clean(file)) + "\t" + strings.TrimSpace(target)
}

func loadBaseline(path string) (map[string]bool, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return parseBaseline(content, path)
}

func parseBaseline(content []byte, source string) (map[string]bool, error) {
	baseline := map[string]bool{}
	for lineNumber, raw := range strings.Split(string(content), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(raw, "\t", 2)
		if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
			return nil, fmt.Errorf("%s:%d: expected source<TAB>target", source, lineNumber+1)
		}
		key := findingKey(strings.TrimSpace(parts[0]), parts[1])
		if baseline[key] {
			return nil, fmt.Errorf("%s:%d: duplicate entry %q", source, lineNumber+1, key)
		}
		baseline[key] = true
	}
	return baseline, nil
}

func loadBaselineAtRef(ctx context.Context, root, ref, baselinePath string) (map[string]bool, bool, error) {
	verify := exec.CommandContext(ctx, "git", "-C", root, "rev-parse", "--verify", ref+"^{commit}")
	verify.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	if output, err := verify.CombinedOutput(); err != nil {
		if ctx.Err() != nil {
			return nil, false, ctx.Err()
		}
		return nil, false, fmt.Errorf("resolve baseline ref %q: %w: %s", ref, err, strings.TrimSpace(string(output)))
	}
	object := ref + ":" + filepath.ToSlash(baselinePath)
	exists := exec.CommandContext(ctx, "git", "-C", root, "cat-file", "-e", object)
	exists.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	if err := exists.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, false, ctx.Err()
		}
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("inspect baseline at %q: %w", ref, err)
	}
	show := exec.CommandContext(ctx, "git", "-C", root, "show", object)
	show.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	content, err := show.Output()
	if err != nil {
		if ctx.Err() != nil {
			return nil, false, ctx.Err()
		}
		return nil, false, fmt.Errorf("read baseline at %q: %w", ref, err)
	}
	baseline, err := parseBaseline(content, object)
	return baseline, true, err
}

func addedBaselineEntries(current, base map[string]bool) []string {
	var added []string
	for key := range current {
		if !base[key] {
			added = append(added, key)
		}
	}
	sort.Strings(added)
	return added
}

func applyBaseline(findings []finding, baseline map[string]bool) ([]finding, []string, int) {
	current := make(map[string]bool, len(findings))
	var outstanding []finding
	for _, item := range findings {
		key := findingKey(item.file, item.target)
		current[key] = true
		if !baseline[key] {
			outstanding = append(outstanding, item)
		}
	}
	var stale []string
	for key := range baseline {
		if !current[key] {
			stale = append(stale, key)
		}
	}
	sort.Strings(stale)
	matched := 0
	for key := range current {
		if baseline[key] {
			matched++
		}
	}
	return outstanding, stale, matched
}
