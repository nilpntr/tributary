package tributarylog

import (
	"context"

	"github.com/nilpntr/tributary/tributarytype"
)

// Logger defines the interface for structured logging used throughout Tributary.
// This interface allows for different logging implementations while maintaining
// consistent logging behavior across the system.
type Logger interface {
	// Debug logs a debug-level message with optional fields.
	Debug(msg string, fields ...tributarytype.Field)

	// Info logs an info-level message with optional fields.
	Info(msg string, fields ...tributarytype.Field)

	// Warn logs a warning-level message with optional fields.
	Warn(msg string, fields ...tributarytype.Field)

	// Error logs an error-level message with optional fields.
	Error(msg string, fields ...tributarytype.Field)

	// With returns a logger with the specified fields pre-populated.
	// Subsequent log calls will include these fields automatically.
	With(fields ...tributarytype.Field) Logger

	// WithContext returns a logger with context information.
	// This enables correlation IDs and other context-aware logging features.
	WithContext(ctx context.Context) Logger
}

// LogLevel represents the logging level.
type LogLevel int

const (
	// LevelDebug enables debug and higher level logs.
	LevelDebug LogLevel = iota

	// LevelInfo enables info and higher level logs.
	LevelInfo

	// LevelWarn enables warning and higher level logs.
	LevelWarn

	// LevelError enables only error level logs.
	LevelError

	// LevelOff disables all logging.
	LevelOff
)

// String returns the string representation of the log level.
func (l LogLevel) String() string {
	switch l {
	case LevelDebug:
		return "debug"
	case LevelInfo:
		return "info"
	case LevelWarn:
		return "warn"
	case LevelError:
		return "error"
	case LevelOff:
		return "off"
	default:
		return "unknown"
	}
}

// ParseLogLevel parses a string into a LogLevel.
func ParseLogLevel(s string) LogLevel {
	switch s {
	case "debug":
		return LevelDebug
	case "info":
		return LevelInfo
	case "warn", "warning":
		return LevelWarn
	case "error":
		return LevelError
	case "off":
		return LevelOff
	default:
		return LevelInfo // Default to info level
	}
}

// LoggerConfig contains configuration for loggers.
type LoggerConfig struct {
	// Level is the minimum log level to output.
	Level LogLevel

	// Structured enables structured logging with fields.
	Structured bool

	// IncludeTimestamp includes timestamps in log output.
	IncludeTimestamp bool

	// IncludeCaller includes caller information in log output.
	IncludeCaller bool

	// TimeFormat is the format to use for timestamps.
	// If empty, RFC3339 format is used.
	TimeFormat string

	// Output specifies where to write logs. If nil, os.Stderr is used.
	Output interface{} // Should be io.Writer but avoiding import for simplicity
}

// DefaultConfig returns a default logger configuration.
func DefaultConfig() *LoggerConfig {
	return &LoggerConfig{
		Level:            LevelInfo,
		Structured:       true,
		IncludeTimestamp: true,
		IncludeCaller:    false,
		TimeFormat:       "", // Will use RFC3339
	}
}
