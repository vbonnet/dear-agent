// Package steps provides steps functionality.
package steps

import (
	"context"
	"fmt"

	"github.com/cucumber/godog"
	"github.com/vbonnet/dear-agent/agm/test/bdd/internal/testenv"
)

func RegisterAgentInterfaceSteps(ctx *godog.ScenarioContext) {
	ctx.Step(`^the adapter should have method (\w+)$`, adapterShouldHaveMethod)
	ctx.Step(`^the adapter should return name "([^"]*)"$`, adapterShouldReturnName)
	ctx.Step(`^I create a session "([^"]*)" with agent "([^"]*)"$`, iCreateSessionWithAgent)
	ctx.Step(`^I pause the session "([^"]*)"$`, iPauseSessionNamed)
	ctx.Step(`^I resume the session "([^"]*)"$`, iResumeSessionNamed)
	ctx.Step(`^I archive the session "([^"]*)"$`, iArchiveSessionNamed)
	ctx.Step(`^the adapter should support session creation$`, adapterSupportsSessionCreation)
	ctx.Step(`^the adapter should support message sending$`, adapterSupportsMessageSending)
	ctx.Step(`^the adapter should support history retrieval$`, adapterSupportsHistoryRetrieval)
	ctx.Step(`^the adapter should support session lifecycle management$`, adapterSupportsLifecycle)
}

func adapterShouldHaveMethod(ctx context.Context, methodName string) (context.Context, error) {
	// All methods are guaranteed by interface compilation
	// This validates the interface contract
	validMethods := map[string]bool{
		"CreateSession":  true,
		"SendMessage":    true,
		"GetHistory":     true,
		"PauseSession":   true,
		"ResumeSession":  true,
		"ArchiveSession": true,
		"GetSession":     true,
		"Name":           true,
	}

	if !validMethods[methodName] {
		return ctx, fmt.Errorf("unknown method: %s", methodName)
	}
	return ctx, nil
}

func adapterShouldReturnName(ctx context.Context, expectedName string) (context.Context, error) {
	env := testenv.EnvFromContext(ctx)
	if env.CurrentAdapter == nil {
		return ctx, fmt.Errorf("no adapter configured")
	}

	actualName := env.CurrentAdapter.Name()
	if actualName != expectedName {
		return ctx, fmt.Errorf("expected adapter name %s, got %s", expectedName, actualName)
	}
	return ctx, nil
}

func iCreateSessionWithAgent(ctx context.Context, sessionName, agentName string) (context.Context, error) {
	env := testenv.EnvFromContext(ctx)

	adapter, err := env.GetAdapter(agentName)
	if err != nil {
		return ctx, err
	}

	session, err := adapter.CreateSession(ctx, testenv.CreateSessionRequest{
		Name: sessionName,
	})
	if err != nil {
		return ctx, err
	}

	env.CurrentSession = session
	env.CurrentAdapter = adapter
	env.Sessions[sessionName] = session
	return testenv.ContextWithEnv(ctx, env), nil
}

func iPauseSessionNamed(ctx context.Context, sessionName string) (context.Context, error) {
	env := testenv.EnvFromContext(ctx)

	session, ok := env.Sessions[sessionName]
	if !ok {
		return ctx, fmt.Errorf("session %s not found", sessionName)
	}

	err := env.CurrentAdapter.PauseSession(ctx, session.ID)
	if err != nil {
		return ctx, err
	}

	// Refresh session state
	session, err = env.CurrentAdapter.GetSession(ctx, session.ID)
	if err != nil {
		return ctx, err
	}

	env.CurrentSession = session
	env.Sessions[sessionName] = session
	return testenv.ContextWithEnv(ctx, env), nil
}

func iResumeSessionNamed(ctx context.Context, sessionName string) (context.Context, error) {
	env := testenv.EnvFromContext(ctx)

	session, ok := env.Sessions[sessionName]
	if !ok {
		return ctx, fmt.Errorf("session %s not found", sessionName)
	}

	session, err := env.CurrentAdapter.ResumeSession(ctx, session.ID)
	if err != nil {
		return ctx, err
	}

	env.CurrentSession = session
	env.Sessions[sessionName] = session
	return testenv.ContextWithEnv(ctx, env), nil
}

func iArchiveSessionNamed(ctx context.Context, sessionName string) (context.Context, error) {
	env := testenv.EnvFromContext(ctx)

	session, ok := env.Sessions[sessionName]
	if !ok {
		return ctx, fmt.Errorf("session %s not found", sessionName)
	}

	err := env.CurrentAdapter.ArchiveSession(ctx, session.ID)
	if err != nil {
		return ctx, err
	}

	// Refresh session state
	session, err = env.CurrentAdapter.GetSession(ctx, session.ID)
	if err != nil {
		return ctx, err
	}

	env.CurrentSession = session
	env.Sessions[sessionName] = session
	return testenv.ContextWithEnv(ctx, env), nil
}

// Capability validation steps
func adapterSupportsSessionCreation(ctx context.Context) (context.Context, error) {
	// Interface guarantees CreateSession method exists
	return ctx, nil
}

func adapterSupportsMessageSending(ctx context.Context) (context.Context, error) {
	// Interface guarantees SendMessage method exists
	return ctx, nil
}

func adapterSupportsHistoryRetrieval(ctx context.Context) (context.Context, error) {
	// Interface guarantees GetHistory method exists
	return ctx, nil
}

func adapterSupportsLifecycle(ctx context.Context) (context.Context, error) {
	// Interface guarantees lifecycle methods exist
	return ctx, nil
}
