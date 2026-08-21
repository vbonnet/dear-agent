package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"sort"
	"strings"
)

// rulesetView is the exact zero-bypass branch-protection subset supported by
// this command and the managed-repo OpenTofu module. Server metadata is ignored;
// unsupported policy fields fail parsing instead of disappearing from drift.
type rulesetView struct {
	Name                  string
	Target                string
	Enforcement           string
	BypassActors          []bypassActor
	RefNameInclude        []string
	RefNameExclude        []string
	Deletion              bool
	NonFastForward        bool
	RequiredLinearHistory bool
	PullRequest           pullRequestRule
	RequiredStatusChecks  requiredStatusChecksRule
}

type bypassActor struct {
	ActorID    int64  `json:"actor_id"`
	ActorType  string `json:"actor_type"`
	BypassMode string `json:"bypass_mode"`
}

type pullRequestRule struct {
	AllowedMergeMethods            []string
	RequiredApprovingReviewCount   int
	DismissStaleReviewsOnPush      bool
	RequireCodeOwnerReview         bool
	RequireLastPushApproval        bool
	RequiredReviewThreadResolution bool
	RequiredReviewers              []requiredReviewer
}

// requiredReviewer is the exact path-scoped reviewer shape supported by the
// managed subset. GitHub currently supports Team reviewers.
type requiredReviewer struct {
	FilePatterns     []string                 `json:"file_patterns"`
	MinimumApprovals int                      `json:"minimum_approvals"`
	Reviewer         requiredReviewerIdentity `json:"reviewer"`
}

type requiredReviewerIdentity struct {
	ID   int64  `json:"id"`
	Type string `json:"type"`
}

type reviewerJSON struct {
	ID   *int64  `json:"id"`
	Type *string `json:"type"`
}

type requiredReviewerJSON struct {
	FilePatterns     *[]string     `json:"file_patterns"`
	MinimumApprovals *int          `json:"minimum_approvals"`
	Reviewer         *reviewerJSON `json:"reviewer"`
}

type requiredStatusChecksRule struct {
	StrictRequiredStatusChecksPolicy bool
	DoNotEnforceOnCreate             bool
	RequiredChecks                   []requiredStatusCheck
}

// requiredStatusCheck is the exact identity supported for one required check.
// GitHub permits integration_id to be omitted, so nil is distinct from zero.
type requiredStatusCheck struct {
	Context       string `json:"context"`
	IntegrationID *int64 `json:"integration_id,omitempty"`
}

type rulesetDocument struct {
	Name         string            `json:"name"`
	Target       string            `json:"target"`
	Enforcement  string            `json:"enforcement"`
	BypassActors json.RawMessage   `json:"bypass_actors"`
	Conditions   json.RawMessage   `json:"conditions"`
	Rules        []json.RawMessage `json:"rules"`
}

// rulesetDrift returns human-readable differences between the canonical source
// and live ruleset. Normalization makes order irrelevant without dropping any
// supported policy field.
func rulesetDrift(localJSON, liveJSON []byte) []string {
	local, err1 := parseRuleset(localJSON)
	live, err2 := parseRuleset(liveJSON)
	if err1 != nil {
		return []string{fmt.Sprintf("ruleset drift: parse canonical policy: %v", err1)}
	}
	if err2 != nil {
		return []string{fmt.Sprintf("ruleset drift: parse live policy: %v", err2)}
	}
	var drift []string
	compareRulesetField(&drift, "name", local.Name, live.Name)
	compareRulesetField(&drift, "target", local.Target, live.Target)
	compareRulesetField(&drift, "enforcement", local.Enforcement, live.Enforcement)
	compareRulesetField(&drift, "bypass_actors", local.BypassActors, live.BypassActors)
	compareRulesetField(&drift, "conditions.ref_name.include", local.RefNameInclude, live.RefNameInclude)
	compareRulesetField(&drift, "conditions.ref_name.exclude", local.RefNameExclude, live.RefNameExclude)
	compareRulesetField(&drift, "rules.deletion", local.Deletion, live.Deletion)
	compareRulesetField(&drift, "rules.non_fast_forward", local.NonFastForward, live.NonFastForward)
	compareRulesetField(&drift, "rules.required_linear_history", local.RequiredLinearHistory, live.RequiredLinearHistory)
	compareRulesetField(&drift, "rules.pull_request", local.PullRequest, live.PullRequest)
	compareRulesetField(&drift, "rules.required_status_checks", local.RequiredStatusChecks, live.RequiredStatusChecks)
	return drift
}

