package steps

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/cucumber/godog"
)

var retiredWayfinderPattern = regexp.MustCompile(`\b(?:W0|D1|D2|D3|D4|S4|S5|S6|S7|S8|S9|S10|S11|V1)(?:\b|[A-Z_])|\b(?:w0|d1|d2|d3|d4|s4|s5|s6|s7|s8|s9|s10|s11|v1)_|WayfinderV1|discovery\.(problem|solutions|approach|requirements)|design\.(tech-lead|security|qa)|roadmap\.(planning|breakdown|dependencies)`)
var retiredWayfinderDocW0Pattern = regexp.MustCompile(`\bW0(?:\.[0-9]+)?\b`)
var retiredWayfinderDocForwardPattern = regexp.MustCompile(`(?i)\b(?:current[_ ]phase|next[_ ]phase|phase)\b[^A-Za-z0-9]{0,12}(?:W0|D1|D2|D3|D4|S4|S5|S6|S7|S8|S9|S10|S11|V1)\b`)
var retiredWayfinderDocReversePattern = regexp.MustCompile(`(?i)\b(?:W0|D1|D2|D3|D4|S4|S5|S6|S7|S8|S9|S10|S11|V1)(?:/(?:W0|D1|D2|D3|D4|S4|S5|S6|S7|S8|S9|S10|S11|V1))*\b[^A-Za-z0-9]{0,12}(?:phase|retrospective)\b`)
var retiredWayfinderDocArtifactPattern = regexp.MustCompile(`(?i)\b(?:W0|D1|D2|D3|D4|S4|S5|S6|S7|S8|S9|S10|S11)-(?:charter|problem|research|design|spec|plan|setup|implementation|validation|retrospective)[A-Za-z0-9_-]*\.md\b`)
var retiredWayfinderStatusLabelPattern = regexp.MustCompile(`(?m)\b(?:Current|Next) Phase:`)
var wayfinderDocPhaseFieldPattern = regexp.MustCompile(`(?i)["']?(?:current[_ ]phase|next[_ ]phase|phase)["']?[ \t]*:[ \t]*["']?([A-Za-z][A-Za-z0-9_-]*)`)

var canonicalWayfinderPhases = map[string]bool{
	"CHARTER":  true,
	"PROBLEM":  true,
	"RESEARCH": true,
	"DESIGN":   true,
	"SPEC":     true,
	"PLAN":     true,
	"SETUP":    true,
	"BUILD":    true,
	"RETRO":    true,
}

const wayfinderV2CommandFeaturePath = "agm/test/bdd/features/wayfinder_v2_command_guardrails.feature"

type wayfinderV2CommandPackageStateKey struct{}
type wayfinderV2CommandStateKey struct{}

type wayfinderV2CommandState struct {
	repoRoot string
	help     string
}

