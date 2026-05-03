package simple

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/vbonnet/dear-agent/engram/internal/consolidation"
)

// StoreMemory stores a memory entry as a JSON file.
//
// File path: {storagePath}/{namespace}/{memory.ID}.json
// Creates parent directories if they don't exist.
// Overwrites existing memory with same ID.
func (p *SimpleFileProvider) StoreMemory(ctx context.Context, namespace []string, memory consolidation.Memory) error {
	start := time.Now()
	var err error
	success := false

	// Emit telemetry on function exit
	defer func() {
		if recorder := consolidation.GetTelemetryRecorder(ctx); recorder != nil {
			consolidation.RecordMemoryEvent(ctx, recorder, consolidation.EventMemoryStored,
				consolidation.MemoryEventData{
					Provider:   "simple",
					Namespace:  namespace,
					MemoryID:   memory.ID,
					MemoryType: memory.Type,
					Latency:    time.Since(start),
					Success:    success,
					ErrorMsg:   getErrorMsg(err),
				})
		}
	}()

	// 1. Validate namespace
	if err = validateNamespace(namespace); err != nil {
		return fmt.Errorf("store memory: %w", err)
	}

	// 2. Ensure schema version is set
	if memory.SchemaVersion == "" {
		memory.SchemaVersion = "1.0"
	}

	// 3. Construct file path
	filePath := p.getMemoryPath(namespace, memory.ID)

	// 4. Create parent directories
	if err = os.MkdirAll(filepath.Dir(filePath), 0o700); err != nil {
		return fmt.Errorf("store memory: create directory: %w", err)
	}

	// 5. Serialize to JSON (pretty-printed for git-friendliness)
	data, jsonErr := json.MarshalIndent(memory, "", "  ")
	if jsonErr != nil {
		err = jsonErr
		return fmt.Errorf("store memory: serialize: %w", err)
	}

	// 6. Write to file atomically
	if err = os.WriteFile(filePath, data, 0o600); err != nil {
		return fmt.Errorf("store memory: write file: %w", err)
	}

	success = true

	// Publish eventbus event
	if bus := consolidation.GetEventBus(ctx); bus != nil {
		consolidation.PublishMemoryEvent(ctx, bus, consolidation.TopicMemoryStored, map[string]interface{}{
			"provider":    "simple",
			"namespace":   namespace,
			"memory_id":   memory.ID,
			"memory_type": string(memory.Type),
		})
	}

	return nil
}

// RetrieveMemory retrieves memories matching the query.
//
// Scans the namespace directory for .json files and filters based on query parameters.
// Returns empty slice (not error) if namespace doesn't exist.
func (p *SimpleFileProvider) RetrieveMemory(ctx context.Context, namespace []string, query consolidation.Query) ([]consolidation.Memory, error) {
	start := time.Now()
	var err error
	success := false
	resultSize := 0

	// Emit telemetry on function exit
	defer func() {
		if recorder := consolidation.GetTelemetryRecorder(ctx); recorder != nil {
			consolidation.RecordMemoryEvent(ctx, recorder, consolidation.EventMemoryRetrieved,
				consolidation.MemoryEventData{
					Provider:   "simple",
					Namespace:  namespace,
					Latency:    time.Since(start),
					Success:    success,
					ErrorMsg:   getErrorMsg(err),
					ResultSize: resultSize,
				})
		}
	}()

	// 1. Validate namespace
	if err = validateNamespace(namespace); err != nil {
		return nil, fmt.Errorf("retrieve memory: %w", err)
	}

	// 2. Get namespace directory path
	dirPath := filepath.Join(append([]string{p.storagePath}, namespace...)...)

	// 3. Check directory exists
	if _, statErr := os.Stat(dirPath); os.IsNotExist(statErr) {
		success = true
		resultSize = 0
		return []consolidation.Memory{}, nil // Empty result, not error
	}

	// 4. Scan directory for .json files
	files, globErr := filepath.Glob(filepath.Join(dirPath, "*.json"))
	if globErr != nil {
		err = globErr
		return nil, fmt.Errorf("retrieve memory: scan directory: %w", err)
	}

	// 5. Read and deserialize each file
	memories := make([]consolidation.Memory, 0, len(files))
	for _, file := range files {
		data, readErr := os.ReadFile(file)
		if readErr != nil {
			continue // Skip unreadable files
		}

		var memory consolidation.Memory
		if unmarshalErr := json.Unmarshal(data, &memory); unmarshalErr != nil {
			continue // Skip invalid JSON
		}

		memories = append(memories, memory)
	}

	// 6. Filter by query parameters
	filtered := filterMemories(memories, query)

	// 7. Sort by timestamp descending (newest first)
	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].Timestamp.After(filtered[j].Timestamp)
	})

	// 8. Apply limit
	if query.Limit > 0 && len(filtered) > query.Limit {
		filtered = filtered[:query.Limit]
	}

	success = true
	resultSize = len(filtered)

	return filtered, nil
}