func compareRulesetField(drift *[]string, field string, source, live any) {
	if !reflect.DeepEqual(source, live) {
		*drift = append(*drift, fmt.Sprintf("ruleset %s drift: source=%v live=%v", field, source, live))
	}
}

// parseRuleset rejects unknown, duplicate, or incomplete supported rules. The
// canonical document is therefore a typed declaration, not arbitrary JSON that
// OpenTofu or the audit might partially ignore.
func parseRuleset(raw []byte) (rulesetView, error) {
	doc, err := parseRulesetDocument(raw)
	if err != nil {
		return rulesetView{}, err
	}
	bypassActors, err := parseBypassActors(doc.BypassActors)
	if err != nil {
		return rulesetView{}, err
	}
	includes, excludes, err := parseRefNameConditions(doc.Conditions)
	if err != nil {
		return rulesetView{}, err
	}
	v := rulesetView{
		Name:           doc.Name,
		Target:         doc.Target,
		Enforcement:    doc.Enforcement,
		BypassActors:   bypassActors,
		RefNameInclude: includes,
		RefNameExclude: excludes,
	}
	seenRules, err := parseRules(doc.Rules, &v)
	if err != nil {
		return rulesetView{}, err
	}
	if err := validateRequiredRules(seenRules, v); err != nil {
		return rulesetView{}, err
	}
	return v, nil
}

func parseRulesetDocument(raw []byte) (rulesetDocument, error) {
	if err := rejectUnknownObjectKeys(raw, "ruleset", []string{
		"name", "target", "enforcement", "bypass_actors", "conditions", "rules",
		// GitHub response metadata: deliberately not part of policy comparison.
		"id", "node_id", "source", "source_type", "_links", "created_at", "updated_at", "current_user_can_bypass",
	}); err != nil {
		return rulesetDocument{}, err
	}
	var doc rulesetDocument
	if err := json.Unmarshal(raw, &doc); err != nil {
		return rulesetDocument{}, err
	}
	if strings.TrimSpace(doc.Name) == "" || strings.TrimSpace(doc.Target) == "" || strings.TrimSpace(doc.Enforcement) == "" {
		return rulesetDocument{}, fmt.Errorf("ruleset name, target, and enforcement must be non-empty")
	}
	return doc, nil
}

func parseBypassActors(raw json.RawMessage) ([]bypassActor, error) {
	var bypassActors []bypassActor
	if len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, fmt.Errorf("ruleset bypass_actors must be an explicit array")
	}
	if err := decodeStrictJSON(raw, &bypassActors); err != nil {
		return nil, fmt.Errorf("parse bypass_actors: %w", err)
	}
	for _, actor := range bypassActors {
		if actor.ActorID <= 0 || strings.TrimSpace(actor.ActorType) == "" || strings.TrimSpace(actor.BypassMode) == "" {
			return nil, fmt.Errorf("bypass actor is incomplete")
		}
	}
	sort.Slice(bypassActors, func(i, j int) bool {
		ai, aj := bypassActors[i], bypassActors[j]
		if ai.ActorID != aj.ActorID {
			return ai.ActorID < aj.ActorID
		}
		if ai.ActorType != aj.ActorType {
			return ai.ActorType < aj.ActorType
		}
		return ai.BypassMode < aj.BypassMode
	})
	return bypassActors, nil
}

func parseRules(rawRules []json.RawMessage, v *rulesetView) (map[string]bool, error) {
	seenRules := make(map[string]bool)
	for _, rawRule := range rawRules {
		var rule rulesetJSONRule
		if err := decodeStrictJSON(rawRule, &rule); err != nil {
			return nil, fmt.Errorf("parse ruleset rule: %w", err)
		}
		if seenRules[rule.Type] {
			return nil, fmt.Errorf("duplicate %s rule", rule.Type)
		}
		seenRules[rule.Type] = true
		if err := applyRule(v, rule); err != nil {
			return nil, err
		}
	}
	return seenRules, nil
}

