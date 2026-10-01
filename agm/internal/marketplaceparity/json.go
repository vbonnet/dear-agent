package marketplaceparity

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

const (
	maxCatalogBytes  int64 = 1 << 20
	maxManifestBytes int64 = 256 << 10
)

func readJSONWithin(root, relative string, dst any) error {
	limit := maxCatalogBytes
	if relative == "plugin.json" || strings.HasSuffix(relative, "/plugin.json") {
		limit = maxManifestBytes
	}
	data, err := readAnchoredRegular(root, relative, limit)
	if err != nil {
		return fmt.Errorf("read %s: %w", relative, err)
	}
	return decodeJSONData(relative, data, dst)
}

func decodeJSONData(name string, data []byte, dst any) error {
	if err := validateNoDuplicateJSONKeys(data); err != nil {
		return fmt.Errorf("parse %s: %w", name, err)
	}
	if err := json.Unmarshal(data, dst); err != nil {
		return fmt.Errorf("parse %s: %w", name, err)
	}
	return nil
}

func decodeJSONObject(data []byte) (map[string]json.RawMessage, error) {
	if err := validateNoDuplicateJSONKeys(data); err != nil {
		return nil, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return nil, err
	}
	if fields == nil {
		return nil, fmt.Errorf("expected JSON object")
	}
	return fields, nil
}

func validateExactCaseFields(subject string, fields map[string]json.RawMessage, canonicalFields []string) error {
	for field := range fields {
		for _, canonical := range canonicalFields {
			if field != canonical && strings.EqualFold(field, canonical) {
				return fmt.Errorf("%s field %q must use exact case %q", subject, field, canonical)
			}
		}
	}
	return nil
}

func validateNoDuplicateJSONKeys(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := validateJSONValue(decoder); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		if err == nil {
			return fmt.Errorf("unexpected data after JSON value")
		}
		return err
	}
	return nil
}

func validateJSONValue(decoder *json.Decoder) error {
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
		seen := make(map[string]bool)
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return fmt.Errorf("expected JSON object key")
			}
			if seen[key] {
				return fmt.Errorf("duplicate JSON object field %q", key)
			}
			seen[key] = true
			if err := validateJSONValue(decoder); err != nil {
				return err
			}
		}
	case '[':
		for decoder.More() {
			if err := validateJSONValue(decoder); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("unexpected JSON delimiter %q", delimiter)
	}
	closing, err := decoder.Token()
	if err != nil {
		return err
	}
	want := json.Delim('}')
	if delimiter == '[' {
		want = ']'
	}
	if closing != want {
		return fmt.Errorf("unexpected JSON closing delimiter %q", closing)
	}
	return nil
}
