package tributarylog

import (
	"context"
	"crypto/rand"
	"encoding/hex"

	"github.com/nilpntr/tributary/tributarytype"
)

// ContextKey is a type for context keys to avoid conflicts.
type ContextKey string

const (
	// CorrelationIDKey is the context key for correlation IDs.
	CorrelationIDKey ContextKey = "correlation_id"

	// RequestIDKey is the context key for request IDs.
	RequestIDKey ContextKey = "request_id"

	// WorkflowExecutionIDKey is the context key for workflow execution IDs.
	WorkflowExecutionIDKey ContextKey = "workflow_execution_id"

	// StepIDKey is the context key for step IDs.
	StepIDKey ContextKey = "step_id"

	// UserIDKey is the context key for user IDs.
	UserIDKey ContextKey = "user_id"

	// LoggerKey is the context key for storing a logger instance.
	LoggerKey ContextKey = "logger"
)

// WithCorrelationID adds a correlation ID to the context.
// If no correlation ID is provided, one will be generated.
func WithCorrelationID(ctx context.Context, correlationID string) context.Context {
	if correlationID == "" {
		correlationID = GenerateCorrelationID()
	}
	return context.WithValue(ctx, CorrelationIDKey, correlationID)
}

// WithRequestID adds a request ID to the context.
func WithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, RequestIDKey, requestID)
}

// WithWorkflowExecutionID adds a workflow execution ID to the context.
func WithWorkflowExecutionID(ctx context.Context, workflowExecutionID int64) context.Context {
	return context.WithValue(ctx, WorkflowExecutionIDKey, workflowExecutionID)
}

// WithStepID adds a step ID to the context.
func WithStepID(ctx context.Context, stepID int64) context.Context {
	return context.WithValue(ctx, StepIDKey, stepID)
}

// WithUserID adds a user ID to the context.
func WithUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, UserIDKey, userID)
}

// WithLogger adds a logger instance to the context.
func WithLogger(ctx context.Context, logger tributarytype.Logger) context.Context {
	return context.WithValue(ctx, LoggerKey, logger)
}

// GetCorrelationID retrieves the correlation ID from the context.
func GetCorrelationID(ctx context.Context) string {
	if id := ctx.Value(CorrelationIDKey); id != nil {
		if correlationID, ok := id.(string); ok {
			return correlationID
		}
	}
	return ""
}

// GetRequestID retrieves the request ID from the context.
func GetRequestID(ctx context.Context) string {
	if id := ctx.Value(RequestIDKey); id != nil {
		if requestID, ok := id.(string); ok {
			return requestID
		}
	}
	return ""
}

// GetWorkflowExecutionID retrieves the workflow execution ID from the context.
func GetWorkflowExecutionID(ctx context.Context) int64 {
	if id := ctx.Value(WorkflowExecutionIDKey); id != nil {
		if workflowExecutionID, ok := id.(int64); ok {
			return workflowExecutionID
		}
	}
	return 0
}

// GetStepID retrieves the step ID from the context.
func GetStepID(ctx context.Context) int64 {
	if id := ctx.Value(StepIDKey); id != nil {
		if stepID, ok := id.(int64); ok {
			return stepID
		}
	}
	return 0
}

// GetUserID retrieves the user ID from the context.
func GetUserID(ctx context.Context) string {
	if id := ctx.Value(UserIDKey); id != nil {
		if userID, ok := id.(string); ok {
			return userID
		}
	}
	return ""
}

// GetLogger retrieves the logger from the context.
// If no logger is found, returns the default logger.
func GetLogger(ctx context.Context) tributarytype.Logger {
	if logger := ctx.Value(LoggerKey); logger != nil {
		if l, ok := logger.(tributarytype.Logger); ok {
			return l
		}
	}
	return DefaultLogger
}

// LogFromContext logs a message using the logger from the context.
// If no logger is found in the context, uses the default logger.
func LogFromContext(ctx context.Context, level LogLevel, msg string, fields ...tributarytype.Field) {
	logger := GetLogger(ctx).WithContext(ctx)

	switch level {
	case LevelDebug:
		logger.Debug(msg, fields...)
	case LevelInfo:
		logger.Info(msg, fields...)
	case LevelWarn:
		logger.Warn(msg, fields...)
	case LevelError:
		logger.Error(msg, fields...)
	}
}

// DebugContext logs a debug message using the context logger.
func DebugContext(ctx context.Context, msg string, fields ...tributarytype.Field) {
	LogFromContext(ctx, LevelDebug, msg, fields...)
}

// InfoContext logs an info message using the context logger.
func InfoContext(ctx context.Context, msg string, fields ...tributarytype.Field) {
	LogFromContext(ctx, LevelInfo, msg, fields...)
}

// WarnContext logs a warning message using the context logger.
func WarnContext(ctx context.Context, msg string, fields ...tributarytype.Field) {
	LogFromContext(ctx, LevelWarn, msg, fields...)
}

// ErrorContext logs an error message using the context logger.
func ErrorContext(ctx context.Context, msg string, fields ...tributarytype.Field) {
	LogFromContext(ctx, LevelError, msg, fields...)
}

// GenerateCorrelationID generates a random correlation ID.
func GenerateCorrelationID() string {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		// Fallback to a simple ID if random generation fails
		return "correlation-id-fallback"
	}
	return hex.EncodeToString(bytes)
}

// ContextualLogger wraps a logger with automatic context field injection.
type ContextualLogger struct {
	logger tributarytype.Logger
	ctx    context.Context
}

// NewContextualLogger creates a new contextual logger.
func NewContextualLogger(logger tributarytype.Logger, ctx context.Context) *ContextualLogger {
	return &ContextualLogger{
		logger: logger,
		ctx:    ctx,
	}
}

// Debug logs a debug message with context fields automatically included.
func (cl *ContextualLogger) Debug(msg string, fields ...tributarytype.Field) {
	cl.logger.WithContext(cl.ctx).Debug(msg, fields...)
}

// Info logs an info message with context fields automatically included.
func (cl *ContextualLogger) Info(msg string, fields ...tributarytype.Field) {
	cl.logger.WithContext(cl.ctx).Info(msg, fields...)
}

// Warn logs a warning message with context fields automatically included.
func (cl *ContextualLogger) Warn(msg string, fields ...tributarytype.Field) {
	cl.logger.WithContext(cl.ctx).Warn(msg, fields...)
}

// Error logs an error message with context fields automatically included.
func (cl *ContextualLogger) Error(msg string, fields ...tributarytype.Field) {
	cl.logger.WithContext(cl.ctx).Error(msg, fields...)
}

// With returns a new logger with additional fields.
func (cl *ContextualLogger) With(fields ...tributarytype.Field) tributarytype.Logger {
	return &ContextualLogger{
		logger: cl.logger.With(fields...),
		ctx:    cl.ctx,
	}
}

// WithContext returns a new logger with updated context.
func (cl *ContextualLogger) WithContext(ctx interface{}) tributarytype.Logger {
	if c, ok := ctx.(context.Context); ok {
		return &ContextualLogger{
			logger: cl.logger,
			ctx:    c,
		}
	}
	return cl
}

// UpdateContext updates the context for the contextual logger.
func (cl *ContextualLogger) UpdateContext(ctx context.Context) {
	cl.ctx = ctx
}
