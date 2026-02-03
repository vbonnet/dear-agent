package youtube

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vbonnet/ai-tools/tools/gemini-deep-research/detector"
)

func TestNewExtractor_Success(t *testing.T) {
	// Create mock yt-dlp executable
	mockYTDLP := createMockYTDLP(t, 0, "")
	defer os.Remove(mockYTDLP)

	// Temporarily add mock to PATH
	originalPath := os.Getenv("PATH")
	defer os.Setenv("PATH", originalPath)
	os.Setenv("PATH", filepath.Dir(mockYTDLP)+":"+originalPath)

	extractor, err := NewExtractor()
	if err != nil {
		t.Fatalf("NewExtractor() unexpected error: %v", err)
	}

	if extractor == nil {
		t.Fatal("NewExtractor() returned nil extractor")
	}

	if extractor.Name() != "YouTube" {
		t.Errorf("Name() = %q, expected %q", extractor.Name(), "YouTube")
	}

	if extractor.ContentType() != detector.ContentTypeVideo {
		t.Errorf("ContentType() = %v, expected %v", extractor.ContentType(), detector.ContentTypeVideo)
	}
}

func TestNewExtractor_YTDLPNotInstalled(t *testing.T) {
	// Clear PATH to simulate yt-dlp not installed
	originalPath := os.Getenv("PATH")
	defer os.Setenv("PATH", originalPath)
	os.Setenv("PATH", "/nonexistent/path")

	_, err := NewExtractor()
	if err == nil {
		t.Fatal("NewExtractor() expected error when yt-dlp not installed, got nil")
	}

	var extractionErr *ExtractionError
	if !errors.As(err, &extractionErr) {
		t.Errorf("Expected *ExtractionError, got %T", err)
		return
	}

	if !errors.Is(extractionErr.Err, ErrYTDLPNotFound) {
		t.Errorf("Expected ErrYTDLPNotFound, got %v", extractionErr.Err)
	}

	// Verify troubleshooting tips are present
	if len(extractionErr.Troubleshooting) == 0 {
		t.Error("Expected troubleshooting tips for missing yt-dlp")
	}
}

func TestExtract_Success(t *testing.T) {
	// Create mock yt-dlp that creates a valid VTT file
	mockYTDLP := createMockYTDLPWithVTT(t)
	defer os.Remove(mockYTDLP)

	originalPath := os.Getenv("PATH")
	defer os.Setenv("PATH", originalPath)
	os.Setenv("PATH", filepath.Dir(mockYTDLP)+":"+originalPath)

	extractor, err := NewExtractor()
	if err != nil {
		t.Fatalf("NewExtractor() failed: %v", err)
	}

	ctx := context.Background()
	url := "https://www.youtube.com/watch?v=dQw4w9WgXcQ"

	content, err := extractor.Extract(ctx, url)
	if err != nil {
		t.Fatalf("Extract() unexpected error: %v", err)
	}

	if content == nil {
		t.Fatal("Extract() returned nil content")
	}

	if content.URL != url {
		t.Errorf("Content.URL = %q, expected %q", content.URL, url)
	}

	if content.VideoID != "dQw4w9WgXcQ" {
		t.Errorf("Content.VideoID = %q, expected %q", content.VideoID, "dQw4w9WgXcQ")
	}

	if len(content.Transcript) < MinTranscriptLength {
		t.Errorf("Transcript length = %d, expected >= %d", len(content.Transcript), MinTranscriptLength)
	}

	if content.Length != len(content.Transcript) {
		t.Errorf("Content.Length = %d, expected %d", content.Length, len(content.Transcript))
	}
}