// RegisterWayfinderV2CommandGuardrailSteps registers canonical command checks.
func RegisterWayfinderV2CommandGuardrailSteps(ctx *godog.ScenarioContext) {
	registerPackageSpecGuardrailSteps(ctx, packageSpecGuardrailConfig{
		stateKey:          wayfinderV2CommandPackageStateKey{},
		label:             "Wayfinder command package",
		featurePath:       wayfinderV2CommandFeaturePath,
		configuredPattern: `^Wayfinder command package "([^"]*)" is configured$`,
		validatePattern:   `^AGM validates Wayfinder command package coverage$`,
		colocatedPattern:  `^Wayfinder command package "([^"]*)" should have a co-located SPEC$`,
	})

	ctx.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		return context.WithValue(ctx, wayfinderV2CommandStateKey{}, &wayfinderV2CommandState{
			repoRoot: packageSpecBDDRepoRoot(),
		}), nil
	})

	ctx.Step(`^AGM inspects the Wayfinder root help contract$`, agmInspectsWayfinderRootHelp)
	ctx.Step(`^Wayfinder help should name all nine canonical phases$`, wayfinderHelpNamesCanonicalPhases)
	ctx.Step(`^Wayfinder help should expose the canonical session command$`, wayfinderHelpExposesV2Session)
	ctx.Step(`^Wayfinder help should not expose retired root executors$`, wayfinderHelpOmitsRetiredExecutors)
	ctx.Step(`^Wayfinder help should not expose retired compatibility commands$`, wayfinderHelpOmitsLegacyMigrationCommands)
	ctx.Step(`^AGM audits Wayfinder command source policy$`, agmAuditsWayfinderCommandPolicy)
	ctx.Step(`^retired root and feature executors should be absent$`, retiredWayfinderExecutorsAreAbsent)
	ctx.Step(`^all Wayfinder session commands should parse only schema 2.0 status$`, normalWayfinderCommandsParseOnlyV2)
	ctx.Step(`^Wayfinder active corpus should omit retired phase identifiers$`, nonMigrationRuntimeOmitsRetiredPhases)
	ctx.Step(`^retired external Wayfinder validators should be absent$`, retiredExternalWayfinderValidatorsAreAbsent)
	ctx.Step(`^active command guidance should use the canonical entrypoint$`, activeCommandGuidanceUsesCanonicalEntrypoint)
	ctx.Step(`^Wayfinder phase enumeration should expose the nine named phases$`, unversionedPhasesDefaultToV2)
	ctx.Step(`^Wayfinder plugin should expose one root skill$`, wayfinderPluginExposesOneRootSkill)
}

func retiredExternalWayfinderValidatorsAreAbsent(ctx context.Context) error {
	state, err := getWayfinderV2CommandState(ctx)
	if err != nil {
		return err
	}
	for _, path := range []string{
		"pkg/validator/wayfinderartifact.go",
		"pkg/validator/retrospectivevalidator.go",
	} {
		if _, statErr := os.Stat(filepath.Join(state.repoRoot, path)); statErr == nil {
			return fmt.Errorf("retired external Wayfinder validator still exists: %s", path)
		} else if !os.IsNotExist(statErr) {
			return statErr
		}
	}
	validatePath := filepath.Join(state.repoRoot, "engram/cmd/engram/cmd/validate.go")
	data, err := os.ReadFile(validatePath)
	if err != nil {
		return err
	}
	for _, retired := range []string{"ValidatorWayfinder", "ValidatorRetrospective", "wayfinder-artifact", "S11 retrospective"} {
		if strings.Contains(string(data), retired) {
			return fmt.Errorf("engram validate still exposes retired Wayfinder surface %q", retired)
		}
	}
	return nil
}