// UpdateMemory applies partial updates to an existing memory.
//
// Reads the existing memory, applies updates, and writes it back atomically.
// Returns ErrNotFound if memory doesn't exist.
func (p *SimpleFileProvider) UpdateMemory(ctx context.Context, namespace []string, memoryID string, updates consolidation.MemoryUpdate) error {
	start := time.Now()
	var err error
	var memoryType consolidation.MemoryType
	success := false

	// Emit telemetry on function exit
	defer func() {
		if recorder := consolidation.GetTelemetryRecorder(ctx); recorder != nil {
			consolidation.RecordMemoryEvent(ctx, recorder, consolidation.EventMemoryUpdated,
				consolidation.MemoryEventData{
					Provider:   "simple",
					Namespace:  namespace,
					MemoryID:   memoryID,
					MemoryType: memoryType,
					Latency:    time.Since(start),
					Success:    success,
					ErrorMsg:   getErrorMsg(err),
				})
		}
	}()

	// 1. Validate namespace
	if err = validateNamespace(namespace); err != nil {
		return fmt.Errorf("update memory: %w", err)
	}

	// 2. Construct file path
	filePath := p.getMemoryPath(namespace, memoryID)

	// 3. Read existing memory
	data, readErr := os.ReadFile(filePath)
	if os.IsNotExist(readErr) {
		err = consolidation.ErrNotFound
		return fmt.Errorf("update memory: %w", err)
	}
	if readErr != nil {
		err = readErr
		return fmt.Errorf("update memory: read file: %w", err)
	}

	var memory consolidation.Memory
	if unmarshalErr := json.Unmarshal(data, &memory); unmarshalErr != nil {
		err = unmarshalErr
		return fmt.Errorf("update memory: deserialize: %w", err)
	}

	memoryType = memory.Type // Capture for telemetry

	// 4. Apply updates
	if err = applyUpdates(&memory, updates); err != nil {
		return fmt.Errorf("update memory: %w", err)
	}

	// 5. Write back atomically
	data, marshalErr := json.MarshalIndent(memory, "", "  ")
	if marshalErr != nil {
		err = marshalErr
		return fmt.Errorf("update memory: serialize: %w", err)
	}

	if err = os.WriteFile(filePath, data, 0o600); err != nil {
		return fmt.Errorf("update memory: write file: %w", err)
	}

	success = true

	// Publish eventbus event
	if bus := consolidation.GetEventBus(ctx); bus != nil {
		consolidation.PublishMemoryEvent(ctx, bus, consolidation.TopicMemoryUpdated, map[string]interface{}{
			"provider":    "simple",
			"namespace":   namespace,
			"memory_id":   memoryID,
			"memory_type": string(memoryType),
		})
	}

	return nil
}