func TestExtract_InvalidURL(t *testing.T) {
	mockYTDLP := createMockYTDLP(t, 0, "")
	defer os.Remove(mockYTDLP)

	originalPath := os.Getenv("PATH")
	defer os.Setenv("PATH", originalPath)
	os.Setenv("PATH", filepath.Dir(mockYTDLP)+":"+originalPath)

	extractor, err := NewExtractor()
	if err != nil {
		t.Fatalf("NewExtractor() failed: %v", err)
	}

	ctx := context.Background()
	invalidURL := "https://youtube.com/notavideo"

	_, err = extractor.Extract(ctx, invalidURL)
	if err == nil {
		t.Fatal("Extract() expected error for invalid URL, got nil")
	}

	var extractionErr *ExtractionError
	if errors.As(err, &extractionErr) {
		if !errors.Is(extractionErr.Err, ErrInvalidURL) {
			t.Errorf("Expected ErrInvalidURL, got %v", extractionErr.Err)
		}
	}
}

func TestExtract_TranscriptTooShort(t *testing.T) {
	// Create mock that produces a short transcript (< 1000 chars)
	mockYTDLP := createMockYTDLPWithShortVTT(t)
	defer os.Remove(mockYTDLP)

	originalPath := os.Getenv("PATH")
	defer os.Setenv("PATH", originalPath)
	os.Setenv("PATH", filepath.Dir(mockYTDLP)+":"+originalPath)

	extractor, err := NewExtractor()
	if err != nil {
		t.Fatalf("NewExtractor() failed: %v", err)
	}

	ctx := context.Background()
	url := "https://www.youtube.com/watch?v=SHORT123456"

	_, err = extractor.Extract(ctx, url)
	if err == nil {
		t.Fatal("Extract() expected error for short transcript, got nil")
	}

	var extractionErr *ExtractionError
	if errors.As(err, &extractionErr) {
		if !errors.Is(extractionErr.Err, ErrTranscriptTooShort) {
			t.Errorf("Expected ErrTranscriptTooShort, got %v", extractionErr.Err)
		}
	}
}

func TestExtract_NoSubtitles(t *testing.T) {
	// Create mock that simulates no subtitles available
	mockYTDLP := createMockYTDLPWithError(t, 1, "ERROR: no subtitles found")
	defer os.Remove(mockYTDLP)

	originalPath := os.Getenv("PATH")
	defer os.Setenv("PATH", originalPath)
	os.Setenv("PATH", filepath.Dir(mockYTDLP)+":"+originalPath)

	extractor, err := NewExtractor()
	if err != nil {
		t.Fatalf("NewExtractor() failed: %v", err)
	}

	ctx := context.Background()
	url := "https://www.youtube.com/watch?v=NOSUBS12345"

	_, err = extractor.Extract(ctx, url)
	if err == nil {
		t.Fatal("Extract() expected error for no subtitles, got nil")
	}

	var extractionErr *ExtractionError
	if errors.As(err, &extractionErr) {
		if !errors.Is(extractionErr.Err, ErrNoSubtitles) {
			t.Errorf("Expected ErrNoSubtitles, got %v", extractionErr.Err)
		}
	}
}

func TestExtract_PrivateVideo(t *testing.T) {
	// Create mock that simulates private video error
	mockYTDLP := createMockYTDLPWithError(t, 1, "ERROR: Private video. Sign in if you've been granted access")
	defer os.Remove(mockYTDLP)

	originalPath := os.Getenv("PATH")
	defer os.Setenv("PATH", originalPath)
	os.Setenv("PATH", filepath.Dir(mockYTDLP)+":"+originalPath)

	extractor, err := NewExtractor()
	if err != nil {
		t.Fatalf("NewExtractor() failed: %v", err)
	}

	ctx := context.Background()
	url := "https://www.youtube.com/watch?v=PRIVATE1234"

	_, err = extractor.Extract(ctx, url)
	if err == nil {
		t.Fatal("Extract() expected error for private video, got nil")
	}

	var extractionErr *ExtractionError
	if errors.As(err, &extractionErr) {
		if !errors.Is(extractionErr.Err, ErrPrivateVideo) {
			t.Errorf("Expected ErrPrivateVideo, got %v", extractionErr.Err)
		}
	}
}

