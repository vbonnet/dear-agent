package steps

import (
	"context"
	"fmt"
	"strings"

	"github.com/cucumber/godog"

	"github.com/vbonnet/ai-tools/claude-session-manager/test/bdd/internal/adapters/mock"
	"github.com/vbonnet/ai-tools/claude-session-manager/test/bdd/internal/testenv"
)

// RegisterSessionSteps registers session-related step definitions
func RegisterSessionSteps(ctx *godog.ScenarioContext) {
	ctx.Step(`^I run "([^"]*)"$`, iRun)
	ctx.Step(`^a session "([^"]*)" should be created$`, sessionShouldBeCreated)
	ctx.Step(`^the manifest should show agent "([^"]*)"$`, manifestShouldShowAgent)
	ctx.Step(`^the session state should be "([^"]*)"$`, sessionStateShouldBe)
	ctx.Step(`^a session "([^"]*)" exists with agent "([^"]*)"$`, aSessionExistsWithAgent)
	ctx.Step(`^I pause the session "([^"]*)"$`, iPauseTheSession)
	ctx.Step(`^I resume the session "([^"]*)"$`, iResumeTheSession)
	ctx.Step(`^the session "([^"]*)" should be active$`, sessionShouldBeActive)
	ctx.Step(`^the session "([^"]*)" should be archived$`, sessionShouldBeArchived)
	ctx.Step(`^the agent should be "([^"]*)"$`, agentShouldBe)
	ctx.Step(`^session "([^"]*)" should have agent "([^"]*)"$`, sessionShouldHaveAgent)
	ctx.Step(`^the session should still use agent "([^"]*)"$`, sessionShouldStillUseAgent)
}

// CommandArgs represents parsed command arguments
type CommandArgs struct {
	Command     string
	Agent       string
	SessionName string
}

func parseCommand(command string) *CommandArgs {
	parts := strings.Fields(command)
	args := &CommandArgs{
		Command: "new", // default
	}

	for i, part := range parts {
		switch part {
		case "csm":
			if i+1 < len(parts) {
				args.Command = parts[i+1]
			}
		case "new", "resume", "archive":
			args.Command = part
		}

		if strings.HasPrefix(part, "--agent=") {
			args.Agent = strings.TrimPrefix(part, "--agent=")
		}
	}

	// Last non-flag argument is session name
	for i := len(parts) - 1; i >= 0; i-- {
		if !strings.HasPrefix(parts[i], "--") && parts[i] != "csm" && parts[i] != args.Command {
			args.SessionName = parts[i]
			break
		}
	}

	return args
}

func iRun(ctx context.Context, command string) (context.Context, error) {
	env := testenv.EnvFromContext(ctx)

	args := parseCommand(command)

	switch args.Command {
	case "new":
		adapter, err := env.GetAdapter(args.Agent)
		if err != nil {
			env.LastError = err
			return ctx, nil // Don't fail here, let assertion step check
		}

		session, err := adapter.CreateSession(ctx, mock.CreateSessionRequest{
			Name: args.SessionName,
			Tags: []string{fmt.Sprintf("agent:%s", args.Agent)},
		})

		env.CurrentSession = session
		if session != nil {
			env.Sessions[session.Name] = session
		}
		env.LastError = err

	case "resume":
		if env.CurrentSession == nil {
			env.LastError = fmt.Errorf("no session to resume")
			return ctx, nil
		}

		adapter, err := env.GetAdapter(env.CurrentSession.Agent)
		if err != nil {
			env.LastError = err
			return ctx, nil
		}

		session, err := adapter.ResumeSession(ctx, env.CurrentSession.ID)
		env.CurrentSession = session
		env.LastError = err

	case "archive":
		if env.CurrentSession == nil {
			env.LastError = fmt.Errorf("no session to archive")
			return ctx, nil
		}

		adapter, err := env.GetAdapter(env.CurrentSession.Agent)
		if err != nil {
			env.LastError = err
			return ctx, nil
		}

		err = adapter.ArchiveSession(ctx, env.CurrentSession.ID)
		env.LastError = err

		// Update current session state
		if env.LastError == nil {
			env.CurrentSession.State = mock.StateArchived
		}
	}

	return ctx, nil
}

