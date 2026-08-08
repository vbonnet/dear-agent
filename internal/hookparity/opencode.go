// Package hookparity owns repository-scoped hook capability contracts shared
// by every active harness transport.
package hookparity

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const openCodePluginReplacement = ".opencode/plugins/spec-contract-guard.mjs"

// OpenCodeLegacyProjectionIsInactive accepts either an absent legacy manifest
// or the exact governed retirement tombstone. Any hook payload, extra metadata,
// or different replacement remains an active or ambiguous legacy projection.
func OpenCodeLegacyProjectionIsInactive(repository string) (bool, error) {
	path := filepath.Join(repository, ".opencode", "hooks.json")
	data, present, safe, err := readOpenCodeLegacyProjection(path)
	if err != nil {
		return false, err
	}
	if !present {
		return true, nil
	}
	if !safe {
		return false, nil
	}
	return isExactOpenCodeRetirementTombstone(data)
}

func readOpenCodeLegacyProjection(path string) ([]byte, bool, bool, error) {
	current, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, true, nil
	}
	if err != nil {
		return nil, false, false, fmt.Errorf("inspect OpenCode legacy hook projection: %w", err)
	}
	if !current.Mode().IsRegular() || current.Mode()&os.ModeSymlink != 0 {
		return nil, true, false, nil
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, true, false, fmt.Errorf("open OpenCode legacy hook projection: %w", err)
	}
	defer file.Close()
	opened, statErr := file.Stat()
	current, lstatErr := os.Lstat(path)
	if statErr != nil || lstatErr != nil || !opened.Mode().IsRegular() || current.Mode()&os.ModeSymlink != 0 || !os.SameFile(opened, current) {
		return nil, true, false, nil
	}
	data, err := io.ReadAll(io.LimitReader(file, 1<<20+1))
	if err != nil {
		return nil, true, false, fmt.Errorf("read OpenCode legacy hook projection: %w", err)
	}
	if len(data) > 1<<20 {
		return nil, true, false, fmt.Errorf("OpenCode legacy hook projection exceeds 1 MiB")
	}
	return data, true, true, nil
}

func isExactOpenCodeRetirementTombstone(data []byte) (bool, error) {
	if err := rejectDuplicateJSONKeys(data); err != nil {
		return false, fmt.Errorf("parse exact OpenCode retirement tombstone: %w", err)
	}
	var document map[string]json.RawMessage
	if err := json.Unmarshal(data, &document); err != nil {
		return false, fmt.Errorf("parse OpenCode legacy hook projection: %w", err)
	}
	metadataJSON, ok := document["_dear_agent"]
	if !ok || len(document) != 1 {
		return false, nil
	}
	var metadata map[string]json.RawMessage
	if err := json.Unmarshal(metadataJSON, &metadata); err != nil {
		return false, fmt.Errorf("parse OpenCode retirement metadata: %w", err)
	}
	if len(metadata) != 3 {
		return false, nil
	}
	return jsonStringEquals(metadata["status"], "retired") &&
		jsonStringEquals(metadata["replacement"], openCodePluginReplacement) &&
		jsonStringEquals(metadata["runtime_claim"], "none"), nil
}

func rejectDuplicateJSONKeys(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decodeUniqueJSONValue(decoder); err != nil {
		return err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return fmt.Errorf("unexpected trailing JSON value")
	}
	return nil
}

func decodeUniqueJSONValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, composite := token.(json.Delim)
	if !composite {
		return nil
	}
	switch delimiter {
	case '{':
		seen := make(map[string]struct{})
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return fmt.Errorf("JSON object key is not a string")
			}
			if _, duplicate := seen[key]; duplicate {
				return fmt.Errorf("duplicate JSON object key %q", key)
			}
			seen[key] = struct{}{}
			if err := decodeUniqueJSONValue(decoder); err != nil {
				return err
			}
		}
	case '[':
		for decoder.More() {
			if err := decodeUniqueJSONValue(decoder); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("unexpected JSON delimiter %q", delimiter)
	}
	end, err := decoder.Token()
	if err != nil {
		return err
	}
	if end != matchingJSONDelimiter(delimiter) {
		return fmt.Errorf("mismatched JSON delimiter %q", end)
	}
	return nil
}

func matchingJSONDelimiter(open json.Delim) json.Delim {
	if open == '{' {
		return '}'
	}
	return ']'
}

func jsonStringEquals(encoded json.RawMessage, expected string) bool {
	var value string
	return json.Unmarshal(encoded, &value) == nil && value == expected
}
