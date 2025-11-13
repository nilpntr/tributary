package tributarytype

import (
	"fmt"
	"time"
)

// StepState represents the current state of a step.
type StepState string

const (
	// StepStateAvailable means the step is ready to be executed.
	StepStateAvailable StepState = "available"

	// StepStateRunning means the step is currently being executed.
	StepStateRunning StepState = "running"

	// StepStateCompleted means the step completed successfully.
	StepStateCompleted StepState = "completed"

	// StepStateCancelled means the step was cancelled.
	StepStateCancelled StepState = "cancelled"

	// StepStateDiscarded means the step failed and will not be retried.
	StepStateDiscarded StepState = "discarded"
)

// WorkflowExecutionState represents the current state of a workflow execution.
type WorkflowExecutionState string

const (
	// WorkflowExecutionStateRunning means the workflow is currently executing.
	WorkflowExecutionStateRunning WorkflowExecutionState = "running"

	// WorkflowExecutionStateCompleted means the workflow completed successfully.
	WorkflowExecutionStateCompleted WorkflowExecutionState = "completed"

	// WorkflowExecutionStateFailed means the workflow failed.
	WorkflowExecutionStateFailed WorkflowExecutionState = "failed"

	// WorkflowExecutionStateCancelled means the workflow was cancelled.
	WorkflowExecutionStateCancelled WorkflowExecutionState = "cancelled"
)

// Field represents a structured logging field.
type Field struct {
	Key   string
	Value interface{}
}

// Logger represents the logging interface that can be implemented by various logging libraries.
type Logger interface {
	Debug(msg string, fields ...Field)
	Info(msg string, fields ...Field)
	Warn(msg string, fields ...Field)
	Error(msg string, fields ...Field)
	With(fields ...Field) Logger
	WithContext(ctx interface{}) Logger
}

// QueueConfig contains configuration for a specific queue.
type QueueConfig struct {
	// MaxWorkers is the maximum number of concurrent workers for this queue.
	MaxWorkers int

	// NumWorkers is the number of worker goroutines to start for this queue.
	// Defaults to MaxWorkers if not specified.
	NumWorkers int
}

// MigrationOptions contains configuration for database migrations.
type MigrationOptions struct {
	// DatabaseURL is the connection string for the database.
	// If empty, the migration system will use the same connection as the client.
	DatabaseURL string

	// SchemaName is the name of the schema to use for migration tracking.
	// Defaults to "public" if not specified.
	SchemaName string

	// Timeout is the maximum time to wait for migrations to complete.
	// Defaults to 5 minutes if not specified.
	Timeout time.Duration

	// DryRun enables dry-run mode where migrations are validated but not applied.
	// Defaults to false.
	DryRun bool

	// LockTimeout is the maximum time to wait for the migration lock.
	// Defaults to 15 minutes if not specified.
	LockTimeout time.Duration

	// MigrationsPath is the path to the migrations directory.
	// If empty, embedded migrations will be used.
	MigrationsPath string
}

// StepArgs is the interface that all step argument types must implement.
// This definition is duplicated from the main tributary package to avoid circular imports.
type StepArgs interface {
	// Kind returns a unique string that identifies the type of step.
	Kind() string
}

// Error types for various packages

// HookError represents an error that occurred during hook execution.
type HookError struct {
	HookName string
	Phase    string
	StepID   int64
	Err      error
}

func (e *HookError) Error() string {
	if e.StepID > 0 {
		return fmt.Sprintf("hook %s failed in phase %s for step %d: %v", e.HookName, e.Phase, e.StepID, e.Err)
	}
	return fmt.Sprintf("hook %s failed in phase %s: %v", e.HookName, e.Phase, e.Err)
}

func (e *HookError) Unwrap() error {
	return e.Err
}

// MigrationError represents an error that occurred during database migration.
type MigrationError struct {
	Operation string
	Version   string
	Err       error
}

func (e *MigrationError) Error() string {
	if e.Version != "" {
		return fmt.Sprintf("migration %s failed for version %s: %v", e.Operation, e.Version, e.Err)
	}
	return fmt.Sprintf("migration %s failed: %v", e.Operation, e.Err)
}

func (e *MigrationError) Unwrap() error {
	return e.Err
}

// ValidationError represents a validation error.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("validation error for field %s: %s", e.Field, e.Message)
}

// QueueConfigError represents an error in queue configuration.
type QueueConfigError struct {
	Queue   string
	Message string
}

func (e *QueueConfigError) Error() string {
	return fmt.Sprintf("queue config error for %s: %s", e.Queue, e.Message)
}

// WorkerError represents an error during worker execution.
type WorkerError struct {
	WorkerKind string
	StepID     int64
	Err        error
}

func (e *WorkerError) Error() string {
	return fmt.Sprintf("worker %s failed for step %d: %v", e.WorkerKind, e.StepID, e.Err)
}

func (e *WorkerError) Unwrap() error {
	return e.Err
}

// WorkerPanicError represents a panic that occurred in a worker.
type WorkerPanicError struct {
	WorkerKind string
	StepID     int64
	PanicValue interface{}
	StackTrace string
}

func (e *WorkerPanicError) Error() string {
	return fmt.Sprintf("worker %s panicked for step %d: %v", e.WorkerKind, e.StepID, e.PanicValue)
}

// Error variables
var (
	ErrNoWorkersConfigured    = fmt.Errorf("no workers configured")
	ErrNoQueuesConfigured     = fmt.Errorf("no queues configured")
	ErrClientNotStarted       = fmt.Errorf("client not started")
	ErrClientAlreadyStarted   = fmt.Errorf("client already started")
	ErrWorkerNotFound         = fmt.Errorf("worker not found")
	ErrInvalidLogger          = fmt.Errorf("invalid logger")
	ErrMigrationFailed        = fmt.Errorf("migration failed")
	ErrInvalidMigrationConfig = fmt.Errorf("invalid migration config")
)
