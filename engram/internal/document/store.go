package document

import "context"

// Store persists and retrieves stateless, versioned documents.
//
// The interface is deliberately narrower than consolidation.Provider: there is
// no in-place update and no append. Documents are immutable once stored, so the
// only way to change one is Put, which appends a new version. This is the
// load-bearing distinction between the document layer (canonical, trusted,
// versioned reference knowledge) and the memory layer (distilled, mutable,
// decaying facts).
//
// All operations are context-aware for cancellation and timeout support.
type Store interface {
	// Put stores doc as a new immutable version under (namespace, doc.ID).
	//
	// The store assigns Version (next number for the ID, starting at 1),
	// computes ContentHash, defaults SchemaVersion and CreatedAt, and returns
	// the stored document. The caller's Version and ContentHash fields are
	// ignored. Existing versions are never modified.
	Put(ctx context.Context, namespace []string, doc Document) (Document, error)

	// Get returns the latest version of the document with the given ID.
	// Returns ErrNotFound if no version exists.
	Get(ctx context.Context, namespace []string, id string) (Document, error)

	// GetVersion returns a specific version of a document.
	// Returns ErrNotFound if that version does not exist.
	GetVersion(ctx context.Context, namespace []string, id string, version int) (Document, error)

	// ListVersions returns every stored version of a document, oldest first.
	// Returns an empty slice (not an error) if the document does not exist.
	ListVersions(ctx context.Context, namespace []string, id string) ([]Document, error)

	// List returns the latest version of each document in the namespace that
	// matches filter, sorted by ID. Returns an empty slice if the namespace
	// is empty or does not exist.
	List(ctx context.Context, namespace []string, filter Filter) ([]Document, error)

	// Delete removes every version of the document with the given ID.
	// Returns ErrNotFound if the document does not exist.
	Delete(ctx context.Context, namespace []string, id string) error
}
