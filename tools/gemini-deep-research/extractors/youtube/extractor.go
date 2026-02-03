package youtube

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"time"

	"github.com/vbonnet/ai-tools/tools/gemini-deep-research/detector"
)

const (
	// MinTranscriptLength is the minimum required transcript length (SPEC FR3.1)
	MinTranscriptLength = 1000

	// DefaultTimeout is the default extraction timeout
	DefaultTimeout = 30 * time.Second
)

var (
	// YouTube URL patterns for video ID extraction
	youtubeWatchPattern = regexp.MustCompile(`[?&]v=([a-zA-Z0-9_-]{11})`)
	youtubeShortenPattern = regexp.MustCompile(`youtu\.be/([a-zA-Z0-9_-]{11})`)
)

// YouTubeExtractor extracts video transcripts using yt-dlp
type YouTubeExtractor struct {
	ytdlpPath string
	timeout   time.Duration
	tempDir   string
}

// NewExtractor creates a new YouTubeExtractor
func NewExtractor() (*YouTubeExtractor, error) {
	// Check if yt-dlp is installed
	ytdlpPath, err := exec.LookPath("yt-dlp")
	if err != nil {
		return nil, NewExtractionError(
			"",
			"check yt-dlp installation",
			ErrYTDLPNotFound,
			"Install yt-dlp: https://github.com/yt-dlp/yt-dlp#installation",
			"On Ubuntu/Debian: sudo apt install yt-dlp",
			"On macOS: brew install yt-dlp",
			"Using pip: pip install yt-dlp",
		)
	}

	return &YouTubeExtractor{
		ytdlpPath: ytdlpPath,
		timeout:   DefaultTimeout,
		tempDir:   os.TempDir(),
	}, nil
}

// Extract extracts transcript from a YouTube URL
func (e *YouTubeExtractor) Extract(ctx context.Context, url string) (*Content, error) {
	// Extract video ID
	videoID, err := extractVideoID(url)
	if err != nil {
		return nil, NewExtractionError(
			url,
			"extract video ID",
			ErrInvalidURL,
			"Verify URL format is correct",
			"Examples: https://www.youtube.com/watch?v=VIDEO_ID or https://youtu.be/VIDEO_ID",
		)
	}

	// Create temporary directory for VTT file
	vttDir := filepath.Join(e.tempDir, "yt-"+videoID)
	if err := os.MkdirAll(vttDir, 0755); err != nil {
		return nil, fmt.Errorf("create temp directory: %w", err)
	}
	defer os.RemoveAll(vttDir) // Clean up temp files

	// Download subtitles using yt-dlp
	vttPath, err := e.downloadSubtitles(ctx, url, vttDir, videoID)
	if err != nil {
		return nil, err
	}

	// Parse VTT to plain text
	transcript, err := ParseVTT(vttPath)
	if err != nil {
		return nil, NewExtractionError(
			url,
			"parse VTT file",
			err,
			"VTT file may be corrupted",
			"Try downloading the video again",
		)
	}

	// Validate transcript length
	if len(transcript) < MinTranscriptLength {
		return nil, NewExtractionError(
			url,
			"validate transcript length",
			fmt.Errorf("%w: %d characters (minimum %d)", ErrTranscriptTooShort, len(transcript), MinTranscriptLength),
			"Video may be too short or have limited subtitles",
			"Try a different video with more content",
		)
	}

	return NewContent(url, videoID, transcript), nil
}

// Name returns the extractor name
func (e *YouTubeExtractor) Name() string {
	return "YouTube"
}

// ContentType returns the content type this extractor handles
func (e *YouTubeExtractor) ContentType() detector.ContentType {
	return detector.ContentTypeVideo
}

// downloadSubtitles downloads subtitles using yt-dlp
func (e *YouTubeExtractor) downloadSubtitles(ctx context.Context, url, outputDir, videoID string) (string, error) {
	// Create context with timeout
	timeoutCtx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()

	// Output pattern for yt-dlp
	outputPattern := filepath.Join(outputDir, videoID)

	// yt-dlp command: download auto-generated subtitles in VTT format
	// --write-auto-sub: Download auto-generated subtitles
	// --skip-download: Don't download video
	// --sub-format vtt: Use VTT format
	// --output: Output file pattern
	args := []string{
		"--write-auto-sub",
		"--skip-download",
		"--sub-format", "vtt",
		"--output", outputPattern,
		url,
	}

	cmd := exec.CommandContext(timeoutCtx, e.ytdlpPath, args...)
	output, err := cmd.CombinedOutput()

	if err != nil {
		// Check for specific error conditions
		outputStr := string(output)

		// Context timeout
		if timeoutCtx.Err() == context.DeadlineExceeded {
			return "", NewExtractionError(
				url,
				"download subtitles",
				ErrTimeout,
				"Network may be slow or video is very long",
				"Try again or increase timeout",
			)
		}

		// Private/restricted video
		if contains(outputStr, "Private video", "This video is unavailable", "Video unavailable") {
			return "", NewExtractionError(
				url,
				"download subtitles",
				ErrPrivateVideo,
				"Check if video is public",
				"Video may be private, unlisted, or deleted",
			)
		}

		// No subtitles available
		if contains(outputStr, "no subtitles", "No subtitles") {
			return "", NewExtractionError(
				url,
				"download subtitles",
				ErrNoSubtitles,
				"Video does not have auto-generated or manual subtitles",
				"Try a different video with captions enabled",
			)
		}

		// Generic error
		return "", NewExtractionError(
			url,
			"download subtitles",
			fmt.Errorf("yt-dlp failed: %s", outputStr),
			"Check if URL is valid",
			"Verify internet connection",
			"Try running: yt-dlp --write-auto-sub --skip-download "+url,
		)
	}

	// Find the VTT file (yt-dlp may add language suffix like .en.vtt)
	vttFiles, err := filepath.Glob(filepath.Join(outputDir, "*.vtt"))
	if err != nil {
		return "", fmt.Errorf("search for VTT files: %w", err)
	}

	if len(vttFiles) == 0 {
		return "", NewExtractionError(
			url,
			"locate VTT file",
			ErrNoSubtitles,
			"yt-dlp succeeded but no VTT file was created",
			"Video may not have subtitles available",
		)
	}

	// Return the first VTT file found (usually the English one)
	return vttFiles[0], nil
}

// extractVideoID extracts the video ID from various YouTube URL formats
func extractVideoID(url string) (string, error) {
	// Try standard youtube.com/watch?v=ID format
	if matches := youtubeWatchPattern.FindStringSubmatch(url); len(matches) > 1 {
		return matches[1], nil
	}

	// Try shortened youtu.be/ID format
	if matches := youtubeShortenPattern.FindStringSubmatch(url); len(matches) > 1 {
		return matches[1], nil
	}

	return "", fmt.Errorf("could not extract video ID from URL: %s", url)
}

// contains checks if any of the substrings are in the string (case-insensitive)
func contains(s string, substrings ...string) bool {
	lower := toLower(s)
	for _, substring := range substrings {
		if containsIgnoreCase(lower, substring) {
			return true
		}
	}
	return false
}

func toLower(s string) string {
	return regexp.MustCompile(`[A-Z]`).ReplaceAllStringFunc(s, func(m string) string {
		return string(m[0] + 32)
	})
}

func containsIgnoreCase(s, substr string) bool {
	return regexp.MustCompile(`(?i)`+regexp.QuoteMeta(substr)).MatchString(s)
}
