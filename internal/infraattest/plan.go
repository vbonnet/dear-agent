package infraattest

import (
	"bytes"
	"encoding/json"
	"slices"
	"strconv"
	"time"
)

type rawPlan struct {
	FormatVersion      string                     `json:"format_version"`
	TerraformVersion   string                     `json:"terraform_version"`
	PriorState         json.RawMessage            `json:"prior_state"`
	Configuration      json.RawMessage            `json:"configuration"`
	PlannedValues      json.RawMessage            `json:"planned_values"`
	Variables          json.RawMessage            `json:"variables"`
	ResourceChanges    []rawResourceChange        `json:"resource_changes"`
	ResourceDrift      []rawResourceChange        `json:"resource_drift"`
	RelevantAttributes []json.RawMessage          `json:"relevant_attributes"`
	OutputChanges      map[string]json.RawMessage `json:"output_changes"`
	Checks             []rawCheck                 `json:"checks"`
	DeferredChanges    []json.RawMessage          `json:"deferred_changes"`
	Applyable          *bool                      `json:"applyable"`
	Complete           *bool                      `json:"complete"`
	Errored            bool                       `json:"errored"`
	Timestamp          string                     `json:"timestamp"`
}

type rawResourceChange struct {
	Address         string          `json:"address"`
	PreviousAddress string          `json:"previous_address"`
	ModuleAddress   string          `json:"module_address"`
	Mode            string          `json:"mode"`
	Type            string          `json:"type"`
	Name            string          `json:"name"`
	Index           json.RawMessage `json:"index"`
	ProviderName    string          `json:"provider_name"`
	Deposed         string          `json:"deposed"`
	Change          rawChange       `json:"change"`
	ActionReason    string          `json:"action_reason"`
}

type rawChange struct {
	Actions         []string          `json:"actions"`
	Before          json.RawMessage   `json:"before"`
	After           json.RawMessage   `json:"after"`
	AfterUnknown    json.RawMessage   `json:"after_unknown"`
	BeforeSensitive json.RawMessage   `json:"before_sensitive"`
	AfterSensitive  json.RawMessage   `json:"after_sensitive"`
	ReplacePaths    []json.RawMessage `json:"replace_paths"`
	Importing       json.RawMessage   `json:"importing"`
	GeneratedConfig string            `json:"generated_config"`
}

type rawCheck struct {
	Address   json.RawMessage    `json:"address"`
	Status    string             `json:"status"`
	Instances []rawCheckInstance `json:"instances"`
}

type rawCheckInstance struct {
	Address  json.RawMessage   `json:"address"`
	Status   string            `json:"status"`
	Problems []json.RawMessage `json:"problems"`
}

type planEvaluation struct {
	Kind            string
	Projection      []byte
	PlanGeneratedAt time.Time
}

type migrationManifest struct {
	Schema          string              `json:"schema"`
	Backend         migrationBackend    `json:"backend"`
	StateEncryption string              `json:"state_encryption"`
	PlanEncryption  string              `json:"plan_encryption"`
	MovedBlocks     []string            `json:"moved_blocks"`
	RemovedBlocks   []string            `json:"removed_blocks"`
	ImportBlocks    []string            `json:"import_blocks"`
	Providers       []migrationProvider `json:"providers"`
	Workspace       string              `json:"workspace"`
}

type migrationBackend struct {
	Type          string          `json:"type"`
	Configuration json.RawMessage `json:"configuration"`
}

type migrationProvider struct {
	Address string `json:"address"`
	Version string `json:"version"`
}

func evaluatePlan(raw []byte, desiredRaw []byte, binding RulesetBinding) (planEvaluation, error) {
	plan, generatedAt, err := parsePlan(raw)
	if err != nil {
		return planEvaluation{}, err
	}
	desired, err := canonicalJSON(desiredRaw)
	if err != nil {
		return planEvaluation{}, reject(CodeRulesetProjection)
	}
	update, hasUpdate, err := selectRoutineUpdate(plan.ResourceChanges)
	if err != nil {
		return planEvaluation{}, err
	}
	if !hasUpdate {
		if len(plan.ResourceDrift) != 0 || plan.Applyable != nil && *plan.Applyable {
			return planEvaluation{}, reject(CodePlanAmbiguous)
		}
		projection, err := canonicalStruct(struct {
			Kind string `json:"kind"`
		}{Kind: "no-op"})
		if err != nil {
			return planEvaluation{}, err
		}
		return planEvaluation{Kind: "no-op", Projection: projection, PlanGeneratedAt: generatedAt}, nil
	}

	if plan.Applyable != nil && !*plan.Applyable {
		return planEvaluation{}, reject(CodePlanAmbiguous)
	}
	if err := validateRulesetUpdate(update, desired, binding); err != nil {
		return planEvaluation{}, err
	}
	if err := validateRoutineDrift(plan.ResourceDrift, update, binding); err != nil {
		return planEvaluation{}, err
	}
	projection, err := canonicalStruct(struct {
		Address string          `json:"address"`
		After   json.RawMessage `json:"after"`
		Kind    string          `json:"kind"`
	}{Address: update.Address, After: desired, Kind: "existing-ruleset-in-place-restoration"})
	if err != nil {
		return planEvaluation{}, err
	}
	return planEvaluation{
		Kind:            "existing-ruleset-in-place-restoration",
		Projection:      projection,
		PlanGeneratedAt: generatedAt,
	}, nil
}

