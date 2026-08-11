package quota

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"time"
)

// Reader produces one Snapshot per call. Implementations must not surface
// credentials, cookies, or full account identifiers in the snapshot or in
// returned errors.
type Reader interface {
	Read(ctx context.Context) (*Snapshot, error)
}

// DefaultCodexBarCommand is the CLI the reader shells out to when no
// command is configured. Resolved through PATH.
const DefaultCodexBarCommand = "codexbar"

// DefaultReadTimeout bounds one CodexBar invocation. The dashboard call
// refreshes providers over the network on a cold cache, so this is sized
// for the slow path; the Meter keeps it off the routing path entirely.
const DefaultReadTimeout = 45 * time.Second

// DefaultFamilyAliases maps CodexBar's provider ids onto dear-agent
// provider families. Several source ids can feed one family: Gemini
// quota is reported under "gemini" when the Gemini CLI is signed in and
// under "antigravity" when the Antigravity CLI holds the Google
// subscription. First reading with usable windows wins; see mergeFamily.
//
// Operators extend this through CodexBarReader.FamilyAliases rather than
// editing the table, so a new CodexBar provider does not need a release.
func DefaultFamilyAliases() map[string]string {
	return map[string]string{
		"claude":      "anthropic",
		"codex":       "openai",
		"openai":      "openai",
		"azureopenai": "openai",
		"gemini":      "gemini",
		"antigravity": "gemini",
		"vertexai":    "gemini",
		"openrouter":  "openrouter",
		"ollama":      "ollama",
	}
}

// CommandRunner is the process-execution seam. Tests substitute a fake
// so the reader's parsing is exercised without a CodexBar install.
type CommandRunner interface {
	Output(ctx context.Context, name string, args ...string) ([]byte, error)
}

type execCommandRunner struct{}

func (execCommandRunner) Output(ctx context.Context, name string, args ...string) ([]byte, error) {
	// #nosec G204 -- name is an operator-configured command path, not
	// request-derived input, and the argument vector is fixed below.
	return exec.CommandContext(ctx, name, args...).Output()
}

// CodexBarReader reads CodexBar's dashboard snapshot.
//
// It always requests redacted identities, so account addresses never
// enter this process in full. It reads no provider credentials itself:
// CodexBar owns the auth, this reader owns only the parse.
type CodexBarReader struct {
	// Command is the codexbar executable. Empty uses DefaultCodexBarCommand.
	Command string

	// Runner executes the command. Empty uses os/exec.
	Runner CommandRunner

	// Timeout bounds one invocation. Zero uses DefaultReadTimeout.
	Timeout time.Duration

	// FamilyAliases overrides the source-id → family mapping. Empty uses
	// DefaultFamilyAliases. A source id absent from the map is reported
	// under its own id as the family, so unmapped providers stay visible
	// instead of vanishing.
	FamilyAliases map[string]string
}

// Read runs `codexbar dashboard --identity redacted` and parses the result.
func (r CodexBarReader) Read(ctx context.Context) (*Snapshot, error) {
	command := r.Command
	if command == "" {
		command = DefaultCodexBarCommand
	}
	timeout := r.Timeout
	if timeout <= 0 {
		timeout = DefaultReadTimeout
	}
	runner := r.Runner
	if runner == nil {
		runner = execCommandRunner{}
	}

	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	out, err := runner.Output(callCtx, command, "dashboard", "--identity", "redacted")
	if err != nil {
		return nil, fmt.Errorf("quota: run %s dashboard: %w", command, redactExecError(err))
	}
	return ParseCodexBarDashboard(out, r.aliases())
}

func (r CodexBarReader) aliases() map[string]string {
	if len(r.FamilyAliases) > 0 {
		return r.FamilyAliases
	}
	return DefaultFamilyAliases()
}

// redactExecError strips a failed command's stderr from the error. CodexBar
// error output can name accounts and cookie stores; the exit status is the
// part a caller can act on.
func redactExecError(err error) error {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return fmt.Errorf("exit status %d (stderr suppressed)", exitErr.ExitCode())
	}
	return err
}

// codexBarDashboard mirrors the fields of CodexBar's dashboard-v1 payload
// that this package consumes. Unknown fields are ignored so a CodexBar
// release that adds keys does not break the parse.
type codexBarDashboard struct {
	SchemaVersion     int    `json:"schemaVersion"`
	GeneratedAt       string `json:"generatedAt"`
	StaleAfterSeconds int    `json:"staleAfterSeconds"`
	Host              struct {
		CodexBarVersion string `json:"codexBarVersion"`
	} `json:"host"`
	Providers []codexBarProvider `json:"providers"`
}

// supportedDashboardSchema is the CodexBar dashboard schema this parser
// was written against. A payload declaring a different version is
// rejected rather than parsed on a guess: the Meter keeps its previous
// reading on a read error, and an unreadable meter routes exactly as an
// absent one, so refusing here degrades safely and stays visible.
const supportedDashboardSchema = 1

type codexBarProvider struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Enabled   bool   `json:"enabled"`
	Source    string `json:"source"`
	UpdatedAt string `json:"updatedAt"`
	Identity  *struct {
		AccountEmail string `json:"accountEmail"`
		Plan         string `json:"plan"`
	} `json:"identity"`
	Error *struct {
		Code    int    `json:"code"`
		Kind    string `json:"kind"`
		Message string `json:"message"`
	} `json:"error"`
	Windows []struct {
		Kind             string  `json:"kind"`
		Label            string  `json:"label"`
		RemainingPercent float64 `json:"remainingPercent"`
		UsedPercent      float64 `json:"usedPercent"`
		ResetAt          string  `json:"resetAt"`
	} `json:"windows"`
}

