package registry

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/vbonnet/dear-agent/pkg/source"
	"github.com/vbonnet/dear-agent/pkg/source/llmwiki"
	"github.com/vbonnet/dear-agent/pkg/source/obsidian"
	"github.com/vbonnet/dear-agent/pkg/source/openviking"
	"github.com/vbonnet/dear-agent/pkg/source/sqlite"
)

// init wires every in-tree adapter into the registry. Plugins that
// live in other modules should mirror this pattern: import their own
// package and call Register from a similar init().
//
// Why register in this file rather than from each adapter's own init:
// keeping the registration centralised means importing a single
// "registry" package transitively pulls in the four built-in
// backends, which is what almost every binary wants. Adapters that
// need to ship without the registry can be imported directly.
func init() {
	Register(sqlite.Name, openSQLite)
	Register(obsidian.Name, openObsidian)
	Register(llmwiki.Name, openLLMWiki)
	Register(openviking.Name, openOpenViking)
}

// openSQLite expects a filesystem path. Empty path is rejected to
// avoid silently opening :memory: and surprising the operator.
func openSQLite(config string) (source.Adapter, error) {
	if config == "" {
		return nil, fmt.Errorf("source/registry: sqlite: path is required (got empty config)")
	}
	return sqlite.Open(config)
}

// openObsidian expects a vault directory path.
func openObsidian(config string) (source.Adapter, error) {
	if config == "" {
		return nil, fmt.Errorf("source/registry: obsidian: vault dir is required (got empty config)")
	}
	return obsidian.Open(config)
}

// openLLMWiki expects a wiki directory path.
func openLLMWiki(config string) (source.Adapter, error) {
	if config == "" {
		return nil, fmt.Errorf("source/registry: llm-wiki: wiki dir is required (got empty config)")
	}
	return llmwiki.Open(config)
}

// openOpenViking accepts either a bolt URL string ("bolt://host:port")
// or a JSON-encoded openviking.Config payload (so callers can supply
// auth without leaking it into a connection string).
func openOpenViking(config string) (source.Adapter, error) {
	cfg := openviking.Config{URL: config}
	if strings.HasPrefix(config, "{") {
		if err := json.Unmarshal([]byte(config), &cfg); err != nil {
			return nil, fmt.Errorf("source/registry: openviking: parse config json: %w", err)
		}
	}
	return openviking.Open(cfg)
}
