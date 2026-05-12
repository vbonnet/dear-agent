package benchmarks

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// JSONFileLoader loads TaskSpecs from a JSON file on disk.
//
// Two on-disk shapes are accepted, distinguished by the first non-whitespace
// byte of the file:
//
//   - A JSON array of TaskSpec objects ("[ {...}, {...} ]").
//   - NDJSON: one TaskSpec object per line. Blank lines and lines starting with
//     "//" are skipped so users can keep comments alongside the data.
//
// The loader is the "real loader" counterpart to builtinLoader: it lets callers
// point a suite at the SWE-Bench / SWE-Atlas datasets they have unpacked locally
// without having to embed those datasets into the binary.
type JSONFileLoader struct {
	// Path is the file to read. Required.
	Path string

	// FilterBySuite, when true, drops any TaskSpec whose Suite field does not
	// match the suite argument passed to Load. When false (the default) the
	// loader returns every TaskSpec in the file and trusts the caller to have
	// scoped the file to a single suite. False is the right default for
	// per-suite files; flip it to true when sharing one file across suites.
	FilterBySuite bool
}

// NewJSONFileLoader is a small convenience constructor.
func NewJSONFileLoader(path string) *JSONFileLoader {
	return &JSONFileLoader{Path: path}
}

// Load reads the configured file and returns the tasks, optionally truncated to
// limit. limit <= 0 means "return everything".
func (l *JSONFileLoader) Load(ctx context.Context, suite Suite, limit int) ([]TaskSpec, error) {
	if l == nil || l.Path == "" {
		return nil, fmt.Errorf("benchmarks: JSONFileLoader requires a non-empty Path")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	clean := filepath.Clean(l.Path)
	f, err := os.Open(clean)
	if err != nil {
		return nil, fmt.Errorf("open tasks file %q: %w", clean, err)
	}
	defer func() { _ = f.Close() }()

	tasks, err := decodeTasks(f)
	if err != nil {
		return nil, fmt.Errorf("decode tasks file %q: %w", clean, err)
	}

	if l.FilterBySuite {
		filtered := tasks[:0]
		for _, t := range tasks {
			if t.Suite == suite {
				filtered = append(filtered, t)
			}
		}
		tasks = filtered
	}

	if limit > 0 && limit < len(tasks) {
		tasks = tasks[:limit]
	}
	return tasks, nil
}

// decodeTasks reads from r and returns the decoded tasks. The shape is sniffed
// from the first non-whitespace byte: '[' implies a JSON array, anything else
// is treated as NDJSON.
func decodeTasks(r io.Reader) ([]TaskSpec, error) {
	br := bufio.NewReader(r)
	first, err := peekFirstNonSpace(br)
	if err != nil {
		if errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("empty file")
		}
		return nil, err
	}
	if first == '[' {
		var tasks []TaskSpec
		if err := json.NewDecoder(br).Decode(&tasks); err != nil {
			return nil, err
		}
		return tasks, nil
	}
	return decodeNDJSON(br)
}

func peekFirstNonSpace(br *bufio.Reader) (byte, error) {
	for {
		b, err := br.ReadByte()
		if err != nil {
			return 0, err
		}
		if b == ' ' || b == '\t' || b == '\n' || b == '\r' {
			continue
		}
		if err := br.UnreadByte(); err != nil {
			return 0, err
		}
		return b, nil
	}
}

func decodeNDJSON(r io.Reader) ([]TaskSpec, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	var tasks []TaskSpec
	line := 0
	for scanner.Scan() {
		line++
		raw := strings.TrimSpace(scanner.Text())
		if raw == "" || strings.HasPrefix(raw, "//") {
			continue
		}
		var t TaskSpec
		if err := json.Unmarshal([]byte(raw), &t); err != nil {
			return nil, fmt.Errorf("line %d: %w", line, err)
		}
		tasks = append(tasks, t)
	}
	if err := scanner.Err(); err != nil {
		// line points at the last line successfully scanned; the scanner
		// failure (e.g. SplitFunc returned, buffer overflow) happened on
		// the next one.
		return nil, fmt.Errorf("line %d: %w", line+1, err)
	}
	return tasks, nil
}
