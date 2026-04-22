// Package engram defines core types for memory traces (engrams) and provides
// parsing utilities for .ai.md files.
//
// An engram is a unit of learned knowledge stored as a markdown file with YAML
// frontmatter. Engrams capture patterns, strategies, and workflows that AI agents
// can retrieve and apply to new contexts.
//
// Engram file structure (.ai.md):
//
//	---
//	type: pattern
//	title: Error Handling in Go
//	description: Idiomatic error handling patterns
//	tags: [languages/go, patterns/errors]
//	agents: [claude-code, cursor]
//	load_when: "Working with Go error handling"
//	---
//	# Error Handling
//
//	Prefer explicit error returns over exceptions...
//
// Engram types:
//   - Pattern: Reusable code patterns and idioms
//   - Strategy: High-level approaches to solving problems
//   - Workflow: Multi-step processes and procedures
//
// The parser reads .ai.md files and extracts both frontmatter metadata
// and markdown content. Engrams are then indexed and retrieved by the
// ecphory system based on query relevance.
//
// Example usage:
//
//	parser := engram.NewParser()
//	eng, err := parser.Parse("engrams/go-errors.ai.md")
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	fmt.Println(eng.Frontmatter.Title)
//	fmt.Println(eng.Content)
package engram

import "time"

// Engram represents a single .ai.md memory trace file
type Engram struct {
	// File path (for tracking purposes)
	Path string

	// Frontmatter metadata
	Frontmatter Frontmatter

	// Content body (markdown)
	Content string
}

// Frontmatter contains engram metadata
type Frontmatter struct {
	// Engram type (pattern, strategy, workflow)
	Type string `yaml:"type"`

	// Title/name
	Title string `yaml:"title"`

	// Description
	Description string `yaml:"description"`

	// Hierarchical tags (e.g., languages/python, frameworks/fastapi)
	Tags []string `yaml:"tags"`

	// Agent platforms this engram applies to (empty = all)
	Agents []string `yaml:"agents,omitempty"`

	// Optional load_when condition (natural language)
	LoadWhen string `yaml:"load_when,omitempty"`

	// Last modified timestamp
	Modified time.Time `yaml:"modified,omitempty"`

	// Memory Strength Tracking Fields

	// EncodingStrength represents the intrinsic quality/importance of this engram.
	// Range: 0.0 (low quality) to 2.0 (exceptional quality)
	// Default: 1.0 (neutral/average quality)
	// Future: May be user-editable or ML-calculated
	EncodingStrength float64 `yaml:"encoding_strength,omitempty"`

	// RetrievalCount tracks how many times this engram has been successfully retrieved.
	// Incremented each time the engram is returned in a query result.
	// Used for: Usage analytics, prioritization, active forgetting decisions
	RetrievalCount int `yaml:"retrieval_count,omitempty"`

	// CreatedAt is the timestamp when this engram was first created.
	// For new engrams: Set to current time on first parse
	// For legacy engrams: Falls back to file mtime
	// Immutable after initialization
	CreatedAt time.Time `yaml:"created_at,omitempty"`

	// LastAccessed is the timestamp of the most recent retrieval.
	// Updated each time the engram is returned in a query result.
	// Used for: Temporal decay calculations, recency-based prioritization
	LastAccessed time.Time `yaml:"last_accessed,omitempty"`

	// Triggers defines event-driven injection rules (optional).
	Triggers []TriggerSpec `yaml:"triggers,omitempty"`
}

// EngramType constants
const (
	TypePattern  = "pattern"
	TypeStrategy = "strategy"
	TypeWorkflow = "workflow"
)
