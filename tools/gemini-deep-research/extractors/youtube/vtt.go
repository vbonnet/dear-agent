package youtube

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strings"
)

var (
	// VTT header patterns to skip
	webvttHeaderPattern = regexp.MustCompile(`^WEBVTT`)
	kindPattern         = regexp.MustCompile(`^Kind:`)
	languagePattern     = regexp.MustCompile(`^Language:`)

	// Timestamp pattern: 00:00:00.000 --> 00:00:03.000
	timestampPattern = regexp.MustCompile(`^\d{2}:\d{2}:\d{2}\.\d{3}\s*-->\s*\d{2}:\d{2}:\d{2}\.\d{3}`)

	// HTML tag patterns to remove
	htmlTagPattern = regexp.MustCompile(`<[^>]+>`)

	// Cue identifier pattern (just numbers)
	cueIDPattern = regexp.MustCompile(`^\d+$`)
)

// ParseVTT parses a VTT file and returns plain text transcript
func ParseVTT(vttPath string) (string, error) {
	file, err := os.Open(vttPath)
	if err != nil {
		return "", fmt.Errorf("open VTT file: %w", err)
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)

	// Process line by line
	for scanner.Scan() {
		line := scanner.Text()
		line = strings.TrimSpace(line)

		// Skip empty lines
		if line == "" {
			continue
		}

		// Skip WEBVTT headers
		if webvttHeaderPattern.MatchString(line) {
			continue
		}

		// Skip Kind: and Language: markers
		if kindPattern.MatchString(line) || languagePattern.MatchString(line) {
			continue
		}

		// Skip timestamp lines
		if timestampPattern.MatchString(line) {
			continue
		}

		// Skip cue IDs (just numbers)
		if cueIDPattern.MatchString(line) {
			continue
		}

		// Remove HTML tags
		line = htmlTagPattern.ReplaceAllString(line, "")

		// Skip if line became empty after tag removal
		if strings.TrimSpace(line) == "" {
			continue
		}

		// Add to lines
		lines = append(lines, line)
	}

	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("scan VTT file: %w", err)
	}

	// Join lines with spaces (subtitles are sentence fragments)
	transcript := strings.Join(lines, " ")

	// Clean up multiple spaces
	transcript = regexp.MustCompile(`\s+`).ReplaceAllString(transcript, " ")
	transcript = strings.TrimSpace(transcript)

	return transcript, nil
}
