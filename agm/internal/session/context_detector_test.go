package session

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

// floatEqual compares two float64 values within a small tolerance.
func floatEqual(a, b float64) bool {
	return math.Abs(a-b) < 1e-9
}

func TestDetectContextFromManifestOrLogReadsExactPiTranscript(t *testing.T) {
	t.Setenv("PI_CODING_AGENT_DIR", t.TempDir())
	dir := t.TempDir()
	path := filepath.Join(dir, "pi.jsonl")
	content := `{"type":"session","id":"pi-context","cwd":"/work"}` + "\n" +
		`{"type":"message","timestamp":"2026-07-21T00:00:03Z","message":{"role":"assistant","model":"openai/gpt-5.4","usage":{"input":1000,"output":25,"cacheRead":500,"cacheWrite":100,"cost":{"total":0.42}}}}` + "\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	m := &manifest.Manifest{Pi: &manifest.Pi{SessionID: "pi-context", SessionDir: dir, TranscriptPath: path}}
	usage, err := DetectContextFromManifestOrLog(m)
	if err != nil {
		t.Fatal(err)
	}
	if usage.Source != "pi_jsonl" || usage.ModelID != "openai/gpt-5.4" || usage.UsedTokens != 1600 {
		t.Fatalf("Pi context usage = %#v", usage)
	}
	if usage.TotalTokens != 272000 || !floatEqual(usage.PercentageUsed, float64(1600)/272000*100) || !floatEqual(usage.EstimatedCost, 0.42) {
		t.Fatalf("Pi context percentage/cost = %#v", usage)
	}

	m.Pi.TranscriptPath = filepath.Join(dir, "different.jsonl")
	if _, err := DetectContextFromManifestOrLog(m); err == nil {
		t.Fatal("persisted Pi transcript mismatch was accepted")
	}
}

func TestDetectContextFromManifestOrLogUsesTrustedPiCustomCatalog(t *testing.T) {
	catalogDir := t.TempDir()
	t.Setenv("PI_CODING_AGENT_DIR", catalogDir)
	marker := filepath.Join(t.TempDir(), "credential-command-ran")
	catalog := `{"providers":{"ollama":{"baseUrl":"http://localhost:11434/v1","apiKey":"!touch ` + marker + `","models":[{"id":"qwen2.5-coder:7b","contextWindow":8192}]}}}`
	if err := os.WriteFile(filepath.Join(catalogDir, "models.json"), []byte(catalog), 0o600); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "pi.jsonl")
	content := `{"type":"session","id":"pi-custom-context","cwd":"/work"}` + "\n" +
		`{"type":"message","timestamp":"2026-07-22T10:11:52Z","message":{"role":"assistant","provider":"ollama","model":"qwen2.5-coder:7b","usage":{"input":3539,"output":4,"cacheRead":23}}}` + "\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	usage, err := DetectContextFromManifestOrLog(&manifest.Manifest{Pi: &manifest.Pi{
		SessionID: "pi-custom-context", SessionDir: dir, TranscriptPath: path,
		CodingAgentDir: catalogDir, CodingAgentDirSet: true,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if usage.ModelID != "ollama/qwen2.5-coder:7b" || usage.TotalTokens != 8192 || usage.UsedTokens != 3562 {
		t.Fatalf("custom Pi context usage = %#v", usage)
	}
	if !floatEqual(usage.PercentageUsed, float64(3562)/8192*100) {
		t.Fatalf("custom Pi context percentage = %f", usage.PercentageUsed)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("models.json apiKey command was evaluated: %v", err)
	}
}