func parsePlan(raw []byte) (rawPlan, time.Time, error) {
	var plan rawPlan
	if _, err := decodeStrict(raw, &plan); err != nil {
		return rawPlan{}, time.Time{}, reject(CodeMalformedPlan)
	}
	generatedAt, err := validatePlanEnvelope(plan)
	if err != nil {
		return rawPlan{}, time.Time{}, err
	}
	return plan, generatedAt, nil
}

func validatePlanEnvelope(plan rawPlan) (time.Time, error) {
	if plan.FormatVersion != PlanFormatVersion || plan.TerraformVersion != OpenTofuVersion {
		return time.Time{}, reject(CodeUnsupportedPlanFormat)
	}
	if !hasRequiredPlanSections(plan) {
		return time.Time{}, reject(CodeMalformedPlan)
	}
	if plan.Errored {
		return time.Time{}, reject(CodePlanErrored)
	}
	if planIsIncomplete(plan) {
		return time.Time{}, reject(CodePlanAmbiguous)
	}
	if len(plan.OutputChanges) != 0 {
		return time.Time{}, reject(CodePlanOutputs)
	}
	if err := validateChecks(plan.Checks); err != nil {
		return time.Time{}, err
	}
	generatedAt, err := time.Parse(time.RFC3339, plan.Timestamp)
	if err != nil || generatedAt.Location() != time.UTC {
		return time.Time{}, reject(CodeMalformedPlan)
	}
	return generatedAt, nil
}

func hasRequiredPlanSections(plan rawPlan) bool {
	return presentJSON(plan.PriorState) && presentJSON(plan.Configuration) &&
		presentJSON(plan.PlannedValues) && presentJSON(plan.Variables) && len(plan.ResourceChanges) != 0
}

// planIsIncomplete fails closed on an absent marker: a plan JSON that
// omits "complete" carries no affirmative evidence that it covers every
// proposed change, so it must not be treated the same as an explicit
// "complete": true (codex review on #1257).
func planIsIncomplete(plan rawPlan) bool {
	return plan.Complete == nil || !*plan.Complete || len(plan.DeferredChanges) != 0
}

func presentJSON(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	return len(trimmed) != 0 && !bytes.Equal(trimmed, []byte("null"))
}

func selectRoutineUpdate(changes []rawResourceChange) (rawResourceChange, bool, error) {
	var update rawResourceChange
	hasUpdate := false
	for index := range changes {
		change := changes[index]
		action, err := classifyActions(&change)
		if err != nil {
			return rawResourceChange{}, false, err
		}
		switch action {
		case "no-op":
			if err := validateNoOp(change.Change); err != nil {
				return rawResourceChange{}, false, err
			}
		case "update":
			if hasUpdate {
				return rawResourceChange{}, false, reject(CodePlanAmbiguous)
			}
			update = change
			hasUpdate = true
		default:
			return rawResourceChange{}, false, reject(CodePlanAmbiguous)
		}
	}
	return update, hasUpdate, nil
}

func validateRoutineDrift(driftChanges []rawResourceChange, update rawResourceChange, binding RulesetBinding) error {
	if len(driftChanges) != 1 {
		return reject(CodePlanAmbiguous)
	}
	drift := driftChanges[0]
	action, err := classifyActions(&drift)
	if err != nil {
		return err
	}
	if action != "update" || drift.Address != update.Address || drift.Type != update.Type ||
		drift.ProviderName != update.ProviderName {
		return reject(CodePlanAmbiguous)
	}
	if err := validateRulesetIdentity(drift, binding); err != nil {
		return err
	}
	driftBefore, err := canonicalJSON(drift.Change.Before)
	if err != nil {
		return reject(CodeMalformedPlan)
	}
	driftAfter, err := canonicalJSON(drift.Change.After)
	if err != nil {
		return reject(CodeMalformedPlan)
	}
	updateBefore, err := canonicalJSON(update.Change.Before)
	if err != nil {
		return reject(CodeMalformedPlan)
	}
	updateAfter, err := canonicalJSON(update.Change.After)
	if err != nil {
		return reject(CodeMalformedPlan)
	}
	if !bytes.Equal(driftBefore, updateAfter) || !bytes.Equal(driftAfter, updateBefore) {
		return reject(CodePlanAmbiguous)
	}
	return nil
}

