package plugin

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"
)

// LogLevel represents the severity of a log message
type LogLevel int

const (
	// LevelDebug for verbose debugging information
	LevelDebug LogLevel = iota
	// LevelInfo for general informational messages
	LevelInfo
	// LevelWarn for warning messages that don't prevent operation
	LevelWarn
	// LevelError for error messages that prevent specific operations
	LevelError
)

// String returns the string representation of a LogLevel
func (l LogLevel) String() string {
	switch l {
	case LevelDebug:
		return "DEBUG"
	case LevelInfo:
		return "INFO"
	case LevelWarn:
		return "WARN"
	case LevelError:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

// toSlogLevel converts LogLevel to slog.Level
func (l LogLevel) toSlogLevel() slog.Level {
	switch l {
	case LevelDebug:
		return slog.LevelDebug
	case LevelInfo:
		return slog.LevelInfo
	case LevelWarn:
		return slog.LevelWarn
	case LevelError:
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// ErrorContext provides structured context for plugin errors
type ErrorContext struct {
	// Plugin name (if applicable)
	Plugin string
	// Command name (if applicable)
	Command string
	// Search path (for loader errors)
	SearchPath string
	// Plugin path (for specific plugin errors)
	PluginPath string
	// Operation being performed
	Operation string
	// Additional key-value context
	Extra map[string]interface{}
}

// Logger provides structured logging for the plugin system
type Logger struct {
	logger *slog.Logger
	level  LogLevel
}

// NewLogger creates a new plugin logger with the specified minimum level
func NewLogger(level LogLevel) *Logger {
	opts := &slog.HandlerOptions{
		Level: level.toSlogLevel(),
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			// Customize timestamp format
			if a.Key == slog.TimeKey {
				t := a.Value.Time()
				a.Value = slog.StringValue(t.Format(time.RFC3339))
			}
			return a
		},
	}

	handler := slog.NewJSONHandler(os.Stderr, opts)
	return &Logger{
		logger: slog.New(handler),
		level:  level,
	}
}

// NewDefaultLogger creates a logger with INFO level
func NewDefaultLogger() *Logger {
	return NewLogger(LevelInfo)
}

// Debug logs a debug message with context
func (l *Logger) Debug(ctx context.Context, msg string, errCtx ErrorContext) {
	l.log(ctx, LevelDebug, msg, errCtx, nil)
}

// Info logs an informational message with context
func (l *Logger) Info(ctx context.Context, msg string, errCtx ErrorContext) {
	l.log(ctx, LevelInfo, msg, errCtx, nil)
}

// Warn logs a warning message with context
func (l *Logger) Warn(ctx context.Context, msg string, errCtx ErrorContext, err error) {
	l.log(ctx, LevelWarn, msg, errCtx, err)
}

// Error logs an error message with context
func (l *Logger) Error(ctx context.Context, msg string, errCtx ErrorContext, err error) {
	l.log(ctx, LevelError, msg, errCtx, err)
}

// log is the internal logging implementation
func (l *Logger) log(ctx context.Context, level LogLevel, msg string, errCtx ErrorContext, err error) {
	// Build attributes from context
	attrs := []slog.Attr{}

	if errCtx.Plugin != "" {
		attrs = append(attrs, slog.String("plugin", errCtx.Plugin))
	}
	if errCtx.Command != "" {
		attrs = append(attrs, slog.String("command", errCtx.Command))
	}
	if errCtx.SearchPath != "" {
		attrs = append(attrs, slog.String("search_path", errCtx.SearchPath))
	}
	if errCtx.PluginPath != "" {
		attrs = append(attrs, slog.String("plugin_path", errCtx.PluginPath))
	}
	if errCtx.Operation != "" {
		attrs = append(attrs, slog.String("operation", errCtx.Operation))
	}
	if err != nil {
		attrs = append(attrs, slog.String("error", err.Error()))
	}

	// Add extra context
	if len(errCtx.Extra) > 0 {
		extraAttrs := []slog.Attr{}
		for k, v := range errCtx.Extra {
			extraAttrs = append(extraAttrs, slog.Any(k, v))
		}
		attrs = append(attrs, slog.Any("extra", extraAttrs))
	}

	// Log with appropriate level
	switch level {
	case LevelDebug:
		l.logger.LogAttrs(ctx, slog.LevelDebug, msg, attrs...)
	case LevelInfo:
		l.logger.LogAttrs(ctx, slog.LevelInfo, msg, attrs...)
	case LevelWarn:
		l.logger.LogAttrs(ctx, slog.LevelWarn, msg, attrs...)
	case LevelError:
		l.logger.LogAttrs(ctx, slog.LevelError, msg, attrs...)
	}
}

// WithPlugin returns an ErrorContext with the plugin name set
func WithPlugin(name string) ErrorContext {
	return ErrorContext{Plugin: name}
}

// WithCommand returns an ErrorContext with the command name set
func WithCommand(plugin, command string) ErrorContext {
	return ErrorContext{Plugin: plugin, Command: command}
}

// WithOperation returns an ErrorContext with the operation set
func WithOperation(op string) ErrorContext {
	return ErrorContext{Operation: op}
}

// WithPath returns an ErrorContext with the plugin path set
func WithPath(path string) ErrorContext {
	return ErrorContext{PluginPath: path}
}

// WithSearchPath returns an ErrorContext with the search path set
func WithSearchPath(path string) ErrorContext {
	return ErrorContext{SearchPath: path}
}

// WithExtra adds extra key-value context
func (e ErrorContext) WithExtra(key string, value interface{}) ErrorContext {
	if e.Extra == nil {
		e.Extra = make(map[string]interface{})
	}
	e.Extra[key] = value
	return e
}

// WithOperation adds operation to context
func (e ErrorContext) WithOperation(op string) ErrorContext {
	e.Operation = op
	return e
}

// WithPlugin adds plugin name to context
func (e ErrorContext) WithPlugin(name string) ErrorContext {
	e.Plugin = name
	return e
}

// WithCommand adds command to context
func (e ErrorContext) WithCommand(cmd string) ErrorContext {
	e.Command = cmd
	return e
}

// WithPath adds plugin path to context
func (e ErrorContext) WithPath(path string) ErrorContext {
	e.PluginPath = path
	return e
}

// WithSearchPath adds search path to context
func (e ErrorContext) WithSearchPath(path string) ErrorContext {
	e.SearchPath = path
	return e
}

// Merge combines two ErrorContext instances
func (e ErrorContext) Merge(other ErrorContext) ErrorContext {
	merged := e

	if other.Plugin != "" {
		merged.Plugin = other.Plugin
	}
	if other.Command != "" {
		merged.Command = other.Command
	}
	if other.SearchPath != "" {
		merged.SearchPath = other.SearchPath
	}
	if other.PluginPath != "" {
		merged.PluginPath = other.PluginPath
	}
	if other.Operation != "" {
		merged.Operation = other.Operation
	}

	// Merge extra maps
	if len(other.Extra) > 0 {
		if merged.Extra == nil {
			merged.Extra = make(map[string]interface{})
		}
		for k, v := range other.Extra {
			merged.Extra[k] = v
		}
	}

	return merged
}

// FormatError creates a formatted error with context
func FormatError(msg string, errCtx ErrorContext, err error) error {
	if err == nil {
		return fmt.Errorf("%s", msg)
	}

	if errCtx.Plugin != "" {
		msg = fmt.Sprintf("[plugin=%s] %s", errCtx.Plugin, msg)
	}
	if errCtx.Command != "" {
		msg = fmt.Sprintf("[command=%s] %s", errCtx.Command, msg)
	}
	if errCtx.Operation != "" {
		msg = fmt.Sprintf("[operation=%s] %s", errCtx.Operation, msg)
	}

	return fmt.Errorf("%s: %w", msg, err)
}
