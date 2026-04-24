package manifest

// Store is the interface for session manifest persistence.
// Dolt is the canonical implementation.
type Store interface {
	// Create inserts a new manifest.
	Create(manifest *Manifest) error

	// Get retrieves a manifest by session ID.
	Get(sessionID string) (*Manifest, error)

	// Update modifies an existing manifest.
	Update(manifest *Manifest) error

	// Delete removes a manifest by session ID.
	Delete(sessionID string) error

	// List returns manifests matching the filter.
	List(filter *Filter) ([]*Manifest, error)

	// Close closes the storage connection.
	Close() error

	// ApplyMigrations applies database schema migrations.
	ApplyMigrations() error
}

// Filter specifies criteria for listing manifests.
type Filter struct {
	Workspace string
	Status    string   // "active", "archived"
	Harness   string
	Tags      []string
	Limit     int
	Offset    int
}