func TestDetectContextFromManifestOrLogUsesPersistedPiCatalogInsteadOfCallerEnvironment(t *testing.T) {
	persistedDir := t.TempDir()
	writePiModelCatalogFixture(t, persistedDir, `{"providers":{"ollama":{"models":[{"id":"qwen2.5-coder:7b","contextWindow":8192}]}}}`)
	callerDir := t.TempDir()
	writePiModelCatalogFixture(t, callerDir, `{"providers":{"ollama":{"models":[{"id":"qwen2.5-coder:7b","contextWindow":4096}]}}}`)
	t.Setenv("PI_CODING_AGENT_DIR", callerDir)

	sessionDir := t.TempDir()
	transcript := filepath.Join(sessionDir, "pi.jsonl")
	content := `{"type":"session","id":"pi-persisted-catalog","cwd":"/work"}` + "\n" +
		`{"type":"message","timestamp":"2026-07-22T10:11:52Z","message":{"role":"assistant","provider":"ollama","model":"qwen2.5-coder:7b","usage":{"input":1000,"output":1}}}` + "\n"
	if err := os.WriteFile(transcript, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	m := &manifest.Manifest{Pi: &manifest.Pi{
		SessionID: "pi-persisted-catalog", SessionDir: sessionDir,
		TranscriptPath: transcript, CodingAgentDir: persistedDir, CodingAgentDirSet: true,
	}}
	usage, err := DetectContextFromManifestOrLog(m)
	if err != nil {
		t.Fatal(err)
	}
	if usage.TotalTokens != 8192 {
		t.Fatalf("persisted Pi catalog context = %d, want 8192", usage.TotalTokens)
	}

	t.Setenv("HOME", t.TempDir())
	m.Pi.CodingAgentDir = ""
	usage, err = DetectContextFromManifestOrLog(m)
	if err != nil {
		t.Fatal(err)
	}
	if usage.TotalTokens != 200000 {
		t.Fatalf("default Pi catalog context inherited caller environment: %d", usage.TotalTokens)
	}

	m.Pi.CodingAgentDirSet = false
	usage, err = DetectContextFromManifestOrLog(m)
	if err != nil {
		t.Fatal(err)
	}
	if usage.TotalTokens != 4096 {
		t.Fatalf("legacy Pi catalog compatibility context = %d, want 4096", usage.TotalTokens)
	}
}

func TestPiConfiguredModelContextWindowTrustBoundaries(t *testing.T) {
	tests := []struct {
		name    string
		model   string
		prepare func(*testing.T, string)
		want    int
	}{
		{
			name: "missing catalog retains conservative fallback", model: "ollama/qwen2.5-coder:7b", want: 200000,
			prepare: func(_ *testing.T, _ string) {},
		},
		{
			name: "custom model uses documented Pi default", model: "ollama/qwen2.5-coder:7b", want: 128000,
			prepare: func(t *testing.T, dir string) {
				writePiModelCatalogFixture(t, dir, `{"providers":{"ollama":{"models":[{"id":"qwen2.5-coder:7b"}]}}}`)
			},
		},
		{
			name: "integral decimal window matches Pi JSON semantics", model: "ollama/qwen2.5-coder:7b", want: 8192,
			prepare: func(t *testing.T, dir string) {
				writePiModelCatalogFixture(t, dir, `{"providers":{"ollama":{"models":[{"id":"qwen2.5-coder:7b","contextWindow":8192.0}]}}}`)
			},
		},
		{
			name: "integral exponent window matches Pi JSON semantics", model: "ollama/qwen2.5-coder:7b", want: 8192,
			prepare: func(t *testing.T, dir string) {
				writePiModelCatalogFixture(t, dir, `{"providers":{"ollama":{"models":[{"id":"qwen2.5-coder:7b","contextWindow":8.192e3}]}}}`)
			},
		},
		{
			name: "fractional window remains invalid without float rounding", model: "ollama/qwen2.5-coder:7b", want: 200000,
			prepare: func(t *testing.T, dir string) {
				writePiModelCatalogFixture(t, dir, `{"providers":{"ollama":{"models":[{"id":"qwen2.5-coder:7b","contextWindow":8192.000000000000001}]}}}`)
			},
		},
		{
			name: "provider model override wins", model: "openai/gpt-5.4", want: 4096,
			prepare: func(t *testing.T, dir string) {
				writePiModelCatalogFixture(t, dir, `{"providers":{"openai":{"modelOverrides":{"gpt-5.4":{"contextWindow":4096}}}}}`)
			},
		},
		{
			name: "integral exponent override matches Pi JSON semantics", model: "openai/gpt-5.4", want: 4096,
			prepare: func(t *testing.T, dir string) {
				writePiModelCatalogFixture(t, dir, `{"providers":{"openai":{"modelOverrides":{"gpt-5.4":{"contextWindow":4.096e3}}}}}`)
			},
		},
		{
			name: "recorded provider override does not depend on static model table", model: "openai/gpt-4.1", want: 4096,
			prepare: func(t *testing.T, dir string) {
				writePiModelCatalogFixture(t, dir, `{"providers":{"openai":{"modelOverrides":{"gpt-4.1":{"contextWindow":4096}}}}}`)
			},
		},
		{
			name: "recorded future provider override does not depend on frozen provider registry", model: "future-provider/future-model", want: 16384,
			prepare: func(t *testing.T, dir string) {
				writePiModelCatalogFixture(t, dir, `{"providers":{"future-provider":{"modelOverrides":{"future-model":{"contextWindow":16384}}}}}`)
			},
		},
		{
			name: "unqualified known route retains its override", model: "gpt-5.4", want: 4096,
			prepare: func(t *testing.T, dir string) {
				writePiModelCatalogFixture(t, dir, `{"providers":{"openai":{"modelOverrides":{"gpt-5.4":{"contextWindow":4096}}}}}`)
			},
		},
		{
			name: "non-context override preserves custom model window", model: "ollama/qwen2.5-coder:7b", want: 8192,
			prepare: func(t *testing.T, dir string) {
				writePiModelCatalogFixture(t, dir, `{"providers":{"ollama":{"models":[{"id":"qwen2.5-coder:7b","contextWindow":8192}],"modelOverrides":{"qwen2.5-coder:7b":{}}}}}`)
			},
		},
		{
			name: "exact recorded custom route honors explicit override", model: "ollama/qwen2.5-coder:7b", want: 4096,
			prepare: func(t *testing.T, dir string) {
				writePiModelCatalogFixture(t, dir, `{"providers":{"ollama":{"modelOverrides":{"qwen2.5-coder:7b":{"contextWindow":4096}}}}}`)
			},
		},
		{
			name: "unqualified orphan override remains conservative", model: "qwen2.5-coder:7b", want: 200000,
			prepare: func(t *testing.T, dir string) {
				writePiModelCatalogFixture(t, dir, `{"providers":{"ollama":{"modelOverrides":{"qwen2.5-coder:7b":{"contextWindow":4096}}}}}`)
			},
		},
		{
			name: "last duplicate custom model wins like Pi", model: "ollama/qwen2.5-coder:7b", want: 16384,
			prepare: func(t *testing.T, dir string) {
				writePiModelCatalogFixture(t, dir, `{"providers":{"ollama":{"models":[{"id":"qwen2.5-coder:7b","contextWindow":8192},{"id":"qwen2.5-coder:7b","contextWindow":16384}]}}}`)
			},
		},
		{
			name: "ambiguous unqualified model falls back", model: "shared-model", want: 200000,
			prepare: func(t *testing.T, dir string) {
				writePiModelCatalogFixture(t, dir, `{"providers":{"one":{"models":[{"id":"shared-model","contextWindow":8192}]},"two":{"models":[{"id":"shared-model","contextWindow":16384}]}}}`)
			},
		},
		{
			name: "equal duplicate unqualified models remain ambiguous", model: "shared-model", want: 200000,
			prepare: func(t *testing.T, dir string) {
				writePiModelCatalogFixture(t, dir, `{"providers":{"one":{"models":[{"id":"shared-model","contextWindow":8192}]},"two":{"models":[{"id":"shared-model","contextWindow":8192}]}}}`)
			},
		},
		{
			name: "invalid duplicate unqualified model poisons the match", model: "shared-model", want: 200000,
			prepare: func(t *testing.T, dir string) {
				writePiModelCatalogFixture(t, dir, `{"providers":{"one":{"models":[{"id":"shared-model","contextWindow":8192}]},"two":{"models":[{"id":"shared-model","contextWindow":null}]}}}`)
			},
		},
		{
			name: "invalid explicit window falls back", model: "ollama/qwen2.5-coder:7b", want: 200000,
			prepare: func(t *testing.T, dir string) {
				writePiModelCatalogFixture(t, dir, `{"providers":{"ollama":{"models":[{"id":"qwen2.5-coder:7b","contextWindow":16777217}]}}}`)
			},
		},
		{
			name: "explicit null window falls back", model: "ollama/qwen2.5-coder:7b", want: 200000,
			prepare: func(t *testing.T, dir string) {
				writePiModelCatalogFixture(t, dir, `{"providers":{"ollama":{"models":[{"id":"qwen2.5-coder:7b","contextWindow":null}]}}}`)
			},
		},
		{
			name: "explicit null override falls back", model: "openai/gpt-5.4", want: 272000,
			prepare: func(t *testing.T, dir string) {
				writePiModelCatalogFixture(t, dir, `{"providers":{"openai":{"modelOverrides":{"gpt-5.4":{"contextWindow":null}}}}}`)
			},
		},
		{
			name: "malformed catalog falls back", model: "ollama/qwen2.5-coder:7b", want: 200000,
			prepare: func(t *testing.T, dir string) {
				writePiModelCatalogFixture(t, dir, `{"providers":`)
			},
		},
		{
			name: "symlinked catalog falls back", model: "ollama/qwen2.5-coder:7b", want: 200000,
			prepare: func(t *testing.T, dir string) {
				target := filepath.Join(t.TempDir(), "models.json")
				if err := os.WriteFile(target, []byte(`{"providers":{"ollama":{"models":[{"id":"qwen2.5-coder:7b","contextWindow":8192}]}}}`), 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, filepath.Join(dir, "models.json")); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "group-writable catalog falls back", model: "ollama/qwen2.5-coder:7b", want: 200000,
			prepare: func(t *testing.T, dir string) {
				writePiModelCatalogFixture(t, dir, `{"providers":{"ollama":{"models":[{"id":"qwen2.5-coder:7b","contextWindow":8192}]}}}`)
				if err := os.Chmod(filepath.Join(dir, "models.json"), 0o620); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "oversized catalog falls back", model: "ollama/qwen2.5-coder:7b", want: 200000,
			prepare: func(t *testing.T, dir string) {
				if err := os.WriteFile(filepath.Join(dir, "models.json"), make([]byte, piModelCatalogMaxBytes+1), 0o600); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "excessive model count falls back", model: "ollama/qwen2.5-coder:7b", want: 200000,
			prepare: func(t *testing.T, dir string) {
				models := make([]piModelCatalogModel, piModelCatalogMaxModels+1)
				for index := range models {
					models[index].ID = fmt.Sprintf("model-%d", index)
				}
				models[0].ID = "qwen2.5-coder:7b"
				data, err := json.Marshal(piModelCatalog{Providers: map[string]piModelCatalogProvider{
					"ollama": {Models: models},
				}})
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, "models.json"), data, 0o600); err != nil {
					t.Fatal(err)
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("PI_CODING_AGENT_DIR", dir)
			test.prepare(t, dir)
			if got := piModelContextWindow(test.model, dir); got != test.want {
				t.Fatalf("piModelContextWindow(%q) = %d, want %d", test.model, got, test.want)
			}
		})
	}
}

func writePiModelCatalogFixture(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "models.json"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestPiModelContextWindowMatchesNativeCatalog(t *testing.T) {
	t.Setenv("PI_CODING_AGENT_DIR", t.TempDir())
	tests := map[string]int{
		"anthropic/claude-fable-5":                     1000000,
		"anthropic/claude-opus-4-8":                    1000000,
		"openai/gpt-5.6-terra":                         272000,
		"openai/gpt-5.4":                               272000,
		"openrouter/anthropic/claude-fable-5":          1000000,
		"openrouter/openai/gpt-5.3-codex":              400000,
		"openrouter/openai/gpt-5.4-mini":               400000,
		"openrouter/openai/gpt-5.4-nano":               400000,
		"openrouter/openai/gpt-5.4":                    1050000,
		"openrouter/openai/gpt-5.5":                    1050000,
		"openrouter/openai/gpt-5.6-sol":                1050000,
		"openrouter/openai/gpt-5.6-terra":              1050000,
		"openrouter/openai/gpt-5.6-luna":               1050000,
		"openrouter/openai/gpt-5.4-pro":                1050000,
		"openrouter/openai/gpt-5.5-pro":                1050000,
		"openrouter/openai/gpt-5.3-chat-latest":        200000,
		"anthropic/openai/gpt-5.4":                     200000,
		"OPENAI/GPT-5.4":                               200000,
		"openai/gpt-5.4-mini":                          400000,
		"openai/gpt-5.4-pro":                           1050000,
		"openai/gpt-5.3-codex":                         400000,
		"google/gemini-3.5-flash":                      1048576,
		"openrouter/google/gemini-3.5-flash":           1048576,
		"openrouter/google/gemini-3.1-flash-lite":      1048576,
		"openrouter/z-ai/glm-5.2":                      1048576,
		"openrouter/deepseek/deepseek-v4-pro":          1048576,
		"openrouter/nvidia/nemotron-3-ultra-550b-a55b": 512288,
		"openrouter/qwen/qwen3.6-max-preview":          262144,
		"custom/model":                                 200000,
	}
	for model, want := range tests {
		if got := piModelContextWindow(model, os.Getenv("PI_CODING_AGENT_DIR")); got != want {
			t.Errorf("piModelContextWindow(%q) = %d, want %d", model, got, want)
		}
	}
}

func TestExtractTokenUsage(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		wantUsed int
		wantPct  float64
		wantNil  bool
	}{
		{
			name:     "standard system reminder",
			content:  "<system-reminder>Token usage: 50000/200000; 150000 remaining</system-reminder>",
			wantUsed: 50000,
			wantPct:  25.0,
		},
		{
			name:     "high usage",
			content:  "Token usage: 180000/200000; 20000 remaining",
			wantUsed: 180000,
			wantPct:  90.0,
		},
		{
			name:    "no token usage",
			content: "Just some regular text without tokens",
			wantNil: true,
		},
		{
			name:    "malformed pattern",
			content: "Token usage: invalid/200000",
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			usage := extractTokenUsage(tt.content)

			if tt.wantNil {
				if usage != nil {
					t.Errorf("expected nil, got %+v", usage)
				}
				return
			}

			if usage == nil {
				t.Fatal("expected usage, got nil")
				return
			}

			if usage.UsedTokens != tt.wantUsed {
				t.Errorf("UsedTokens = %d, want %d", usage.UsedTokens, tt.wantUsed)
			}

			if usage.PercentageUsed != tt.wantPct {
				t.Errorf("PercentageUsed = %.1f, want %.1f", usage.PercentageUsed, tt.wantPct)
			}

			if usage.Source != "conversation_log" {
				t.Errorf("Source = %s, want conversation_log", usage.Source)
			}
		})
	}
}

func TestContainsTokenUsage(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    bool
	}{
		{
			name:    "contains token usage",
			content: "<system-reminder>Token usage: 50000/200000; 150000 remaining</system-reminder>",
			want:    true,
		},
		{
			name:    "no token usage",
			content: "Just some text",
			want:    false,
		},
		{
			name:    "token usage without reminder tags",
			content: "Token usage: 12345/200000",
			want:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := containsTokenUsage(tt.content); got != tt.want {
				t.Errorf("containsTokenUsage() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDetectContextFromConversationLog(t *testing.T) {
	// Create temporary conversation log using actual Claude Code path layout:
	// ~/.claude/projects/{project-path-hash}/{sessionID}.jsonl
	tempDir := t.TempDir()
	sessionID := "test-session-123"
	projectHash := "-home-user-src"
	logPath := filepath.Join(tempDir, ".claude", "projects", projectHash, sessionID+".jsonl")

	// Create directory structure
	if err := os.MkdirAll(filepath.Dir(logPath), 0755); err != nil {
		t.Fatal(err)
	}

	// Write sample conversation log
	content := `{"type":"user_message","content":"Run tests","timestamp":"2026-03-15T10:00:00Z"}
{"type":"assistant_message","content":"I'll run the tests","timestamp":"2026-03-15T10:00:05Z"}
{"type":"system_reminder","content":"<system-reminder>Token usage: 50000/200000; 150000 remaining</system-reminder>","timestamp":"2026-03-15T10:00:10Z"}
{"type":"user_message","content":"Check status","timestamp":"2026-03-15T10:05:00Z"}
{"type":"system_reminder","content":"<system-reminder>Token usage: 75000/200000; 125000 remaining</system-reminder>","timestamp":"2026-03-15T10:05:10Z"}
`

	if err := os.WriteFile(logPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	// Mock home directory
	t.Setenv("HOME", tempDir)

	// Clear cache
	ClearDetectorCache()

	// Test detection
	usage, err := DetectContextFromConversationLog(sessionID)
	if err != nil {
		t.Fatalf("DetectContextFromConversationLog() error = %v", err)
	}

	if usage == nil {
		t.Fatal("expected usage, got nil")
		return
	}

	// Should get most recent token usage (75000/200000 = 37.5%)
	if usage.UsedTokens != 75000 {
		t.Errorf("UsedTokens = %d, want 75000", usage.UsedTokens)
	}

	if usage.PercentageUsed != 37.5 {
		t.Errorf("PercentageUsed = %.1f, want 37.5", usage.PercentageUsed)
	}

	if usage.Source != "conversation_log" {
		t.Errorf("Source = %s, want conversation_log", usage.Source)
	}

	// Test caching
	usage2, err := DetectContextFromConversationLog(sessionID)
	if err != nil {
		t.Fatalf("cached lookup error = %v", err)
	}

	if usage2.Source != "conversation_log_cached" {
		t.Errorf("expected cached source, got %s", usage2.Source)
	}
}

func TestDetectContextFromConversationLog_NoTokenUsage(t *testing.T) {
	tempDir := t.TempDir()
	sessionID := "test-session-no-tokens"
	projectHash := "-home-user-project"
	logPath := filepath.Join(tempDir, ".claude", "projects", projectHash, sessionID+".jsonl")

	if err := os.MkdirAll(filepath.Dir(logPath), 0755); err != nil {
		t.Fatal(err)
	}

	// Write log without token usage
	content := `{"type":"user_message","content":"Hello","timestamp":"2026-03-15T10:00:00Z"}
{"type":"assistant_message","content":"Hi there","timestamp":"2026-03-15T10:00:05Z"}
`

	if err := os.WriteFile(logPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("HOME", tempDir)

	ClearDetectorCache()

	usage, err := DetectContextFromConversationLog(sessionID)
	if err == nil {
		t.Errorf("expected error, got usage: %+v", usage)
	}
}

func TestDetectContextFromConversationLog_FileNotFound(t *testing.T) {
	ClearDetectorCache()

	usage, err := DetectContextFromConversationLog("nonexistent-session")
	if err == nil {
		t.Errorf("expected error, got usage: %+v", usage)
	}
}

func TestCacheExpiration(t *testing.T) {
	tempDir := t.TempDir()
	sessionID := "test-cache-expiry"
	projectHash := "-home-user-cache-test"
	logPath := filepath.Join(tempDir, ".claude", "projects", projectHash, sessionID+".jsonl")

	if err := os.MkdirAll(filepath.Dir(logPath), 0755); err != nil {
		t.Fatal(err)
	}

	content := `{"type":"system_reminder","content":"Token usage: 50000/200000","timestamp":"2026-03-15T10:00:00Z"}
`
	if err := os.WriteFile(logPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("HOME", tempDir)

	ClearDetectorCache()

	// First call - should cache
	usage1, err := DetectContextFromConversationLog(sessionID)
	if err != nil {
		t.Fatal(err)
	}

	if usage1.Source != "conversation_log" {
		t.Errorf("expected conversation_log source, got %s", usage1.Source)
	}

	// Second call within cache window - should use cache
	usage2, err := DetectContextFromConversationLog(sessionID)
	if err != nil {
		t.Fatal(err)
	}

	if usage2.Source != "conversation_log_cached" {
		t.Errorf("expected cached source, got %s", usage2.Source)
	}

	// Modify file (updates mtime)
	time.Sleep(10 * time.Millisecond)
	content2 := `{"type":"system_reminder","content":"Token usage: 100000/200000","timestamp":"2026-03-15T10:05:00Z"}
`
	if err := os.WriteFile(logPath, []byte(content2), 0644); err != nil {
		t.Fatal(err)
	}

	// Third call - file modified, should re-parse
	usage3, err := DetectContextFromConversationLog(sessionID)
	if err != nil {
		t.Fatal(err)
	}

	// Should detect new value (50%)
	if usage3.PercentageUsed != 50.0 {
		t.Errorf("PercentageUsed = %.1f, want 50.0 (cache should be invalidated)", usage3.PercentageUsed)
	}
}

func TestExtractUsageFromJSONL(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		wantNil  bool
		wantUsed int
		wantPct  float64
	}{
		{
			name:     "valid assistant message with usage data",
			line:     `{"type":"assistant","timestamp":"2026-03-15T10:00:00Z","message":{"model":"claude-sonnet-4-5-20250929","usage":{"input_tokens":1000,"cache_creation_input_tokens":4000,"cache_read_input_tokens":5000,"output_tokens":500}}}`,
			wantUsed: 10000,
			wantPct:  5.0,
		},
		{
			name:    "non-assistant message type",
			line:    `{"type":"user","timestamp":"2026-03-15T10:00:00Z","message":{"content":"hello"}}`,
			wantNil: true,
		},
		{
			name:    "missing message field",
			line:    `{"type":"assistant","timestamp":"2026-03-15T10:00:00Z"}`,
			wantNil: true,
		},
		{
			name:    "zero token usage",
			line:    `{"type":"assistant","timestamp":"2026-03-15T10:00:00Z","message":{"model":"claude-sonnet-4-5-20250929","usage":{"input_tokens":0,"cache_creation_input_tokens":0,"cache_read_input_tokens":0,"output_tokens":0}}}`,
			wantNil: true,
		},
		{
			name:     "unknown claude model uses fallback 200000",
			line:     `{"type":"assistant","timestamp":"2026-03-15T10:00:00Z","message":{"model":"claude-future-99","usage":{"input_tokens":50000,"cache_creation_input_tokens":0,"cache_read_input_tokens":0,"output_tokens":100}}}`,
			wantUsed: 50000,
			wantPct:  25.0,
		},
		{
			name:     "large token counts auto-upgrade to 1M context",
			line:     `{"type":"assistant","timestamp":"2026-03-15T10:00:00Z","message":{"model":"claude-sonnet-4-5-20250929","usage":{"input_tokens":100000,"cache_creation_input_tokens":80000,"cache_read_input_tokens":70000,"output_tokens":5000}}}`,
			wantUsed: 250000,
			wantPct:  25.0, // 250k / 1M = 25% (auto-upgraded from 200k to 1M window)
		},
		{
			name:    "malformed JSON",
			line:    `{not valid json at all`,
			wantNil: true,
		},
		{
			name:     "real-world JSONL line from Claude Code",
			line:     `{"type":"assistant","timestamp":"2026-03-20T14:32:11.456Z","message":{"id":"msg_abc123","type":"message","role":"assistant","model":"claude-sonnet-4-5-20250929","content":[{"type":"text","text":"Let me check that."}],"usage":{"input_tokens":12,"cache_creation_input_tokens":4014,"cache_read_input_tokens":23244,"output_tokens":1}}}`,
			wantUsed: 27270,
			wantPct:  13.635,
		},
		{
			name:    "non-claude model returns nil",
			line:    `{"type":"assistant","timestamp":"2026-03-15T10:00:00Z","message":{"model":"gpt-4o","usage":{"input_tokens":5000,"cache_creation_input_tokens":0,"cache_read_input_tokens":0,"output_tokens":100}}}`,
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			usage := extractUsageFromJSONL(tt.line)
			if tt.wantNil {
				if usage != nil {
					t.Errorf("expected nil, got %+v", usage)
				}
				return
			}
			if usage == nil {
				t.Fatal("expected usage, got nil")
				return
			}
			if usage.UsedTokens != tt.wantUsed {
				t.Errorf("UsedTokens = %d, want %d", usage.UsedTokens, tt.wantUsed)
			}
			if !floatEqual(usage.PercentageUsed, tt.wantPct) {
				t.Errorf("PercentageUsed = %f, want %f", usage.PercentageUsed, tt.wantPct)
			}
			if usage.Source != "conversation_log" {
				t.Errorf("Source = %s, want conversation_log", usage.Source)
			}
		})
	}
}

func TestGetModelContextWindow(t *testing.T) {
	tests := []struct {
		name  string
		model string
		want  int
	}{
		// Longest-prefix match: opus-4-6 must resolve to 1M even though
		// the shorter prefix "claude-opus-4" is also in the map.
		{"claude-opus-4-6 exact", "claude-opus-4-6", 1000000},
		{"claude-opus-4-6 with date suffix", "claude-opus-4-6-20251001", 1000000},
		// Shorter opus-4 prefix (no -6) stays at 200k.
		{"claude-opus-4 prefix no -6", "claude-opus-4-20260101", 200000},
		{"claude-sonnet-4 prefix", "claude-sonnet-4-5-20250929", 200000},
		{"claude-haiku-4 prefix", "claude-haiku-4-20260101", 200000},
		{"claude-3-5 prefix", "claude-3-5-sonnet-20241022", 200000},
		{"unknown claude model default", "claude-future-model-99", 200000},
		{"non-claude model", "gpt-4o-2024-05-13", 0},
		{"empty string", "", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getModelContextWindow(tt.model)
			if got != tt.want {
				t.Errorf("getModelContextWindow(%q) = %d, want %d", tt.model, got, tt.want)
			}
		})
	}
}

func TestExtractUsageFromJSONL_Integration(t *testing.T) {
	tempDir := t.TempDir()
	sessionID := "test-jsonl-integration"
	projectHash := "-home-user-integration"
	logPath := filepath.Join(tempDir, ".claude", "projects", projectHash, sessionID+".jsonl")

	if err := os.MkdirAll(filepath.Dir(logPath), 0755); err != nil {
		t.Fatal(err)
	}

	content := `{"type":"user","timestamp":"2026-03-20T10:00:00Z","message":{"content":"Hello"}}
{"type":"assistant","timestamp":"2026-03-20T10:00:05Z","message":{"model":"claude-sonnet-4-5-20250929","usage":{"input_tokens":100,"cache_creation_input_tokens":2000,"cache_read_input_tokens":3000,"output_tokens":50}}}
{"type":"progress","timestamp":"2026-03-20T10:00:06Z","message":{"status":"thinking"}}
{"type":"user","timestamp":"2026-03-20T10:01:00Z","message":{"content":"Run tests"}}
{"type":"assistant","timestamp":"2026-03-20T10:01:05Z","message":{"model":"claude-sonnet-4-5-20250929","usage":{"input_tokens":500,"cache_creation_input_tokens":5000,"cache_read_input_tokens":20000,"output_tokens":200}}}
{"type":"user","timestamp":"2026-03-20T10:02:00Z","message":{"content":"Check status"}}
{"type":"assistant","timestamp":"2026-03-20T10:02:05Z","message":{"model":"claude-sonnet-4-5-20250929","usage":{"input_tokens":800,"cache_creation_input_tokens":6000,"cache_read_input_tokens":40000,"output_tokens":300}}}
`

	if err := os.WriteFile(logPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("HOME", tempDir)

	ClearDetectorCache()

	usage, err := DetectContextFromConversationLog(sessionID)
	if err != nil {
		t.Fatalf("DetectContextFromConversationLog() error = %v", err)
	}
	if usage == nil {
		t.Fatal("expected usage, got nil")
		return
	}

	// Last assistant: input=800 + cache_creation=6000 + cache_read=40000 = 46800
	// 46800/200000 = 23.4%
	if usage.UsedTokens != 46800 {
		t.Errorf("UsedTokens = %d, want 46800", usage.UsedTokens)
	}
	if !floatEqual(usage.PercentageUsed, 23.4) {
		t.Errorf("PercentageUsed = %f, want 23.4", usage.PercentageUsed)
	}
	if usage.Source != "conversation_log" {
		t.Errorf("Source = %s, want conversation_log", usage.Source)
	}

	expectedTime, _ := time.Parse(time.RFC3339, "2026-03-20T10:02:05Z")
	if !usage.LastUpdated.Equal(expectedTime) {
		t.Errorf("LastUpdated = %v, want %v", usage.LastUpdated, expectedTime)
	}
}

func TestFindConversationLog(t *testing.T) {
	tempDir := t.TempDir()

	tests := []struct {
		name      string
		setupPath string
		sessionID string
		wantFound bool
		wantPath  string // substring to check in returned path
	}{
		{
			name:      "finds via glob in projects directory",
			setupPath: filepath.Join(tempDir, ".claude", "projects", "-home-user-src", "test-123.jsonl"),
			sessionID: "test-123",
			wantFound: true,
			wantPath:  "test-123.jsonl",
		},
		{
			name:      "finds via glob with different project hash",
			setupPath: filepath.Join(tempDir, ".claude", "projects", "-workspace-myproject", "sess-789.jsonl"),
			sessionID: "sess-789",
			wantFound: true,
			wantPath:  "sess-789.jsonl",
		},
		{
			name:      "falls back to legacy projects path",
			setupPath: filepath.Join(tempDir, ".claude", "projects", "test-456", "conversation.jsonl"),
			sessionID: "test-456",
			wantFound: true,
			wantPath:  "conversation.jsonl",
		},
		{
			name:      "falls back to legacy sessions path",
			setupPath: filepath.Join(tempDir, ".claude", "sessions", "test-legacy", "conversation.jsonl"),
			sessionID: "test-legacy",
			wantFound: true,
			wantPath:  "conversation.jsonl",
		},
		{
			name:      "not found",
			setupPath: "",
			sessionID: "nonexistent",
			wantFound: false,
		},
	}

	t.Setenv("HOME", tempDir)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			if tt.setupPath != "" {
				if err := os.MkdirAll(filepath.Dir(tt.setupPath), 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(tt.setupPath, []byte("test"), 0644); err != nil {
					t.Fatal(err)
				}
			}

			// Test
			path, err := findConversationLog(tt.sessionID)

			if tt.wantFound {
				if err != nil {
					t.Errorf("expected to find log, got error: %v", err)
				}
				if path == "" {
					t.Error("expected path, got empty string")
				}
				if tt.wantPath != "" && !filepath.IsAbs(path) {
					t.Errorf("expected absolute path, got: %s", path)
				}
			} else if err == nil {
				t.Errorf("expected error, got path: %s", path)
			}
		})
	}
}

func TestDetectContextFromStatusLine(t *testing.T) {
	tmpDir := t.TempDir()
	originalDir := statusLineDir
	statusLineDir = tmpDir
	defer func() { statusLineDir = originalDir }()

	sessionID := "test-statusline-session"
	data := statusLineFileData{
		SessionID: sessionID,
	}
	data.ContextWindow.UsedPercentage = 42.5
	data.ContextWindow.ContextWindowSize = 200000
	data.ContextWindow.TotalInputTokens = 85000
	data.ContextWindow.TotalOutputTokens = 3000
	data.Cost.TotalCostUSD = 0.15

	raw, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}

	filePath := filepath.Join(tmpDir, sessionID+".json")
	if err := os.WriteFile(filePath, raw, 0644); err != nil {
		t.Fatal(err)
	}

	usage, err := DetectContextFromStatusLine(sessionID)
	if err != nil {
		t.Fatalf("DetectContextFromStatusLine() error = %v", err)
	}
	if usage == nil {
		t.Fatal("expected usage, got nil")
		return
	}
	if usage.TotalTokens != 200000 {
		t.Errorf("TotalTokens = %d, want 200000", usage.TotalTokens)
	}
	if usage.UsedTokens != 85000 {
		t.Errorf("UsedTokens = %d, want 85000", usage.UsedTokens)
	}
	if !floatEqual(usage.PercentageUsed, 42.5) {
		t.Errorf("PercentageUsed = %f, want 42.5", usage.PercentageUsed)
	}
	if usage.Source != "statusline" {
		t.Errorf("Source = %s, want statusline", usage.Source)
	}
}

func TestDetectContextFromStatusLine_CumulativeExceedsWindow(t *testing.T) {
	tmpDir := t.TempDir()
	originalDir := statusLineDir
	statusLineDir = tmpDir
	defer func() { statusLineDir = originalDir }()

	sessionID := "cumulative-exceeds-window"
	data := statusLineFileData{SessionID: sessionID}
	data.ContextWindow.UsedPercentage = 41.0
	data.ContextWindow.ContextWindowSize = 200000
	data.ContextWindow.TotalInputTokens = 373686 // cumulative lifetime counter — must NOT be used
	data.ContextWindow.TotalOutputTokens = 0

	raw, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	filePath := filepath.Join(tmpDir, sessionID+".json")
	if err := os.WriteFile(filePath, raw, 0644); err != nil {
		t.Fatal(err)
	}

	usage, err := DetectContextFromStatusLine(sessionID)
	if err != nil {
		t.Fatalf("DetectContextFromStatusLine() error = %v", err)
	}
	if usage == nil {
		t.Fatal("expected usage, got nil")
		return
	}
	// UsedTokens must be derived from percentage × window, not TotalInputTokens
	wantUsed := int(math.Round(41.0 / 100.0 * 200000)) // 82000
	if usage.UsedTokens != wantUsed {
		t.Errorf("UsedTokens = %d, want %d (must be pct×window, not cumulative %d)",
			usage.UsedTokens, wantUsed, 373686)
	}
	if usage.TotalTokens != 200000 {
		t.Errorf("TotalTokens = %d, want 200000", usage.TotalTokens)
	}
}

func TestDetectContextFromStatusLine_PostCompactNearZero(t *testing.T) {
	tmpDir := t.TempDir()
	originalDir := statusLineDir
	statusLineDir = tmpDir
	defer func() { statusLineDir = originalDir }()

	sessionID := "post-compact-near-zero"
	data := statusLineFileData{SessionID: sessionID}
	data.ContextWindow.UsedPercentage = 6.0
	data.ContextWindow.ContextWindowSize = 200000
	data.ContextWindow.TotalInputTokens = 6 // near-zero after compaction — must NOT be used
	data.ContextWindow.TotalOutputTokens = 0

	raw, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	filePath := filepath.Join(tmpDir, sessionID+".json")
	if err := os.WriteFile(filePath, raw, 0644); err != nil {
		t.Fatal(err)
	}

	usage, err := DetectContextFromStatusLine(sessionID)
	if err != nil {
		t.Fatalf("DetectContextFromStatusLine() error = %v", err)
	}
	if usage == nil {
		t.Fatal("expected usage, got nil")
		return
	}
	wantUsed := int(math.Round(6.0 / 100.0 * 200000)) // 12000
	if usage.UsedTokens != wantUsed {
		t.Errorf("UsedTokens = %d, want %d (must be pct×window, not raw %d)",
			usage.UsedTokens, wantUsed, 6)
	}
}

func TestDetectContextFromStatusLine_StaleFile(t *testing.T) {
	tmpDir := t.TempDir()
	originalDir := statusLineDir
	statusLineDir = tmpDir
	defer func() { statusLineDir = originalDir }()

	sessionID := "stale-session"
	data := statusLineFileData{SessionID: sessionID}
	data.ContextWindow.UsedPercentage = 50.0
	data.ContextWindow.ContextWindowSize = 200000
	data.ContextWindow.TotalInputTokens = 100000

	raw, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	filePath := filepath.Join(tmpDir, sessionID+".json")
	if err := os.WriteFile(filePath, raw, 0644); err != nil {
		t.Fatal(err)
	}

	// Back-date the file by 3 minutes (beyond the 2-minute TTL)
	past := time.Now().Add(-3 * time.Minute)
	if err := os.Chtimes(filePath, past, past); err != nil {
		t.Fatal(err)
	}

	usage, err := DetectContextFromStatusLine(sessionID)
	if err == nil {
		t.Errorf("expected error for stale file, got usage: %+v", usage)
	}
	if usage != nil {
		t.Errorf("expected nil usage for stale file, got %+v", usage)
	}
	if err != nil && !strings.Contains(err.Error(), "stale") {
		t.Errorf("error %q does not contain 'stale'", err.Error())
	}
}

func TestIsStatusLineFileFresh(t *testing.T) {
	tmpDir := t.TempDir()
	originalDir := statusLineDir
	statusLineDir = tmpDir
	defer func() { statusLineDir = originalDir }()

	sessionID := "fresh-check-session"
	filePath := filepath.Join(tmpDir, sessionID+".json")

	// Missing file → false
	if isStatusLineFileFresh(sessionID) {
		t.Error("isStatusLineFileFresh() = true for missing file, want false")
	}

	// Write file with current mtime → fresh → true
	if err := os.WriteFile(filePath, []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}
	if !isStatusLineFileFresh(sessionID) {
		t.Error("isStatusLineFileFresh() = false for just-written file, want true")
	}

	// Back-date by 3 minutes → stale → false
	past := time.Now().Add(-3 * time.Minute)
	if err := os.Chtimes(filePath, past, past); err != nil {
		t.Fatal(err)
	}
	if isStatusLineFileFresh(sessionID) {
		t.Error("isStatusLineFileFresh() = true for 3-min-old file (TTL=2min), want false")
	}
}

func TestDetectContextFromStatusLine_FileNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	originalDir := statusLineDir
	statusLineDir = tmpDir
	defer func() { statusLineDir = originalDir }()

	usage, err := DetectContextFromStatusLine("nonexistent-session")
	if err == nil {
		t.Errorf("expected error, got usage: %+v", usage)
	}
	if usage != nil {
		t.Errorf("expected nil usage, got %+v", usage)
	}
}

func TestDetectContextFromStatusLine_MalformedJSON(t *testing.T) {
	tmpDir := t.TempDir()
	originalDir := statusLineDir
	statusLineDir = tmpDir
	defer func() { statusLineDir = originalDir }()

	sessionID := "malformed-session"
	filePath := filepath.Join(tmpDir, sessionID+".json")
	if err := os.WriteFile(filePath, []byte("{not valid json!!!"), 0644); err != nil {
		t.Fatal(err)
	}

	usage, err := DetectContextFromStatusLine(sessionID)
	if err == nil {
		t.Errorf("expected error for malformed JSON, got usage: %+v", usage)
	}
	if usage != nil {
		t.Errorf("expected nil usage, got %+v", usage)
	}
}

// TestGetModelPricing tests pricing lookup for each model family plus unknown
func TestGetModelPricing(t *testing.T) {
	tests := []struct {
		name    string
		modelID string
		wantOK  bool
		wantIn  float64 // InputPerM
	}{
		{"opus model", "claude-opus-4-6-20251001", true, 15.0},
		{"sonnet model", "claude-sonnet-4-5-20250929", true, 3.0},
		{"haiku model", "claude-haiku-4-20260101", true, 0.80},
		{"unknown model", "gpt-4o-2024-05", false, 0},
		{"empty string", "", false, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pricing, ok := getModelPricing(tt.modelID)
			if ok != tt.wantOK {
				t.Errorf("getModelPricing(%q) ok = %v, want %v", tt.modelID, ok, tt.wantOK)
			}
			if ok && !floatEqual(pricing.InputPerM, tt.wantIn) {
				t.Errorf("InputPerM = %f, want %f", pricing.InputPerM, tt.wantIn)
			}
		})
	}
}

// TestEstimateCostFromUsage tests cost estimation from JSONL lines
func TestEstimateCostFromUsage(t *testing.T) {
	t.Run("valid sonnet line", func(t *testing.T) {
		line := `{"type":"assistant","timestamp":"2026-03-20T10:00:05Z","message":{"model":"claude-sonnet-4-5-20250929","usage":{"input_tokens":1000,"cache_creation_input_tokens":2000,"cache_read_input_tokens":5000,"output_tokens":500}}}`
		cost := estimateCostFromUsage("claude-sonnet-4-5-20250929", line)
		// input: 1000/1e6 * 3.0 = 0.003
		// output: 500/1e6 * 15.0 = 0.0075
		// cache_read: 5000/1e6 * 0.30 = 0.0015
		// cache_write: 2000/1e6 * 3.75 = 0.0075
		expected := 0.003 + 0.0075 + 0.0015 + 0.0075 // 0.0195
		if math.Abs(cost-expected) > 1e-6 {
			t.Errorf("estimateCostFromUsage() = %f, want %f", cost, expected)
		}
	})

	t.Run("empty model ID returns zero", func(t *testing.T) {
		cost := estimateCostFromUsage("", `{"type":"assistant"}`)
		if cost != 0 {
			t.Errorf("estimateCostFromUsage with empty model = %f, want 0", cost)
		}
	})

	t.Run("unknown model returns zero", func(t *testing.T) {
		cost := estimateCostFromUsage("gpt-4o", `{"type":"assistant","message":{"model":"gpt-4o","usage":{"input_tokens":1000}}}`)
		if cost != 0 {
			t.Errorf("estimateCostFromUsage with unknown model = %f, want 0", cost)
		}
	})

	t.Run("malformed JSON returns zero", func(t *testing.T) {
		cost := estimateCostFromUsage("claude-sonnet-4-5-20250929", `{bad json}`)
		if cost != 0 {
			t.Errorf("estimateCostFromUsage with bad JSON = %f, want 0", cost)
		}
	})

	t.Run("progress type entry with valid assistant", func(t *testing.T) {
		line := `{"type":"progress","data":{"message":{"type":"assistant","message":{"model":"claude-sonnet-4-5-20250929","usage":{"input_tokens":1000,"cache_creation_input_tokens":0,"cache_read_input_tokens":0,"output_tokens":500}}}}}`
		cost := estimateCostFromUsage("claude-sonnet-4-5-20250929", line)
		// input: 1000/1e6 * 3.0 = 0.003
		// output: 500/1e6 * 15.0 = 0.0075
		expected := 0.003 + 0.0075
		if math.Abs(cost-expected) > 1e-6 {
			t.Errorf("estimateCostFromUsage(progress) = %f, want %f", cost, expected)
		}
	})

	t.Run("progress type with non-assistant message returns zero", func(t *testing.T) {
		line := `{"type":"progress","data":{"message":{"type":"tool_use","message":{"model":"claude-sonnet-4-5-20250929"}}}}`
		cost := estimateCostFromUsage("claude-sonnet-4-5-20250929", line)
		if cost != 0 {
			t.Errorf("estimateCostFromUsage(progress non-assistant) = %f, want 0", cost)
		}
	})

	t.Run("user type returns zero", func(t *testing.T) {
		line := `{"type":"user","message":{"content":"hello"}}`
		cost := estimateCostFromUsage("claude-sonnet-4-5-20250929", line)
		if cost != 0 {
			t.Errorf("estimateCostFromUsage(user type) = %f, want 0", cost)
		}
	})
}

// TestScanConversationLog tests scanning with mixed entry types
func TestScanConversationLog(t *testing.T) {
	t.Run("mixed entries with cost accumulation", func(t *testing.T) {
		tmpDir := t.TempDir()
		logPath := filepath.Join(tmpDir, "mixed.jsonl")

		// Mix of system_reminder, assistant (JSONL format), and non-matching entries
		content := `{"type":"user","content":"hello"}
{"type":"assistant","timestamp":"2026-03-20T10:00:05Z","message":{"model":"claude-sonnet-4-5-20250929","usage":{"input_tokens":1000,"cache_creation_input_tokens":0,"cache_read_input_tokens":0,"output_tokens":500}}}
{"type":"system_reminder","content":"Token usage: 10000/200000; 190000 remaining","timestamp":"2026-03-15T10:00:10Z"}
{"type":"assistant","timestamp":"2026-03-20T10:01:05Z","message":{"model":"claude-sonnet-4-5-20250929","usage":{"input_tokens":2000,"cache_creation_input_tokens":0,"cache_read_input_tokens":0,"output_tokens":1000}}}
`
		if err := os.WriteFile(logPath, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}

		file, err := os.Open(logPath)
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()

		usage, err := scanConversationLog(file)
		if err != nil {
			t.Fatalf("scanConversationLog() error = %v", err)
		}
		if usage == nil {
			t.Fatal("expected usage, got nil")
			return
		}
		// Last JSONL assistant entry should be the final context reading
		// 2000 input / 200000 = 1%
		if usage.UsedTokens != 2000 {
			t.Errorf("UsedTokens = %d, want 2000 (from last assistant entry)", usage.UsedTokens)
		}
		// EstimatedCost should be accumulated across both assistant entries
		if usage.EstimatedCost <= 0 {
			t.Errorf("EstimatedCost = %f, want > 0 (accumulated from two assistant entries)", usage.EstimatedCost)
		}
	})

	t.Run("no matching entries", func(t *testing.T) {
		tmpDir := t.TempDir()
		logPath := filepath.Join(tmpDir, "empty.jsonl")

		content := `{"type":"user","content":"hello"}
{"type":"tool_use","content":"some tool"}
`
		if err := os.WriteFile(logPath, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}

		file, err := os.Open(logPath)
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()

		usage, err := scanConversationLog(file)
		if err != nil {
			t.Fatalf("scanConversationLog() error = %v", err)
		}
		if usage != nil {
			t.Errorf("expected nil for no matching entries, got %+v", usage)
		}
	})
}

// TestExtractUsageFromJSONL_ProgressType tests progress-type entries
func TestExtractUsageFromJSONL_ProgressType(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		wantNil  bool
		wantUsed int
	}{
		{
			name:     "valid progress with assistant message",
			line:     `{"type":"progress","data":{"message":{"type":"assistant","message":{"model":"claude-sonnet-4-5-20250929","usage":{"input_tokens":500,"cache_creation_input_tokens":1000,"cache_read_input_tokens":3000,"output_tokens":200}}}}}`,
			wantUsed: 4500,
		},
		{
			name:    "progress with non-assistant message",
			line:    `{"type":"progress","data":{"message":{"type":"tool_use","message":{"model":"claude-sonnet-4-5-20250929"}}}}`,
			wantNil: true,
		},
		{
			name:    "progress with nil data",
			line:    `{"type":"progress"}`,
			wantNil: true,
		},
		{
			name:    "progress with malformed data",
			line:    `{"type":"progress","data":"not an object"}`,
			wantNil: true,
		},
		{
			name:    "progress with nil inner message",
			line:    `{"type":"progress","data":{"message":{"type":"assistant"}}}`,
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			usage := extractUsageFromJSONL(tt.line)
			if tt.wantNil {
				if usage != nil {
					t.Errorf("expected nil, got %+v", usage)
				}
				return
			}
			if usage == nil {
				t.Fatal("expected usage, got nil")
				return
			}
			if usage.UsedTokens != tt.wantUsed {
				t.Errorf("UsedTokens = %d, want %d", usage.UsedTokens, tt.wantUsed)
			}
		})
	}
}

