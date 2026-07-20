package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/pkg/llm/provider"
	"github.com/vbonnet/dear-agent/pkg/llm/router"
	"github.com/vbonnet/dear-agent/pkg/workflow/roles"
)

var (
	reviewRole        string // --role: which router role to route through (default "reviewer")
	reviewInstruction string // --instruction: focus for the critique
	reviewRolesPath   string // --roles: explicit roles config path override
	reviewMaxTokens   int    // --max-tokens: response cap
)

// defaultReviewSystemPrompt frames the routed model as an adversarial
// critic per the reviewer role's intent (config/roles.yaml: "Adversarial
// critique of artifacts produced by implementer").
const defaultReviewSystemPrompt = `You are an adversarial code/document reviewer. ` +
	`You did not write the artifact under review and you do not share its author's blind spots. ` +
	`Critique it directly: call out correctness bugs, unhandled edge cases, unsafe assumptions, ` +
	`and unclear or missing reasoning. Be specific and cite the relevant part of the artifact. ` +
	`If the artifact is sound, say so plainly rather than inventing problems.`

var reviewCmd = &cobra.Command{
	Use:   "review <artifact>",
	Short: "Route an artifact through a cross-model review role",
	Long: `Send an artifact to a reviewer model via the role-based LLM router.

The reviewer role is deliberately backed by a different vendor than the
implementer role (see config/roles.yaml) so the critique
is unlikely to share the implementer's blind spots. The router walks the
role's primary → secondary → tertiary model chain, falling through on
failure.

The roles config is resolved in this order:
  1. --roles <path> (explicit override)
  2. $DEAR_AGENT_ROLES
  3. ./.dear-agent/roles.yaml
  4. ~/.config/dear-agent/roles.yaml
  5. compiled-in builtin registry

Examples:
  # Review a file with the default reviewer role
  agm review path/to/patch.diff

  # Focus the critique
  agm review proposal.md --instruction "Focus on the migration's rollback story"

  # Route through a different role
  agm review design.md --role research`,
	Args: cobra.ExactArgs(1),
	RunE: runReview,
}

func init() {
	reviewCmd.Flags().StringVar(&reviewRole, "role", "reviewer", "router role to route the artifact through")
	reviewCmd.Flags().StringVar(&reviewInstruction, "instruction", "", "additional focus for the review")
	reviewCmd.Flags().StringVar(&reviewRolesPath, "roles", "", "explicit roles config path (overrides discovery)")
	reviewCmd.Flags().IntVar(&reviewMaxTokens, "max-tokens", 4096, "maximum response length")

	rootCmd.AddCommand(reviewCmd)
}

// loadReviewRouter resolves the roles registry, bridges it into the
// router's config shape, and constructs a Router. rolesPath, when
// non-empty, forces a specific config file; otherwise discovery falls
// through env → cwd → home → builtin (see roles.AutoLoad).
func loadReviewRouter(rolesPath string) (*router.Router, string, error) {
	var reg *roles.Registry
	source := rolesPath
	if rolesPath != "" {
		r, err := roles.LoadFile(rolesPath)
		if err != nil {
			return nil, source, fmt.Errorf("load roles config: %w", err)
		}
		reg = r
	} else {
		cwd, _ := os.Getwd()
		home, _ := os.UserHomeDir()
		r, src, err := roles.AutoLoad(os.Getenv("DEAR_AGENT_ROLES"), cwd, home)
		if err != nil {
			return nil, src, fmt.Errorf("load roles config: %w", err)
		}
		reg, source = r, src
	}

	cfg := registryToRouterConfig(reg)
	r, err := router.New(router.Options{Config: cfg})
	if err != nil {
		return nil, source, fmt.Errorf("init router: %w", err)
	}
	return r, source, nil
}

// registryToRouterConfig bridges the richer pkg/workflow/roles registry
// (per-tier model + effort + cost) into the router's flat string-per-tier
// RoleSpec. The router only needs the candidate model ids in order; the
// resolver maps them to providers. A tier with an empty model id is
// skipped so RoleSpec.Candidates() stays clean.
func registryToRouterConfig(reg *roles.Registry) *router.Config {
	cfg := &router.Config{Version: 1, Roles: map[string]router.RoleSpec{}}
	if reg == nil {
		return cfg
	}
	for name, role := range reg.Roles {
		cfg.Roles[name] = router.RoleSpec{
			Primary:   tierModel(role.Primary),
			Secondary: tierModel(role.Secondary),
			Tertiary:  tierModel(role.Tertiary),
		}
	}
	return cfg
}

func tierModel(t *roles.Tier) string {
	if t == nil {
		return ""
	}
	return t.Model
}

func runReview(cmd *cobra.Command, args []string) error {
	artifactPath := args[0]

	content, err := os.ReadFile(artifactPath)
	if err != nil {
		return fmt.Errorf("read artifact %q: %w", artifactPath, err)
	}

	r, source, err := loadReviewRouter(reviewRolesPath)
	if err != nil {
		return err
	}

	if !r.HasRole(reviewRole) {
		return fmt.Errorf("role %q is not defined in roles config (%s)", reviewRole, source)
	}

	systemPrompt := defaultReviewSystemPrompt
	if reviewInstruction != "" {
		systemPrompt += "\n\nReviewer focus for this artifact: " + reviewInstruction
	}

	prompt := fmt.Sprintf("Review the following artifact (%s):\n\n%s",
		filepath.Base(artifactPath), string(content))

	req := &provider.GenerateRequest{
		Prompt:       prompt,
		SystemPrompt: systemPrompt,
		MaxTokens:    reviewMaxTokens,
		Metadata: map[string]any{
			"agm_command": "review",
			"artifact":    artifactPath,
		},
	}

	fmt.Fprintf(os.Stderr, "Reviewing %s via role %q (roles: %s)...\n", artifactPath, reviewRole, source)

	resp, err := r.Generate(context.Background(), reviewRole, req)
	if err != nil {
		return fmt.Errorf("review failed: %w", err)
	}

	fmt.Println(resp.Text)
	fmt.Fprintf(os.Stderr, "\n— reviewed by %s (in %d / out %d tokens, $%.4f)\n",
		resp.Model, resp.Usage.InputTokens, resp.Usage.OutputTokens, resp.Usage.CostUSD)

	return nil
}
