package retrieval

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/vbonnet/dear-agent/engram/ecphory"
	"github.com/vbonnet/dear-agent/internal/testutil"
)

// TestNewService tests the Service constructor
func TestNewService(t *testing.T) {
	service := NewService()
	if service == nil {
		t.Fatal("NewService() returned nil")
	}
	if service.parser == nil {
		t.Error("NewService() did not initialize parser")
	}
}

// TestService_LimitResults tests the limitResults helper method
func TestService_LimitResults(t *testing.T) {
	service := NewService()

	tests := []struct {
		name       string
		candidates []string
		limit      int
		want       int // expected result length
	}{
		{
			name:       "no limit (0)",
			candidates: []string{"a", "b", "c"},
			limit:      0,
			want:       3,
		},
		{
			name:       "limit greater than length",
			candidates: []string{"a", "b"},
			limit:      5,
			want:       2,
		},
		{
			name:       "limit less than length",
			candidates: []string{"a", "b", "c", "d"},
			limit:      2,
			want:       2,
		},
		{
			name:       "limit equals length",
			candidates: []string{"a", "b"},
			limit:      2,
			want:       2,
		},
		{
			name:       "negative limit returns all",
			candidates: []string{"a", "b", "c"},
			limit:      -1,
			want:       3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := service.limitResults(tt.candidates, tt.limit)
			if len(got) != tt.want {
				t.Errorf("limitResults() returned %d results, want %d", len(got), tt.want)
			}

			// Verify the returned slice contains correct elements
			if tt.limit > 0 && tt.limit < len(tt.candidates) {
				// Should match first N elements
				for i := 0; i < tt.want; i++ {
					if got[i] != tt.candidates[i] {
						t.Errorf("limitResults()[%d] = %s, want %s", i, got[i], tt.candidates[i])
					}
				}
			}
		})
	}
}

// TestService_ResolveEngramPath tests path resolution logic
func TestService_ResolveEngramPath(t *testing.T) {
	service := NewService()

	tests := []struct {
		name    string
		path    string
		setup   func(t *testing.T) string
		wantErr bool
	}{
		{
			name: "absolute path exists",
			setup: func(t *testing.T) string {
				tmpdir := t.TempDir()
				t.Cleanup(func() { os.RemoveAll(tmpdir) })
				return tmpdir
			},
			wantErr: false,
		},
		{
			name:    "absolute path not found",
			path:    "/nonexistent/path/engrams",
			wantErr: true,
		},
		{
			name: "relative path from cwd",
			setup: func(t *testing.T) string {
				cwd, err := os.Getwd()
				if err != nil {
					t.Fatalf("failed to get cwd: %v", err)
				}
				tmpdir := filepath.Join(cwd, "test-engrams-relative")
				if err := os.MkdirAll(tmpdir, 0755); err != nil {
					t.Fatalf("setup failed: %v", err)
				}
				t.Cleanup(func() { os.RemoveAll(tmpdir) })
				return "test-engrams-relative"
			},
			wantErr: false,
		},
		{
			name:    "empty path uses default",
			path:    "",
			wantErr: false, // Will try default paths, may or may not exist
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := tt.path
			if tt.setup != nil {
				path = tt.setup(t)
			}

			got, err := service.resolveEngramPath(path)
			if (err != nil) != tt.wantErr {
				t.Errorf("resolveEngramPath() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if got == "" {
					t.Error("resolveEngramPath() returned empty path, want valid path")
				}
				// Verify the path exists
				if _, err := os.Stat(got); os.IsNotExist(err) {
					t.Errorf("resolveEngramPath() returned non-existent path: %s", got)
				}
			}
		})
	}
}

// TestService_FilterCandidates tests tag/type filtering logic
func TestService_FilterCandidates(t *testing.T) {
	service := NewService()
	tmpdir := testutil.SetupTestEngrams(t)

	// Build index
	index := ecphory.NewIndex()
	if err := index.Build(tmpdir); err != nil {
		t.Fatalf("failed to build index: %v", err)
	}

	tests := []struct {
		name string
		opts SearchOptions
		want int // expected result count
	}{
		{
			name: "filter by single tag",
			opts: SearchOptions{Tags: []string{"go"}},
			want: 3, // pattern1, pattern2, strategy1
		},
		{
			name: "filter by multiple tags (OR)",
			opts: SearchOptions{Tags: []string{"go", "python"}},
			want: 4, // pattern1, pattern2, strategy1, workflow1
		},
		{
			name: "filter by type pattern",
			opts: SearchOptions{Type: "pattern"},
			want: 3, // pattern1, pattern2, pattern3
		},
		{
			name: "filter by type workflow",
			opts: SearchOptions{Type: "workflow"},
			want: 1, // workflow1
		},
		{
			name: "no filters returns all",
			opts: SearchOptions{},
			want: 5, // all engrams
		},
		{
			name: "empty tags returns all",
			opts: SearchOptions{Tags: []string{}},
			want: 5,
		},
		{
			name: "non-existent tag",
			opts: SearchOptions{Tags: []string{"nonexistent"}},
			want: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := service.filterCandidates(index, tt.opts)
			if len(got) != tt.want {
				t.Errorf("filterCandidates() returned %d results, want %d", len(got), tt.want)
			}
		})
	}
}

