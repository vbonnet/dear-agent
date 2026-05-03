package hippocampus

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// AutodreamConfig configures the cross-session memory consolidation pipeline.
type AutodreamConfig struct {
	MinSessionGap   time.Duration // minimum time since last consolidation (default: 24h)
	MinSessionCount int           // minimum sessions since last consolidation (default: 5)
	MaxMemoryLines  int           // maximum MEMORY.md line count (default: 200)
	MaxTopicFiles   int           // maximum topic files (default: 20)
	DryRun          bool          // preview changes without writing
	Verbose         bool          // verbose output
	StateFile       string        // trigger state file path
}

// DefaultConfig returns the default autodream configuration.
func DefaultConfig() AutodreamConfig {
	home, _ := os.UserHomeDir()
	return AutodreamConfig{
		MinSessionGap:   24 * time.Hour,
		MinSessionCount: 5,
		MaxMemoryLines:  200,
		MaxTopicFiles:   20,
		StateFile:       filepath.Join(home, ".engram", "consolidation", "trigger-state.json"),
	}
}

// Autodream orchestrates the 4-phase cross-session memory consolidation pipeline.
type Autodream struct {
	memoryDir string         // path to MEMORY.md directory
	harness   HarnessAdapter // session discovery + transcript reading
	llm       LLMProvider    // optional LLM for V2 extraction
	config    AutodreamConfig
}

// NewAutodream creates a new Autodream pipeline.
func NewAutodream(memoryDir string, harness HarnessAdapter, llm LLMProvider, config AutodreamConfig) *Autodream {
	if llm == nil {
		llm = &NoopLLM{}
	}
	return &Autodream{
		memoryDir: memoryDir,
		harness:   harness,
		llm:       llm,
		config:    config,
	}
}

// ConsolidationReport describes all changes made during consolidation.
type ConsolidationReport struct {
	Timestamp       time.Time
	SessionsScanned int
	SignalsFound    int
	EntriesAdded    int
	EntriesUpdated  int
	EntriesPruned   int
	Contradictions  []Contradiction
	DryRun          bool
	Diff            string // unified diff of MEMORY.md changes
}

// MemoryState represents the current state of the memory system.
type MemoryState struct {
	MemoryDoc         *MemoryDocument
	MemoryPath        string
	TopicFiles        []TopicFile
	LastConsolidation time.Time
	SessionsSinceLast int
}

// TopicFile represents a topic-specific memory file.
type TopicFile struct {
	Name    string
	Path    string
	Content string
	ModTime time.Time
}

// Run executes the 4-phase consolidation pipeline.
func (a *Autodream) Run(ctx context.Context) (*ConsolidationReport, error) {
	report := &ConsolidationReport{
		Timestamp: time.Now(),
		DryRun:    a.config.DryRun,
	}

	// Phase 1: Orient
	state, err := a.orient(ctx)
	if err != nil {
		return nil, fmt.Errorf("orient: %w", err)
	}

	// Phase 2: Gather
	signals, err := a.gather(ctx, state)
	if err != nil {
		return nil, fmt.Errorf("gather: %w", err)
	}
	report.SignalsFound = len(signals)

	if len(signals) == 0 {
		return report, nil // nothing to consolidate
	}

	// Phase 3: Consolidate
	result, err := a.consolidate(ctx, state, signals)
	if err != nil {
		return nil, fmt.Errorf("consolidate: %w", err)
	}
	report.EntriesAdded = len(result.added)
	report.EntriesUpdated = len(result.updated)
	report.Contradictions = result.contradictions

	// Phase 4: Prune
	pruneResult, err := a.prune(ctx, state, result)
	if err != nil {
		return nil, fmt.Errorf("prune: %w", err)
	}
	report.EntriesPruned = pruneResult.entriesPruned

	// Generate diff
	originalContent, _ := os.ReadFile(state.MemoryPath)
	report.Diff = generateDiff(string(originalContent), state.MemoryDoc.Render())

	// Write results
	if !a.config.DryRun {
		if err := a.writeResults(state, pruneResult); err != nil {
			return nil, fmt.Errorf("write results: %w", err)
		}

		// Update trigger state
		triggerState, _ := LoadTriggerState(a.config.StateFile)
		triggerState.ResetAfterConsolidation()
		if err := SaveTriggerState(a.config.StateFile, triggerState); err != nil {
			return nil, fmt.Errorf("save trigger state: %w", err)
		}
	}

	return report, nil
}