// DeleteMemory removes a memory entry.
//
// Returns ErrNotFound if memory doesn't exist.
func (p *SimpleFileProvider) DeleteMemory(ctx context.Context, namespace []string, memoryID string) error {
	start := time.Now()
	var err error
	success := false

	// Emit telemetry on function exit
	defer func() {
		if recorder := consolidation.GetTelemetryRecorder(ctx); recorder != nil {
			consolidation.RecordMemoryEvent(ctx, recorder, consolidation.EventMemoryDeleted,
				consolidation.MemoryEventData{
					Provider:  "simple",
					Namespace: namespace,
					MemoryID:  memoryID,
					Latency:   time.Since(start),
					Success:   success,
					ErrorMsg:  getErrorMsg(err),
				})
		}
	}()

	// 1. Validate namespace
	if err = validateNamespace(namespace); err != nil {
		return fmt.Errorf("delete memory: %w", err)
	}

	// 2. Construct file path
	filePath := p.getMemoryPath(namespace, memoryID)

	// 3. Check file exists
	if _, statErr := os.Stat(filePath); os.IsNotExist(statErr) {
		err = consolidation.ErrNotFound
		return fmt.Errorf("delete memory: %w", err)
	}

	// 4. Delete file
	if err = os.Remove(filePath); err != nil {
		return fmt.Errorf("delete memory: %w", err)
	}

	success = true

	// Publish eventbus event
	if bus := consolidation.GetEventBus(ctx); bus != nil {
		consolidation.PublishMemoryEvent(ctx, bus, consolidation.TopicMemoryDeleted, map[string]interface{}{
			"provider":  "simple",
			"namespace": namespace,
			"memory_id": memoryID,
		})
	}

	return nil
}

// getMemoryPath constructs the file path for a memory.
func (p *SimpleFileProvider) getMemoryPath(namespace []string, memoryID string) string {
	parts := append([]string{p.storagePath}, namespace...)
	parts = append(parts, memoryID+".json")
	return filepath.Join(parts...)
}

// validateNamespace checks that namespace is valid.
//
// Rejects empty namespaces, empty parts, and path traversal attempts.
func validateNamespace(namespace []string) error {
	if len(namespace) == 0 {
		return consolidation.ErrInvalidNamespace
	}
	for _, part := range namespace {
		if part == "" || part == "." || part == ".." {
			return consolidation.ErrInvalidNamespace
		}
	}
	return nil
}

// filterMemories applies query filters to memories.
func filterMemories(memories []consolidation.Memory, query consolidation.Query) []consolidation.Memory {
	result := make([]consolidation.Memory, 0, len(memories))

	for _, mem := range memories {
		// Filter by type
		if query.Type != "" && mem.Type != query.Type {
			continue
		}

		// Filter by importance
		if mem.Importance < query.MinImportance {
			continue
		}

		// Filter by time range
		if query.TimeRange != nil {
			if mem.Timestamp.Before(query.TimeRange.Start) ||
				!mem.Timestamp.Before(query.TimeRange.End) {
				continue
			}
		}

		// Filter by text (simple substring match)
		if query.Text != "" {
			contentStr := fmt.Sprintf("%v", mem.Content)
			if !strings.Contains(strings.ToLower(contentStr), strings.ToLower(query.Text)) {
				continue
			}
		}

		result = append(result, mem)
	}

	return result
}

// applyUpdates applies MemoryUpdate to a memory.
func applyUpdates(memory *consolidation.Memory, updates consolidation.MemoryUpdate) error {
	// SetContent: replace entire content
	if updates.SetContent != nil {
		memory.Content = *updates.SetContent
	}

	// AppendContent: append to string content
	if updates.AppendContent != nil {
		if currentStr, ok := memory.Content.(string); ok {
			memory.Content = currentStr + *updates.AppendContent
		} else {
			return fmt.Errorf("cannot append to non-string content (type: %T)", memory.Content)
		}
	}

	// SetMetadata: merge metadata
	if updates.SetMetadata != nil {
		if memory.Metadata == nil {
			memory.Metadata = make(map[string]interface{})
		}
		for k, v := range updates.SetMetadata {
			memory.Metadata[k] = v
		}
	}

	// SetImportance: update importance score
	if updates.SetImportance != nil {
		memory.Importance = *updates.SetImportance
	}

	// SetType: change memory type
	if updates.SetType != nil {
		memory.Type = *updates.SetType
	}

	// SetEmbedding: update embedding vector
	if len(updates.SetEmbedding) > 0 {
		memory.Embedding = updates.SetEmbedding
	}

	return nil
}

// getErrorMsg extracts error message for telemetry
func getErrorMsg(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