func applyRule(v *rulesetView, rule rulesetJSONRule) error {
	switch rule.Type {
	case "deletion":
		if !emptyRuleParameters(rule.Parameters) {
			return fmt.Errorf("deletion rule has unsupported parameters")
		}
		v.Deletion = true
	case "non_fast_forward":
		if !emptyRuleParameters(rule.Parameters) {
			return fmt.Errorf("non_fast_forward rule has unsupported parameters")
		}
		v.NonFastForward = true
	case "required_linear_history":
		if !emptyRuleParameters(rule.Parameters) {
			return fmt.Errorf("required_linear_history rule has unsupported parameters")
		}
		v.RequiredLinearHistory = true
	case "pull_request":
		parsed, err := parsePullRequestRule(rule.Parameters)
		if err != nil {
			return err
		}
		v.PullRequest = parsed
	case "required_status_checks":
		parsed, err := parseRequiredStatusChecksRule(rule.Parameters)
		if err != nil {
			return err
		}
		v.RequiredStatusChecks = parsed
	default:
		return fmt.Errorf("unsupported ruleset rule %q", rule.Type)
	}
	return nil
}

func validateRequiredRules(seenRules map[string]bool, v rulesetView) error {
	if len(seenRules) != 5 || !v.Deletion || !v.NonFastForward || !v.RequiredLinearHistory || !seenRules["pull_request"] || !seenRules["required_status_checks"] {
		return fmt.Errorf("ruleset must contain exactly the supported branch-protection rules")
	}
	return nil
}

type rulesetJSONRule struct {
	Type       string          `json:"type"`
	Parameters json.RawMessage `json:"parameters"`
}

func parseRefNameConditions(raw json.RawMessage) ([]string, []string, error) {
	var conditions map[string]json.RawMessage
	if err := json.Unmarshal(raw, &conditions); err != nil {
		return nil, nil, fmt.Errorf("parse ruleset conditions: %w", err)
	}
	refName, ok := conditions["ref_name"]
	if !ok || len(conditions) != 1 {
		return nil, nil, fmt.Errorf("ruleset must contain exactly ref_name conditions")
	}
	var ref struct {
		Include *[]string `json:"include"`
		Exclude *[]string `json:"exclude"`
	}
	if err := decodeStrictJSON(refName, &ref); err != nil {
		return nil, nil, fmt.Errorf("parse ref_name conditions: %w", err)
	}
	if ref.Include == nil || ref.Exclude == nil || len(*ref.Include) == 0 {
		return nil, nil, fmt.Errorf("ref_name include and exclude must be explicit and include non-empty")
	}
	return normalizedStrings(*ref.Include), normalizedStrings(*ref.Exclude), nil
}

func parsePullRequestRule(raw json.RawMessage) (pullRequestRule, error) {
	var params struct {
		AllowedMergeMethods            *[]string               `json:"allowed_merge_methods"`
		RequiredApprovingReviewCount   *int                    `json:"required_approving_review_count"`
		DismissStaleReviewsOnPush      *bool                   `json:"dismiss_stale_reviews_on_push"`
		RequireCodeOwnerReview         *bool                   `json:"require_code_owner_review"`
		RequireLastPushApproval        *bool                   `json:"require_last_push_approval"`
		RequiredReviewThreadResolution *bool                   `json:"required_review_thread_resolution"`
		RequiredReviewers              *[]requiredReviewerJSON `json:"required_reviewers"`
	}
	if err := decodeStrictJSON(raw, &params); err != nil {
		return pullRequestRule{}, fmt.Errorf("parse pull_request rule: %w", err)
	}
	if params.AllowedMergeMethods == nil || params.RequiredApprovingReviewCount == nil || params.DismissStaleReviewsOnPush == nil || params.RequireCodeOwnerReview == nil || params.RequireLastPushApproval == nil || params.RequiredReviewThreadResolution == nil || params.RequiredReviewers == nil {
		return pullRequestRule{}, fmt.Errorf("pull_request rule omits a supported parameter")
	}
	reviewers, err := parseRequiredReviewers(*params.RequiredReviewers)
	if err != nil {
		return pullRequestRule{}, err
	}
	return pullRequestRule{
		AllowedMergeMethods:            normalizedStrings(*params.AllowedMergeMethods),
		RequiredApprovingReviewCount:   *params.RequiredApprovingReviewCount,
		DismissStaleReviewsOnPush:      *params.DismissStaleReviewsOnPush,
		RequireCodeOwnerReview:         *params.RequireCodeOwnerReview,
		RequireLastPushApproval:        *params.RequireLastPushApproval,
		RequiredReviewThreadResolution: *params.RequiredReviewThreadResolution,
		RequiredReviewers:              reviewers,
	}, nil
}