// TestService_Search_IndexOnly tests search without API ranking
func TestService_Search_IndexOnly(t *testing.T) {
	service := NewService()

	tests := []struct {
		name    string
		opts    SearchOptions
		setup   func(t *testing.T) SearchOptions
		want    int // expected result count
		wantErr bool
	}{
		{
			name: "valid search with tags",
			setup: func(t *testing.T) SearchOptions {
				tmpdir := testutil.SetupTestEngrams(t)
				return SearchOptions{
					EngramPath: tmpdir,
					Tags:       []string{"go"},
					Limit:      10,
				}
			},
			want:    3, // pattern1, pattern2, strategy1
			wantErr: false,
		},
		{
			name: "valid search with type",
			setup: func(t *testing.T) SearchOptions {
				tmpdir := testutil.SetupTestEngrams(t)
				return SearchOptions{
					EngramPath: tmpdir,
					Type:       "pattern",
					Limit:      10,
				}
			},
			want:    3, // pattern1, pattern2, pattern3
			wantErr: false,
		},
		{
			name: "search with no results",
			setup: func(t *testing.T) SearchOptions {
				tmpdir := testutil.SetupTestEngrams(t)
				return SearchOptions{
					EngramPath: tmpdir,
					Tags:       []string{"nonexistent"},
					Limit:      10,
				}
			},
			want:    0,
			wantErr: false,
		},
		{
			name: "search with limit",
			setup: func(t *testing.T) SearchOptions {
				tmpdir := testutil.SetupTestEngrams(t)
				return SearchOptions{
					EngramPath: tmpdir,
					Limit:      2,
				}
			},
			want:    2,
			wantErr: false,
		},
		{
			name: "search all engrams (no filters)",
			setup: func(t *testing.T) SearchOptions {
				tmpdir := testutil.SetupTestEngrams(t)
				return SearchOptions{
					EngramPath: tmpdir,
					Limit:      10,
				}
			},
			want:    5,
			wantErr: false,
		},
		{
			name: "invalid engram path",
			opts: SearchOptions{
				EngramPath: "/nonexistent/path",
			},
			wantErr: true,
		},
		{
			name: "empty directory returns no results",
			setup: func(t *testing.T) SearchOptions {
				tmpdir, _ := os.MkdirTemp("", "empty-*")
				t.Cleanup(func() { os.RemoveAll(tmpdir) })
				return SearchOptions{
					EngramPath: tmpdir,
					Limit:      10,
				}
			},
			want:    0,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := tt.opts
			if tt.setup != nil {
				opts = tt.setup(t)
			}

			results, err := service.Search(context.Background(), opts)
			if (err != nil) != tt.wantErr {
				t.Errorf("Search() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr {
				return
			}

			if len(results) != tt.want {
				t.Errorf("Search() returned %d results, want %d", len(results), tt.want)
			}

			// Verify result structure
			for i, result := range results {
				if result == nil {
					t.Errorf("Search() result[%d] is nil", i)
					continue
				}
				if result.Path == "" {
					t.Errorf("Search() result[%d].Path is empty", i)
				}
				if result.Engram == nil {
					t.Errorf("Search() result[%d].Engram is nil", i)
				}
				// Index-only search should not have scores/rankings
				if result.Score != 0 {
					t.Errorf("Search() result[%d].Score = %f, want 0 (index-only)", i, result.Score)
				}
				if result.Ranking != "" {
					t.Errorf("Search() result[%d].Ranking = %q, want empty (index-only)", i, result.Ranking)
				}
			}
		})
	}
}

// TestService_Search_WithAPIFallback tests API ranking fallback behavior
func TestService_Search_WithAPIFallback(t *testing.T) {
	service := NewService()

	t.Run("api key missing fallback", func(t *testing.T) {
		// Ensure ANTHROPIC_API_KEY is not set
		originalKey := os.Getenv("ANTHROPIC_API_KEY")
		os.Unsetenv("ANTHROPIC_API_KEY")
		defer func() {
			if originalKey != "" {
				t.Setenv("ANTHROPIC_API_KEY", originalKey)
			}
		}()

		tmpdir := testutil.SetupTestEngrams(t)
		opts := SearchOptions{
			EngramPath: tmpdir,
			UseAPI:     true, // Request API but will fallback
			Limit:      5,
		}

		results, err := service.Search(context.Background(), opts)
		if err != nil {
			t.Fatalf("Search() with API fallback failed: %v", err)
		}

		// Should still get results via index-only fallback
		if len(results) == 0 {
			t.Error("Search() with API fallback returned no results, expected fallback to index")
		}

		// Verify no scores/rankings (index-only fallback)
		for i, r := range results {
			if r.Score != 0 || r.Ranking != "" {
				t.Errorf("result[%d] has Score=%f or Ranking=%q, expected empty (fallback mode)",
					i, r.Score, r.Ranking)
			}
		}
	})

	t.Run("useAPI false skips ranking", func(t *testing.T) {
		tmpdir := testutil.SetupTestEngrams(t)
		opts := SearchOptions{
			EngramPath: tmpdir,
			UseAPI:     false, // Explicitly skip API
			Limit:      3,
		}

		results, err := service.Search(context.Background(), opts)
		if err != nil {
			t.Fatalf("Search() with UseAPI=false failed: %v", err)
		}

		if len(results) == 0 {
			t.Error("Search() with UseAPI=false returned no results")
		}

		// Verify index-only results
		for i, r := range results {
			if r.Score != 0 || r.Ranking != "" {
				t.Errorf("result[%d] has Score=%f or Ranking=%q, expected empty (API disabled)",
					i, r.Score, r.Ranking)
			}
		}
	})
}

