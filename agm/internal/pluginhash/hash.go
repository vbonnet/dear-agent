// Package pluginhash owns the content-hash convention for AGM plugin Markdown.
package pluginhash

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"regexp"
)

var contentHashLine = regexp.MustCompile(`(?m)^content-hash: (?:[0-9a-fA-F]+|PLACEHOLDER)\r?$`)

func normalizeLineEndings(data []byte) []byte {
	return bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
}

// SumBody returns the lowercase SHA-256 for a plugin Markdown body.
func SumBody(body []byte) string {
	trimmed := bytes.TrimRight(body, "\n")
	return fmt.Sprintf("%x", sha256.Sum256(trimmed))
}

// Compute returns the hash of everything after the closing YAML delimiter.
func Compute(markdown []byte) (string, error) {
	markdown = normalizeLineEndings(markdown)
	bodyStart, err := bodyStart(markdown)
	if err != nil {
		return "", err
	}
	return SumBody(markdown[bodyStart:]), nil
}

func bodyStart(markdown []byte) (int, error) {
	if !bytes.HasPrefix(markdown, []byte("---\n")) {
		return 0, fmt.Errorf("plugin Markdown has no opening YAML delimiter")
	}
	closing := bytes.Index(markdown[4:], []byte("\n---\n"))
	if closing < 0 {
		return 0, fmt.Errorf("plugin Markdown has no closing YAML delimiter")
	}
	return 4 + closing + len("\n---\n"), nil
}

// Stamp replaces the existing content-hash field with the computed body hash.
func Stamp(markdown []byte) ([]byte, error) {
	markdown = normalizeLineEndings(markdown)
	bodyOffset, err := bodyStart(markdown)
	if err != nil {
		return nil, err
	}
	frontmatter := markdown[:bodyOffset]
	matches := contentHashLine.FindAllIndex(frontmatter, -1)
	if len(matches) != 1 {
		return nil, fmt.Errorf("plugin Markdown must have exactly one content-hash field in frontmatter")
	}
	hash := SumBody(markdown[bodyOffset:])
	stampedFrontmatter := contentHashLine.ReplaceAll(frontmatter, []byte("content-hash: "+hash))
	return append(stampedFrontmatter, markdown[bodyOffset:]...), nil
}