func parseRequiredReviewers(rawReviewers []requiredReviewerJSON) ([]requiredReviewer, error) {
	reviewers := make([]requiredReviewer, 0, len(rawReviewers))
	seenReviewers := make(map[string]struct{}, len(rawReviewers))
	for _, rawReviewer := range rawReviewers {
		if rawReviewer.FilePatterns == nil || rawReviewer.MinimumApprovals == nil || rawReviewer.Reviewer == nil || rawReviewer.Reviewer.ID == nil || rawReviewer.Reviewer.Type == nil {
			return nil, fmt.Errorf("pull_request required reviewer omits a supported field")
		}
		if *rawReviewer.MinimumApprovals < 0 || *rawReviewer.Reviewer.ID <= 0 || strings.TrimSpace(*rawReviewer.Reviewer.Type) == "" {
			return nil, fmt.Errorf("pull_request required reviewer is incomplete")
		}
		reviewer := requiredReviewer{
			FilePatterns:     normalizedStrings(*rawReviewer.FilePatterns),
			MinimumApprovals: *rawReviewer.MinimumApprovals,
			Reviewer: requiredReviewerIdentity{
				ID:   *rawReviewer.Reviewer.ID,
				Type: *rawReviewer.Reviewer.Type,
			},
		}
		identity := requiredReviewerKey(reviewer)
		if _, exists := seenReviewers[identity]; exists {
			return nil, fmt.Errorf("duplicate pull_request required reviewer %s", identity)
		}
		seenReviewers[identity] = struct{}{}
		reviewers = append(reviewers, reviewer)
	}
	sort.Slice(reviewers, func(i, j int) bool {
		ri, rj := reviewers[i], reviewers[j]
		if ri.Reviewer.ID != rj.Reviewer.ID {
			return ri.Reviewer.ID < rj.Reviewer.ID
		}
		if ri.Reviewer.Type != rj.Reviewer.Type {
			return ri.Reviewer.Type < rj.Reviewer.Type
		}
		if ri.MinimumApprovals != rj.MinimumApprovals {
			return ri.MinimumApprovals < rj.MinimumApprovals
		}
		if len(ri.FilePatterns) != len(rj.FilePatterns) {
			return len(ri.FilePatterns) < len(rj.FilePatterns)
		}
		for k := range ri.FilePatterns {
			if ri.FilePatterns[k] != rj.FilePatterns[k] {
				return ri.FilePatterns[k] < rj.FilePatterns[k]
			}
		}
		return false
	})
	return reviewers, nil
}

func parseRequiredStatusChecksRule(raw json.RawMessage) (requiredStatusChecksRule, error) {
	var params struct {
		StrictRequiredStatusChecksPolicy *bool                  `json:"strict_required_status_checks_policy"`
		DoNotEnforceOnCreate             *bool                  `json:"do_not_enforce_on_create"`
		RequiredStatusChecks             *[]requiredStatusCheck `json:"required_status_checks"`
	}
	if err := decodeStrictJSON(raw, &params); err != nil {
		return requiredStatusChecksRule{}, fmt.Errorf("parse required_status_checks rule: %w", err)
	}
	if params.StrictRequiredStatusChecksPolicy == nil || params.DoNotEnforceOnCreate == nil || params.RequiredStatusChecks == nil {
		return requiredStatusChecksRule{}, fmt.Errorf("required_status_checks rule omits a supported parameter")
	}
	checks := append([]requiredStatusCheck(nil), (*params.RequiredStatusChecks)...)
	seenChecks := make(map[string]struct{}, len(checks))
	for _, check := range checks {
		if strings.TrimSpace(check.Context) == "" || (check.IntegrationID != nil && *check.IntegrationID <= 0) {
			return requiredStatusChecksRule{}, fmt.Errorf("required status check is incomplete")
		}
		identity := requiredCheckIdentity(check)
		if _, exists := seenChecks[identity]; exists {
			return requiredStatusChecksRule{}, fmt.Errorf("duplicate required status check %s", identity)
		}
		seenChecks[identity] = struct{}{}
	}
	sort.Slice(checks, func(i, j int) bool {
		ci, cj := checks[i], checks[j]
		if ci.Context != cj.Context {
			return ci.Context < cj.Context
		}
		if (ci.IntegrationID == nil) != (cj.IntegrationID == nil) {
			return ci.IntegrationID != nil
		}
		if ci.IntegrationID != nil && cj.IntegrationID != nil {
			return *ci.IntegrationID < *cj.IntegrationID
		}
		return false
	})
	return requiredStatusChecksRule{
		StrictRequiredStatusChecksPolicy: *params.StrictRequiredStatusChecksPolicy,
		DoNotEnforceOnCreate:             *params.DoNotEnforceOnCreate,
		RequiredChecks:                   checks,
	}, nil
}

