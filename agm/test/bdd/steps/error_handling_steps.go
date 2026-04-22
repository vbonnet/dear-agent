package steps

import (
	"context"
	"fmt"
	"strings"

	"github.com/cucumber/godog"
	"github.com/vbonnet/dear-agent/agm/test/bdd/internal/testenv"
)

func RegisterErrorHandlingSteps(ctx *godog.ScenarioContext) {
	ctx.Step(`^an error should occur$`, anErrorShouldOccur)
	ctx.Step(`^the error message should contain "([^"]*)"$`, errorMessageShouldContain)
	ctx.Step(`^I try to resume session "([^"]*)"$`, iTryToResumeSession)
	ctx.Step(`^I request adapter "([^"]*)" from environment$`, iRequestAdapterFromEnvironment)
	ctx.Step(`^the adapter name should be "([^"]*)"$`, adapterNameShouldBe)
	ctx.Step(`^session "([^"]*)" history should contain "([^"]*)"$`, sessionHistoryShouldContain)
	ctx.Step(`^session "([^"]*)" history should not contain "([^"]*)"$`, sessionHistoryShouldNotContain)
}

func anErrorShouldOccur(ctx context.Context) (context.Context, error) {
	env := testenv.EnvFromContext(ctx)
	if env.LastError == nil {
		return ctx, fmt.Errorf("expected an error but got nil")
	}
	return ctx, nil
}

func errorMessageShouldContain(ctx context.Context, expectedSubstring string) (context.Context, error) {
	env := testenv.EnvFromContext(ctx)
	if env.LastError == nil {
		return ctx, fmt.Errorf("expected error containing '%s', but got nil", expectedSubstring)
	}

	errMsg := env.LastError.Error()
	if !strings.Contains(errMsg, expectedSubstring) {
		return ctx, fmt.Errorf("expected error containing '%s', got '%s'", expectedSubstring, errMsg)
	}
	return ctx, nil
}

func iTryToResumeSession(ctx context.Context, sessionName string) (context.Context, error) {
	env := testenv.EnvFromContext(ctx)

	session, ok := env.Sessions[sessionName]
	if !ok {
		env.LastError = fmt.Errorf("session %s not found in test environment", sessionName)
		return testenv.ContextWithEnv(ctx, env), nil
	}

	_, err := env.CurrentAdapter.ResumeSession(ctx, session.ID)
	env.LastError = err
	return testenv.ContextWithEnv(ctx, env), nil
}

func iRequestAdapterFromEnvironment(ctx context.Context, adapterName string) (context.Context, error) {
	env := testenv.EnvFromContext(ctx)

	adapter, err := env.GetAdapter(adapterName)
	env.LastError = err
	if err == nil {
		env.CurrentAdapter = adapter
	}
	return testenv.ContextWithEnv(ctx, env), nil
}

func adapterNameShouldBe(ctx context.Context, expectedName string) (context.Context, error) {
	env := testenv.EnvFromContext(ctx)
	if env.CurrentAdapter == nil {
		return ctx, fmt.Errorf("no current adapter set")
	}

	actualName := env.CurrentAdapter.Name()
	if actualName != expectedName {
		return ctx, fmt.Errorf("expected adapter name '%s', got '%s'", expectedName, actualName)
	}
	return ctx, nil
}

func sessionHistoryShouldContain(ctx context.Context, sessionName, expectedMessage string) (context.Context, error) {
	env := testenv.EnvFromContext(ctx)

	session, ok := env.Sessions[sessionName]
	if !ok {
		return ctx, fmt.Errorf("session %s not found", sessionName)
	}

	history, err := env.CurrentAdapter.GetHistory(ctx, session.ID)
	if err != nil {
		return ctx, err
	}

	for _, msg := range history {
		if strings.Contains(msg.Content, expectedMessage) {
			return ctx, nil
		}
	}

	return ctx, fmt.Errorf("session %s history does not contain '%s'", sessionName, expectedMessage)
}

func sessionHistoryShouldNotContain(ctx context.Context, sessionName, unexpectedMessage string) (context.Context, error) {
	env := testenv.EnvFromContext(ctx)

	session, ok := env.Sessions[sessionName]
	if !ok {
		return ctx, fmt.Errorf("session %s not found", sessionName)
	}

	history, err := env.CurrentAdapter.GetHistory(ctx, session.ID)
	if err != nil {
		return ctx, err
	}

	for _, msg := range history {
		if strings.Contains(msg.Content, unexpectedMessage) {
			return ctx, fmt.Errorf("session %s history should not contain '%s' but does", sessionName, unexpectedMessage)
		}
	}

	return ctx, nil
}