// TestService_Search_ParseErrors tests handling of unparseable engrams
func TestService_Search_ParseErrors(t *testing.T) {
	service := NewService()

	t.Run("skip unparseable engrams", func(t *testing.T) {
		// Create temp directory
		tmpdir := t.TempDir()
		t.Cleanup(func() { os.RemoveAll(tmpdir) })

		// Create valid engram
		testutil.CreateTestEngram(t, tmpdir, "valid.ai.md", "pattern", []string{"go"})

		// Create invalid engram (malformed YAML)
		invalidPath := filepath.Join(tmpdir, "invalid.ai.md")
		invalidContent := `---
title: Invalid
this is not valid yaml!!!
---

Content
`
		if err := os.WriteFile(invalidPath, []byte(invalidContent), 0644); err != nil {
			t.Fatalf("failed to create invalid engram: %v", err)
		}

		opts := SearchOptions{
			EngramPath: tmpdir,
			Limit:      10,
		}

		results, err := service.Search(context.Background(), opts)
		if err != nil {
			t.Fatalf("Search() failed: %v", err)
		}

		// Should get only the valid engram
		if len(results) != 1 {
			t.Errorf("Search() returned %d results, expected 1 (invalid should be skipped)", len(results))
		}
	})
}

// TestService_ResolveEngramPath_DefaultPaths tests default path resolution
func TestService_ResolveEngramPath_DefaultPaths(t *testing.T) {
	service := NewService()

	t.Run("create default path if it exists", func(t *testing.T) {
		// This test checks the default ~/.engram/core/engrams path behavior
		// We'll create it temporarily if it doesn't exist
		home, err := os.UserHomeDir()
		if err != nil {
			t.Skip("cannot get home directory")
		}

		defaultPath := filepath.Join(home, ".engram/core/engrams")
		pathExists := false
		if _, err := os.Stat(defaultPath); err == nil {
			pathExists = true
		} else {
			// Create temporarily for test
			if err := os.MkdirAll(defaultPath, 0755); err != nil {
				t.Skip("cannot create default path for test")
			}
			t.Cleanup(func() {
				// Only remove if we created it
				os.RemoveAll(filepath.Join(home, ".engram"))
			})
		}

		// Test empty string uses default
		got, err := service.resolveEngramPath("")
		if err != nil {
			// If default doesn't exist and can't be created, that's okay
			if !pathExists {
				return
			}
			t.Errorf("resolveEngramPath(\"\") failed: %v", err)
			return
		}

		// Should resolve to some valid path
		if got == "" {
			t.Error("resolveEngramPath(\"\") returned empty string")
		}
	})
}

// TestService_Search_WithQuery tests search with Query parameter
func TestService_Search_WithQuery(t *testing.T) {
	service := NewService()

	t.Run("search with query parameter set", func(t *testing.T) {
		tmpdir := testutil.SetupTestEngrams(t)
		opts := SearchOptions{
			EngramPath: tmpdir,
			Query:      "error handling", // Query is set but won't be used without API
			Limit:      10,
		}

		results, err := service.Search(context.Background(), opts)
		if err != nil {
			t.Fatalf("Search() with query failed: %v", err)
		}

		// Should still return results (query ignored without API)
		if len(results) == 0 {
			t.Error("Search() with query returned no results")
		}
	})

	t.Run("search with query and useAPI but no key", func(t *testing.T) {
		// Ensure no API key
		t.Setenv("ANTHROPIC_API_KEY", "") // restored on test cleanup
		os.Unsetenv("ANTHROPIC_API_KEY")

		tmpdir := testutil.SetupTestEngrams(t)
		opts := SearchOptions{
			EngramPath: tmpdir,
			Query:      "test query",
			UseAPI:     true, // Request API but will fallback
			Limit:      5,
		}

		results, err := service.Search(context.Background(), opts)
		if err != nil {
			t.Fatalf("Search() failed: %v", err)
		}

		// Should fallback to index-only
		if len(results) == 0 {
			t.Error("expected fallback results")
		}
	})
}