func activeCommandGuidanceUsesCanonicalEntrypoint(ctx context.Context) (resultErr error) {
	state, err := getWayfinderV2CommandState(ctx)
	if err != nil {
		return err
	}
	rootFS, err := os.OpenRoot(state.repoRoot)
	if err != nil {
		return fmt.Errorf("open repository root: %w", err)
	}
	defer preserveRootCloseError(rootFS, &resultErr)
	retiredBinary := regexp.MustCompile(`wayfinder-session(?:[ \t]|$)`)
	for _, rel := range []string{
		"AGENTS.md",
		"GOAL.md",
		"wayfinder/README.md",
		"wayfinder/internal/project/detect.go",
		"wayfinder/cmd/wayfinder-session/commands",
		"wayfinder/cmd/wayfinder-session/internal/retrospective/appender.go",
		"wayfinder/cmd/wayfinder-session/internal/validator",
	} {
		root := filepath.Join(state.repoRoot, rel)
		err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				if entry.Name() == "testdata" {
					return filepath.SkipDir
				}
				return nil
			}
			if strings.HasSuffix(path, "_test.go") {
				return nil
			}
			relPath, relErr := filepath.Rel(state.repoRoot, path)
			if relErr != nil {
				return relErr
			}
			data, readErr := rootFS.ReadFile(filepath.ToSlash(relPath))
			if readErr != nil {
				return readErr
			}
			if retiredBinary.Match(data) {
				return fmt.Errorf("active guidance still names retired wayfinder-session binary: %s", path)
			}
			return nil
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func agmInspectsWayfinderRootHelp(ctx context.Context) error {
	state, err := getWayfinderV2CommandState(ctx)
	if err != nil {
		return err
	}
	path := filepath.Join(state.repoRoot, "wayfinder/cmd/wayfinder/cmd/root.go")
	data, readErr := os.ReadFile(path)
	if readErr != nil {
		return fmt.Errorf("read Wayfinder root help contract: %w", readErr)
	}
	state.help = string(data)
	return nil
}

func wayfinderHelpNamesCanonicalPhases(ctx context.Context) error {
	state, err := getWayfinderV2CommandState(ctx)
	if err != nil {
		return err
	}
	for _, phase := range []string{"CHARTER", "PROBLEM", "RESEARCH", "DESIGN", "SPEC", "PLAN", "SETUP", "BUILD", "RETRO"} {
		if !strings.Contains(state.help, phase) {
			return fmt.Errorf("wayfinder help does not name canonical phase %s", phase)
		}
	}
	return nil
}

func wayfinderHelpExposesV2Session(ctx context.Context) error {
	state, err := getWayfinderV2CommandState(ctx)
	if err != nil {
		return err
	}
	if !strings.Contains(state.help, "session") || !strings.Contains(state.help, "canonical 9-phase") {
		return fmt.Errorf("wayfinder help does not expose the canonical session surface: %s", state.help)
	}
	return nil
}

func wayfinderHelpOmitsRetiredExecutors(ctx context.Context) error {
	state, err := getWayfinderV2CommandState(ctx)
	if err != nil {
		return err
	}
	for _, command := range []string{"start", "autopilot", "features", "abort"} {
		if strings.Contains(state.help, "  "+command+" ") {
			return fmt.Errorf("wayfinder help exposes retired direct command %q", command)
		}
	}
	return nil
}

func wayfinderHelpOmitsLegacyMigrationCommands(ctx context.Context) error {
	state, err := getWayfinderV2CommandState(ctx)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(filepath.Join(state.repoRoot, "wayfinder/cmd/wayfinder/cmd/session.go"))
	if err != nil {
		return err
	}
	for _, command := range []string{"MigrateCmd", "MigrateAllCmd"} {
		if strings.Contains(string(data), command) {
			return fmt.Errorf("wayfinder session command still registers %s", command)
		}
	}
	return nil
}

func agmAuditsWayfinderCommandPolicy(ctx context.Context) error {
	_, err := getWayfinderV2CommandState(ctx)
	return err
}

func retiredWayfinderExecutorsAreAbsent(ctx context.Context) error {
	state, err := getWayfinderV2CommandState(ctx)
	if err != nil {
		return err
	}
	for _, path := range []string{
		"wayfinder/cmd/wayfinder/cmd/start.go",
		"wayfinder/cmd/wayfinder/cmd/autopilot.go",
		"wayfinder/cmd/wayfinder/cmd/features.go",
		"wayfinder/cmd/wayfinder/cmd/abort.go",
	} {
		if _, statErr := os.Stat(filepath.Join(state.repoRoot, path)); statErr == nil {
			return fmt.Errorf("retired Wayfinder executor still exists: %s", path)
		}
	}
	featureRoot := filepath.Join(state.repoRoot, "wayfinder/cmd/wayfinder-features")
	if _, statErr := os.Stat(featureRoot); os.IsNotExist(statErr) {
		return nil
	}
	return filepath.WalkDir(featureRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() && strings.HasSuffix(path, ".go") {
			return fmt.Errorf("retired Wayfinder feature executor still exists: %s", path)
		}
		return nil
	})
}

