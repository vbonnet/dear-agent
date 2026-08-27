package specpackage

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
)

const manifestDigestDomain = "dear-agent/spec-governance-package-manifest/v1\x00"

type packageManifest struct {
	SchemaVersion string        `json:"schema_version"`
	Files         []FileReceipt `json:"files"`
}

func canonicalManifest(files []FileReceipt) ([]byte, Receipt, error) {
	files = append([]FileReceipt(nil), files...)
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	for index, file := range files {
		if _, ok := entryByPackagePath(file.Path); !ok {
			return nil, Receipt{}, fmt.Errorf("manifest contains unexpected payload path %q", file.Path)
		}
		if index > 0 && files[index-1].Path == file.Path {
			return nil, Receipt{}, fmt.Errorf("manifest contains duplicate payload path %q", file.Path)
		}
	}
	if len(files) != len(payloadLayout) {
		return nil, Receipt{}, fmt.Errorf("manifest payload count is %d, want %d", len(files), len(payloadLayout))
	}
	encoded, err := json.MarshalIndent(packageManifest{
		SchemaVersion: SchemaVersion,
		Files:         files,
	}, "", "  ")
	if err != nil {
		return nil, Receipt{}, fmt.Errorf("encode canonical package manifest: %w", err)
	}
	encoded = append(encoded, '\n')
	if len(encoded) > maxManifestBytes {
		return nil, Receipt{}, fmt.Errorf("canonical package manifest exceeds %d bytes", maxManifestBytes)
	}
	digest := sha256.New()
	_, _ = digest.Write([]byte(manifestDigestDomain))
	_, _ = digest.Write(encoded)
	return encoded, Receipt{
		SchemaVersion:  SchemaVersion,
		ManifestSHA256: hex.EncodeToString(digest.Sum(nil)),
		Files:          files,
	}, nil
}

func compareCanonicalManifest(actual, expected []byte) error {
	if !bytes.Equal(actual, expected) {
		return fmt.Errorf("%s does not contain the exact canonical package manifest", manifestPath)
	}
	return nil
}
