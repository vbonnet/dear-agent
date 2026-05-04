package provider

import (
	"fmt"
	"strings"
)

// Resolver maps a model identifier string (e.g. "claude-opus-4-7",
// "gpt-5-pro", "openai/gpt-4o", "ollama:llama3.2") to a provider family
// and a normalized model name suitable for passing to that family's
// provider as GenerateRequest.Model.
//
// The resolver is intentionally small: it does not call any provider, it
// does not know about credentials, and it does not persist state. Its
// only job is the syntactic mapping. The router (pkg/llm/router) holds
// the policy on top.
//
// Two syntaxes are accepted:
//
//	1. Bare id        e.g. "claude-opus-4-7", "gpt-4o", "gemini-2.5-flash"
//	2. Prefixed id    e.g. "openai/gpt-4o", "anthropic:claude-opus-4-7",
//	                  "ollama:llama3.2", "openrouter/anthropic/claude-3-5-sonnet"
//
// Prefixed ids let operators force-route a model that the heuristic
// would otherwise misclassify (or that lives only on OpenRouter).
type Resolver struct {
	// extraFamily lets callers extend the heuristic without modifying
	// this file. Map key is a lowercase prefix to match against the
	// model id; value is the family to route to. Longest prefix wins.
	extraFamily map[string]string
}

// NewResolver returns a resolver populated with the built-in mappings.
func NewResolver() *Resolver {
	return &Resolver{}
}

// Register attaches a custom prefix→family mapping. Useful for in-house
// model names that don't follow any vendor's convention.
//
// Example: r.Register("internal-llm-", "ollama") would route
// "internal-llm-7b" to the ollama family.
func (r *Resolver) Register(prefix, family string) {
	if prefix == "" || family == "" {
		return
	}
	if r.extraFamily == nil {
		r.extraFamily = make(map[string]string)
	}
	r.extraFamily[strings.ToLower(prefix)] = family
}

// Resolve returns (family, model, error). The returned model is the bare
// model id with any "family:" or "family/" prefix stripped, ready to pass
// into Factory.NewProvider(family, model) and GenerateRequest.Model.
//
// Resolution rules, in order:
//
//	1. Explicit prefix syntaxes ("family://model", "family:model",
//	     "family/model") force routing when the prefix is a known family.
//	2. Built-in heuristic against the bare id (gpt-/o1- → openai,
//	     claude- → anthropic, gemini- → gemini, llama/mistral/qwen/phi →
//	     ollama).
//	3. Registered extra prefix mappings (longest match wins).
//	4. Return an error if nothing matched.
func (r *Resolver) Resolve(id string) (family, model string, err error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return "", "", fmt.Errorf("resolver: empty model id")
	}
	if fam, m, ok, perr := resolveExplicitPrefix(id); ok || perr != nil {
		return fam, m, perr
	}
	if fam, m, ok := resolveByHeuristic(id); ok {
		return fam, m, nil
	}
	if fam, m, ok := r.resolveByRegistered(id); ok {
		return fam, m, nil
	}
	return "", "", fmt.Errorf("resolver: cannot determine provider family for model %q "+
		"(use a prefix like \"openai:%s\" to force routing)", id, id)
}

// resolveExplicitPrefix handles the "family://model", "family:model",
// and "family/model" syntaxes. Returns (family, model, ok, err): ok is
// true when an explicit prefix was found and accepted; err is non-nil
// only for the "scheme://" form when the scheme is an unknown family
// (an unambiguous user error worth surfacing).
func resolveExplicitPrefix(id string) (family, model string, ok bool, err error) {
	if i := strings.Index(id, "://"); i > 0 {
		fam := strings.ToLower(id[:i])
		if !knownFamily(fam) {
			return "", "", false, fmt.Errorf("resolver: unknown family scheme %q in %q", fam, id)
		}
		return fam, id[i+3:], true, nil
	}
	if i := strings.IndexByte(id, ':'); i > 0 {
		fam := strings.ToLower(id[:i])
		if knownFamily(fam) {
			return fam, id[i+1:], true, nil
		}
	}
	// Take only the FIRST slash so OpenRouter-style ids
	// ("openrouter/anthropic/claude-3-5-sonnet") survive intact.
	if i := strings.IndexByte(id, '/'); i > 0 {
		fam := strings.ToLower(id[:i])
		if knownFamily(fam) {
			return fam, id[i+1:], true, nil
		}
	}
	return "", "", false, nil
}

// resolveByHeuristic applies the built-in vendor prefix table. Returns
// ok=false if no prefix matched.
func resolveByHeuristic(id string) (family, model string, ok bool) {
	lower := strings.ToLower(id)
	switch {
	case looksLikeOpenAIModel(id):
		return "openai", id, true
	case strings.HasPrefix(lower, "claude-"):
		return "anthropic", id, true
	case strings.HasPrefix(lower, "gemini-"):
		return "gemini", id, true
	case strings.HasPrefix(lower, "llama"),
		strings.HasPrefix(lower, "mistral"),
		strings.HasPrefix(lower, "qwen"),
		strings.HasPrefix(lower, "phi"):
		return "ollama", id, true
	}
	return "", "", false
}

// resolveByRegistered consults the user-registered prefix map; the
// longest matching prefix wins so callers can layer specific overrides
// over more general ones.
func (r *Resolver) resolveByRegistered(id string) (family, model string, ok bool) {
	if r.extraFamily == nil {
		return "", "", false
	}
	lower := strings.ToLower(id)
	var bestPrefix, bestFam string
	for prefix, fam := range r.extraFamily {
		if strings.HasPrefix(lower, prefix) && len(prefix) > len(bestPrefix) {
			bestPrefix = prefix
			bestFam = fam
		}
	}
	if bestFam == "" {
		return "", "", false
	}
	return bestFam, id, true
}

// knownFamily reports whether the given lowercase family name is one
// the factory supports. Kept in sync with Factory.NewProvider's switch.
func knownFamily(family string) bool {
	switch family {
	case "anthropic", "claude",
		"gemini", "google",
		"openai",
		"openrouter",
		"ollama", "local":
		return true
	}
	return false
}