func normalWayfinderCommandsParseOnlyV2(ctx context.Context) error {
	state, err := getWayfinderV2CommandState(ctx)
	if err != nil {
		return err
	}
	for _, path := range []string{
		"wayfinder/cmd/wayfinder-session/commands/start.go",
		"wayfinder/cmd/wayfinder-session/commands/start_phase.go",
		"wayfinder/cmd/wayfinder-session/commands/complete_phase.go",
		"wayfinder/cmd/wayfinder-session/commands/next_phase.go",
		"wayfinder/cmd/wayfinder-session/commands/end.go",
		"wayfinder/cmd/wayfinder-session/commands/status.go",
		"wayfinder/cmd/wayfinder-session/commands/set_lifecycle_state.go",
		"wayfinder/cmd/wayfinder-session/commands/rewind.go",
	} {
		data, readErr := os.ReadFile(filepath.Join(state.repoRoot, path))
		if readErr != nil {
			return readErr
		}
		text := string(data)
		for _, forbidden := range []string{"status.ReadFrom(", "status.LoadAnyVersion(", "status.DetectFromFilesystem(", "runEndV1"} {
			if strings.Contains(text, forbidden) {
				return fmt.Errorf("normal command %s retains legacy path %q", path, forbidden)
			}
		}
		if !strings.Contains(text, "ParseV2FromDir") && !strings.Contains(text, "ParseV2(") && !strings.Contains(text, "runEndV2") {
			return fmt.Errorf("normal command %s has no canonical V2 parser path", path)
		}
	}
	return nil
}

func nonMigrationRuntimeOmitsRetiredPhases(ctx context.Context) error {
	state, err := getWayfinderV2CommandState(ctx)
	if err != nil {
		return err
	}
	for _, root := range []struct {
		path       string
		extensions map[string]bool
	}{
		{path: "wayfinder", extensions: map[string]bool{".go": true, ".md": true, ".json": true, ".yaml": true, ".yml": true}},
		{path: "agm/cmd/agm-mcp-server", extensions: map[string]bool{".go": true}},
		{path: "agm/internal/a2a/wayfinder", extensions: map[string]bool{".go": true}},
		{path: "engram/cmd/engram-mcp", extensions: map[string]bool{".go": true}},
		{path: "engram/cmd/engram/cmd", extensions: map[string]bool{".go": true}},
		{path: "engram/mcp/src", extensions: map[string]bool{".ts": true}},
		{path: "engram/internal/analytics", extensions: map[string]bool{".go": true}},
		{path: "engram/internal/consolidation", extensions: map[string]bool{".go": true}},
		{path: "engram/internal/memory", extensions: map[string]bool{".go": true}},
		{path: "engram/internal/providers/simple", extensions: map[string]bool{".go": true}},
		{path: "engram/internal/reflection", extensions: map[string]bool{".go": true}},
		{path: "internal/safepr", extensions: map[string]bool{".go": true}},
		{path: "cmd/safe-pr", extensions: map[string]bool{".go": true}},
		{path: "pkg/phaseengram", extensions: map[string]bool{".go": true}},
		{path: "pkg/progress", extensions: map[string]bool{".go": true}},
		{path: "pkg/validation/scope", extensions: map[string]bool{".go": true}},
	} {
		if err := scanActiveWayfinderRoot(state.repoRoot, root.path, root.extensions, retiredWayfinderPattern); err != nil {
			return err
		}
	}
	if err := scanLivingWayfinderDocumentation(state.repoRoot); err != nil {
		return err
	}
	return validateCanonicalWayfinderConsumers(state.repoRoot)
}

