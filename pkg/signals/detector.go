// Package signals implements signal detection for Hybrid Progressive Rigor.
//
// Analyzes context to detect complexity signals and recommend rigor levels.
// Implements multi-signal fusion with confidence scoring.
package signals

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

// SignalType represents the type of signal detected.
type SignalType string

// Signal type constants.
const (
	SignalTypeKeyword       SignalType = "keyword"
	SignalTypeEffort        SignalType = "effort"
	SignalTypeFile          SignalType = "file"
	SignalTypeBeads         SignalType = "beads"
	SignalTypePreviousPhase SignalType = "previous_phase"
	SignalTypeUserHistory   SignalType = "user_history"
)

// Signal represents a detected complexity indicator.
type Signal struct {
	Type       SignalType `json:"type"`
	Value      string     `json:"value"`
	Confidence float64    `json:"confidence"` // 0.0-1.0
	Weight     float64    `json:"weight"`     // 0.0-1.0 (how much this signal contributes)
	Source     string     `json:"source"`     // Where signal came from (for transparency)
}

// RigorLevelName represents the rigor level.
type RigorLevelName string

// Rigor level constants.
const (
	RigorLevelMinimal       RigorLevelName = "minimal"
	RigorLevelStandard      RigorLevelName = "standard"
	RigorLevelThorough      RigorLevelName = "thorough"
	RigorLevelComprehensive RigorLevelName = "comprehensive"
)

// RigorLevel represents a recommended rigor level with metadata.
type RigorLevel struct {
	Name         RigorLevelName `json:"name"`
	TimeEstimate string         `json:"timeEstimate"`
	TokenCost    int            `json:"tokenCost"`
	Confidence   float64        `json:"confidence"` // 0.0-1.0 (confidence in this recommendation)
	Reasoning    []string       `json:"reasoning"`
	Signals      []Signal       `json:"signals"`
}

// UserAction represents the recommended user action.
type UserAction string

// User action constants.
const (
	UserActionAuto  UserAction = "auto"  // auto-escalate
	UserActionOffer UserAction = "offer" // offer choice
	UserActionNone  UserAction = "none"  // stay minimal
)

// EscalationDecision represents the decision to escalate rigor level.
type EscalationDecision struct {
	ShouldEscalate bool           `json:"shouldEscalate"`
	SuggestedLevel RigorLevelName `json:"suggestedLevel"`
	Confidence     float64        `json:"confidence"`
	Reasoning      []string       `json:"reasoning"`
	Signals        []Signal       `json:"signals"`
	UserAction     UserAction     `json:"userAction"` // auto-escalate, offer choice, or stay minimal
}

// BeadsTask represents a Beads task.
type BeadsTask struct {
	Description    string   `json:"description"`
	Labels         []string `json:"labels"`
	EstimatedHours *float64 `json:"estimatedHours,omitempty"`
}

// PreviousPhaseOutput represents output from a previous phase.
type PreviousPhaseOutput struct {
	Phase    string         `json:"phase"`
	Level    RigorLevelName `json:"level"`
	Findings string         `json:"findings"`
}

// UserHistoryEntry represents a user history entry.
type UserHistoryEntry struct {
	Phase        string          `json:"phase"`
	Signals      []Signal        `json:"signals"`
	LevelUsed    RigorLevelName  `json:"levelUsed"`
	UserOverride *RigorLevelName `json:"userOverride,omitempty"`
}

// Context represents the analysis context.
type Context struct {
	UserDescription      string                `json:"userDescription"`
	Conversation         []string              `json:"conversation,omitempty"`
	RecentFiles          []string              `json:"recentFiles,omitempty"`
	BeadsTask            *BeadsTask            `json:"beadsTask,omitempty"`
	PreviousPhaseOutputs []PreviousPhaseOutput `json:"previousPhaseOutputs,omitempty"`
	UserHistory          []UserHistoryEntry    `json:"userHistory,omitempty"`
}

