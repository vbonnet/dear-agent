// Package pluginhash owns the content-hash convention for AGM plugin Markdown.
package pluginhash

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"regexp"
)

var contentHashLine = regexp.MustCompile(`(?m)^content-hash: [0-9a-fA-F]+|^content-hash: PLACEHOLDER$`)

// SumBody returns the lowercase SHA-256 for a plugin Markdown body.
func SumBody(body []byte) string {
	trimmed := bytes.TrimRight(body, "\n")
	return fmt.Sprintf("%x", sha256.Sum256(trimmed))
}

// Compute returns the hash of everything after the closing YAML delimiter.
func Compute(markdown []byte) (string, error) {
	if !bytes.HasPrefix(markdown, []byte("---\n")) {
		return "", fmt.Errorf("plugin Markdown has no opening YAML delimiter")
	}
	closing := bytes.Index(markdown[4:], []byte("\n---\n"))
	if closing < 0 {
		return "", fmt.Errorf("plugin Markdown has no closing YAML delimiter")
	}
	bodyStart := 4 + closing + len("\n---\n")
	return SumBody(markdown[bodyStart:]), nil
}

// Stamp replaces the existing content-hash field with the computed body hash.
func Stamp(markdown []byte) ([]byte, error) {
	hash, err := Compute(markdown)
	if err != nil {
		return nil, err
	}
	if !contentHashLine.Match(markdown) {
		return nil, fmt.Errorf("plugin Markdown has no content-hash field")
	}
	return contentHashLine.ReplaceAll(markdown, []byte("content-hash: "+hash)), nil
}
