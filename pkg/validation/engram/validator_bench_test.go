package engram

import (
	"os"
	"testing"
)

// Benchmark validating a single file
func BenchmarkValidateFile(b *testing.B) {
	tmpFile := createBenchFile(b, validEngram)
	defer os.Remove(tmpFile) //nolint:errcheck // Cleanup in benchmarks

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ValidateFile(tmpFile)
	}
}

// Benchmark with context references (more regex matching)
func BenchmarkValidateFileWithErrors(b *testing.B) {
	tmpFile := createBenchFile(b, contextReferences)
	defer os.Remove(tmpFile) //nolint:errcheck // Cleanup in benchmarks

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ValidateFile(tmpFile)
	}
}

// Benchmark frontmatter parsing
func BenchmarkValidateFrontmatter(b *testing.B) {
	validator := NewValidator("/dev/null")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		validator.validateFrontmatter(validEngram)
	}
}

// Benchmark context reference detection
func BenchmarkDetectContextReferences(b *testing.B) {
	validator := NewValidator("/dev/null")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		validator.detectContextReferences(contextReferences)
	}
}

// Benchmark vague verb detection
func BenchmarkDetectVagueVerbs(b *testing.B) {
	validator := NewValidator("/dev/null")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		validator.detectVagueVerbs(vagueVerbsFixture)
	}
}

// Helper function for benchmarks
func createBenchFile(b *testing.B, content string) string {
	b.Helper()
	tmpFile, err := os.CreateTemp(b.TempDir(), "engram-bench-*.ai.md")
	if err != nil {
		b.Fatalf("Failed to create temp file: %v", err)
	}
	if _, err := tmpFile.WriteString(content); err != nil {
		b.Fatalf("Failed to write temp file: %v", err)
	}
	if err := tmpFile.Close(); err != nil {
		b.Fatalf("Failed to close temp file: %v", err)
	}
	return tmpFile.Name()
}