// orient reads the current memory state (Phase 1).
func (a *Autodream) orient(ctx context.Context) (*MemoryState, error) {
	_ = ctx // reserved for future use

	memPath := filepath.Join(a.memoryDir, "MEMORY.md")

	content, err := os.ReadFile(memPath)
	if err != nil {
		if os.IsNotExist(err) {
			// No MEMORY.md yet — start fresh
			doc, _ := ParseMemoryMD("")
			return &MemoryState{
				MemoryDoc:  doc,
				MemoryPath: memPath,
			}, nil
		}
		return nil, fmt.Errorf("read MEMORY.md: %w", err)
	}

	doc, err := ParseMemoryMD(string(content))
	if err != nil {
		return nil, fmt.Errorf("parse MEMORY.md: %w", err)
	}

	// Load topic files
	topicFiles, err := a.loadTopicFiles()
	if err != nil {
		return nil, fmt.Errorf("load topic files: %w", err)
	}

	// Load trigger state
	triggerState, _ := LoadTriggerState(a.config.StateFile)

	return &MemoryState{
		MemoryDoc:         doc,
		MemoryPath:        memPath,
		TopicFiles:        topicFiles,
		LastConsolidation: triggerState.LastConsolidation,
		SessionsSinceLast: triggerState.SessionCount,
	}, nil
}

// gather scans session transcripts for consolidation signals (Phase 2).
func (a *Autodream) gather(ctx context.Context, state *MemoryState) ([]Signal, error) {
	// Discover sessions since last consolidation
	sessions, err := a.harness.DiscoverSessions(ctx, "", state.LastConsolidation)
	if err != nil {
		return nil, fmt.Errorf("discover sessions: %w", err)
	}

	var allSignals []Signal

	for _, session := range sessions {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		transcript, err := a.harness.ReadTranscript(ctx, session)
		if err != nil {
			continue // skip sessions we can't read
		}

		// V1: Pattern-based signal extraction
		signals := extractSignalsV1(transcript, session.ID)
		allSignals = append(allSignals, signals...)

		// V2: LLM-enhanced extraction (if configured)
		llmSignals, err := a.llm.ExtractSignals(ctx, transcript)
		if err == nil && len(llmSignals) > 0 {
			allSignals = append(allSignals, llmSignals...)
		}
	}

	// V3: Extract signals from daily logs (KAIROS-lite)
	dailyLogger := NewDailyLogger("")
	logPaths, err := dailyLogger.FeedToAutodream(7)
	if err == nil {
		for _, logPath := range logPaths {
			content, err := os.ReadFile(logPath)
			if err != nil {
				continue
			}
			signals := extractSignalsV1(string(content), "daily-log")
			allSignals = append(allSignals, signals...)
		}
	}

	return deduplicateSignals(allSignals), nil
}

// consolidationResult holds the merge result.
type consolidationResult struct {
	added          []string
	updated        []string
	contradictions []Contradiction
}

