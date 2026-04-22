package rbac

// Common permission sets shared across profiles to reduce duplication.
var (
	readOnlyShell = []string{
		"Bash(grep:*)",
		"Bash(find:*)",
		"Bash(ls:*)",
		"Bash(cat:*)",
		"Bash(head:*)",
		"Bash(tail:*)",
		"Bash(wc:*)",
		"Bash(diff:*)",
	}

	gitReadOnly = []string{
		"Bash(git log:*)",
		"Bash(git show:*)",
		"Bash(git diff:*)",
		"Bash(git status:*)",
		"Bash(git branch:*)",
	}

	codeReadOnly = []string{
		"Read(~/src/**)",
		"Glob(~/src/**)",
		"Grep(~/src/**)",
	}

	codeReadWrite = []string{
		"Read(~/src/**)",
		"Edit(~/src/**)",
		"Write(~/src/**)",
		"Glob(~/src/**)",
		"Grep(~/src/**)",
	}

	agmAccess = []string{
		"Bash(agm:*)",
		"Skill(agm:*)",
	}

	// orchestratorAGM contains explicit agm subcommand patterns for
	// orchestrator-tier roles. These are needed in addition to the agmAccess
	// wildcard because Claude Code matches subcommand patterns separately
	// from colon-prefix wildcards.
	orchestratorAGM = []string{
		// Session management
		"Bash(agm session list *)",
		"Bash(agm session archive *)",
		"Bash(agm session gc *)",
		"Bash(agm session health *)",
		"Bash(agm session summary *)",
		"Bash(agm session tag *)",
		"Bash(agm session select-option *)",
		"Bash(agm session status *)",
		"Bash(agm session new *)",
		"Bash(agm session resume *)",
		"Bash(agm session associate *)",
		// Messaging
		"Bash(agm send *)",
		"Bash(agm send msg *)",
		// Verification & trust
		"Bash(agm verify *)",
		"Bash(agm trust score *)",
		"Bash(agm trust record *)",
		// Observability
		"Bash(agm metrics *)",
		"Bash(agm dashboard *)",
		"Bash(agm scan *)",
		// UI escape hatch
		"Bash(agm escape-ui *)",
	}

	// orchestratorTmux contains tmux patterns needed for pane monitoring
	// and session interaction in orchestrator-tier roles.
	orchestratorTmux = []string{
		"Bash(tmux capture-pane *)",
		"Bash(tmux list-sessions *)",
		"Bash(tmux list-windows *)",
		"Bash(tmux list-panes *)",
		"Bash(tmux display-message *)",
		"Bash(tmux send-keys *)",
		"Bash(tmux select-pane *)",
		"Bash(tmux select-window *)",
	}

	// orchestratorGit contains git patterns for commit verification
	// and cross-repo inspection from orchestrator-tier roles.
	orchestratorGit = []string{
		"Bash(git -C *)",
		"Bash(git -C * log *)",
		"Bash(git -C * diff *)",
		"Bash(git -C * show *)",
		"Bash(git -C * status *)",
		"Bash(git -C * rev-parse *)",
		"Bash(git -C * branch *)",
	}

	buildTools = []string{
		"Bash(go:*)",
		"Bash(make:*)",
		"Bash(npm:*)",
		"Bash(pip:*)",
		"Bash(cargo:*)",
	}

	// agmFileAccess grants read/write access to ~/.agm/ directory via
	// Claude Code tools. Supervisor-tier roles need this to inspect and
	// manage session manifests, heartbeats, and other AGM state files.
	agmFileAccess = []string{
		"Read(~/.agm/**)",
		"Write(~/.agm/**)",
		"Edit(~/.agm/**)",
		"Glob(~/.agm/**)",
		"Grep(~/.agm/**)",
	}

	// docsReadAccess grants read access to docs/ directories within the
	// workspace. Supervisor-tier roles need this to reference ADRs,
	// architecture docs, and other project documentation.
	docsReadAccess = []string{
		"Read(docs/**)",
		"Read(*/docs/**)",
		"Glob(docs/**)",
		"Glob(*/docs/**)",
		"Grep(docs/**)",
		"Grep(*/docs/**)",
	}
)

// flatten concatenates multiple string slices.
func flatten(slices ...[]string) []string {
	var total int
	for _, s := range slices {
		total += len(s)
	}
	result := make([]string, 0, total)
	for _, s := range slices {
		result = append(result, s...)
	}
	return result
}

