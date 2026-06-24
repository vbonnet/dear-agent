package document

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// FSStore is a filesystem-backed Store reference implementation.
//
// Layout: each document gets its own directory and each version is a separate
// immutable JSON file:
//
//	{root}/{namespace...}/{id}/v{N}.json
//
// One file per version (rather than a single mutable file) makes immutability
// the natural state on disk and keeps version history git-friendly. It is not
// optimized for production (no caching, full directory scans on list).
type FSStore struct {
	root string
}

// NewFSStore creates a filesystem document store rooted at root.
// The directory is created lazily on first write.
func NewFSStore(root string) (*FSStore, error) {
	if strings.TrimSpace(root) == "" {
		return nil, fmt.Errorf("%w: empty root", ErrInvalidDocument)
	}
	return &FSStore{root: root}, nil
}

const versionPrefix = "v"

// Put stores doc as the next version under (namespace, doc.ID).
func (s *FSStore) Put(ctx context.Context, namespace []string, doc Document) (Document, error) {
	if err := ctx.Err(); err != nil {
		return Document{}, err
	}
	if err := validateNamespace(namespace); err != nil {
		return Document{}, fmt.Errorf("put document: %w", err)
	}
	if err := validateID(doc.ID); err != nil {
		return Document{}, fmt.Errorf("put document: %w", err)
	}
	if doc.Kind != "" && !doc.Kind.Valid() {
		return Document{}, fmt.Errorf("put document: %w: unknown kind %q", ErrInvalidDocument, doc.Kind)
	}

	dir, err := s.docDir(namespace, doc.ID)
	if err != nil {
		return Document{}, fmt.Errorf("put document: %w", err)
	}

	next, err := s.nextVersion(dir)
	if err != nil {
		return Document{}, fmt.Errorf("put document: %w", err)
	}

	// Stamp store-owned fields. Caller-supplied Version/ContentHash are ignored
	// so versions stay monotonic and hashes stay trustworthy.
	doc.SchemaVersion = SchemaVersion
	doc.Version = next
	doc.Namespace = append([]string(nil), namespace...)
	doc.ContentHash = HashContent(doc.Content)
	if doc.CreatedAt.IsZero() {
		doc.CreatedAt = time.Now().UTC()
	}

	if err := os.MkdirAll(dir, 0o700); err != nil {
		return Document{}, fmt.Errorf("put document: create directory: %w", err)
	}

	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return Document{}, fmt.Errorf("put document: serialize: %w", err)
	}

	path := filepath.Join(dir, versionFile(next))
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return Document{}, fmt.Errorf("put document: write file: %w", err)
	}

	return doc, nil
}

// Get returns the latest version of a document.
func (s *FSStore) Get(ctx context.Context, namespace []string, id string) (Document, error) {
	if err := ctx.Err(); err != nil {
		return Document{}, err
	}
	versions, err := s.ListVersions(ctx, namespace, id)
	if err != nil {
		return Document{}, err
	}
	if len(versions) == 0 {
		return Document{}, fmt.Errorf("get document: %w", ErrNotFound)
	}
	return versions[len(versions)-1], nil
}

// GetVersion returns a specific version of a document.
func (s *FSStore) GetVersion(ctx context.Context, namespace []string, id string, version int) (Document, error) {
	if err := ctx.Err(); err != nil {
		return Document{}, err
	}
	if err := validateNamespace(namespace); err != nil {
		return Document{}, fmt.Errorf("get document version: %w", err)
	}
	if err := validateID(id); err != nil {
		return Document{}, fmt.Errorf("get document version: %w", err)
	}
	if version <= 0 {
		return Document{}, fmt.Errorf("get document version: %w: version must be positive", ErrInvalidDocument)
	}

	dir, err := s.docDir(namespace, id)
	if err != nil {
		return Document{}, fmt.Errorf("get document version: %w", err)
	}

	doc, err := readDocument(filepath.Join(dir, versionFile(version)))
	if err != nil {
		return Document{}, fmt.Errorf("get document version: %w", err)
	}
	return doc, nil
}

// ListVersions returns every version of a document, oldest first.
func (s *FSStore) ListVersions(ctx context.Context, namespace []string, id string) ([]Document, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := validateNamespace(namespace); err != nil {
		return nil, fmt.Errorf("list document versions: %w", err)
	}
	if err := validateID(id); err != nil {
		return nil, fmt.Errorf("list document versions: %w", err)
	}

	dir, err := s.docDir(namespace, id)
	if err != nil {
		return nil, fmt.Errorf("list document versions: %w", err)
	}

	if _, statErr := os.Stat(dir); os.IsNotExist(statErr) {
		return []Document{}, nil
	}

	files, err := filepath.Glob(filepath.Join(dir, versionPrefix+"*.json"))
	if err != nil {
		return nil, fmt.Errorf("list document versions: scan: %w", err)
	}

	docs := make([]Document, 0, len(files))
	for _, f := range files {
		doc, readErr := readDocument(f)
		if readErr != nil {
			continue // skip unreadable/invalid version files
		}
		docs = append(docs, doc)
	}

	sort.Slice(docs, func(i, j int) bool { return docs[i].Version < docs[j].Version })
	return docs, nil
}

