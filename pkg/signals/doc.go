// Package signals implements signal detection for Hybrid Progressive Rigor.
//
// It analyzes context to detect complexity signals from keywords, effort
// estimates, file types, bead counts, previous phases, and user history.
// Detected signals are fused with confidence scoring to recommend an
// appropriate rigor level (minimal, standard, thorough, or comprehensive).
package signals