func emptyRuleParameters(raw json.RawMessage) bool {
	return len(bytes.TrimSpace(raw)) == 0
}

func decodeStrictJSON(raw []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}

func rejectUnknownObjectKeys(raw []byte, object string, allowed []string) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return err
	}
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, key := range allowed {
		allowedSet[key] = struct{}{}
	}
	var unknown []string
	for key := range fields {
		if _, ok := allowedSet[key]; !ok {
			unknown = append(unknown, key)
		}
	}
	if len(unknown) == 0 {
		return nil
	}
	sort.Strings(unknown)
	return fmt.Errorf("%s contains unsupported fields: %s", object, strings.Join(unknown, ", "))
}

func normalizedStrings(values []string) []string {
	out := append([]string(nil), values...)
	sort.Strings(out)
	return out
}

func requiredReviewerKey(reviewer requiredReviewer) string {
	return fmt.Sprintf("%d/%s/%d/%q", reviewer.Reviewer.ID, reviewer.Reviewer.Type, reviewer.MinimumApprovals, reviewer.FilePatterns)
}

// validateCanonicalRuleset checks v against the zero-bypass branch-protection
// subset (DECL-RULESET-04). repo scopes the invariants that are specific to
// dear-agent's own checked-in declaration: the mandatory GitHub Actions
// integration ID on every required check. Other managed fleet repositories
// are inventory-owned and legitimately declare context-only required checks
// with no integration ID (infra/variables.tf required_checks); an omitted
// integration ID normalizes as an explicit context-only identity for them.
func validateCanonicalRuleset(v rulesetView, repo string) error {
	if v.Target != "branch" {
		return fmt.Errorf("unsupported target %q", v.Target)
	}
	if !containsDefaultBranchRef(v.RefNameInclude) {
		return fmt.Errorf("ref_name include must contain ~DEFAULT_BRANCH")
	}
	if len(v.RefNameExclude) != 0 {
		return fmt.Errorf("ref_name exclude must be empty; default branch cannot be excluded from a zero-bypass policy")
	}
	if v.Enforcement != "active" {
		return fmt.Errorf("enforcement must be active")
	}
	if len(v.BypassActors) != 0 {
		return fmt.Errorf("bypass actors must be empty")
	}
	if !v.Deletion || !v.NonFastForward || !v.RequiredLinearHistory {
		return fmt.Errorf("deletion, non_fast_forward, and required_linear_history rules are required")
	}
	if !reflect.DeepEqual(v.PullRequest.AllowedMergeMethods, []string{"squash"}) {
		return fmt.Errorf("allowed merge methods must be exactly squash")
	}
	if !v.RequiredStatusChecks.StrictRequiredStatusChecksPolicy {
		return fmt.Errorf("required status checks must be strict")
	}
	if len(v.RequiredStatusChecks.RequiredChecks) == 0 {
		return fmt.Errorf("at least one required status check is required")
	}
	return validateRequiredCheckIdentities(v.RequiredStatusChecks.RequiredChecks, repo)
}

// validateRequiredCheckIdentities enforces the integration-ID invariant for
// dear-agent's own required checks. Split out of validateCanonicalRuleset to
// keep that function's cyclomatic complexity under the repo's gocyclo limit.
func validateRequiredCheckIdentities(checks []requiredStatusCheck, repo string) error {
	if !isDearAgentRepo(repo) {
		return nil
	}
	for _, check := range checks {
		if check.IntegrationID == nil || *check.IntegrationID != githubActionsIntegrationID {
			return fmt.Errorf("required status check %q must use integration_id %d", check.Context, githubActionsIntegrationID)
		}
	}
	return nil
}

// containsDefaultBranchRef reports whether include contains the literal
// "~DEFAULT_BRANCH" ref-name pattern GitHub uses to target a repository's
// default branch regardless of its actual name.
func containsDefaultBranchRef(include []string) bool {
	return slices.Contains(include, "~DEFAULT_BRANCH")
}

func requiredCheckIdentity(check requiredStatusCheck) string {
	if check.IntegrationID == nil {
		return fmt.Sprintf("%q (integration_id=<none>)", check.Context)
	}
	return fmt.Sprintf("%q (integration_id=%d)", check.Context, *check.IntegrationID)
}
