// Package document defines Engram's stateless knowledge layer.
//
// Engram has two distinct memory layers that were historically conflated:
//
//   - Documents (this package): stateless, immutable, versioned knowledge
//     blobs — specs, architecture docs, research findings, reference material.
//     A Document is authored content that is trusted as-is and never mutated
//     in place. Editing a Document produces a new, immutable version; old
//     versions are retained. Documents carry no decay, importance score, or
//     consolidation lifecycle.
//
//   - Memories (see internal/consolidation): extracted, mutable facts derived
//     from session history. A Memory is a small claim ("the user prefers
//     concise responses") that is learned, updated, merged, and pruned by the
//     hippocampus consolidation pipeline.
//
// Keeping the layers separate sharpens recall precision and prevents stale
// extracted facts from contaminating canonical reference knowledge. This maps
// to the supermemory OSS pattern of separating documents (source-of-truth
// blobs) from memories (distilled, evolving facts).
package document

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"
)

// SchemaVersion is the current Document schema version. It enables schema
// evolution and backward-compatible reads of older stored documents.
const SchemaVersion = "1.0"

// Kind categorizes a document by the type of knowledge it holds.
//
// Unlike consolidation.MemoryType (which describes a cognitive function),
// Kind describes the editorial role of an authored reference blob.
type Kind string

const (
	// KindSpec is a product or functional specification.
	KindSpec Kind = "spec"

	// KindArchitecture is an architecture or design document.
	KindArchitecture Kind = "architecture"

	// KindResearch is a research finding or investigation writeup.
	KindResearch Kind = "research"

	// KindReference is general reference material (APIs, runbooks, guides).
	KindReference Kind = "reference"

	// KindADR is an Architecture Decision Record.
	KindADR Kind = "adr"
)

// validKinds is the set of recognized document kinds.
var validKinds = map[Kind]struct{}{
	KindSpec:         {},
	KindArchitecture: {},
	KindResearch:     {},
	KindReference:    {},
	KindADR:          {},
}

// Valid reports whether k is a recognized document kind.
func (k Kind) Valid() bool {
	_, ok := validKinds[k]
	return ok
}

// Document is a stateless, immutable, versioned knowledge blob.
//
// A Document is identified by its logical ID (stable across versions) plus a
// monotonic Version number. The (ID, Version) pair is immutable once stored:
// to "edit" a document, a caller stores a new version with the same ID, which
// the store assigns the next version number. This contrasts with
// consolidation.Memory, which is mutated in place via partial updates.
type Document struct {
	// SchemaVersion indicates the document schema version (e.g. "1.0").
	SchemaVersion string `json:"schema_version"`

	// ID is the stable logical identifier shared by every version of this
	// document (e.g. "engram-spec").
	ID string `json:"id"`

	// Version is the monotonic, 1-based version number. The store assigns it
	// on Put; callers leave it zero.
	Version int `json:"version"`

	// Kind categorizes the document (spec, architecture, research, ...).
	Kind Kind `json:"kind"`

	// Namespace scopes the document hierarchically, mirroring the memory
	// layer's namespacing (e.g. ["project", "engram"]).
	Namespace []string `json:"namespace"`

	// Title is a short human-readable label.
	Title string `json:"title"`

	// Content is the canonical knowledge blob (typically markdown or text).
	Content string `json:"content"`

	// ContentHash is the lowercase hex SHA-256 of Content. The store computes
	// it on Put; it enables integrity checks and dedup of identical content.
	ContentHash string `json:"content_hash"`

	// Source records provenance (file path, URL, or generating session).
	Source string `json:"source,omitempty"`

	// Metadata stores additional key-value attributes.
	Metadata map[string]any `json:"metadata,omitempty"`

	// CreatedAt is when this version was stored.
	CreatedAt time.Time `json:"created_at"`
}

// HashContent returns the lowercase hex SHA-256 of the given content.
func HashContent(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

// Filter narrows a List query over documents.
//
// A zero Filter matches every document in the namespace.
type Filter struct {
	// Kind, when non-empty, restricts results to documents of this kind.
	Kind Kind

	// TitleContains, when non-empty, restricts results to documents whose
	// title contains this substring (case-insensitive).
	TitleContains string

	// Limit, when > 0, caps the number of results returned.
	Limit int
}

// Sentinel errors returned by Store implementations. Callers should compare
// with errors.Is rather than matching error strings.
var (
	// ErrNotFound indicates the requested document or version does not exist.
	ErrNotFound = errors.New("document: not found")

	// ErrInvalidDocument indicates the supplied document failed validation.
	ErrInvalidDocument = errors.New("document: invalid document")

	// ErrInvalidNamespace indicates an empty or unsafe namespace.
	ErrInvalidNamespace = errors.New("document: invalid namespace")

	// ErrInvalidID indicates an empty or unsafe document ID.
	ErrInvalidID = errors.New("document: invalid id")
)
