// Package llm_test verifies the llm meta-package compiles.
// All LLM implementation lives in sub-packages (auth, config, provider, etc.);
// this package re-exports nothing and is a doc-only anchor.
package llm_test

import "testing"

func TestPackageCompiles(t *testing.T) {
	t.Parallel()
}
