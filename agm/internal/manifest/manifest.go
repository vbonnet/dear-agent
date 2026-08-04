package manifest

import "time"

// Manifest represents an AGM session manifest (v2 schema)
type Manifest struct {
	SchemaVersion           string            `yaml:"schema_version"`
	SessionID               string            `yaml:"session_id"`
	Name                    string            `yaml:"name"`
	ParentSessionID         *string           `yaml:"parent_session_id,omitempty"` // Parent session ID for execution sessions
	State                   string            `yaml:"state,omitempty"`             // Session readiness state (DONE|WORKING|USER_PROMPT|COMPACTING|OFFLINE)
	StateUpdatedAt          time.Time         `yaml:"state_updated_at,omitempty"`  // When state was last updated
	StateSource             string            `yaml:"state_source,omitempty"`      // How state was detected (hook|tmux|manual)
	CreatedAt               time.Time         `yaml:"created_at"`
	UpdatedAt               time.Time         `yaml:"updated_at"`
	Lifecycle               string            `yaml:"lifecycle"`           // "" (active/stopped) or "archived"
	Outcome                 SessionOutcome    `yaml:"outcome,omitempty"`   // How the session ended, stamped at archive time (completed|crashed|killed|gc-stale)
	Workspace               string            `yaml:"workspace,omitempty"` // Workspace name (e.g., "oss", "acme")
	Context                 Context           `yaml:"context"`
	Claude                  Claude            `yaml:"claude"`
	Codex                   *Codex            `yaml:"codex,omitempty" json:"codex,omitempty"`   // Codex CLI saved-session metadata
	OpenAI                  *OpenAI           `yaml:"openai,omitempty" json:"openai,omitempty"` // Pure OpenAI API session runtime locator and legacy fallback configuration
	Agy                     *Agy              `yaml:"agy,omitempty" json:"agy,omitempty"`       // AGY saved-conversation metadata
	Pi                      *Pi               `yaml:"pi,omitempty" json:"pi,omitempty"`         // Pi native session metadata
	Tmux                    Tmux              `yaml:"tmux"`
	OpenCode                *OpenCode         `yaml:"opencode,omitempty"`   // OpenCode session metadata
	Harness                 string            `yaml:"harness,omitempty"`    // Harness specifies the AI harness (claude-code, codex-cli, agy, opencode-cli, pi-cli; gemini-cli deprecated)
	Model                   string            `yaml:"model,omitempty"`      // Model specifies the AI model within the harness
	ModelTier               string            `yaml:"model_tier,omitempty"` // ModelTier is the cost tier assigned by the model router (cheap, mid, expensive)
	EngramMetadata          *EngramMetadata   `yaml:"engram_metadata,omitempty"`
	ContextUsage            *ContextUsage     `yaml:"context_usage,omitempty"`              // Context usage tracking for status line
	PermissionPolicy        *PermissionPolicy `yaml:"permission_policy,omitempty"`          // Resolved AGM role/profile permission policy
	PermissionMode          string            `yaml:"permission_mode,omitempty"`            // AGM permission mode (default, plan, auto)
	PermissionModeUpdatedAt *time.Time        `yaml:"permission_mode_updated_at,omitempty"` // When mode was last changed
	PermissionModeSource    string            `yaml:"permission_mode_source,omitempty"`     // How mode was detected (hook, manual, resume)
	IsTest                  bool              `yaml:"is_test,omitempty"`                    // Whether this is a test session (created with --test)
	Sandbox                 *SandboxConfig    `yaml:"sandbox,omitempty"`                    // Sandbox isolation metadata
	WorkingDirectory        string            `yaml:"working_directory,omitempty"`          // Working directory when session was associated
	LastKnownCost           float64           `yaml:"last_known_cost,omitempty"`            // Cached cost from statusline file
	LastKnownModel          string            `yaml:"last_known_model,omitempty"`           // Cached model display name from statusline file
	LastKnownModelAt        time.Time         `yaml:"last_known_model_at,omitempty"`        // When model was last cached
	Disposable              bool              `yaml:"disposable,omitempty"`                 // Whether this is a disposable session with TTL-based auto-archive
	DisposableTTL           string            `yaml:"disposable_ttl,omitempty"`             // TTL for disposable sessions (e.g., "1h", "4h", "30m")
	Monitors                []string          `yaml:"monitors,omitempty"`                   // Sessions that monitor this session's loop heartbeat
	WorkflowPhase           string            `yaml:"workflow_phase,omitempty"`             // Session workflow phase: research, delegate, wait, verify, exit
	WorkflowPhaseUpdatedAt  *time.Time        `yaml:"workflow_phase_updated_at,omitempty"`  // When workflow phase was last changed
	CostTracking            *CostTracking     `yaml:"cost_tracking,omitempty"`              // Token usage and cost tracking
	Resources               *ResourceManifest `yaml:"resources,omitempty"`                  // Git worktrees and branches created by this session
}

