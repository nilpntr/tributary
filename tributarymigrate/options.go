package tributarymigrate

import (
	"time"

	"github.com/nilpntr/tributary/tributarytype"
)

// Options contains configuration for database migrations.
type Options struct {
	// SchemaName is the name of the schema to use for migration tracking.
	// Defaults to "public" if not specified.
	SchemaName string

	// MigrationsTable is the name of the table used to track migration state.
	// Defaults to "schema_migrations" if not specified.
	MigrationsTable string

	// Timeout is the maximum time to wait for migrations to complete.
	// Defaults to 5 minutes if not specified.
	Timeout time.Duration

	// LockTimeout is the maximum time to wait for the migration lock.
	// Defaults to 15 minutes if not specified.
	LockTimeout time.Duration

	// DryRun enables dry-run mode where migrations are validated but not applied.
	// Defaults to false.
	DryRun bool

	// Verbose enables verbose logging during migrations.
	// Defaults to false.
	Verbose bool

	// StatementTimeout sets the timeout for individual SQL statements.
	// Defaults to 1 hour if not specified.
	StatementTimeout time.Duration
}

// DefaultOptions returns a default migration configuration.
func DefaultOptions() *Options {
	return &Options{
		SchemaName:       "public",
		MigrationsTable:  "schema_migrations",
		Timeout:          5 * time.Minute,
		LockTimeout:      15 * time.Minute,
		DryRun:           false,
		Verbose:          false,
		StatementTimeout: 1 * time.Hour,
	}
}

// Validate checks if the migration options are valid.
func (o *Options) Validate() error {
	if o.SchemaName == "" {
		return &tributarytype.ValidationError{
			Field:   "SchemaName",
			Message: "cannot be empty",
		}
	}

	if o.MigrationsTable == "" {
		return &tributarytype.ValidationError{
			Field:   "MigrationsTable",
			Message: "cannot be empty",
		}
	}

	if o.Timeout <= 0 {
		return &tributarytype.ValidationError{
			Field:   "Timeout",
			Message: "must be greater than 0",
		}
	}

	if o.LockTimeout <= 0 {
		return &tributarytype.ValidationError{
			Field:   "LockTimeout",
			Message: "must be greater than 0",
		}
	}

	if o.StatementTimeout <= 0 {
		return &tributarytype.ValidationError{
			Field:   "StatementTimeout",
			Message: "must be greater than 0",
		}
	}

	return nil
}

// SetDefaults sets default values for unspecified options.
func (o *Options) SetDefaults() {
	if o.SchemaName == "" {
		o.SchemaName = "public"
	}

	if o.MigrationsTable == "" {
		o.MigrationsTable = "schema_migrations"
	}

	if o.Timeout == 0 {
		o.Timeout = 5 * time.Minute
	}

	if o.LockTimeout == 0 {
		o.LockTimeout = 15 * time.Minute
	}

	if o.StatementTimeout == 0 {
		o.StatementTimeout = 1 * time.Hour
	}
}

// Clone creates a copy of the options.
func (o *Options) Clone() *Options {
	return &Options{
		SchemaName:       o.SchemaName,
		MigrationsTable:  o.MigrationsTable,
		Timeout:          o.Timeout,
		LockTimeout:      o.LockTimeout,
		DryRun:           o.DryRun,
		Verbose:          o.Verbose,
		StatementTimeout: o.StatementTimeout,
	}
}

// MergeFrom merges options from another Options instance.
// Non-zero values from the other instance will override values in this instance.
func (o *Options) MergeFrom(other *Options) {
	if other == nil {
		return
	}

	if other.SchemaName != "" {
		o.SchemaName = other.SchemaName
	}

	if other.MigrationsTable != "" {
		o.MigrationsTable = other.MigrationsTable
	}

	if other.Timeout != 0 {
		o.Timeout = other.Timeout
	}

	if other.LockTimeout != 0 {
		o.LockTimeout = other.LockTimeout
	}

	if other.StatementTimeout != 0 {
		o.StatementTimeout = other.StatementTimeout
	}

	// For booleans, we accept the other value regardless
	o.DryRun = other.DryRun
	o.Verbose = other.Verbose
}