// TestReadStatusLineFile tests reading and parsing statusline JSON files
func TestReadStatusLineFile(t *testing.T) {
	tmpDir := t.TempDir()
	originalDir := statusLineDir
	statusLineDir = tmpDir
	defer func() { statusLineDir = originalDir }()

	t.Run("valid file", func(t *testing.T) {
		sessionID := "read-test"
		slData := statusLineFileData{SessionID: sessionID}
		slData.Cost.TotalCostUSD = 3.14
		slData.Model.ID = "claude-opus-4-6-20251001"
		slData.Model.DisplayName = "Opus 4.6 (1M context)"

		raw, _ := json.Marshal(slData)
		if err := os.WriteFile(filepath.Join(tmpDir, sessionID+".json"), raw, 0644); err != nil {
			t.Fatal(err)
		}

		result, err := ReadStatusLineFile(sessionID)
		if err != nil {
			t.Fatalf("ReadStatusLineFile() error = %v", err)
		}
		if !floatEqual(result.Cost.TotalCostUSD, 3.14) {
			t.Errorf("TotalCostUSD = %f, want 3.14", result.Cost.TotalCostUSD)
		}
		if result.Model.DisplayName != "Opus 4.6 (1M context)" {
			t.Errorf("DisplayName = %q, want %q", result.Model.DisplayName, "Opus 4.6 (1M context)")
		}
	})

	t.Run("missing file", func(t *testing.T) {
		_, err := ReadStatusLineFile("no-such-session")
		if err == nil {
			t.Error("expected error for missing file")
		}
	})

	t.Run("malformed JSON", func(t *testing.T) {
		sessionID := "malformed-read"
		if err := os.WriteFile(filepath.Join(tmpDir, sessionID+".json"), []byte("{bad}"), 0644); err != nil {
			t.Fatal(err)
		}
		_, err := ReadStatusLineFile(sessionID)
		if err == nil {
			t.Error("expected error for malformed JSON")
		}
	})
}

