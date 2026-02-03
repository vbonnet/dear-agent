package youtube

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseVTT_Success(t *testing.T) {
	// Create sample VTT file
	vttContent := `WEBVTT
Kind: captions
Language: en

1
00:00:00.000 --> 00:00:03.000
This is the first subtitle line

2
00:00:03.000 --> 00:00:06.000
And this is the second line with <c>tags</c>

3
00:00:06.000 --> 00:00:09.000
<i>Italic text</i> and normal text
`

	vttPath := createTempVTT(t, vttContent)
	defer os.Remove(vttPath)

	transcript, err := ParseVTT(vttPath)
	if err != nil {
		t.Fatalf("ParseVTT() unexpected error: %v", err)
	}

	// Verify content
	if !strings.Contains(transcript, "This is the first subtitle line") {
		t.Error("Transcript should contain first subtitle")
	}

	if !strings.Contains(transcript, "And this is the second line with tags") {
		t.Error("Transcript should contain second subtitle with tags removed")
	}

	if !strings.Contains(transcript, "Italic text and normal text") {
		t.Error("Transcript should contain italic text with tags removed")
	}

	// Verify headers removed
	if strings.Contains(transcript, "WEBVTT") {
		t.Error("Transcript should not contain WEBVTT header")
	}

	if strings.Contains(transcript, "Kind:") {
		t.Error("Transcript should not contain Kind marker")
	}

	if strings.Contains(transcript, "Language:") {
		t.Error("Transcript should not contain Language marker")
	}

	// Verify timestamps removed
	if strings.Contains(transcript, "00:00:00") {
		t.Error("Transcript should not contain timestamps")
	}

	// Verify HTML tags removed
	if strings.Contains(transcript, "<c>") || strings.Contains(transcript, "</c>") {
		t.Error("Transcript should not contain <c> tags")
	}

	if strings.Contains(transcript, "<i>") || strings.Contains(transcript, "</i>") {
		t.Error("Transcript should not contain <i> tags")
	}
}

func TestParseVTT_ComplexFormatting(t *testing.T) {
	// Test with various HTML tags and formatting
	vttContent := `WEBVTT

1
00:00:00.000 --> 00:00:03.000
Text with <c.yellow>colored</c> words

2
00:00:03.000 --> 00:00:06.000
<b>Bold text</b> and <u>underlined</u>

3
00:00:06.000 --> 00:00:09.000
Multiple   spaces    should    collapse
`

	vttPath := createTempVTT(t, vttContent)
	defer os.Remove(vttPath)

	transcript, err := ParseVTT(vttPath)
	if err != nil {
		t.Fatalf("ParseVTT() unexpected error: %v", err)
	}

	// Verify tags removed
	if strings.Contains(transcript, "<c.yellow>") {
		t.Error("Transcript should not contain color tags")
	}

	if strings.Contains(transcript, "<b>") || strings.Contains(transcript, "<u>") {
		t.Error("Transcript should not contain formatting tags")
	}

	// Verify multiple spaces collapsed
	if strings.Contains(transcript, "  ") {
		t.Error("Transcript should not contain multiple consecutive spaces")
	}

	// Verify content preserved
	if !strings.Contains(transcript, "colored words") {
		t.Error("Transcript should preserve content after tag removal")
	}
}

func TestParseVTT_EmptyLines(t *testing.T) {
	vttContent := `WEBVTT

1
00:00:00.000 --> 00:00:03.000
First line


2
00:00:03.000 --> 00:00:06.000
Second line
`

	vttPath := createTempVTT(t, vttContent)
	defer os.Remove(vttPath)

	transcript, err := ParseVTT(vttPath)
	if err != nil {
		t.Fatalf("ParseVTT() unexpected error: %v", err)
	}

	// Verify empty lines don't create extra spaces
	if strings.Contains(transcript, "  ") {
		t.Error("Empty lines should not create multiple spaces")
	}

	// Verify both lines present
	if !strings.Contains(transcript, "First line") || !strings.Contains(transcript, "Second line") {
		t.Error("Transcript should contain both lines")
	}
}

func TestParseVTT_CueIDs(t *testing.T) {
	// VTT files may have just numbers as cue IDs
	vttContent := `WEBVTT

1
00:00:00.000 --> 00:00:03.000
First subtitle

2
00:00:03.000 --> 00:00:06.000
Second subtitle
`

	vttPath := createTempVTT(t, vttContent)
	defer os.Remove(vttPath)

	transcript, err := ParseVTT(vttPath)
	if err != nil {
		t.Fatalf("ParseVTT() unexpected error: %v", err)
	}

	// Verify cue IDs are removed (not part of transcript)
	// The transcript should not start with "1" or "2"
	if strings.HasPrefix(strings.TrimSpace(transcript), "1") {
		t.Error("Transcript should not contain cue ID numbers")
	}
}

func TestParseVTT_FileNotFound(t *testing.T) {
	_, err := ParseVTT("/nonexistent/file.vtt")
	if err == nil {
		t.Fatal("ParseVTT() expected error for non-existent file, got nil")
	}

	if !strings.Contains(err.Error(), "open VTT file") {
		t.Errorf("Error should mention 'open VTT file', got: %v", err)
	}
}

func TestParseVTT_MinimalVTT(t *testing.T) {
	// Minimal valid VTT
	vttContent := `WEBVTT

00:00:00.000 --> 00:00:03.000
Single subtitle line
`

	vttPath := createTempVTT(t, vttContent)
	defer os.Remove(vttPath)

	transcript, err := ParseVTT(vttPath)
	if err != nil {
		t.Fatalf("ParseVTT() unexpected error: %v", err)
	}

	if transcript != "Single subtitle line" {
		t.Errorf("Expected 'Single subtitle line', got: %q", transcript)
	}
}

func TestParseVTT_MultilineSubtitles(t *testing.T) {
	// Subtitles can span multiple lines
	vttContent := `WEBVTT

1
00:00:00.000 --> 00:00:03.000
This is a long subtitle
that spans multiple lines

2
00:00:03.000 --> 00:00:06.000
Another multi-line
subtitle here
`

	vttPath := createTempVTT(t, vttContent)
	defer os.Remove(vttPath)

	transcript, err := ParseVTT(vttPath)
	if err != nil {
		t.Fatalf("ParseVTT() unexpected error: %v", err)
	}

	// Verify all lines are included
	if !strings.Contains(transcript, "This is a long subtitle") {
		t.Error("Should contain first part of multi-line subtitle")
	}

	if !strings.Contains(transcript, "that spans multiple lines") {
		t.Error("Should contain second part of multi-line subtitle")
	}

	if !strings.Contains(transcript, "Another multi-line") {
		t.Error("Should contain multi-line subtitle")
	}
}

// Helper function to create temporary VTT file
func createTempVTT(t *testing.T, content string) string {
	t.Helper()

	tmpDir := t.TempDir()
	vttPath := filepath.Join(tmpDir, "test.vtt")

	if err := os.WriteFile(vttPath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create temp VTT file: %v", err)
	}

	return vttPath
}
