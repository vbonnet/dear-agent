// Package testenv provides testenv functionality.
package testenv

import "context"

type contextKey string

const (
	// EnvKey is the context key for the test environment
	EnvKey contextKey = "environment"
)

// ContextWithEnv returns a new context with the environment attached
func ContextWithEnv(ctx context.Context, env *Environment) context.Context {
	return context.WithValue(ctx, EnvKey, env)
}

// EnvFromContext retrieves the environment from the context
func EnvFromContext(ctx context.Context) *Environment {
	return ctx.Value(EnvKey).(*Environment)
}
