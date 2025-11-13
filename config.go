package tributary

import (
	"time"

	"github.com/nilpntr/tributary/tributarylog"
	"github.com/nilpntr/tributary/tributarytype"
)

// Re-export types from tributarytype for convenience
type QueueConfig = tributarytype.QueueConfig
type Logger = tributarytype.Logger
type Field = tributarytype.Field
type MigrationOptions = tributarytype.MigrationOptions

// Config contains configuration for a Tributary client.
type Config struct {
	// Queues is a map of queue names to their configurations.
	// Each queue can have different worker pool sizes and other settings.
	Queues map[string]QueueConfig

	// Workers is the registry of step workers.
	Workers *Workers

	// Hooks is a list of hooks to run during step lifecycle.
	// Hooks are executed in the order they appear in this slice.
	Hooks []Hook

	// Logger is the logger instance to use for structured logging.
	// If nil, a no-op logger will be used.
	Logger Logger `json:"-"` // Interface, not serializable

	// LogLevel is the minimum log level to output.
	// Valid values: "debug", "info", "warn", "error"
	// Defaults to "info" if not specified.
	LogLevel string

	// StructuredLogging enables structured logging with fields.
	// When false, logs are output in a simple text format.
	// Defaults to true.
	StructuredLogging bool

	// FetchCooldown is the minimum amount of time to wait before fetching new steps
	// after the last fetch attempt returned no steps.
	// Defaults to 100ms if not specified.
	FetchCooldown time.Duration

	// FetchPollInterval is the interval at which to poll for new steps
	// when not using LISTEN/NOTIFY.
	// Defaults to 1s if not specified.
	FetchPollInterval time.Duration

	// MaxAttempts is the default maximum number of attempts for a step.
	// Can be overridden per-step via InsertOpts.
	// Defaults to 25 if not specified.
	MaxAttempts int

	// RetryBackoffBase is the base duration for exponential backoff between retries.
	// The actual delay is calculated as: RetryBackoffBase * 2^(attempt-1)
	// Defaults to 1s if not specified.
	RetryBackoffBase time.Duration

	// AutoMigrate enables automatic database migration on client start.
	// When true, migrations will be applied automatically.
	// Defaults to false for safety.
	AutoMigrate bool

	// MigrationOptions contains configuration for database migrations.
	// Only used when AutoMigrate is true.
	MigrationOptions *MigrationOptions

	// MetricsEnabled enables metrics collection.
	// Defaults to false.
	MetricsEnabled bool

	// TracingEnabled enables distributed tracing.
	// Defaults to false.
	TracingEnabled bool

	// PanicRecovery enables panic recovery in worker execution.
	// When true, panics are caught and converted to errors.
	// Defaults to true for safety.
	PanicRecovery bool
}

// SetDefaults sets default values for unspecified config fields.
func (c *Config) SetDefaults() {
	if c.Logger == nil {
		c.Logger = tributarylog.DefaultLogger
	}

	if c.FetchCooldown == 0 {
		c.FetchCooldown = 100 * time.Millisecond
	}

	if c.FetchPollInterval == 0 {
		c.FetchPollInterval = 1 * time.Second
	}

	if c.MaxAttempts == 0 {
		c.MaxAttempts = 25
	}

	if c.RetryBackoffBase == 0 {
		c.RetryBackoffBase = 1 * time.Second
	}

	if c.LogLevel == "" {
		c.LogLevel = "info"
	}

	// Set defaults for queue configs
	for name, queueConfig := range c.Queues {
		if queueConfig.NumWorkers == 0 {
			queueConfig.NumWorkers = queueConfig.MaxWorkers
		}
		c.Queues[name] = queueConfig
	}

	// Set defaults for migration options if AutoMigrate is enabled
	if c.AutoMigrate && c.MigrationOptions != nil {
		if c.MigrationOptions.SchemaName == "" {
			c.MigrationOptions.SchemaName = "public"
		}
		if c.MigrationOptions.Timeout == 0 {
			c.MigrationOptions.Timeout = 5 * time.Minute
		}
		if c.MigrationOptions.LockTimeout == 0 {
			c.MigrationOptions.LockTimeout = 15 * time.Minute
		}
	}
}

// Validate checks if the configuration is valid.
func (c *Config) Validate() error {
	if c.Workers == nil {
		return ErrNoWorkersConfigured
	}

	if len(c.Queues) == 0 {
		return ErrNoQueuesConfigured
	}

	for name, queueConfig := range c.Queues {
		if queueConfig.MaxWorkers <= 0 {
			return &QueueConfigError{Queue: name, Message: "MaxWorkers must be > 0"}
		}
	}

	// Validate log level
	switch c.LogLevel {
	case "debug", "info", "warn", "error":
		// Valid log levels
	default:
		return &ValidationError{
			Field:   "LogLevel",
			Message: "must be one of: debug, info, warn, error",
		}
	}

	// Validate migration options if AutoMigrate is enabled
	if c.AutoMigrate && c.MigrationOptions != nil {
		if c.MigrationOptions.Timeout <= 0 {
			return &ValidationError{
				Field:   "MigrationOptions.Timeout",
				Message: "must be greater than 0",
			}
		}
		if c.MigrationOptions.LockTimeout <= 0 {
			return &ValidationError{
				Field:   "MigrationOptions.LockTimeout",
				Message: "must be greater than 0",
			}
		}
	}

	return nil
}

// GetEffectivePanicRecovery returns the effective panic recovery setting.
// Returns true by default for safety.
func (c *Config) GetEffectivePanicRecovery() bool {
	// Since we want panic recovery enabled by default, but bool zero value is false,
	// we need to determine if it was explicitly set to false.
	// For now, we'll assume true is always the desired default.
	return true // TODO: Could be made configurable with a pointer bool
}

// GetEffectiveStructuredLogging returns the effective structured logging setting.
// Returns true by default.
func (c *Config) GetEffectiveStructuredLogging() bool {
	// Similar to panic recovery, structured logging should default to true
	return true // TODO: Could be made configurable with a pointer bool
}