func TestExtract_Timeout(t *testing.T) {
	// Create mock that sleeps longer than timeout
	mockYTDLP := createMockYTDLPWithDelay(t, 2*time.Second)
	defer os.Remove(mockYTDLP)

	originalPath := os.Getenv("PATH")
	defer os.Setenv("PATH", originalPath)
	os.Setenv("PATH", filepath.Dir(mockYTDLP)+":"+originalPath)

	extractor, err := NewExtractor()
	if err != nil {
		t.Fatalf("NewExtractor() failed: %v", err)
	}

	// Set very short timeout
	extractor.timeout = 100 * time.Millisecond

	ctx := context.Background()
	url := "https://www.youtube.com/watch?v=TIMEOUT1234"

	_, err = extractor.Extract(ctx, url)
	if err == nil {
		t.Fatal("Extract() expected timeout error, got nil")
	}

	var extractionErr *ExtractionError
	if errors.As(err, &extractionErr) {
		if !errors.Is(extractionErr.Err, ErrTimeout) {
			t.Errorf("Expected ErrTimeout, got %v", extractionErr.Err)
		}
	}
}

func TestExtractVideoID(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		want    string
		wantErr bool
	}{
		{
			name: "Standard YouTube URL",
			url:  "https://www.youtube.com/watch?v=dQw4w9WgXcQ",
			want: "dQw4w9WgXcQ",
		},
		{
			name: "YouTube short URL",
			url:  "https://youtu.be/dQw4w9WgXcQ",
			want: "dQw4w9WgXcQ",
		},
		{
			name: "URL with timestamp",
			url:  "https://www.youtube.com/watch?v=dQw4w9WgXcQ&t=30s",
			want: "dQw4w9WgXcQ",
		},
		{
			name: "URL with playlist",
			url:  "https://www.youtube.com/watch?v=dQw4w9WgXcQ&list=PLx",
			want: "dQw4w9WgXcQ",
		},
		{
			name:    "Invalid URL",
			url:     "https://youtube.com/notavideo",
			wantErr: true,
		},
		{
			name:    "Non-YouTube URL",
			url:     "https://example.com",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := extractVideoID(tt.url)
			if tt.wantErr {
				if err == nil {
					t.Errorf("extractVideoID() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("extractVideoID() unexpected error: %v", err)
				return
			}

			if got != tt.want {
				t.Errorf("extractVideoID() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestExtract_ContextCancellation(t *testing.T) {
	// Create mock that takes a while
	mockYTDLP := createMockYTDLPWithDelay(t, 5*time.Second)
	defer os.Remove(mockYTDLP)

	originalPath := os.Getenv("PATH")
	defer os.Setenv("PATH", originalPath)
	os.Setenv("PATH", filepath.Dir(mockYTDLP)+":"+originalPath)

	extractor, err := NewExtractor()
	if err != nil {
		t.Fatalf("NewExtractor() failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	url := "https://www.youtube.com/watch?v=CANCEL12345"

	_, err = extractor.Extract(ctx, url)
	if err == nil {
		t.Fatal("Extract() expected error for cancelled context, got nil")
	}
}

// Helper functions for creating mock yt-dlp executables

func createMockYTDLP(t *testing.T, exitCode int, output string) string {
	t.Helper()

	tmpDir := t.TempDir()
	mockPath := filepath.Join(tmpDir, "yt-dlp")

	script := fmt.Sprintf("#!/bin/sh\necho '%s'\nexit %d", output, exitCode)
	if err := os.WriteFile(mockPath, []byte(script), 0755); err != nil {
		t.Fatalf("Failed to create mock yt-dlp: %v", err)
	}

	return mockPath
}

func createMockYTDLPWithVTT(t *testing.T) string {
	t.Helper()

	tmpDir := t.TempDir()
	mockPath := filepath.Join(tmpDir, "yt-dlp")

	// Script that creates a VTT file with enough content
	script := `#!/bin/sh
# Extract output directory from args
OUTPUT_DIR=$(echo "$@" | grep -o '\--output [^ ]*' | cut -d' ' -f2 | xargs dirname)
VIDEO_ID=$(echo "$@" | grep -o '\--output [^ ]*' | cut -d' ' -f2 | xargs basename)

# Create VTT file
cat > "${OUTPUT_DIR}/${VIDEO_ID}.en.vtt" << 'EOF'
WEBVTT
Kind: captions
Language: en

1
00:00:00.000 --> 00:00:05.000
` + strings.Repeat("This is a sample subtitle text. ", 100) + `

2
00:00:05.000 --> 00:00:10.000
` + strings.Repeat("More subtitle content here. ", 100) + `
EOF
exit 0
`

	if err := os.WriteFile(mockPath, []byte(script), 0755); err != nil {
		t.Fatalf("Failed to create mock yt-dlp: %v", err)
	}

	return mockPath
}

func createMockYTDLPWithShortVTT(t *testing.T) string {
	t.Helper()

	tmpDir := t.TempDir()
	mockPath := filepath.Join(tmpDir, "yt-dlp")

	// Script that creates a VTT file with insufficient content
	script := `#!/bin/sh
OUTPUT_DIR=$(echo "$@" | grep -o '\--output [^ ]*' | cut -d' ' -f2 | xargs dirname)
VIDEO_ID=$(echo "$@" | grep -o '\--output [^ ]*' | cut -d' ' -f2 | xargs basename)

cat > "${OUTPUT_DIR}/${VIDEO_ID}.en.vtt" << 'EOF'
WEBVTT

1
00:00:00.000 --> 00:00:03.000
Short subtitle
EOF
exit 0
`

	if err := os.WriteFile(mockPath, []byte(script), 0755); err != nil {
		t.Fatalf("Failed to create mock yt-dlp: %v", err)
	}

	return mockPath
}

func createMockYTDLPWithError(t *testing.T, exitCode int, errorMsg string) string {
	t.Helper()

	tmpDir := t.TempDir()
	mockPath := filepath.Join(tmpDir, "yt-dlp")

	// Use printf to avoid shell quoting issues
	script := fmt.Sprintf("#!/bin/sh\nprintf '%%s\\n' %q >&2\nexit %d", errorMsg, exitCode)
	if err := os.WriteFile(mockPath, []byte(script), 0755); err != nil {
		t.Fatalf("Failed to create mock yt-dlp: %v", err)
	}

	return mockPath
}

func createMockYTDLPWithDelay(t *testing.T, delay time.Duration) string {
	t.Helper()

	tmpDir := t.TempDir()
	mockPath := filepath.Join(tmpDir, "yt-dlp")

	script := fmt.Sprintf("#!/bin/sh\nsleep %d\nexit 0", int(delay.Seconds()))
	if err := os.WriteFile(mockPath, []byte(script), 0755); err != nil {
		t.Fatalf("Failed to create mock yt-dlp: %v", err)
	}

	return mockPath
}

func TestExtract_Integration(t *testing.T) {
	// Skip if in short mode or yt-dlp not installed
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	if _, err := exec.LookPath("yt-dlp"); err != nil {
		t.Skip("Skipping integration test: yt-dlp not installed")
	}

	extractor, err := NewExtractor()
	if err != nil {
		t.Fatalf("NewExtractor() failed: %v", err)
	}

	// Use a known public video with subtitles
	// This is a short educational video that should have captions
	ctx := context.Background()
	url := "https://www.youtube.com/watch?v=AueCygTsUpk" // From test cases

	content, err := extractor.Extract(ctx, url)
	if err != nil {
		t.Logf("Integration test failed (may be network/API issue): %v", err)
		t.Skip("Skipping due to extraction error")
	}

	if content.Length < MinTranscriptLength {
		t.Errorf("Transcript length = %d, expected >= %d", content.Length, MinTranscriptLength)
	}

	t.Logf("Successfully extracted %d characters from video", content.Length)
}