// EscalationLogDecision represents the logged decision type.
type EscalationLogDecision string

// Escalation log decision constants.
const (
	EscalationLogDecisionAutoEscalate  EscalationLogDecision = "auto-escalate"
	EscalationLogDecisionOfferEscalate EscalationLogDecision = "offer-escalate"
	EscalationLogDecisionStayMinimal   EscalationLogDecision = "stay-minimal"
)

// EscalationLog represents a logged escalation decision for learning.
type EscalationLog struct {
	Phase        string                `json:"phase"`
	Decision     EscalationLogDecision `json:"decision"`
	FromLevel    RigorLevelName        `json:"fromLevel"`
	ToLevel      RigorLevelName        `json:"toLevel"`
	Confidence   float64               `json:"confidence"`
	Signals      []Signal              `json:"signals"`
	UserOverride *RigorLevelName       `json:"userOverride,omitempty"`
	Timestamp    time.Time             `json:"timestamp"`
}

// keywordConfig represents keyword configuration.
type keywordConfig struct {
	keywords   []string
	confidence float64
	level      RigorLevelName
}

// Tier 1 Keywords (High confidence 0.85-0.95, weight 0.4)
var tier1Keywords = map[string]keywordConfig{
	"compliance": {
		keywords:   []string{"HIPAA", "SOC2", "SOC 2", "GDPR", "PCI-DSS", "PCI DSS", "FDA 510(k)", "FedRAMP", "ISO 27001"},
		confidence: 0.90,
		level:      RigorLevelComprehensive,
	},
	"architecture": {
		keywords:   []string{"platform migration", "microservices", "distributed system", "event-driven architecture"},
		confidence: 0.85,
		level:      RigorLevelComprehensive,
	},
	"security": {
		keywords:   []string{"authentication system", "authorization framework", "encryption", "PKI", "identity management"},
		confidence: 0.90,
		level:      RigorLevelComprehensive,
	},
}

// Tier 2 Keywords (Medium confidence 0.65-0.75, weight 0.3)
var tier2Keywords = map[string]keywordConfig{
	"integration": {
		keywords:   []string{"OAuth", "SAML", "SSO", "single sign-on", "real-time", "API gateway", "webhook"},
		confidence: 0.70,
		level:      RigorLevelThorough,
	},
	"data": {
		keywords:   []string{"sensitive data", "user privacy", "PII", "PHI", "financial data", "payment processing"},
		confidence: 0.75,
		level:      RigorLevelThorough,
	},
	"scale": {
		keywords:   []string{"high traffic", "distributed", "load balancing", "caching", "performance critical"},
		confidence: 0.65,
		level:      RigorLevelThorough,
	},
}

// effortThreshold represents effort-based threshold.
type effortThreshold struct {
	minHours   float64
	level      RigorLevelName
	confidence float64
}

// Effort thresholds (weight 0.3)
var effortThresholds = []effortThreshold{
	{minHours: 20, level: RigorLevelComprehensive, confidence: 0.80},
	{minHours: 8, level: RigorLevelThorough, confidence: 0.70},
	{minHours: 2, level: RigorLevelStandard, confidence: 0.60},
	{minHours: 0, level: RigorLevelMinimal, confidence: 0.50},
}

// filePattern represents file pattern matching.
type filePattern struct {
	pattern    *regexp.Regexp
	context    string
	confidence float64
}

// File patterns (weight 0.1)
var filePatterns = []filePattern{
	{pattern: regexp.MustCompile(`(?i)auth.*\.(ts|js|go|py)$`), context: "security", confidence: 0.60},
	{pattern: regexp.MustCompile(`(?i)compliance\/.*\.md$`), context: "compliance", confidence: 0.70},
	{pattern: regexp.MustCompile(`(?i)security\.md$`), context: "security", confidence: 0.65},
	{pattern: regexp.MustCompile(`(?i)(oauth|saml|sso).*\.(ts|js|go|py)$`), context: "authentication", confidence: 0.65},
}

