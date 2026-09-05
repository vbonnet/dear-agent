package safegit

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestRequiredCheckNamesForBranchRulesetOnly(t *testing.T) {
	installRequiredCheckFakeGH(t, `
[ "${GH_PROMPT_DISABLED:-}" = "1" ] || { printf '%s\n' 'GH prompts not disabled' >&2; exit 3; }
[ "${GH_NO_UPDATE_NOTIFIER:-}" = "1" ] || { printf '%s\n' 'GH update notifier not disabled' >&2; exit 3; }
[ "${GIT_TERMINAL_PROMPT:-}" = "0" ] || { printf '%s\n' 'git terminal prompts not disabled' >&2; exit 3; }
case "$*" in
  *rules/branches*) printf '%s\n' '[[{"type":"required_status_checks","parameters":{"required_status_checks":[{"context":"Ruleset Build"}]}}]]' ;;
  *protection/required_status_checks*) printf '%s\n' 'gh: Branch not protected (HTTP 404)' >&2; exit 1 ;;
  *) printf '%s\n' 'unexpected gh invocation' >&2; exit 2 ;;
esac
`)

	names, err := RequiredCheckNamesForBranch(context.Background(), "owner/repo", "main")
	if err != nil {
		t.Fatalf("RequiredCheckNamesForBranch() error = %v", err)
	}
	if !names["Ruleset Build"] || len(names) != 1 {
		t.Fatalf("required names = %#v, want only ruleset context", names)
	}
}

func TestRequiredCheckNamesForBranchUnionsLayers(t *testing.T) {
	installRequiredCheckFakeGH(t, `
case "$*" in
  *rules/branches*) printf '%s\n' '[[{"type":"required_status_checks","parameters":{"required_status_checks":[{"context":"Ruleset"}]}}]]' ;;
  *protection/required_status_checks*) printf '%s\n' '{"contexts":["Classic"]}' ;;
  *) printf '%s\n' 'unexpected gh invocation' >&2; exit 2 ;;
esac
`)

	names, err := RequiredCheckNamesForBranch(context.Background(), "owner/repo", "main")
	if err != nil {
		t.Fatalf("RequiredCheckNamesForBranch() error = %v", err)
	}
	if !names["Ruleset"] || !names["Classic"] || len(names) != 2 {
		t.Fatalf("required names = %#v, want layered union", names)
	}
}

func TestRequiredCheckNamesForBranchAuthoritativeEmpty(t *testing.T) {
	installRequiredCheckFakeGH(t, `
case "$*" in
  *rules/branches*) printf '%s\n' '[[]]' ;;
  *protection/required_status_checks*) printf '%s\n' 'gh: Branch not protected (HTTP 404)' >&2; exit 1 ;;
  *) printf '%s\n' 'unexpected gh invocation' >&2; exit 2 ;;
esac
`)

	names, err := RequiredCheckNamesForBranch(context.Background(), "owner/repo", "main")
	if err != nil {
		t.Fatalf("RequiredCheckNamesForBranch() error = %v", err)
	}
	if names == nil || len(names) != 0 {
		t.Fatalf("authoritative-empty names = %#v, want non-nil empty set", names)
	}
}

func TestRequiredCheckNamesForBranchRejectsPartialPolicy(t *testing.T) {
	installRequiredCheckFakeGH(t, `
case "$*" in
  *rules/branches*) printf '%s\n' '[[{"type":"required_status_checks","parameters":{"required_status_checks":[{"context":"Ruleset"}]}}]]' ;;
  *protection/required_status_checks*) printf '%s\n' 'gh: provider unavailable (HTTP 500)' >&2; exit 1 ;;
  *) printf '%s\n' 'unexpected gh invocation' >&2; exit 2 ;;
esac
`)

	names, err := RequiredCheckNamesForBranch(context.Background(), "owner/repo", "main")
	if err == nil {
		t.Fatal("partial policy must fail closed")
	}
	if names != nil {
		t.Fatalf("partial policy returned names %#v, want nil on error", names)
	}
}

func TestRequiredCheckNamesForBranchRejectsMalformedClassicPolicy(t *testing.T) {
	installRequiredCheckFakeGH(t, `
case "$*" in
  *rules/branches*) printf '%s\n' '[[{"type":"required_status_checks","parameters":{"required_status_checks":[{"context":"Ruleset"}]}}]]' ;;
  *protection/required_status_checks*) printf '%s\n' '{not-json' ;;
  *) printf '%s\n' 'unexpected gh invocation' >&2; exit 2 ;;
esac
`)

	names, err := RequiredCheckNamesForBranch(context.Background(), "owner/repo", "main")
	if err == nil || !strings.Contains(err.Error(), "parsing classic required checks") {
		t.Fatalf("RequiredCheckNamesForBranch() = %#v, %v; want classic parse error", names, err)
	}
	if names != nil {
		t.Fatalf("malformed classic policy returned names %#v, want nil", names)
	}
}

func TestRequiredCheckNamesForBranchRejectsUnsupportedIdentity(t *testing.T) {
	cases := []struct {
		name  string
		rules string
		want  string
	}{
		{
			name:  "required workflow",
			rules: `[[{"type":"workflows","parameters":{}}]]`,
			want:  "required workflow",
		},
		{
			name:  "integration scoped check",
			rules: `[[{"type":"required_status_checks","parameters":{"required_status_checks":[{"context":"Build","integration_id":42}]}}]]`,
			want:  "integration-scoped",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			installRequiredCheckFakeGH(t, fmt.Sprintf(`
case "$*" in
  *rules/branches*) printf '%%s\n' '%s' ;;
  *protection/required_status_checks*) printf '%%s\n' 'gh: Branch not protected (HTTP 404)' >&2; exit 1 ;;
  *) printf '%%s\n' 'unexpected gh invocation' >&2; exit 2 ;;
esac
`, tc.rules))

			names, err := RequiredCheckNamesForBranch(context.Background(), "owner/repo", "main")
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("RequiredCheckNamesForBranch() = %#v, %v; want error containing %q", names, err, tc.want)
			}
			if names != nil {
				t.Fatalf("unsupported policy returned names %#v, want nil", names)
			}
		})
	}
}
