package metacontext

import (
	"math"
	"sort"
	"time"
)

// AnalyzeContext contains contextual information for importance scoring.
// Used in importance-based signal prioritization (Section 2.11).
type AnalyzeContext struct {
	FileModTimes        map[string]time.Time // File path → last modified time
	ConversationMatches map[string]int       // Signal name → mention count
	PrimarySignals      map[string]bool      // Signal name → is primary (most files)
}

// SignalScore pairs a signal with its calculated importance score.
type SignalScore struct {
	Signal     Signal
	Importance float64
}

// deduplicateSignalsWithImportance deduplicates and truncates signals by importance.
// CRITICAL FIX #3: Sorts by importance (not confidence) to prevent user-mentioned signal truncation.
func deduplicateSignalsWithImportance(signals []Signal, context AnalyzeContext, maxSignals int) []Signal {
	// 1. Group by name, merge duplicates
	grouped := make(map[string]Signal)
	for _, sig := range signals {
		existing, exists := grouped[sig.Name]
		if !exists || sig.Confidence > existing.Confidence {
			grouped[sig.Name] = sig
		}
	}

	// 2. Calculate importance scores
	scored := make([]SignalScore, 0, len(grouped))
	for _, sig := range grouped {
		importance := calculateImportance(sig, context)
		scored = append(scored, SignalScore{
			Signal:     sig,
			Importance: importance,
		})
	}

	// 3. Sort by importance (NOT confidence)
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].Importance > scored[j].Importance
	})

	// 4. Truncate to max
	if len(scored) > maxSignals {
		scored = scored[:maxSignals]
	}

	// 5. Extract signals
	result := make([]Signal, len(scored))
	for i, s := range scored {
		result[i] = s.Signal
	}

	return result
}

// calculateImportance computes multi-factor importance score.
// Factors: Confidence (40%), Recency (20%), User mentions (30%), Primary (10%).
// Implements Context Window Expert Review finding (Section 2.11).
func calculateImportance(sig Signal, context AnalyzeContext) float64 {
	score := 0.0

	// Factor 1: Confidence (40% weight) - prevalence in codebase
	score += sig.Confidence * 0.4

	// Factor 2: Recency (20% weight) - recently modified files
	if lastMod, ok := context.FileModTimes[sig.Source]; ok {
		daysSince := time.Since(lastMod).Hours() / 24.0
		recency := math.Max(0.0, 1.0-(daysSince/30.0)) // Decay over 30 days
		score += recency * 0.2
	}

	// Factor 3: User mentions (30% weight) - conversation signals
	mentions := context.ConversationMatches[sig.Name]
	mentionScore := math.Min(1.0, float64(mentions)/5.0) // Normalize to 0-1
	score += mentionScore * 0.3

	// Factor 4: Primary signal (10% weight) - dominant technology
	if context.PrimarySignals[sig.Name] {
		score += 0.1
	}

	return score
}

// buildAnalyzeContext constructs context for importance scoring.
func buildAnalyzeContext(req *AnalyzeRequest, allSignals []Signal) AnalyzeContext {
	context := AnalyzeContext{
		FileModTimes:        make(map[string]time.Time),
		ConversationMatches: make(map[string]int),
		PrimarySignals:      make(map[string]bool),
	}

	// Extract conversation matches (count mentions per signal)
	for _, sig := range allSignals {
		if sig.Source == "conversation" {
			context.ConversationMatches[sig.Name]++
		}
	}

	// Identify primary signals (highest confidence per category)
	maxConfidence := make(map[string]float64)
	for _, sig := range allSignals {
		if sig.Confidence > maxConfidence[sig.Source] {
			maxConfidence[sig.Source] = sig.Confidence
			context.PrimarySignals[sig.Name] = true
		}
	}

	return context
}

// groupByType categorizes signals into languages, frameworks, tools.
func groupByType(signals []Signal) (languages, frameworks, tools []Signal) {
	// Simple heuristic: source-based categorization
	for _, sig := range signals {
		switch sig.Source {
		case "file":
			if isLanguage(sig.Name) {
				languages = append(languages, sig)
			} else {
				frameworks = append(frameworks, sig)
			}
		case "dependency":
			frameworks = append(frameworks, sig)
		case "git":
			tools = append(tools, sig)
		case "conversation":
			// Conversation signals can be any type
			if isLanguage(sig.Name) {
				languages = append(languages, sig)
			} else if isFramework(sig.Name) {
				frameworks = append(frameworks, sig)
			} else {
				tools = append(tools, sig)
			}
		default:
			tools = append(tools, sig)
		}
	}

	return languages, frameworks, tools
}

// isLanguage checks if signal name is a programming language.
func isLanguage(name string) bool {
	languages := map[string]bool{
		"Go": true, "TypeScript": true, "JavaScript": true,
		"Python": true, "Java": true, "C++": true, "C": true,
		"Rust": true, "Ruby": true, "PHP": true,
	}
	return languages[name]
}

// isFramework checks if signal name is a framework.
func isFramework(name string) bool {
	frameworks := map[string]bool{
		"React": true, "Vue": true, "Angular": true, "Next.js": true,
		"Django": true, "Flask": true, "FastAPI": true, "Express": true,
		"Gin": true, "Fiber": true, "Echo": true,
	}
	return frameworks[name]
}