// beadsLabelMapping represents Beads label mapping.
type beadsLabelMapping struct {
	context    string
	confidence float64
}

// Beads label mappings (weight 0.1)
var beadsLabels = map[string]beadsLabelMapping{
	"security":     {context: "security", confidence: 0.65},
	"compliance":   {context: "compliance", confidence: 0.75},
	"architecture": {context: "architecture", confidence: 0.60},
	"performance":  {context: "performance", confidence: 0.55},
}

// Confidence thresholds for escalation decisions
const (
	confidenceAutoEscalate  = 0.80 // Auto-escalate without asking (high confidence)
	confidenceOfferEscalate = 0.50 // Offer escalation (medium confidence)
	// Below 0.50: Stay minimal (low confidence)
)

// detectKeywordSignals detects keyword signals from text.
func detectKeywordSignals(text string) []Signal {
	signals := []Signal{}
	lowerText := strings.ToLower(text)

	// Tier 1 keywords (high confidence, high weight)
	for category, config := range tier1Keywords {
		for _, keyword := range config.keywords {
			if strings.Contains(lowerText, strings.ToLower(keyword)) {
				signals = append(signals, Signal{
					Type:       SignalTypeKeyword,
					Value:      keyword,
					Confidence: config.confidence,
					Weight:     0.4, // Tier 1 gets 40% weight
					Source:     "Tier 1 " + category + " keyword",
				})
			}
		}
	}

	// Tier 2 keywords (medium confidence, medium weight)
	for category, config := range tier2Keywords {
		for _, keyword := range config.keywords {
			if strings.Contains(lowerText, strings.ToLower(keyword)) {
				signals = append(signals, Signal{
					Type:       SignalTypeKeyword,
					Value:      keyword,
					Confidence: config.confidence,
					Weight:     0.3, // Tier 2 gets 30% weight
					Source:     "Tier 2 " + category + " keyword",
				})
			}
		}
	}

	return signals
}

// detectEffortSignals detects effort signals from context.
func detectEffortSignals(ctx Context) []Signal {
	signals := []Signal{}

	// From Beads task
	if ctx.BeadsTask != nil && ctx.BeadsTask.EstimatedHours != nil {
		hours := *ctx.BeadsTask.EstimatedHours
		var threshold effortThreshold

		// Find matching threshold
		for _, t := range effortThresholds {
			if hours >= t.minHours {
				threshold = t
				break
			}
		}

		signals = append(signals, Signal{
			Type:       SignalTypeEffort,
			Value:      formatHours(hours),
			Confidence: threshold.confidence,
			Weight:     0.3, // Effort gets 30% weight
			Source:     "Beads task estimate",
		})
	}

	return signals
}

// formatHours formats hours as a string.
func formatHours(hours float64) string {
	if hours == float64(int(hours)) {
		return fmt.Sprintf("%d hours", int(hours))
	}
	return fmt.Sprintf("%.1f hours", hours)
}

// detectFileSignals detects file pattern signals.
func detectFileSignals(files []string) []Signal {
	signals := []Signal{}

	for _, file := range files {
		for _, fp := range filePatterns {
			if fp.pattern.MatchString(file) {
				signals = append(signals, Signal{
					Type:       SignalTypeFile,
					Value:      file,
					Confidence: fp.confidence,
					Weight:     0.1, // Files get 10% weight
					Source:     "File pattern (" + fp.context + ")",
				})
			}
		}
	}

	return signals
}

// detectBeadsSignals detects Beads label signals.
func detectBeadsSignals(beadsTask *BeadsTask) []Signal {
	signals := []Signal{}

	if beadsTask == nil || len(beadsTask.Labels) == 0 {
		return signals
	}

	for _, label := range beadsTask.Labels {
		if mapping, ok := beadsLabels[label]; ok {
			signals = append(signals, Signal{
				Type:       SignalTypeBeads,
				Value:      label,
				Confidence: mapping.confidence,
				Weight:     0.1, // Beads labels get 10% weight
				Source:     "Beads label (" + mapping.context + ")",
			})
		}
	}

	return signals
}