// PermissionPolicy records the resolved role/profile permission policy used
// when AGM created a session. Claude Code receives the allowlist natively;
// other active harnesses use this manifest field as the shared control-plane
// record plus their harness-specific startup/runtime permission surface.
type PermissionPolicy struct {
	Profile       string                   `yaml:"profile,omitempty"`
	ProfileSource string                   `yaml:"profile_source,omitempty"` // flag or role
	Sources       []string                 `yaml:"sources,omitempty"`        // defaults, explicit, profile, parent
	InheritParent bool                     `yaml:"inherit_parent,omitempty"`
	Explicit      []string                 `yaml:"explicit,omitempty"`
	Allow         []string                 `yaml:"allow,omitempty"`
	Targets       []PermissionPolicyTarget `yaml:"targets,omitempty"`
}

// PermissionPolicyTarget describes how a harness carries the resolved policy.
type PermissionPolicyTarget struct {
	Harness           string `yaml:"harness"`
	PolicySurface     string `yaml:"policy_surface"`
	StartupSurface    string `yaml:"startup_surface"`
	RuntimeSurface    string `yaml:"runtime_surface"`
	NativeEnforcement string `yaml:"native_enforcement"`
}

// IsExpired returns true if the session is disposable and its TTL has elapsed.
func (m *Manifest) IsExpired() bool {
	if !m.Disposable || m.DisposableTTL == "" {
		return false
	}
	ttl, err := time.ParseDuration(m.DisposableTTL)
	if err != nil {
		return false
	}
	return time.Since(m.CreatedAt) > ttl
}

// State constants for session display state
const (
	StateReady            = "READY"
	StateDone             = "DONE"
	StateWorking          = "WORKING"
	StateUserPrompt       = "USER_PROMPT"
	StatePermissionPrompt = "PERMISSION_PROMPT"
	StateCompacting       = "COMPACTING"
	StateOffline          = "OFFLINE"
	StateWaitingAgent     = "WAITING_AGENT"
	StateLooping          = "LOOPING"
	StateBackgroundTasks  = "BACKGROUND_TASKS"
)

// Alive represents whether a session exists and is running.
// This is orthogonal to display state and CanReceive (defined in state package).
type Alive string

// Session liveness values.
const (
	AliveYes      Alive = "YES"
	AliveStopped  Alive = "STOPPED"
	AliveArchived Alive = "ARCHIVED"
	AliveNotFound Alive = "NOT_FOUND"
)

// EngramMetadata holds Engram integration metadata
type EngramMetadata struct {
	Enabled   bool      `yaml:"enabled"`
	Query     string    `yaml:"query"`
	EngramIDs []string  `yaml:"engram_ids"`
	LoadedAt  time.Time `yaml:"loaded_at"`
	Count     int       `yaml:"count"`
}

// ContextUsage tracks context window usage for status line display
type ContextUsage struct {
	TotalTokens    int       `yaml:"total_tokens"`             // Total available tokens in context window
	UsedTokens     int       `yaml:"used_tokens"`              // Currently used tokens
	PercentageUsed float64   `yaml:"percentage_used"`          // Percentage of context used (0-100)
	LastUpdated    time.Time `yaml:"last_updated"`             // When usage was last updated
	Source         string    `yaml:"source"`                   // Source of update: "claude-cli", "manual", "hook"
	ModelID        string    `yaml:"model_id,omitempty"`       // Model ID from conversation log
	EstimatedCost  float64   `yaml:"estimated_cost,omitempty"` // Fallback cost estimate from token counts; prefer statusline exact cost for interactive sessions
}