func scanLivingWayfinderDocumentation(repoRoot string) (resultErr error) {
	rootFS, err := os.OpenRoot(repoRoot)
	if err != nil {
		return fmt.Errorf("open repository root: %w", err)
	}
	defer preserveRootCloseError(rootFS, &resultErr)
	return filepath.WalkDir(repoRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(repoRoot, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if entry.IsDir() {
			if rel != "." && skipLivingDocumentationDir(rel) {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".md" || strings.EqualFold(filepath.Base(path), "CHANGELOG.md") {
			return nil
		}
		data, err := rootFS.ReadFile(rel)
		if err != nil {
			return err
		}
		return validateLivingWayfinderDocument(rel, string(data))
	})
}

func validateLivingWayfinderDocument(relativePath, content string) error {
	if strings.Contains(content, "WAYFINDER-STATUS.md") {
		if label := retiredWayfinderStatusLabelPattern.FindString(content); label != "" {
			return fmt.Errorf("living documentation %s parses retired Wayfinder status label %s", relativePath, label)
		}
	}
	state := livingWayfinderDocState{}
	for lineNumber, line := range strings.Split(content, "\n") {
		if err := validateLivingWayfinderLine(relativePath, lineNumber+1, line, &state); err != nil {
			return err
		}
	}
	return nil
}

type livingWayfinderDocState struct {
	contextLines   int
	inFence        bool
	wayfinderFence bool
}

func validateLivingWayfinderLine(relativePath string, lineNumber int, line string, state *livingWayfinderDocState) error {
	trimmed := strings.TrimSpace(line)
	isFence := strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~")
	openingFence := isFence && !state.inFence
	if openingFence {
		state.inFence = true
		state.wayfinderFence = false
	}
	mentionsWayfinder := strings.Contains(strings.ToLower(line), "wayfinder")
	if state.inFence && mentionsWayfinder {
		state.wayfinderFence = true
	}
	hasWayfinderContext := state.contextLines > 0 || state.wayfinderFence || mentionsWayfinder
	if token := retiredWayfinderDocToken(line, hasWayfinderContext); token != "" {
		return fmt.Errorf("living documentation %s:%d contains retired Wayfinder token %s", relativePath, lineNumber, token)
	}
	if err := validateLivingWayfinderPhase(relativePath, lineNumber, line, hasWayfinderContext); err != nil {
		return err
	}
	if mentionsWayfinder {
		state.contextLines = 2
	} else if state.contextLines > 0 {
		state.contextLines--
	}
	if isFence && !openingFence {
		state.inFence = false
		state.wayfinderFence = false
	}
	return nil
}

func validateLivingWayfinderPhase(relativePath string, lineNumber int, line string, hasWayfinderContext bool) error {
	if !hasWayfinderContext {
		return nil
	}
	if phase := wayfinderDocPhaseValue(line); phase != "" && !canonicalWayfinderPhases[phase] {
		return fmt.Errorf("living documentation %s:%d contains noncanonical Wayfinder phase %s", relativePath, lineNumber, phase)
	}
	return nil
}

func wayfinderDocPhaseValue(line string) string {
	match := wayfinderDocPhaseFieldPattern.FindStringSubmatch(line)
	if len(match) != 2 {
		return ""
	}
	return strings.ToUpper(match[1])
}

func skipLivingDocumentationDir(relativePath string) bool {
	for part := range strings.SplitSeq(relativePath, "/") {
		switch strings.ToLower(part) {
		case ".git", ".worktrees", "worktrees", "node_modules", "vendor", "archive", "archives", "adr", "adrs":
			return true
		}
	}
	return relativePath == "docs/policies" || strings.HasPrefix(relativePath, "docs/policies/")
}

func retiredWayfinderDocToken(line string, hasWayfinderContext bool) string {
	for _, pattern := range []*regexp.Regexp{
		retiredWayfinderDocW0Pattern,
		retiredWayfinderDocForwardPattern,
		retiredWayfinderDocReversePattern,
		retiredWayfinderDocArtifactPattern,
	} {
		if token := pattern.FindString(line); token != "" {
			return token
		}
	}
	if hasWayfinderContext {
		return retiredWayfinderPattern.FindString(line)
	}
	return ""
}

func scanActiveWayfinderRoot(repoRoot, relativeRoot string, activeExtensions map[string]bool, retired *regexp.Regexp) (resultErr error) {
	root := filepath.Join(repoRoot, relativeRoot)
	rootFS, err := os.OpenRoot(root)
	if err != nil {
		return fmt.Errorf("open active Wayfinder corpus root %s: %w", relativeRoot, err)
	}
	defer preserveRootCloseError(rootFS, &resultErr)
	return filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !activeExtensions[filepath.Ext(path)] {
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		data, readErr := rootFS.ReadFile(filepath.ToSlash(rel))
		if readErr != nil {
			return readErr
		}
		if token := retired.FindString(string(data)); token != "" {
			return fmt.Errorf("active Wayfinder corpus %s contains retired phase token %s", filepath.Join(relativeRoot, rel), token)
		}
		return nil
	})
}

func validateCanonicalWayfinderConsumers(repoRoot string) error {
	retiredPackage := filepath.Join(repoRoot, "pkg/w0")
	if _, err := os.Stat(retiredPackage); err == nil {
		return fmt.Errorf("retired Wayfinder package still exists: pkg/w0")
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect retired Wayfinder package: %w", err)
	}

	checks := []struct {
		path      string
		required  []string
		forbidden []string
	}{
		{path: "wayfinder/coordinator/monitor.go", required: []string{"statusread.Parse(content)", "canonical.CurrentWaypoint", "canonical.Status", "canonical.Progress"}, forbidden: []string{"Current Phase:", `yaml:"current_waypoint"`, "FindStringSubmatch"}},
		{path: "engram/cmd/engram-mcp/readtools.go", required: []string{"statusread.Parse(data)", "summary.CurrentWaypoint", "summary.Status"}, forbidden: []string{"rePhase", "Current Phase:"}},
		{path: "engram/mcp/src/index.ts", required: []string{"parseWayfinderStatus(content)", "canonical.phase", "canonical.progress", "canonical.status"}, forbidden: []string{"Current Phase:", "phaseMatch", "progressMatch", "statusMatch"}},
		{path: "engram/mcp/src/wayfinder_status.ts", required: []string{"parseDocument", "schema_version", "current_waypoint", "waypoint_history"}, forbidden: []string{"Current Phase:", "Progress:", "Status:"}},
		{path: "agm/cmd/agm-mcp-server/wayfinder.go", required: []string{"statusread.Parse(data)", `fmString(fm, "current_waypoint")`, `fmString(fm, "project_name")`}, forbidden: []string{`fmString(fm, "current_phase"`, `fmString(fm, "project_name",`}},
		{path: "agm/internal/a2a/wayfinder/wayfinder.go", required: []string{"ProjectName", "CurrentWaypoint", `case "in-progress"`}, forbidden: []string{"SessionID", "CurrentPhase", `"in_progress"`, `"design"`, `"planning"`, `"delivery"`}},
		{path: "internal/telemetry/wayfinder_roi_logger.go", required: []string{`"phase_name": "DESIGN"`, `"wayfinder-" + phaseName`}, forbidden: []string{`"phase_name": "D3"`}},
		{path: "pkg/costtrack/sink.go", required: []string{`"wayfinder-BUILD"`}, forbidden: []string{`"wayfinder-D1"`}},
		{path: "internal/safepr/safepr.go", required: []string{`statusread.ParseFromDir(dir)`, "st.ProjectName", "st.Beads"}, forbidden: []string{`yaml:"schema_version"`, `yaml:"project_name"`, `yaml:"session_id"`, "st.SessionID"}},
		{path: "engram/internal/config/config.go", forbidden: []string{"WayfinderConfig", "W0Config", `yaml:"w0"`}},
		{path: "engram/internal/config/loader.go", forbidden: []string{"mergeWayfinder", "mergeW0", "hasW0"}},
		{path: "engram/cmd/engram/cmd/config_show.go", forbidden: []string{"Wayfinder", "w0."}},
		{path: "engram/cmd/engram/cmd/phase_engram.go", forbidden: []string{"numeric aliases", "W0", "D1-D4", "S4-S11"}},
	}
	for _, check := range checks {
		data, err := os.ReadFile(filepath.Join(repoRoot, check.path))
		if err != nil {
			return err
		}
		content := string(data)
		for _, required := range check.required {
			if !strings.Contains(content, required) {
				return fmt.Errorf("wayfinder consumer %s lacks canonical status syntax %q", check.path, required)
			}
		}
		for _, forbidden := range check.forbidden {
			if strings.Contains(content, forbidden) {
				return fmt.Errorf("wayfinder consumer %s retains retired status syntax %q", check.path, forbidden)
			}
		}
	}
	return nil
}

func wayfinderPluginExposesOneRootSkill(ctx context.Context) error {
	state, err := getWayfinderV2CommandState(ctx)
	if err != nil {
		return err
	}
	root := filepath.Join(state.repoRoot, "wayfinder")
	if _, err := os.Stat(filepath.Join(root, "SKILL.md")); err != nil {
		return fmt.Errorf("canonical root skill is missing: %w", err)
	}
	commandsPath := filepath.Join(root, "commands")
	if _, err := os.Stat(commandsPath); err == nil {
		return fmt.Errorf("retired Wayfinder command directory remains: %s", commandsPath)
	} else if !os.IsNotExist(err) {
		return err
	}
	skillLink := filepath.Join(root, "skills", "wayfinder", "SKILL.md")
	info, err := os.Lstat(skillLink)
	if err != nil {
		return fmt.Errorf("canonical claude skill link is missing: %w", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return fmt.Errorf("claude skill surface must be a symlink to canonical SKILL.md: %s", skillLink)
	}
	resolvedSkill, err := filepath.EvalSymlinks(skillLink)
	if err != nil {
		return fmt.Errorf("resolve canonical claude skill link: %w", err)
	}
	canonicalSkill, err := filepath.EvalSymlinks(filepath.Join(root, "SKILL.md"))
	if err != nil {
		return fmt.Errorf("resolve canonical root skill: %w", err)
	}
	if resolvedSkill != canonicalSkill {
		return fmt.Errorf("claude skill link resolves to %s, want %s", resolvedSkill, canonicalSkill)
	}
	manifestPath := filepath.Join(root, ".claude-plugin", "plugin.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return err
	}
	var manifest map[string]json.RawMessage
	if err := json.Unmarshal(data, &manifest); err != nil {
		return fmt.Errorf("parse Wayfinder plugin manifest: %w", err)
	}
	if _, exists := manifest["commands"]; exists {
		return fmt.Errorf("wayfinder manifest still declares retired commands")
	}
	if _, exists := manifest["skills"]; !exists {
		return fmt.Errorf("wayfinder manifest does not expose its canonical Claude skill link")
	}
	return nil
}

func preserveRootCloseError(rootFS *os.Root, resultErr *error) {
	if closeErr := rootFS.Close(); *resultErr == nil && closeErr != nil {
		*resultErr = fmt.Errorf("close Wayfinder source root: %w", closeErr)
	}
}

func unversionedPhasesDefaultToV2(ctx context.Context) error {
	state, err := getWayfinderV2CommandState(ctx)
	if err != nil {
		return err
	}
	typesPath := filepath.Join(state.repoRoot, "wayfinder/cmd/wayfinder-session/internal/status/types.go")
	data, err := os.ReadFile(typesPath)
	if err != nil {
		return err
	}
	if !strings.Contains(string(data), "func AllPhases() []string") {
		return fmt.Errorf("canonical AllPhases must not accept a version selector")
	}
	phaseData, err := os.ReadFile(filepath.Join(state.repoRoot, "wayfinder/cmd/wayfinder-session/internal/status/types_v2.go"))
	if err != nil {
		return err
	}
	for _, phase := range []string{"CHARTER", "PROBLEM", "RESEARCH", "DESIGN", "SPEC", "PLAN", "SETUP", "BUILD", "RETRO"} {
		if !strings.Contains(string(phaseData), `= "`+phase+`"`) {
			return fmt.Errorf("canonical status does not declare named phase %s", phase)
		}
	}
	return nil
}

func getWayfinderV2CommandState(ctx context.Context) (*wayfinderV2CommandState, error) {
	state, ok := ctx.Value(wayfinderV2CommandStateKey{}).(*wayfinderV2CommandState)
	if !ok || state == nil {
		return nil, fmt.Errorf("wayfinder V2 command state not initialized")
	}
	return state, nil
}