// detectPreviousPhaseSignals detects signals from previous phase outputs.
func detectPreviousPhaseSignals(previousPhases []PreviousPhaseOutput) []Signal {
	signals := []Signal{}

	if len(previousPhases) == 0 {
		return signals
	}

	lastPhase := previousPhases[len(previousPhases)-1]

	// If previous phase used comprehensive/thorough, suggest similar level
	switch lastPhase.Level {
	case RigorLevelComprehensive:
		signals = append(signals, Signal{
			Type:       SignalTypePreviousPhase,
			Value:      "Previous phase (" + lastPhase.Phase + ") used comprehensive",
			Confidence: 0.80,
			Weight:     0.2, // Previous phase gets 20% weight
			Source:     "Previous phase level",
		})
	case RigorLevelThorough:
		signals = append(signals, Signal{
			Type:       SignalTypePreviousPhase,
			Value:      "Previous phase (" + lastPhase.Phase + ") used thorough",
			Confidence: 0.70,
			Weight:     0.2,
			Source:     "Previous phase level",
		})
	case RigorLevelMinimal, RigorLevelStandard:
		// Minimal/standard levels don't generate escalation signals
	}

	return signals
}

// detectUserHistorySignals detects signals from user history (learning).
func detectUserHistorySignals(userHistory []UserHistoryEntry) []Signal {
	signals := []Signal{}

	if len(userHistory) == 0 {
		return signals
	}

	// Look for patterns in user overrides
	// Example: User always chooses minimal for OAuth tasks
	oauthTasks := []UserHistoryEntry{}
	for _, h := range userHistory {
		for _, s := range h.Signals {
			if strings.Contains(strings.ToLower(s.Value), "oauth") {
				oauthTasks = append(oauthTasks, h)
				break
			}
		}
	}

	if len(oauthTasks) >= 3 {
		overridesToMinimal := 0
		for _, t := range oauthTasks {
			if t.UserOverride != nil && *t.UserOverride == RigorLevelMinimal {
				overridesToMinimal++
			}
		}

		if float64(overridesToMinimal)/float64(len(oauthTasks)) > 0.7 {
			// User prefers minimal for OAuth tasks
			signals = append(signals, Signal{
				Type:       SignalTypeUserHistory,
				Value:      "User typically chooses minimal for OAuth tasks",
				Confidence: 0.60,
				Weight:     0.1, // User history gets 10% weight
				Source:     fmt.Sprintf("User history (%d OAuth tasks)", len(oauthTasks)),
			})
		}
	}

	return signals
}

// fuseSignals fuses multiple signals into single confidence score.
// Uses weighted average of signal confidences.
func fuseSignals(signals []Signal) (confidence float64, level RigorLevelName) {
	if len(signals) == 0 {
		return 0.30, RigorLevelMinimal
	}

	// Calculate weighted confidence
	totalWeightedConfidence := 0.0
	totalWeight := 0.0

	for _, signal := range signals {
		totalWeightedConfidence += signal.Confidence * signal.Weight
		totalWeight += signal.Weight
	}

	fusedConfidence := 0.30
	if totalWeight > 0 {
		fusedConfidence = totalWeightedConfidence / totalWeight
	}

	// Determine level based on fused confidence
	suggestedLevel := RigorLevelMinimal
	switch {
	case fusedConfidence >= 0.85:
		suggestedLevel = RigorLevelComprehensive
	case fusedConfidence >= 0.70:
		suggestedLevel = RigorLevelThorough
	case fusedConfidence >= 0.50:
		suggestedLevel = RigorLevelStandard
	}

	return fusedConfidence, suggestedLevel
}