func classifyActions(change *rawResourceChange) (string, error) {
	if err := validateChangeMetadata(change); err != nil {
		return "", err
	}
	return classifyActionList(change.Change.Actions)
}

func validateChangeMetadata(change *rawResourceChange) error {
	if !hasRequiredChangeFields(change) {
		return reject(CodeMalformedPlan)
	}
	if err := validateChangeAddressing(change); err != nil {
		return err
	}
	if err := validateChangeMigration(change.Change); err != nil {
		return err
	}
	return validateChangeMarkers(change.Change)
}

func hasRequiredChangeFields(change *rawResourceChange) bool {
	return change.Address != "" && change.Mode != "" && change.Type != "" && change.Name != "" &&
		change.ProviderName != "" && len(change.Change.Actions) != 0 && len(change.Change.Before) != 0 &&
		len(change.Change.After) != 0 && len(change.Change.AfterUnknown) != 0 &&
		len(change.Change.BeforeSensitive) != 0 && len(change.Change.AfterSensitive) != 0
}

func validateChangeAddressing(change *rawResourceChange) error {
	if change.PreviousAddress != "" {
		return reject(CodePlanMove)
	}
	if change.Deposed != "" {
		return reject(CodePlanDeposed)
	}
	if change.ActionReason != "" {
		return reject(CodePlanAmbiguous)
	}
	return nil
}

func validateChangeMigration(change rawChange) error {
	if len(change.Importing) != 0 && !bytes.Equal(bytes.TrimSpace(change.Importing), []byte("null")) ||
		change.GeneratedConfig != "" {
		return reject(CodePlanImport)
	}
	if len(change.ReplacePaths) != 0 {
		return reject(CodePlanReplace)
	}
	return nil
}

func validateChangeMarkers(change rawChange) error {
	unknown, err := containsMarker(change.AfterUnknown)
	if err != nil {
		return reject(CodeMalformedPlan)
	}
	if unknown {
		return reject(CodePlanUnknown)
	}
	for _, raw := range []json.RawMessage{change.BeforeSensitive, change.AfterSensitive} {
		sensitive, err := containsMarker(raw)
		if err != nil {
			return reject(CodeMalformedPlan)
		}
		if sensitive {
			return reject(CodePlanSensitive)
		}
	}
	return nil
}

func classifyActionList(actions []string) (string, error) {
	switch {
	case equalStrings(actions, []string{"no-op"}):
		return "no-op", nil
	case equalStrings(actions, []string{"update"}):
		return "update", nil
	case equalStrings(actions, []string{"create"}):
		return "", reject(CodePlanCreate)
	case equalStrings(actions, []string{"delete"}) || equalStrings(actions, []string{"forget"}):
		return "", reject(CodePlanDelete)
	case equalStrings(actions, []string{"delete", "create"}) || equalStrings(actions, []string{"create", "delete"}):
		return "", reject(CodePlanReplace)
	case equalStrings(actions, []string{"read"}):
		return "", reject(CodePlanRead)
	default:
		return "", reject(CodePlanAmbiguous)
	}
}

func validateNoOp(change rawChange) error {
	before, err := canonicalJSON(change.Before)
	if err != nil {
		return reject(CodeMalformedPlan)
	}
	after, err := canonicalJSON(change.After)
	if err != nil || !bytes.Equal(before, after) {
		return reject(CodePlanAmbiguous)
	}
	return nil
}

func validateRulesetUpdate(change rawResourceChange, desired []byte, binding RulesetBinding) error {
	if change.Mode != "managed" || change.Type != "github_repository_ruleset" ||
		change.ProviderName != ProviderAddress {
		return reject(CodePlanResourceType)
	}
	if change.Address != binding.Address || binding.Address == "" || binding.ImmutableID == "" {
		return reject(CodeRulesetBinding)
	}
	if err := validateRulesetIdentity(change, binding); err != nil {
		return err
	}
	after, err := canonicalJSON(change.Change.After)
	if err != nil || !bytes.Equal(after, desired) {
		return reject(CodeRulesetProjection)
	}
	before, err := canonicalJSON(change.Change.Before)
	if err != nil || bytes.Equal(before, after) {
		return reject(CodePlanAmbiguous)
	}
	return nil
}