// ParseCodexBarDashboard converts CodexBar dashboard-v1 JSON into a
// Snapshot, folding CodexBar's provider ids onto dear-agent families via
// aliases. A nil or empty aliases map falls back to the defaults.
//
// Timestamps that fail to parse are dropped rather than failing the whole
// read: a snapshot with a usable percentage and no reset clock is still
// worth routing on.
func ParseCodexBarDashboard(data []byte, aliases map[string]string) (*Snapshot, error) {
	if len(aliases) == 0 {
		aliases = DefaultFamilyAliases()
	}
	var payload codexBarDashboard
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("quota: parse codexbar dashboard json: %w", err)
	}
	if payload.SchemaVersion != 0 && payload.SchemaVersion != supportedDashboardSchema {
		return nil, fmt.Errorf("quota: codexbar dashboard schema %d is not supported (this build reads %d)",
			payload.SchemaVersion, supportedDashboardSchema)
	}

	snapshot := &Snapshot{
		Source:        "codexbar",
		SourceVersion: payload.Host.CodexBarVersion,
		GeneratedAt:   parseTime(payload.GeneratedAt),
		StaleAfter:    time.Duration(payload.StaleAfterSeconds) * time.Second,
	}

	byFamily := make(map[string]ProviderQuota, len(payload.Providers))
	order := make([]string, 0, len(payload.Providers))
	for _, p := range payload.Providers {
		quota := convertProvider(p, aliases)
		existing, seen := byFamily[quota.Family]
		if !seen {
			order = append(order, quota.Family)
			byFamily[quota.Family] = quota
			continue
		}
		byFamily[quota.Family] = mergeFamily(existing, quota)
	}

	snapshot.Providers = make([]ProviderQuota, 0, len(order))
	for _, family := range order {
		snapshot.Providers = append(snapshot.Providers, byFamily[family])
	}
	sort.SliceStable(snapshot.Providers, func(i, j int) bool {
		return snapshot.Providers[i].Family < snapshot.Providers[j].Family
	})
	return snapshot, nil
}

// convertProvider maps one CodexBar provider entry onto a ProviderQuota,
// classifying why a reading is missing when it is.
func convertProvider(p codexBarProvider, aliases map[string]string) ProviderQuota {
	family, ok := aliases[strings.ToLower(p.ID)]
	if !ok {
		family = strings.ToLower(p.ID)
	}
	quota := ProviderQuota{
		Family:    family,
		SourceID:  p.ID,
		UpdatedAt: parseTime(p.UpdatedAt),
	}
	if p.Identity != nil {
		quota.Account = p.Identity.AccountEmail
		quota.Plan = p.Identity.Plan
	}
	for _, w := range p.Windows {
		quota.Windows = append(quota.Windows, Window{
			ID:               w.Kind,
			Label:            w.Label,
			RemainingPercent: w.RemainingPercent,
			UsedPercent:      w.UsedPercent,
			ResetAt:          parseTime(w.ResetAt),
		})
	}

	var message string
	if p.Error != nil {
		message = p.Error.Message
	}

	// Windows outrank the error field. CodexBar reports partial failures
	// (a cost-refresh timeout, say) alongside good rate-limit windows; a
	// provider that told us its remaining percentage is readable.
	if len(quota.Windows) > 0 {
		quota.Availability = AvailabilityOK
		quota.Note = message
		return quota
	}

	switch {
	case !p.Enabled:
		quota.Availability = AvailabilityDisabled
		quota.Note = "provider disabled in codexbar"
	case classifyAuthFailure(message):
		quota.Availability = AvailabilityAuthRequired
		quota.Note = message
	case message != "":
		quota.Availability = AvailabilityUnavailable
		quota.Note = message
	default:
		quota.Availability = AvailabilityUnavailable
		quota.Note = "provider reported no usage windows"
	}
	return quota
}

// authFailureMarkers are the phrases CodexBar uses when a provider needs
// a credential rather than when it is out of quota. CodexBar tags every
// provider failure with kind "provider", so the message is the only
// signal available; the match is deliberately broad because the cost of a
// false positive (report "auth required" instead of "unavailable") is far
// lower than treating a sign-in problem as exhaustion.
var authFailureMarkers = []string{
	"api key",
	"authenticate",
	"credential",
	"log in",
	"logged in",
	"login",
	"not authorized",
	"oauth",
	"session cookies",
	"session found",
	"session key",
	"sign in",
	"signed in",
	"token missing",
	"unauthorized",
}

func classifyAuthFailure(message string) bool {
	lower := strings.ToLower(message)
	for _, marker := range authFailureMarkers {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

// mergeFamily folds a second source id into a family that already has a
// reading. A readable entry always beats an unreadable one, so Gemini
// resolves to whichever of gemini/antigravity is actually signed in. When
// both are readable the more constrained one wins, because both budgets
// bind the same family.
func mergeFamily(existing, incoming ProviderQuota) ProviderQuota {
	switch {
	case existing.Availability.Known() && !incoming.Availability.Known():
		return existing
	case !existing.Availability.Known() && incoming.Availability.Known():
		return incoming
	case !existing.Availability.Known() && !incoming.Availability.Known():
		// Neither is usable: keep the more actionable classification.
		if existing.Availability == AvailabilityAuthRequired {
			return existing
		}
		return incoming
	}

	existingWorst, _ := existing.MostConstrained()
	incomingWorst, _ := incoming.MostConstrained()
	if incomingWorst.RemainingPercent < existingWorst.RemainingPercent {
		return incoming
	}
	return existing
}

// parseTime accepts RFC3339 and returns the zero time for anything else,
// including the empty string.
func parseTime(value string) time.Time {
	if value == "" {
		return time.Time{}
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}
	}
	return parsed
}
