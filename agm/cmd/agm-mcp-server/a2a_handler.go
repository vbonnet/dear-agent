package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/a2aproject/a2a-go/a2a"
	a2agen "github.com/vbonnet/dear-agent/agm/internal/a2a"
	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

// a2aHandler serves A2A Agent Cards over HTTP.
type a2aHandler struct {
	log *slog.Logger
}

func newA2AHandler(log *slog.Logger) *a2aHandler {
	return &a2aHandler{log: log}
}

// ServeHTTP routes requests to the appropriate handler.
func (h *a2aHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	switch {
	case r.URL.Path == "/.well-known/agent.json":
		h.handleAggregateCard(w, r)
	case strings.HasPrefix(r.URL.Path, "/.well-known/agents/") && strings.HasSuffix(r.URL.Path, ".json"):
		h.handleSessionCard(w, r)
	default:
		http.NotFound(w, r)
	}
}

// handleAggregateCard returns an aggregate Agent Card representing all active sessions.
func (h *a2aHandler) handleAggregateCard(w http.ResponseWriter, _ *http.Request) {
	manifests, err := h.listActiveManifests()
	if err != nil {
		h.log.Error("Failed to list sessions", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Build per-session cards and aggregate skills
	var allSkills []a2a.AgentSkill
	for _, m := range manifests {
		card := a2agen.GenerateCard(m)
		allSkills = append(allSkills, card.Skills...)
	}

	aggregate := a2a.AgentCard{
		Name:               "agm",
		Description:        fmt.Sprintf("AGM agent hub (%d active sessions)", len(manifests)),
		ProtocolVersion:    string(a2a.Version),
		Skills:             allSkills,
		DefaultInputModes:  []string{"text/plain"},
		DefaultOutputModes: []string{"text/plain"},
	}

	h.writeJSON(w, aggregate)
}

// handleSessionCard returns an Agent Card for a single session by name.
func (h *a2aHandler) handleSessionCard(w http.ResponseWriter, r *http.Request) {
	// Extract name from /.well-known/agents/{name}.json
	path := r.URL.Path
	name := strings.TrimPrefix(path, "/.well-known/agents/")
	name = strings.TrimSuffix(name, ".json")

	if name == "" {
		http.NotFound(w, r)
		return
	}

	m, err := h.getManifestByName(name)
	if err != nil {
		h.log.Error("Failed to get session", "name", name, "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	if m == nil {
		http.NotFound(w, r)
		return
	}

	card := a2agen.GenerateCard(m)
	h.writeJSON(w, card)
}

// listActiveManifests returns manifests for all active sessions.
func (h *a2aHandler) listActiveManifests() ([]*manifest.Manifest, error) {
	storage, cleanup, err := newA2AStorage()
	if err != nil {
		return nil, err
	}
	defer cleanup()

	return storage.ListSessions(&dolt.SessionFilter{
		ExcludeArchived: true,
		ExcludeTest:     true,
		Limit:           1000,
	})
}

// getManifestByName finds a session by name.
func (h *a2aHandler) getManifestByName(name string) (*manifest.Manifest, error) {
	storage, cleanup, err := newA2AStorage()
	if err != nil {
		return nil, err
	}
	defer cleanup()

	manifests, err := storage.ListSessions(&dolt.SessionFilter{
		ExcludeArchived: true,
		ExcludeTest:     true,
		Limit:           1000,
	})
	if err != nil {
		return nil, err
	}

	for _, m := range manifests {
		if m.Name == name {
			return m, nil
		}
	}
	return nil, nil
}

// newA2AStorage creates a Dolt storage adapter for A2A handlers.
func newA2AStorage() (dolt.Storage, func(), error) {
	cfg, err := dolt.DefaultConfig()
	if err != nil {
		return nil, func() {}, fmt.Errorf("dolt config: %w", err)
	}

	adapter, err := dolt.New(cfg)
	if err != nil {
		return nil, func() {}, fmt.Errorf("dolt connect: %w", err)
	}

	return adapter, func() { adapter.Close() }, nil
}

func (h *a2aHandler) writeJSON(w http.ResponseWriter, v any) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		h.log.Error("Failed to marshal JSON", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}