func TestDetectContextFromStatusLine_1MContext(t *testing.T) {
	tmpDir := t.TempDir()
	originalDir := statusLineDir
	statusLineDir = tmpDir
	defer func() { statusLineDir = originalDir }()

	sessionID := "1m-context-session"
	data := statusLineFileData{
		SessionID: sessionID,
	}
	data.ContextWindow.UsedPercentage = 35.0
	data.ContextWindow.ContextWindowSize = 1000000
	data.ContextWindow.TotalInputTokens = 350000
	data.ContextWindow.TotalOutputTokens = 10000
	data.Cost.TotalCostUSD = 1.25

	raw, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}

	filePath := filepath.Join(tmpDir, sessionID+".json")
	if err := os.WriteFile(filePath, raw, 0644); err != nil {
		t.Fatal(err)
	}

	usage, err := DetectContextFromStatusLine(sessionID)
	if err != nil {
		t.Fatalf("DetectContextFromStatusLine() error = %v", err)
	}
	if usage == nil {
		t.Fatal("expected usage, got nil")
		return
	}
	if usage.TotalTokens != 1000000 {
		t.Errorf("TotalTokens = %d, want 1000000 (1M context window)", usage.TotalTokens)
	}
	if usage.UsedTokens != 350000 {
		t.Errorf("UsedTokens = %d, want 350000", usage.UsedTokens)
	}
	if !floatEqual(usage.PercentageUsed, 35.0) {
		t.Errorf("PercentageUsed = %f, want 35.0", usage.PercentageUsed)
	}
	if usage.Source != "statusline" {
		t.Errorf("Source = %s, want statusline", usage.Source)
	}
}