func validateRulesetIdentity(change rawResourceChange, binding RulesetBinding) error {
	before, err := parseRulesetIdentity(change.Change.Before)
	if err != nil {
		return reject(CodeRulesetBinding)
	}
	after, err := parseRulesetIdentity(change.Change.After)
	if err != nil {
		return reject(CodeRulesetBinding)
	}
	if binding.ImmutableID == "" || binding.Repository == "" ||
		before.id != binding.ImmutableID || after.id != binding.ImmutableID ||
		before.rulesetID != binding.ImmutableID || after.rulesetID != binding.ImmutableID ||
		before.repository != binding.Repository || after.repository != binding.Repository {
		return reject(CodeRulesetBinding)
	}
	return nil
}

type rulesetIdentity struct {
	id         string
	rulesetID  string
	repository string
}

func parseRulesetIdentity(raw json.RawMessage) (rulesetIdentity, error) {
	object, err := objectValue(raw)
	if err != nil {
		return rulesetIdentity{}, err
	}
	id, idOK := object["id"].(string)
	repository, repositoryOK := object["repository"].(string)
	rulesetID, rulesetIDOK := canonicalPositiveInteger(object["ruleset_id"])
	if !idOK || id == "" || !repositoryOK || repository == "" || !rulesetIDOK {
		return rulesetIdentity{}, reject(CodeRulesetBinding)
	}
	return rulesetIdentity{id: id, rulesetID: rulesetID, repository: repository}, nil
}

func canonicalPositiveInteger(value any) (string, bool) {
	number, ok := value.(json.Number)
	if !ok {
		return "", false
	}
	parsed, err := strconv.ParseUint(number.String(), 10, 64)
	if err != nil || parsed == 0 || strconv.FormatUint(parsed, 10) != number.String() {
		return "", false
	}
	return number.String(), true
}

func validateChecks(checks []rawCheck) error {
	for _, check := range checks {
		if check.Status != "pass" {
			return reject(CodePlanChecks)
		}
		for _, instance := range check.Instances {
			if instance.Status != "pass" || len(instance.Problems) != 0 {
				return reject(CodePlanChecks)
			}
		}
	}
	return nil
}

func containsMarker(raw json.RawMessage) (bool, error) {
	if len(raw) == 0 {
		return false, nil
	}
	canonical, err := canonicalJSON(raw)
	if err != nil {
		return false, err
	}
	var value any
	decoder := json.NewDecoder(bytes.NewReader(canonical))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return false, err
	}
	return markerValue(value), nil
}

func markerValue(value any) bool {
	switch typed := value.(type) {
	case nil:
		return false
	case bool:
		return typed
	case map[string]any:
		for _, child := range typed {
			if markerValue(child) {
				return true
			}
		}
		return false
	case []any:
		return slices.ContainsFunc(typed, markerValue)
	default:
		return true
	}
}

func objectValue(raw json.RawMessage) (map[string]any, error) {
	canonical, err := canonicalJSON(raw)
	if err != nil {
		return nil, err
	}
	var object map[string]any
	decoder := json.NewDecoder(bytes.NewReader(canonical))
	decoder.UseNumber()
	if err := decoder.Decode(&object); err != nil || object == nil {
		return nil, reject(CodeMalformedJSON)
	}
	return object, nil
}

func evaluateMigrationManifest(raw []byte) ([]byte, error) {
	var manifest migrationManifest
	canonical, err := decodeStrict(raw, &manifest)
	if err != nil {
		return nil, reject(CodeMigrationSurface)
	}
	if !validMigrationSettings(manifest) || !hasNoMigrationBlocks(manifest) {
		return nil, reject(CodeMigrationSurface)
	}
	backend, err := objectValue(manifest.Backend.Configuration)
	if err != nil || len(backend) == 0 {
		return nil, reject(CodeMigrationSurface)
	}
	return canonical, nil
}

func validMigrationSettings(manifest migrationManifest) bool {
	return manifest.Schema == MigrationSchema && manifest.Backend.Type == "s3" &&
		manifest.StateEncryption == "disabled" && manifest.PlanEncryption == "enforced" &&
		manifest.Workspace == "default" && len(manifest.Providers) == 1 &&
		manifest.Providers[0].Address == ProviderAddress && manifest.Providers[0].Version == ProviderVersion
}

func hasNoMigrationBlocks(manifest migrationManifest) bool {
	return manifest.MovedBlocks != nil && manifest.RemovedBlocks != nil && manifest.ImportBlocks != nil &&
		len(manifest.MovedBlocks) == 0 && len(manifest.RemovedBlocks) == 0 && len(manifest.ImportBlocks) == 0
}
