package web

import (
	"encoding/json"
	"time"
)

// Content represents extracted web content
type Content struct {
	// Title is the article title
	Title string

	// Authors is the list of article authors (comma-separated)
	Authors string

	// Abstract is the article abstract or excerpt
	Abstract string

	// Content is the main article content (plain text)
	Content string

	// Metadata contains additional metadata
	Metadata map[string]string

	// Upvotes is the number of upvotes (HuggingFace papers)
	Upvotes int

	// Collections are the collections the paper belongs to (HuggingFace papers)
	Collections []string

	// PublishedDate is the publication date
	PublishedDate time.Time
}

// HuggingFaceAPIResponse represents the response from the HuggingFace API
type HuggingFaceAPIResponse struct {
	ID          string          `json:"id"`
	Title       string          `json:"title"`
	Authors     json.RawMessage `json:"authors"` // Can be array or object
	Abstract    string          `json:"abstract"`
	Upvotes     int             `json:"upvotes"`
	Collections []string        `json:"collections"`
	PublishedAt string          `json:"publishedAt"`
}