// Context holds session context information
type Context struct {
	Project string   `yaml:"project"`
	Purpose string   `yaml:"purpose,omitempty"`
	Tags    []string `yaml:"tags,omitempty"`
	Notes   string   `yaml:"notes,omitempty"`
}

// Claude represents Claude session metadata
type Claude struct {
	UUID         string `yaml:"uuid,omitempty"`          // Claude session UUID (required for resume)
	PreviousUUID string `yaml:"previous_uuid,omitempty"` // Previous UUID preserved when overwritten (enables recovery)
}

// Codex represents Codex CLI saved-session metadata.
type Codex struct {
	SessionID      string `yaml:"session_id,omitempty"`      // Codex saved-session UUID (required for codex resume)
	TranscriptPath string `yaml:"transcript_path,omitempty"` // Resolved rollout JSONL path at import time
}

// OpenAI records the non-secret information needed to locate and reconstruct a
// pure OpenAI API session. New sessions persist the authoritative client
// settings in their own metadata; these fields remain a backward-compatible
// fallback for sessions created before that metadata existed. API keys are
// never stored in manifests.
type OpenAI struct {
	SessionsDir     string  `yaml:"sessions_dir,omitempty" json:"sessions_dir,omitempty"`
	BaseURL         string  `yaml:"base_url,omitempty" json:"base_url,omitempty"`
	IsAzure         bool    `yaml:"is_azure,omitempty" json:"is_azure,omitempty"`
	AzureAPIVersion string  `yaml:"azure_api_version,omitempty" json:"azure_api_version,omitempty"`
	Temperature     float32 `yaml:"temperature,omitempty" json:"temperature,omitempty"`
	MaxTokens       int     `yaml:"max_tokens,omitempty" json:"max_tokens,omitempty"`
}

// Agy represents AGY saved-conversation metadata.
type Agy struct {
	ConversationID string `yaml:"conversation_id,omitempty"`      // AGY conversation UUID (required for agy resume)
	WorkspacePath  string `yaml:"workspace_path,omitempty"`       // Original AGY workspace path for discovery/audit
	ConversationDB string `yaml:"conversation_db_path,omitempty"` // Resolved AGY sqlite conversation DB path
	TranscriptPath string `yaml:"transcript_path,omitempty"`      // Resolved AGY transcript JSONL path
}

// Pi represents Pi native session metadata.
type Pi struct {
	SessionID         string `yaml:"session_id,omitempty" json:"session_id,omitempty"`
	SessionDir        string `yaml:"session_dir,omitempty" json:"session_dir,omitempty"`
	TranscriptPath    string `yaml:"transcript_path,omitempty" json:"transcript_path,omitempty"`
	CodingAgentDir    string `yaml:"coding_agent_dir,omitempty" json:"coding_agent_dir,omitempty"`
	CodingAgentDirSet bool   `yaml:"coding_agent_dir_set,omitempty" json:"coding_agent_dir_set,omitempty"`
}

// Tmux represents tmux session metadata
type Tmux struct {
	SessionName     string `yaml:"session_name"`
	SessionRevision string `yaml:"-" json:"-"` // Internal optimistic-write token; never part of the manifest surface.
}

// OpenCode represents OpenCode session metadata
type OpenCode struct {
	ServerPort int       `yaml:"server_port,omitempty"` // OpenCode server port (default: 4096)
	ServerHost string    `yaml:"server_host,omitempty"` // OpenCode server host (default: localhost)
	AttachTime time.Time `yaml:"attach_time,omitempty"` // When session attached to OpenCode
}

