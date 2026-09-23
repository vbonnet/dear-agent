package marketplaceparity

import (
	"encoding/json"
	"fmt"
	"slices"
)

var neutralCatalogCanonicalFields = []string{
	"schema_version",
	"name",
	"description",
	"owner",
	"plugins",
	"harnesses",
}

var neutralOwnerCanonicalFields = []string{"name", "email"}

var neutralPluginCanonicalFields = []string{
	"name",
	"source",
	"description",
	"version",
	"author",
	"repository",
	"license",
	"capabilities",
}

var neutralHarnessCanonicalFields = []string{"name", "mode", "catalog"}
var pluginAuthorCanonicalFields = []string{"name", "email", "url"}

// UnmarshalJSON rejects case aliases for neutral catalog authority fields while
// retaining forward-compatible supplemental fields.
func (catalog *Catalog) UnmarshalJSON(data []byte) error {
	fields, err := decodeJSONObject(data)
	if err != nil {
		return err
	}
	if err := validateExactCaseFields("neutral marketplace catalog", fields, neutralCatalogCanonicalFields); err != nil {
		return err
	}
	type catalogJSON Catalog
	var decoded catalogJSON
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*catalog = Catalog(decoded)
	return nil
}

// UnmarshalJSON rejects case aliases for known plugin authority fields while
// retaining supplemental neutral metadata.
func (plugin *PluginEntry) UnmarshalJSON(data []byte) error {
	fields, err := decodeJSONObject(data)
	if err != nil {
		return err
	}
	if err := validateExactCaseFields("neutral marketplace plugin entry", fields, neutralPluginCanonicalFields); err != nil {
		return err
	}
	type pluginEntryJSON PluginEntry
	var decoded pluginEntryJSON
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*plugin = PluginEntry(decoded)
	return nil
}

// UnmarshalJSON rejects case aliases for known owner identity fields while
// retaining supplemental root-owner metadata.
func (owner *Owner) UnmarshalJSON(data []byte) error {
	fields, err := decodeJSONObject(data)
	if err != nil {
		return err
	}
	if err := validateExactCaseFields("marketplace owner", fields, neutralOwnerCanonicalFields); err != nil {
		return err
	}
	type ownerJSON Owner
	var decoded ownerJSON
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*owner = Owner(decoded)
	return nil
}

// UnmarshalJSON keeps author authority closed to Claude's documented fields.
func (author *PluginAuthor) UnmarshalJSON(data []byte) error {
	fields, err := decodeJSONObject(data)
	if err != nil {
		return err
	}
	if err := validateExactCaseFields("plugin author", fields, pluginAuthorCanonicalFields); err != nil {
		return err
	}
	for field := range fields {
		if !containsField(pluginAuthorCanonicalFields, field) {
			return fmt.Errorf("plugin author defines forbidden field %q", field)
		}
	}
	type authorJSON PluginAuthor
	var decoded authorJSON
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*author = PluginAuthor(decoded)
	return nil
}

// UnmarshalJSON rejects case aliases for known harness routing fields while
// retaining supplemental neutral metadata.
func (surface *HarnessSurface) UnmarshalJSON(data []byte) error {
	fields, err := decodeJSONObject(data)
	if err != nil {
		return err
	}
	if err := validateExactCaseFields("neutral marketplace harness surface", fields, neutralHarnessCanonicalFields); err != nil {
		return err
	}
	type harnessSurfaceJSON HarnessSurface
	var decoded harnessSurfaceJSON
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*surface = HarnessSurface(decoded)
	return nil
}

func containsField(fields []string, candidate string) bool {
	return slices.Contains(fields, candidate)
}
