package benchmarks

import "context"

// builtinLoader returns a fixed task list. It is meant for scaffolding,
// tests, and dry runs — real datasets are loaded by suite-specific loaders
// that pull from disk or HuggingFace.
type builtinLoader struct {
	suite Suite
	tasks []TaskSpec
}

// Load returns the built-in fixture tasks, optionally truncated to limit.
func (l builtinLoader) Load(ctx context.Context, suite Suite, limit int) ([]TaskSpec, error) {
	_ = ctx
	tasks := l.tasks
	if limit > 0 && limit < len(tasks) {
		tasks = tasks[:limit]
	}
	out := make([]TaskSpec, len(tasks))
	copy(out, tasks)
	return out, nil
}

func sweLiteFixture() []TaskSpec {
	return []TaskSpec{
		{
			ID:         "astropy__astropy-12907",
			Suite:      SuiteSWEBenchLite,
			Repo:       "astropy/astropy",
			Prompt:     "Modeling's separability_matrix does not compute separability correctly for nested CompoundModels",
			BaseCommit: "d16bfe05a744909de4b27f5875fe0d4ed41ce607",
		},
		{
			ID:         "django__django-11099",
			Suite:      SuiteSWEBenchLite,
			Repo:       "django/django",
			Prompt:     "ASCIIUsernameValidator and UnicodeUsernameValidator allow trailing newlines because the regex uses $ instead of \\Z",
			BaseCommit: "17455e924e243e7a55e8a38f45966d8cbb27d80d",
		},
		{
			ID:         "sympy__sympy-20154",
			Suite:      SuiteSWEBenchLite,
			Repo:       "sympy/sympy",
			Prompt:     "partitions() iterator reuses the output dictionaries, causing unexpected behavior when collecting results",
			BaseCommit: "2ac6f584eb3e9f1fd1bf3527de7a76a5e5f378d8",
		},
	}
}

func sweVerifiedFixture() []TaskSpec {
	return []TaskSpec{
		{
			ID:     "verified-fixture-1",
			Suite:  SuiteSWEBenchVerified,
			Repo:   "scikit-learn/scikit-learn",
			Prompt: "VotingClassifier failed to fit when sample_weight is provided to fit and a base estimator does not support it.",
		},
		{
			ID:     "verified-fixture-2",
			Suite:  SuiteSWEBenchVerified,
			Repo:   "pallets/flask",
			Prompt: "url_for raises ValueError on internal-only routes when host_matching is enabled.",
		},
	}
}

func sweAtlasFixture() []TaskSpec {
	return []TaskSpec{
		{
			ID:       "atlas-qna-1",
			Suite:    SuiteSWEAtlas,
			Pillar:   string(PillarQnA),
			Prompt:   "Where in the codebase is the rate limiter configured, and what is its default capacity?",
			Metadata: map[string]any{"pillar": string(PillarQnA)},
		},
		{
			ID:       "atlas-test-1",
			Suite:    SuiteSWEAtlas,
			Pillar:   string(PillarTestWriting),
			Prompt:   "Write tests for the public API of pkg/cache, covering eviction, TTL expiry, and concurrent access.",
			Metadata: map[string]any{"pillar": string(PillarTestWriting)},
		},
		{
			ID:       "atlas-refactor-1",
			Suite:    SuiteSWEAtlas,
			Pillar:   string(PillarRefactoring),
			Prompt:   "Refactor the monolithic handler in pkg/api/server.go into per-endpoint handlers without changing behavior.",
			Metadata: map[string]any{"pillar": string(PillarRefactoring)},
		},
	}
}

func vibeBenchFixture() []TaskSpec {
	return []TaskSpec{
		{
			ID:     "vibe-1",
			Suite:  SuiteVibeBench,
			Prompt: "Build a single-page todo app with localStorage persistence, keyboard shortcuts, and a dark-mode toggle.",
		},
		{
			ID:     "vibe-2",
			Suite:  SuiteVibeBench,
			Prompt: "Build a CSV → SQLite import tool with a small TUI that previews the schema before importing.",
		},
	}
}