// profiles maps each role to its permission profile.
var profiles = map[Role]Profile{
	RoleMetaOrchestrator: {
		Name:        RoleMetaOrchestrator,
		Description: "Supervisory oversight — observes, advises, detects cross-session patterns",
		TrustLevel:  TrustTrusted,
		AllowedTools: flatten(
			codeReadOnly,
			orchestratorTmux,
			orchestratorAGM,
			orchestratorGit,
			[]string{
				"Bash(git:*)",
			},
			agmAccess,
			agmFileAccess,
			docsReadAccess,
			readOnlyShell,
			gitReadOnly,
		),
	},

	RoleOrchestrator: {
		Name:        RoleOrchestrator,
		Description: "Session coordination, worker management, permission approvals",
		TrustLevel:  TrustTrusted,
		AllowedTools: flatten(
			codeReadOnly,
			orchestratorTmux,
			orchestratorAGM,
			orchestratorGit,
			[]string{
				"Bash(tmux:*)",
				"Bash(git:*)",
			},
			agmAccess,
			agmFileAccess,
			docsReadAccess,
			readOnlyShell,
			gitReadOnly,
		),
	},

	RoleOverseer: {
		Name:        RoleOverseer,
		Description: "Read-only monitoring with dashboards and AGM admin commands",
		TrustLevel:  TrustTrusted,
		AllowedTools: flatten(
			codeReadOnly,
			orchestratorTmux,
			orchestratorAGM,
			orchestratorGit,
			[]string{
				"Bash(git:*)",
			},
			agmAccess,
			agmFileAccess,
			docsReadAccess,
			readOnlyShell,
			gitReadOnly,
		),
	},

	// RoleSupervisor is a unified profile alias for supervisor-tier roles.
	// Use --permission-profile supervisor when the specific sub-role
	// (orchestrator, overseer, meta-orchestrator) is not yet known.
	RoleSupervisor: {
		Name:        RoleSupervisor,
		Description: "Unified supervisor profile — git, tmux, agm, docs, .agm/ access",
		TrustLevel:  TrustTrusted,
		AllowedTools: flatten(
			codeReadOnly,
			orchestratorTmux,
			orchestratorAGM,
			orchestratorGit,
			[]string{
				"Bash(tmux:*)",
				"Bash(git:*)",
			},
			agmAccess,
			agmFileAccess,
			docsReadAccess,
			readOnlyShell,
			gitReadOnly,
		),
	},

	RoleWorker: {
		Name:        RoleWorker,
		Description: "General-purpose code implementation with full build toolchain",
		TrustLevel:  TrustStandard,
		AllowedTools: flatten(
			codeReadWrite,
			[]string{
				"Bash(tmux:*)",
				"Bash(git:*)",
			},
			buildTools,
			agmAccess,
			readOnlyShell,
			gitReadOnly,
		),
	},

	RoleImplementer: {
		Name:        RoleImplementer,
		Description: "Code development scoped to worktree paths — no tmux/orchestration",
		TrustLevel:  TrustStandard,
		AllowedTools: flatten(
			codeReadWrite,
			[]string{
				"Bash(git:*)",
			},
			buildTools,
			[]string{
				"Bash(agm session status *)",
			},
			readOnlyShell,
			gitReadOnly,
			[]string{
				"Bash(go build:*)",
				"Bash(go test:*)",
				"Bash(go vet:*)",
				"Bash(git add:*)",
				"Bash(git commit:*)",
				"Bash(git push:*)",
				"Bash(git merge:*)",
			},
		),
	},

	RoleResearcher: {
		Name:        RoleResearcher,
		Description: "Investigation, analysis, and research document production",
		TrustLevel:  TrustStandard,
		AllowedTools: flatten(
			codeReadWrite,
			[]string{
				"Bash(tmux:*)",
				"Bash(git:*)",
			},
			[]string{
				"Bash(go:*)",
				"Bash(make:*)",
			},
			[]string{
				"WebSearch(*)",
				"WebFetch(*)",
			},
			agmAccess,
			readOnlyShell,
		),
	},

	RoleVerifier: {
		Name:        RoleVerifier,
		Description: "Read-only access plus test runners for validation work",
		TrustLevel:  TrustSandboxed,
		AllowedTools: flatten(
			codeReadOnly,
			[]string{
				"Bash(go test *)",
				"Bash(go -C * test *)",
				"Bash(npm test *)",
				"Bash(pytest *)",
				"Bash(make test *)",
				"Bash(cargo test *)",
				"Bash(agm session status *)",
				"Bash(agm session send *)",
			},
			[]string{"Skill(agm:*)"},
			readOnlyShell,
			[]string{
				"Bash(git log:*)",
				"Bash(git show:*)",
				"Bash(git diff:*)",
			},
		),
	},

	RoleRequester: {
		Name:        RoleRequester,
		Description: "Read-only access with planning tools — no code changes",
		TrustLevel:  TrustSandboxed,
		AllowedTools: flatten(
			codeReadOnly,
			[]string{
				"Bash(git:*)",
				"Bash(agm session status *)",
				"Bash(agm session list *)",
			},
			[]string{"Skill(agm:*)"},
		),
	},

	RoleAuditor: {
		Name:        RoleAuditor,
		Description: "Periodic health/quality checks — read-only, time-limited",
		TrustLevel:  TrustSandboxed,
		AllowedTools: flatten(
			codeReadOnly,
			[]string{
				"Bash(git log *)",
				"Bash(git diff *)",
				"Bash(git show *)",
				"Bash(git blame *)",
			},
		),
	},

	RoleMonitor: {
		Name:        RoleMonitor,
		Description: "Session monitoring with tmux and git read access",
		TrustLevel:  TrustStandard,
		AllowedTools: flatten(
			codeReadOnly,
			[]string{
				"Bash(tmux:*)",
				"Bash(git:*)",
			},
			agmAccess,
		),
	},
}