// consolidate merges signals into the memory document (Phase 3).
func (a *Autodream) consolidate(ctx context.Context, state *MemoryState, signals []Signal) (*consolidationResult, error) {
	result := &consolidationResult{}

	// Collect incoming entries for contradiction detection
	var incomingEntries []string
	for _, signal := range signals {
		incomingEntries = append(incomingEntries, formatSignalEntry(signal))
	}

	// Detect contradictions between existing memory and incoming signals
	var existingEntries []string
	for _, sec := range state.MemoryDoc.Sections {
		existingEntries = append(existingEntries, sec.Content...)
		for _, child := range sec.Children {
			existingEntries = append(existingEntries, child.Content...)
		}
	}

	if len(existingEntries) > 0 && len(incomingEntries) > 0 {
		contradictions, err := a.llm.DetectContradictions(ctx, existingEntries, incomingEntries)
		if err == nil && len(contradictions) > 0 {
			result.contradictions = contradictions

			// Apply contradiction resolutions: remove losers from existing entries
			for _, c := range contradictions {
				if c.Winner == "new" {
					// Remove the contradicted existing entry
					for i := range state.MemoryDoc.Sections {
						sec := &state.MemoryDoc.Sections[i]
						sec.Content = removeEntry(sec.Content, c.Existing)
						for j := range sec.Children {
							sec.Children[j].Content = removeEntry(sec.Children[j].Content, c.Existing)
						}
					}
				}
			}
		}
	}

	for _, signal := range signals {
		section := signalToSection(signal.Type)
		entry := formatSignalEntry(signal)

		// Skip incoming entries that lost contradiction resolution
		if isContradictionLoser(result.contradictions, entry) {
			continue
		}

		// Check for existing similar entry
		sec := state.MemoryDoc.FindSection(section)
		if sec != nil {
			existingIdx := findSimilarEntry(sec.Content, entry)
			if existingIdx >= 0 {
				// Update existing entry (newer wins)
				sec.Content[existingIdx] = entry
				result.updated = append(result.updated, entry)
				continue
			}
		}

		// Add new entry
		state.MemoryDoc.AddEntry(section, entry)
		result.added = append(result.added, entry)
	}

	return result, nil
}

// removeEntry removes the first entry containing the target text.
func removeEntry(entries []string, target string) []string {
	for i, e := range entries {
		if strings.Contains(e, target) {
			return append(entries[:i], entries[i+1:]...)
		}
	}
	return entries
}

// isContradictionLoser returns true if entry was the losing side of a contradiction.
func isContradictionLoser(contradictions []Contradiction, entry string) bool {
	for _, c := range contradictions {
		if c.Winner == "existing" && strings.Contains(entry, c.New) {
			return true
		}
	}
	return false
}

// pruneResult holds pruning outcome.
type pruneResult struct {
	entriesPruned int
	topicOverflow map[string]string // topic file name -> content
	archived      []string          // topic files moved to archive
}

// prune enforces size limits and manages topic files (Phase 4).
func (a *Autodream) prune(_ context.Context, state *MemoryState, _ *consolidationResult) (*pruneResult, error) {
	result := &pruneResult{
		topicOverflow: make(map[string]string),
	}

	// Check if MEMORY.md exceeds line limit
	lineCount := state.MemoryDoc.LineCount()
	if lineCount <= a.config.MaxMemoryLines {
		return result, nil // within limits
	}

	// Overflow strategy: move large sections to topic files
	for i := range state.MemoryDoc.Sections {
		if state.MemoryDoc.LineCount() <= a.config.MaxMemoryLines {
			break
		}

		for j := range state.MemoryDoc.Sections[i].Children {
			child := &state.MemoryDoc.Sections[i].Children[j]
			if len(child.Content) > 5 { // only overflow large sections
				topicName := sectionToTopicFile(child.Heading)
				result.topicOverflow[topicName] = renderSectionContent(child)

				// Replace content with topic file reference
				ref := fmt.Sprintf("- See `%s` for details", topicName)
				result.entriesPruned += len(child.Content) - 1
				child.Content = []string{ref}
			}
		}
	}

	// Enforce MaxTopicFiles: archive oldest when over limit
	if a.config.MaxTopicFiles > 0 {
		totalTopics := len(state.TopicFiles) + len(result.topicOverflow)
		if totalTopics > a.config.MaxTopicFiles {
			result.archived = a.archiveOldestTopics(state, totalTopics-a.config.MaxTopicFiles)
		}
	}

	return result, nil
}