// SandboxConfig represents sandbox isolation metadata for a session
type SandboxConfig struct {
	Enabled               bool      `yaml:"enabled" json:"enabled"`           // Whether sandbox is enabled for this session
	ID                    string    `yaml:"id,omitempty" json:"id,omitempty"` // Sandbox ID (usually matches SessionID)
	Provider              string    `yaml:"provider,omitempty" json:"provider,omitempty"`
	MergedPath            string    `yaml:"merged_path,omitempty" json:"merged_path,omitempty"` // Root and cleanup boundary
	WorkingDir            string    `yaml:"working_dir,omitempty" json:"working_dir,omitempty"` // Provider-mapped harness directory
	CreatedAt             time.Time `yaml:"created_at,omitempty" json:"created_at,omitzero"`
	ExtraAddDirs          []string  `yaml:"extra_add_dirs,omitempty" json:"extra_add_dirs,omitempty"`
	BypassCodexHookTrust  bool      `yaml:"bypass_codex_hook_trust,omitempty" json:"bypass_codex_hook_trust,omitempty"`
	CodexHookSourceRepo   string    `yaml:"codex_hook_source_repo,omitempty" json:"codex_hook_source_repo,omitempty"`
	CodexHookSourceCommit string    `yaml:"codex_hook_source_commit,omitempty" json:"codex_hook_source_commit,omitempty"`
	CodexHookDigest       string    `yaml:"codex_hook_digest,omitempty" json:"codex_hook_digest,omitempty"`
	CodexHookRoot         string    `yaml:"codex_hook_root,omitempty" json:"codex_hook_root,omitempty"`
	// BypassCodexHookTrustReason is the justification bound into each private
	// launch handoff. It is persisted so resume can authorize again at its
	// executable boundary rather than inherit the original decision.
	BypassCodexHookTrustReason string `yaml:"bypass_codex_hook_trust_reason,omitempty" json:"bypass_codex_hook_trust_reason,omitempty"`
}

// ResourceManifest records git worktrees and branches created during a session.
// Agents that create worktrees should update this field so cleanup is deterministic.
type ResourceManifest struct {
	Worktrees []WorktreeResource `yaml:"worktrees,omitempty"`
	Branches  []BranchResource   `yaml:"branches,omitempty"`
}

// WorktreeResource describes a git worktree created during a session.
type WorktreeResource struct {
	Path      string    `yaml:"path"`
	Branch    string    `yaml:"branch"`
	Repo      string    `yaml:"repo"`
	CreatedAt time.Time `yaml:"created_at"`
}

// BranchResource describes a git branch created during a session.
type BranchResource struct {
	Name      string    `yaml:"name"`
	Repo      string    `yaml:"repo"`
	CreatedAt time.Time `yaml:"created_at"`
}

// CostTracking holds token usage and cost data for a session
type CostTracking struct {
	TokensIn     int64     `yaml:"tokens_in"`            // Total input tokens consumed
	TokensOut    int64     `yaml:"tokens_out"`           // Total output tokens consumed
	APICallCount int       `yaml:"api_call_count"`       // Number of API calls made
	StartTime    time.Time `yaml:"start_time,omitempty"` // When the session started work
	EndTime      time.Time `yaml:"end_time,omitempty"`   // When the session finished work
}

// ManifestV1 represents the legacy v1 manifest schema (for migration)
type ManifestV1 struct {
	SchemaVersion string     `yaml:"schema_version"`
	SessionID     string     `yaml:"session_id"`
	Status        string     `yaml:"status"`
	CreatedAt     time.Time  `yaml:"created_at"`
	LastActivity  time.Time  `yaml:"last_activity"`
	Worktree      WorktreeV1 `yaml:"worktree"`
	Claude        ClaudeV1   `yaml:"claude"`
	Tmux          TmuxV1     `yaml:"tmux"`
}

// WorktreeV1 represents the working directory for a Claude session (v1)
type WorktreeV1 struct {
	Path string `yaml:"path"`
}

// ClaudeV1 represents Claude session metadata (v1)
type ClaudeV1 struct {
	SessionID       string    `yaml:"session_id"`
	SessionEnvPath  string    `yaml:"session_env_path"`
	FileHistoryPath string    `yaml:"file_history_path"`
	StartedAt       time.Time `yaml:"started_at"`
	LastActivity    time.Time `yaml:"last_activity"`
}

// TmuxV1 represents tmux session metadata (v1)
type TmuxV1 struct {
	SessionName string    `yaml:"session_name"`
	WindowName  string    `yaml:"window_name"`
	CreatedAt   time.Time `yaml:"created_at"`
}

// Status constants (v1 - deprecated)
const (
	StatusActive     = "active"
	StatusDiscovered = "discovered"
	StatusArchived   = "archived"
)
