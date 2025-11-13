package tributarylog

import (
	"github.com/nilpntr/tributary/tributarytype"
)

// NoopLogger is a logger implementation that does nothing.
// It's useful for testing or when logging is not desired.
type NoopLogger struct{}

// NewNoopLogger creates a new no-operation logger.
func NewNoopLogger() *NoopLogger {
	return &NoopLogger{}
}

// Debug does nothing.
func (l *NoopLogger) Debug(msg string, fields ...tributarytype.Field) {
	// No-op
}

// Info does nothing.
func (l *NoopLogger) Info(msg string, fields ...tributarytype.Field) {
	// No-op
}

// Warn does nothing.
func (l *NoopLogger) Warn(msg string, fields ...tributarytype.Field) {
	// No-op
}

// Error does nothing.
func (l *NoopLogger) Error(msg string, fields ...tributarytype.Field) {
	// No-op
}

// With returns the same no-op logger instance.
func (l *NoopLogger) With(fields ...tributarytype.Field) tributarytype.Logger {
	return l
}

// WithContext returns the same no-op logger instance.
func (l *NoopLogger) WithContext(ctx interface{}) tributarytype.Logger {
	return l
}

// DefaultLogger returns the default logger instance.
// This can be used when no specific logger is configured.
var DefaultLogger tributarytype.Logger = NewNoopLogger()
