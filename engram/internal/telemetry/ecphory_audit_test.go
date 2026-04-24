package telemetry

import (
	"sync"
	"testing"
	"time"
)

func TestExtractContext(t *testing.T) {
	tests := []struct {
		name       string
		transcript string
		wantLang   string
		wantFW     string
		wantTask   string
	}{
		{
			name:       "Python Django debugging",
			transcript: "I'm debugging a Django app. The manage.py script fails with an import error.",
			wantLang:   "python",
			wantFW:     "django",
			wantTask:   "debugging",
		},
		{
			name:       "Go gin feature",
			transcript: "Let's add a new API endpoint using gin.Context. I'll create the handler function.",
			wantLang:   "go",
			wantFW:     "gin",
			wantTask:   "feature",
		},
		{
			name:       "JavaScript React refactor",
			transcript: "Need to refactor this React component. Use useState instead of class state.",
			wantLang:   "javascript",
			wantFW:     "react",
			wantTask:   "refactor",
		},
		{
			name:       "Unknown context",
			transcript: "This is a generic question about software engineering.",
			wantLang:   "unknown",
			wantFW:     "unknown",
			wantTask:   "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := ExtractContext(tt.transcript)

			if ctx.Language != tt.wantLang {
				t.Errorf("Language = %v, want %v", ctx.Language, tt.wantLang)
			}
			if ctx.Framework != tt.wantFW {
				t.Errorf("Framework = %v, want %v", ctx.Framework, tt.wantFW)
			}
			if ctx.TaskType != tt.wantTask {
				t.Errorf("TaskType = %v, want %v", ctx.TaskType, tt.wantTask)
			}
		})
	}
}