// generateReasoning generates human-readable reasoning from signals.
func generateReasoning(signals []Signal, confidence float64, level RigorLevelName) []string {
	reasoning := []string{}

	// Sort signals by impact (confidence * weight)
	sortedSignals := make([]Signal, len(signals))
	copy(sortedSignals, signals)
	sort.Slice(sortedSignals, func(i, j int) bool {
		return (sortedSignals[i].Confidence * sortedSignals[i].Weight) >
			(sortedSignals[j].Confidence * sortedSignals[j].Weight)
	})

	// Take top 3 signals
	topN := 3
	if len(sortedSignals) < topN {
		topN = len(sortedSignals)
	}

	for i := 0; i < topN; i++ {
		signal := sortedSignals[i]
		reasoning = append(reasoning,
			signal.Value+" (confidence: "+formatFloat(signal.Confidence)+", source: "+signal.Source+")")
	}

	reasoning = append(reasoning,
		"Fused confidence: "+formatFloat(confidence)+" → "+string(level)+" level")

	return reasoning
}

// formatFloat formats a float64 to 2 decimal places.
func formatFloat(f float64) string {
	return fmt.Sprintf("%.2f", f)
}

// AnalyzeContext analyzes context and recommends rigor level.
func AnalyzeContext(ctx Context) EscalationDecision {
	allSignals := []Signal{}

	// Detect all signal types
	allSignals = append(allSignals, detectKeywordSignals(ctx.UserDescription)...)
	conversationText := strings.Join(ctx.Conversation, " ")
	allSignals = append(allSignals, detectKeywordSignals(conversationText)...)
	allSignals = append(allSignals, detectEffortSignals(ctx)...)
	allSignals = append(allSignals, detectFileSignals(ctx.RecentFiles)...)
	allSignals = append(allSignals, detectBeadsSignals(ctx.BeadsTask)...)
	allSignals = append(allSignals, detectPreviousPhaseSignals(ctx.PreviousPhaseOutputs)...)
	allSignals = append(allSignals, detectUserHistorySignals(ctx.UserHistory)...)

	// Fuse signals
	confidence, level := fuseSignals(allSignals)

	// Generate reasoning
	reasoning := generateReasoning(allSignals, confidence, level)

	// Determine user action based on confidence
	var userAction UserAction
	var shouldEscalate bool

	switch {
	case confidence >= confidenceAutoEscalate:
		// High confidence → auto-escalate
		userAction = UserActionAuto
		shouldEscalate = level != RigorLevelMinimal
	case confidence >= confidenceOfferEscalate:
		// Medium confidence → offer escalation
		userAction = UserActionOffer
		shouldEscalate = level != RigorLevelMinimal
	default:
		// Low confidence → stay minimal
		userAction = UserActionNone
		shouldEscalate = false
	}

	suggestedLevel := level
	if !shouldEscalate {
		suggestedLevel = RigorLevelMinimal
	}

	return EscalationDecision{
		ShouldEscalate: shouldEscalate,
		SuggestedLevel: suggestedLevel,
		Confidence:     confidence,
		Reasoning:      reasoning,
		Signals:        allSignals,
		UserAction:     userAction,
	}
}

// LogEscalation logs escalation decision for learning.
func LogEscalation(phase string, decision EscalationDecision, userOverride *RigorLevelName) EscalationLog {
	var logDecision EscalationLogDecision
	switch decision.UserAction {
	case UserActionAuto:
		logDecision = EscalationLogDecisionAutoEscalate
	case UserActionOffer:
		logDecision = EscalationLogDecisionOfferEscalate
	case UserActionNone:
		logDecision = EscalationLogDecisionStayMinimal
	}

	toLevel := decision.SuggestedLevel
	if userOverride != nil {
		toLevel = *userOverride
	}

	return EscalationLog{
		Phase:        phase,
		Decision:     logDecision,
		FromLevel:    RigorLevelMinimal,
		ToLevel:      toLevel,
		Confidence:   decision.Confidence,
		Signals:      decision.Signals,
		UserOverride: userOverride,
		Timestamp:    time.Now(),
	}
}