// archiveOldestTopics moves the oldest topic files to an archive subdirectory.
func (a *Autodream) archiveOldestTopics(state *MemoryState, count int) []string {
	if count <= 0 || len(state.TopicFiles) == 0 {
		return nil
	}

	sorted := make([]TopicFile, len(state.TopicFiles))
	copy(sorted, state.TopicFiles)
	sortTopicsByAge(sorted)

	if count > len(sorted) {
		count = len(sorted)
	}

	archiveDir := filepath.Join(a.memoryDir, "archive")
	var archived []string

	for _, tf := range sorted[:count] {
		if err := os.MkdirAll(archiveDir, 0o700); err != nil {
			continue
		}
		dest := filepath.Join(archiveDir, tf.Name)
		if err := os.Rename(tf.Path, dest); err != nil {
			continue
		}
		archived = append(archived, tf.Name)
	}

	return archived
}

// sortTopicsByAge sorts topic files by ModTime ascending (oldest first).
func sortTopicsByAge(topics []TopicFile) {
	for i := 1; i < len(topics); i++ {
		for j := i; j > 0 && topics[j].ModTime.Before(topics[j-1].ModTime); j-- {
			topics[j], topics[j-1] = topics[j-1], topics[j]
		}
	}
}

// writeResults atomically writes updated MEMORY.md and topic files.
func (a *Autodream) writeResults(state *MemoryState, prune *pruneResult) error {
	// Write MEMORY.md atomically
	content := state.MemoryDoc.Render()
	if err := atomicWriteFile(state.MemoryPath, []byte(content)); err != nil {
		return fmt.Errorf("write MEMORY.md: %w", err)
	}

	// Write topic files
	for name, topicContent := range prune.topicOverflow {
		path := filepath.Join(a.memoryDir, name)
		if err := atomicWriteFile(path, []byte(topicContent)); err != nil {
			return fmt.Errorf("write topic file %s: %w", name, err)
		}
	}

	return nil
}

// loadTopicFiles reads all .md files in the memory directory (except MEMORY.md).
func (a *Autodream) loadTopicFiles() ([]TopicFile, error) {
	entries, err := os.ReadDir(a.memoryDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var topics []TopicFile
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".md") || name == "MEMORY.md" {
			continue
		}

		path := filepath.Join(a.memoryDir, name)
		content, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		info, _ := entry.Info()
		modTime := time.Time{}
		if info != nil {
			modTime = info.ModTime()
		}

		topics = append(topics, TopicFile{
			Name:    name,
			Path:    path,
			Content: string(content),
			ModTime: modTime,
		})
	}

	return topics, nil
}

// atomicWriteFile writes content to a file atomically.
// Creates .bak backup, writes to temp file, then renames.
func atomicWriteFile(path string, content []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}

	// Backup existing file
	if _, err := os.Stat(path); err == nil {
		backupPath := path + ".bak"
		data, err := os.ReadFile(path)
		if err == nil {
			_ = os.WriteFile(backupPath, data, 0o600)
		}
	}

	// Write to temp file
	tmp, err := os.CreateTemp(dir, ".memory-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()

	if _, err := tmp.Write(content); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("write temp file: %w", err)
	}
	tmp.Close()

	// Atomic rename
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("rename temp to target: %w", err)
	}

	return nil
}

// --- V1 Pattern-Based Signal Extraction ---

var (
	correctionPatterns = regexp.MustCompile(`(?i)(no,?\s*actually|that's wrong|don't do that|stop doing|never\s+\w+|not like that)`)
	preferencePatterns = regexp.MustCompile(`(?i)(always use|I prefer|from now on|remember to|use \w+ instead)`)
	decisionPatterns   = regexp.MustCompile(`(?i)(let's go with|we decided|decision:|the plan is|agreed to)`)
	learningPatterns   = regexp.MustCompile(`(?i)(discovered|learned|turns out|important:|TIL|found that)`)
)

func extractSignalsV1(transcript string, sessionID string) []Signal {
	var signals []Signal
	now := time.Now()

	lines := strings.Split(transcript, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "user: ") {
			content := strings.TrimPrefix(line, "user: ")

			if correctionPatterns.MatchString(content) {
				signals = append(signals, Signal{
					Type: SignalCorrection, Content: content,
					Source: sessionID, Timestamp: now, Confidence: 0.7,
				})
			}
			if preferencePatterns.MatchString(content) {
				signals = append(signals, Signal{
					Type: SignalPreference, Content: content,
					Source: sessionID, Timestamp: now, Confidence: 0.8,
				})
			}
			if decisionPatterns.MatchString(content) {
				signals = append(signals, Signal{
					Type: SignalDecision, Content: content,
					Source: sessionID, Timestamp: now, Confidence: 0.8,
				})
			}
			if learningPatterns.MatchString(content) {
				signals = append(signals, Signal{
					Type: SignalLearning, Content: content,
					Source: sessionID, Timestamp: now, Confidence: 0.6,
				})
			}
		}
	}

	return signals
}