// List returns the latest version of each document matching filter.
func (s *FSStore) List(ctx context.Context, namespace []string, filter Filter) ([]Document, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := validateNamespace(namespace); err != nil {
		return nil, fmt.Errorf("list documents: %w", err)
	}

	nsDir := filepath.Join(append([]string{s.root}, namespace...)...)
	entries, err := os.ReadDir(nsDir)
	if os.IsNotExist(err) {
		return []Document{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("list documents: read namespace: %w", err)
	}

	results := make([]Document, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		doc, getErr := s.Get(ctx, namespace, entry.Name())
		if getErr != nil {
			continue // not a valid document directory
		}
		if !matchesFilter(doc, filter) {
			continue
		}
		results = append(results, doc)
	}

	sort.Slice(results, func(i, j int) bool { return results[i].ID < results[j].ID })

	if filter.Limit > 0 && len(results) > filter.Limit {
		results = results[:filter.Limit]
	}
	return results, nil
}

// Delete removes every version of a document.
func (s *FSStore) Delete(ctx context.Context, namespace []string, id string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := validateNamespace(namespace); err != nil {
		return fmt.Errorf("delete document: %w", err)
	}
	if err := validateID(id); err != nil {
		return fmt.Errorf("delete document: %w", err)
	}

	dir, err := s.docDir(namespace, id)
	if err != nil {
		return fmt.Errorf("delete document: %w", err)
	}

	if _, statErr := os.Stat(dir); os.IsNotExist(statErr) {
		return fmt.Errorf("delete document: %w", ErrNotFound)
	}
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("delete document: %w", err)
	}
	return nil
}

func matchesFilter(doc Document, filter Filter) bool {
	if filter.Kind != "" && doc.Kind != filter.Kind {
		return false
	}
	if filter.TitleContains != "" &&
		!strings.Contains(strings.ToLower(doc.Title), strings.ToLower(filter.TitleContains)) {
		return false
	}
	return true
}

// nextVersion returns the version number a new Put should use: one greater than
// the highest existing version, or 1 if none exist.
func (s *FSStore) nextVersion(dir string) (int, error) {
	files, err := filepath.Glob(filepath.Join(dir, versionPrefix+"*.json"))
	if err != nil {
		return 0, err
	}
	highest := 0
	for _, f := range files {
		if v := parseVersion(filepath.Base(f)); v > highest {
			highest = v
		}
	}
	return highest + 1, nil
}

func versionFile(v int) string { return versionPrefix + strconv.Itoa(v) + ".json" }

// parseVersion extracts N from "vN.json"; returns 0 if the name does not match.
func parseVersion(name string) int {
	if !strings.HasPrefix(name, versionPrefix) || !strings.HasSuffix(name, ".json") {
		return 0
	}
	num := strings.TrimSuffix(strings.TrimPrefix(name, versionPrefix), ".json")
	v, err := strconv.Atoi(num)
	if err != nil || v <= 0 {
		return 0
	}
	return v
}

func readDocument(path string) (Document, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return Document{}, ErrNotFound
	}
	if err != nil {
		return Document{}, err
	}
	var doc Document
	if err := json.Unmarshal(data, &doc); err != nil {
		return Document{}, fmt.Errorf("%w: corrupt version file: %w", ErrInvalidDocument, err)
	}
	return doc, nil
}

// docDir builds the directory for a document and verifies it stays inside the
// configured root. namespace and id reach this code from CLI flags, MCP tool
// arguments, and workflow YAML — all treated as hostile — so the post-join
// containment check is the load-bearing defense, intentionally redundant with
// validateNamespace / validateID. Mirrors the simple memory provider.
func (s *FSStore) docDir(namespace []string, id string) (string, error) {
	parts := append([]string{s.root}, namespace...)
	parts = append(parts, id)
	joined := filepath.Join(parts...)

	cleanRoot := filepath.Clean(s.root)
	cleanJoined := filepath.Clean(joined)
	prefix := cleanRoot + string(filepath.Separator)
	if cleanJoined != cleanRoot && !strings.HasPrefix(cleanJoined, prefix) {
		return "", ErrInvalidID
	}
	return joined, nil
}

// validateNamespace rejects empty namespaces and path-traversal components.
func validateNamespace(namespace []string) error {
	if len(namespace) == 0 {
		return ErrInvalidNamespace
	}
	for _, part := range namespace {
		if part == "" || part == "." || part == ".." {
			return ErrInvalidNamespace
		}
		if strings.Contains(part, "..") || strings.ContainsAny(part, "/\\") {
			return ErrInvalidNamespace
		}
		if strings.ContainsRune(part, 0) {
			return ErrInvalidNamespace
		}
	}
	return nil
}

// validateID rejects document IDs that would escape the storage root.
func validateID(id string) error {
	if id == "" || id == "." || id == ".." {
		return ErrInvalidID
	}
	if strings.Contains(id, "..") || strings.ContainsAny(id, "/\\") {
		return ErrInvalidID
	}
	if strings.ContainsRune(id, 0) {
		return ErrInvalidID
	}
	return nil
}

// Ensure FSStore satisfies Store.
var _ Store = (*FSStore)(nil)