func sessionShouldBeCreated(ctx context.Context, sessionName string) error {
	env := testenv.EnvFromContext(ctx)

	if env.LastError != nil {
		return fmt.Errorf("session creation failed: %w", env.LastError)
	}

	if env.CurrentSession == nil {
		return fmt.Errorf("no session was created")
	}

	if env.CurrentSession.Name != sessionName {
		return fmt.Errorf("expected session %q, got %q", sessionName, env.CurrentSession.Name)
	}

	return nil
}

func manifestShouldShowAgent(ctx context.Context, agent string) error {
	env := testenv.EnvFromContext(ctx)

	if env.CurrentSession == nil {
		return fmt.Errorf("no current session")
	}

	if env.CurrentSession.Agent != agent {
		return fmt.Errorf("expected agent %q, got %q", agent, env.CurrentSession.Agent)
	}

	return nil
}

func sessionStateShouldBe(ctx context.Context, state string) error {
	env := testenv.EnvFromContext(ctx)

	if env.CurrentSession == nil {
		return fmt.Errorf("no current session")
	}

	expectedState := mock.SessionState(state)
	if env.CurrentSession.State != expectedState {
		return fmt.Errorf("expected state %q, got %q", expectedState, env.CurrentSession.State)
	}

	return nil
}

func aSessionExistsWithAgent(ctx context.Context, sessionName, agent string) (context.Context, error) {
	env := testenv.EnvFromContext(ctx)

	adapter, err := env.GetAdapter(agent)
	if err != nil {
		return ctx, err
	}

	session, err := adapter.CreateSession(ctx, mock.CreateSessionRequest{
		Name: sessionName,
		Tags: []string{fmt.Sprintf("agent:%s", agent)},
	})

	if err != nil {
		return ctx, err
	}

	env.CurrentSession = session
	env.Sessions[sessionName] = session // Store session by name for later retrieval
	return ctx, nil
}

func iPauseTheSession(ctx context.Context, sessionID string) (context.Context, error) {
	env := testenv.EnvFromContext(ctx)

	if env.CurrentSession == nil {
		return ctx, fmt.Errorf("no current session")
	}

	adapter, err := env.GetAdapter(env.CurrentSession.Agent)
	if err != nil {
		return ctx, err
	}

	err = adapter.PauseSession(ctx, env.CurrentSession.ID)
	if err != nil {
		return ctx, err
	}

	// Update current session state
	env.CurrentSession.State = mock.StatePaused

	return ctx, nil
}

func iResumeTheSession(ctx context.Context, sessionID string) (context.Context, error) {
	env := testenv.EnvFromContext(ctx)

	if env.CurrentSession == nil {
		return ctx, fmt.Errorf("no current session")
	}

	adapter, err := env.GetAdapter(env.CurrentSession.Agent)
	if err != nil {
		return ctx, err
	}

	session, err := adapter.ResumeSession(ctx, env.CurrentSession.ID)
	if err != nil {
		return ctx, err
	}

	env.CurrentSession = session
	return ctx, nil
}

func sessionShouldBeActive(ctx context.Context, sessionName string) error {
	env := testenv.EnvFromContext(ctx)

	if env.CurrentSession == nil {
		return fmt.Errorf("no current session")
	}

	if env.CurrentSession.State != mock.StateActive {
		return fmt.Errorf("expected session to be active, got state %q", env.CurrentSession.State)
	}

	return nil
}

func sessionShouldBeArchived(ctx context.Context, sessionName string) error {
	env := testenv.EnvFromContext(ctx)

	if env.CurrentSession == nil {
		return fmt.Errorf("no current session")
	}

	if env.CurrentSession.State != mock.StateArchived {
		return fmt.Errorf("expected session to be archived, got state %q", env.CurrentSession.State)
	}

	return nil
}

func agentShouldBe(ctx context.Context, agent string) error {
	env := testenv.EnvFromContext(ctx)

	if env.CurrentSession == nil {
		return fmt.Errorf("no current session")
	}

	if env.CurrentSession.Agent != agent {
		return fmt.Errorf("expected agent %q, got %q", agent, env.CurrentSession.Agent)
	}

	return nil
}

func sessionShouldHaveAgent(ctx context.Context, sessionName, agent string) error {
	env := testenv.EnvFromContext(ctx)

	session, exists := env.Sessions[sessionName]
	if !exists {
		return fmt.Errorf("session %q not found", sessionName)
	}

	if session.Agent != agent {
		return fmt.Errorf("expected agent %q, got %q", agent, session.Agent)
	}

	return nil
}

func sessionShouldStillUseAgent(ctx context.Context, agent string) error {
	return agentShouldBe(ctx, agent)
}
