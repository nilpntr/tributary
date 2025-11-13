package tributary

import (
	"github.com/nilpntr/tributary/tributarytype"
)

// Re-export error types and variables from tributarytype for backward compatibility
var (
	ErrNoWorkersConfigured    = tributarytype.ErrNoWorkersConfigured
	ErrNoQueuesConfigured     = tributarytype.ErrNoQueuesConfigured
	ErrClientNotStarted       = tributarytype.ErrClientNotStarted
	ErrClientAlreadyStarted   = tributarytype.ErrClientAlreadyStarted
	ErrWorkerNotFound         = tributarytype.ErrWorkerNotFound
	ErrInvalidLogger          = tributarytype.ErrInvalidLogger
	ErrMigrationFailed        = tributarytype.ErrMigrationFailed
	ErrInvalidMigrationConfig = tributarytype.ErrInvalidMigrationConfig
)

// Re-export error types
type QueueConfigError = tributarytype.QueueConfigError
type WorkerError = tributarytype.WorkerError
type WorkerPanicError = tributarytype.WorkerPanicError
type MigrationError = tributarytype.MigrationError
type ValidationError = tributarytype.ValidationError
type HookError = tributarytype.HookError