func deduplicateSignals(signals []Signal) []Signal {
	seen := make(map[string]bool)
	var unique []Signal

	for _, s := range signals {
		key := string(s.Type) + "|" + normalizeForDedup(s.Content)
		if !seen[key] {
			seen[key] = true
			unique = append(unique, s)
		}
	}

	return unique
}

// normalizeForDedup normalizes text for deduplication: lowercases and collapses whitespace.
func normalizeForDedup(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(s)), " ")
}

func signalToSection(st SignalType) string {
	switch st {
	case SignalCorrection:
		return "Corrections"
	case SignalPreference:
		return "Preferences"
	case SignalDecision:
		return "Decisions"
	case SignalLearning:
		return "Learnings"
	case SignalFact:
		return "Facts"
	default:
		return "Notes"
	}
}

func formatSignalEntry(s Signal) string {
	date := s.Timestamp.Format("2006-01-02")
	return fmt.Sprintf("- %s (%s)", s.Content, date)
}

func findSimilarEntry(entries []string, newEntry string) int {
	// Simple keyword overlap check
	newWords := extractKeywords(newEntry)

	for i, entry := range entries {
		if !strings.HasPrefix(entry, "- ") {
			continue
		}
		existingWords := extractKeywords(entry)
		overlap := keywordOverlap(newWords, existingWords)
		if overlap > 0.5 { // >50% keyword overlap
			return i
		}
	}

	return -1
}

func extractKeywords(text string) map[string]bool {
	words := make(map[string]bool)
	for _, w := range strings.Fields(strings.ToLower(text)) {
		w = strings.Trim(w, ".,!?:;\"'()-*`")
		if len(w) > 3 { // skip short words
			words[w] = true
		}
	}
	return words
}

func keywordOverlap(a, b map[string]bool) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}

	matches := 0
	for w := range a {
		if b[w] {
			matches++
		}
	}

	smaller := len(a)
	if len(b) < smaller {
		smaller = len(b)
	}

	return float64(matches) / float64(smaller)
}

func sectionToTopicFile(heading string) string {
	name := strings.ToLower(heading)
	name = strings.ReplaceAll(name, " ", "-")
	name = regexp.MustCompile(`[^a-z0-9-]`).ReplaceAllString(name, "")
	return name + ".md"
}

func renderSectionContent(sec *MemorySection) string {
	var b strings.Builder
	b.WriteString("# " + sec.Heading + "\n\n")
	for _, line := range sec.Content {
		b.WriteString(line + "\n")
	}
	return b.String()
}

func generateDiff(original, updated string) string {
	if original == updated {
		return "(no changes)"
	}

	origLines := strings.Split(original, "\n")
	newLines := strings.Split(updated, "\n")

	var diff strings.Builder
	diff.WriteString("--- MEMORY.md (before)\n+++ MEMORY.md (after)\n")
	fmt.Fprintf(&diff, "@@ -%d lines +%d lines @@\n", len(origLines), len(newLines))

	// Simple line-by-line diff (not unified, but informative)
	maxLines := len(origLines)
	if len(newLines) > maxLines {
		maxLines = len(newLines)
	}

	for i := 0; i < maxLines; i++ {
		origLine := ""
		newLine := ""
		if i < len(origLines) {
			origLine = origLines[i]
		}
		if i < len(newLines) {
			newLine = newLines[i]
		}

		if origLine != newLine {
			if origLine != "" {
				diff.WriteString("- " + origLine + "\n")
			}
			if newLine != "" {
				diff.WriteString("+ " + newLine + "\n")
			}
		}
	}

	return diff.String()
}