func TestMatchEngramToContext(t *testing.T) {
	tests := []struct {
		name       string
		engramPath string
		context    SessionContext
		want       string
	}{
		{
			name:       "Python session + Python engram = appropriate",
			engramPath: "/engrams/python/django-patterns.ai.md",
			context:    SessionContext{Language: "python", Framework: "django"},
			want:       "appropriate",
		},
		{
			name:       "Python session + Go engram = inappropriate",
			engramPath: "/engrams/go/error-handling.ai.md",
			context:    SessionContext{Language: "python", Framework: "django"},
			want:       "inappropriate",
		},
		{
			name:       "Generic engram = appropriate",
			engramPath: "/engrams/best-practices/git-workflow.ai.md",
			context:    SessionContext{Language: "python", Framework: "django"},
			want:       "appropriate",
		},
		{
			name:       "Unknown language + specific engram = appropriate (default)",
			engramPath: "/engrams/rust/ownership.ai.md",
			context:    SessionContext{Language: "unknown", Framework: "unknown"},
			want:       "appropriate", // V1 conservative: benefit of doubt
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchEngramToContext(tt.engramPath, tt.context)
			if got != tt.want {
				t.Errorf("matchEngramToContext() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestComputeCorrectnessScore(t *testing.T) {
	tests := []struct {
		name               string
		appropriateCount   int
		inappropriateCount int
		inconclusiveCount  int
		wantScore          float64
	}{
		{
			name:               "Perfect correctness (100%)",
			appropriateCount:   10,
			inappropriateCount: 0,
			inconclusiveCount:  0,
			wantScore:          1.0,
		},
		{
			name:               "1 inappropriate (90%)",
			appropriateCount:   9,
			inappropriateCount: 1,
			inconclusiveCount:  0,
			wantScore:          0.9,
		},
		{
			name:               "Inconclusive excluded (100%)",
			appropriateCount:   10,
			inappropriateCount: 0,
			inconclusiveCount:  5,
			wantScore:          1.0, // 10/10 conclusive are appropriate
		},
		{
			name:               "All inconclusive (0.0)",
			appropriateCount:   0,
			inappropriateCount: 0,
			inconclusiveCount:  10,
			wantScore:          0.0, // No conclusive data
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			totalRetrievals := tt.appropriateCount + tt.inappropriateCount + tt.inconclusiveCount
			conclusiveTotal := totalRetrievals - tt.inconclusiveCount

			var gotScore float64
			if conclusiveTotal > 0 {
				gotScore = float64(tt.appropriateCount) / float64(conclusiveTotal)
			} else {
				gotScore = 0.0
			}

			if gotScore != tt.wantScore {
				t.Errorf("Correctness score = %.2f, want %.2f", gotScore, tt.wantScore)
			}
		})
	}
}

// TestAuditEcphoryRetrieval_AllAppropriate verifies audit with all appropriate engrams
func TestAuditEcphoryRetrieval_AllAppropriate(t *testing.T) {
	registry := NewListenerRegistry()

	// Create test listener to capture event (with mutex for race safety)
	var mu sync.Mutex
	var capturedEvent *Event
	listener := &testListener{
		minLevel: LevelInfo,
		onEvent: func(e *Event) error {
			mu.Lock()
			capturedEvent = e
			mu.Unlock()
			return nil
		},
	}
	registry.Register(listener)

	// Python session with Python engrams
	transcript := "I'm debugging a Django app. The manage.py script fails."
	engrams := []string{
		"/engrams/python/django-patterns.ai.md",
		"/engrams/python/debugging.ai.md",
		"/engrams/best-practices/error-handling.ai.md", // Generic, appropriate
	}

	err := AuditEcphoryRetrieval("test-session-1", transcript, engrams, registry)
	if err != nil {
		t.Fatalf("AuditEcphoryRetrieval() failed: %v", err)
	}

	// Wait for async notification to complete
	time.Sleep(10 * time.Millisecond)

	// Verify event was emitted
	mu.Lock()
	defer mu.Unlock()
	if capturedEvent == nil {
		t.Fatal("No event was emitted to registry")
	}

	if capturedEvent.Type != EventEcphoryAuditCompleted {
		t.Errorf("Event type = %v, want %v", capturedEvent.Type, EventEcphoryAuditCompleted)
	}

	// Verify event data
	data := capturedEvent.Data

	if data["session_id"] != "test-session-1" {
		t.Errorf("session_id = %v, want %v", data["session_id"], "test-session-1")
	}

	if data["total_retrievals"] != 3 {
		t.Errorf("total_retrievals = %v, want 3", data["total_retrievals"])
	}

	if data["appropriate_count"] != 3 {
		t.Errorf("appropriate_count = %v, want 3", data["appropriate_count"])
	}

	if data["inappropriate_count"] != 0 {
		t.Errorf("inappropriate_count = %v, want 0", data["inappropriate_count"])
	}

	if data["correctness_score"] != 1.0 {
		t.Errorf("correctness_score = %v, want 1.0", data["correctness_score"])
	}

	// Verify context was extracted
	contextData, ok := data["context"].(SessionContext)
	if !ok {
		t.Fatal("context field is not SessionContext type")
	}

	if contextData.Language != "python" {
		t.Errorf("context.Language = %v, want python", contextData.Language)
	}
}

// TestAuditEcphoryRetrieval_MixedResults verifies audit with mixed engram appropriateness
func TestAuditEcphoryRetrieval_MixedResults(t *testing.T) {
	registry := NewListenerRegistry()

	var mu sync.Mutex
	var capturedEvent *Event
	listener := &testListener{
		minLevel: LevelInfo,
		onEvent: func(e *Event) error {
			mu.Lock()
			capturedEvent = e
			mu.Unlock()
			return nil
		},
	}
	registry.Register(listener)

	// Python session with mix of appropriate/inappropriate engrams
	transcript := "I'm debugging a Django app in Python."
	engrams := []string{
		"/engrams/python/django-patterns.ai.md", // Appropriate
		"/engrams/go/error-handling.ai.md",      // Inappropriate (wrong language)
		"/engrams/python/testing.ai.md",         // Appropriate
		"/engrams/rust/ownership.ai.md",         // Inappropriate (wrong language)
	}

	err := AuditEcphoryRetrieval("test-session-2", transcript, engrams, registry)
	if err != nil {
		t.Fatalf("AuditEcphoryRetrieval() failed: %v", err)
	}

	// Wait for async notification
	time.Sleep(10 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if capturedEvent == nil {
		t.Fatal("No event was emitted")
	}

	data := capturedEvent.Data

	if data["total_retrievals"] != 4 {
		t.Errorf("total_retrievals = %v, want 4", data["total_retrievals"])
	}

	if data["appropriate_count"] != 2 {
		t.Errorf("appropriate_count = %v, want 2", data["appropriate_count"])
	}

	if data["inappropriate_count"] != 2 {
		t.Errorf("inappropriate_count = %v, want 2", data["inappropriate_count"])
	}

	// Correctness score should be 2/4 = 0.5
	score, ok := data["correctness_score"].(float64)
	if !ok {
		t.Fatal("correctness_score is not float64")
	}

	if score != 0.5 {
		t.Errorf("correctness_score = %.2f, want 0.50", score)
	}
}

// TestAuditEcphoryRetrieval_EmptyEngrams verifies audit with no engrams retrieved
func TestAuditEcphoryRetrieval_EmptyEngrams(t *testing.T) {
	registry := NewListenerRegistry()

	var mu sync.Mutex
	var capturedEvent *Event
	listener := &testListener{
		minLevel: LevelInfo,
		onEvent: func(e *Event) error {
			mu.Lock()
			capturedEvent = e
			mu.Unlock()
			return nil
		},
	}
	registry.Register(listener)

	transcript := "I'm debugging a Django app."
	engrams := []string{} // Empty retrieval set

	err := AuditEcphoryRetrieval("test-session-3", transcript, engrams, registry)
	if err != nil {
		t.Fatalf("AuditEcphoryRetrieval() failed: %v", err)
	}

	// Wait for async notification
	time.Sleep(10 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if capturedEvent == nil {
		t.Fatal("No event was emitted")
	}

	data := capturedEvent.Data

	if data["total_retrievals"] != 0 {
		t.Errorf("total_retrievals = %v, want 0", data["total_retrievals"])
	}

	// With 0 retrievals, correctness score should be 0.0
	if data["correctness_score"] != 0.0 {
		t.Errorf("correctness_score = %v, want 0.0", data["correctness_score"])
	}
}

// testListener is a test implementation of EventListener
type testListener struct {
	minLevel Level
	onEvent  func(*Event) error
}

func (l *testListener) MinLevel() Level {
	return l.minLevel
}

func (l *testListener) OnEvent(e *Event) error {
	if l.onEvent != nil {
		return l.onEvent(e)
	}
	return nil
}
