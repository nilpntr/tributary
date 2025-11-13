package tributaryhook

import (
	"context"

	"github.com/nilpntr/tributary/tributarytype"
)

// Hook provides lifecycle callbacks for step execution.
// Hooks can be used for cross-cutting concerns like encryption, logging, metrics, etc.
type Hook interface {
	// BeforeInsert is called before a step is inserted into the database.
	// This can be used to modify or validate step arguments before insertion.
	// Return an error to prevent the insertion.
	BeforeInsert(ctx context.Context, args tributarytype.StepArgs, argsBytes []byte) ([]byte, error)

	// AfterInsert is called after a step has been inserted into the database.
	AfterInsert(ctx context.Context, stepID int64, args tributarytype.StepArgs)

	// BeforeWork is called before a step is executed.
	// This can be used to decrypt arguments or perform setup.
	// Return an error to skip the execution.
	BeforeWork(ctx context.Context, stepID int64, kind string, argsBytes []byte) ([]byte, error)

	// AfterWork is called after a step has been executed.
	// This can be used to encrypt results or perform cleanup.
	AfterWork(ctx context.Context, stepID int64, result []byte, err error) ([]byte, error)
}

// BaseHook provides a default implementation of Hook that does nothing.
// Embed this in your hook implementation and override only the methods you need.
type BaseHook struct{}

func (BaseHook) BeforeInsert(ctx context.Context, args tributarytype.StepArgs, argsBytes []byte) ([]byte, error) {
	return argsBytes, nil
}

func (BaseHook) AfterInsert(ctx context.Context, stepID int64, args tributarytype.StepArgs) {}

func (BaseHook) BeforeWork(ctx context.Context, stepID int64, kind string, argsBytes []byte) ([]byte, error) {
	return argsBytes, nil
}

func (BaseHook) AfterWork(ctx context.Context, stepID int64, result []byte, err error) ([]byte, error) {
	return result, nil
}

// RunBeforeInsertHooks runs all BeforeInsert hooks in sequence.
func RunBeforeInsertHooks(ctx context.Context, hooks []Hook, args tributarytype.StepArgs, argsBytes []byte) ([]byte, error) {
	result := argsBytes
	for _, hook := range hooks {
		var err error
		result, err = hook.BeforeInsert(ctx, args, result)
		if err != nil {
			return nil, &tributarytype.HookError{
				HookName: getHookName(hook),
				Phase:    "BeforeInsert",
				Err:      err,
			}
		}
	}
	return result, nil
}

// RunAfterInsertHooks runs all AfterInsert hooks in sequence.
func RunAfterInsertHooks(ctx context.Context, hooks []Hook, stepID int64, args tributarytype.StepArgs) {
	for _, hook := range hooks {
		hook.AfterInsert(ctx, stepID, args)
	}
}

// RunBeforeWorkHooks runs all BeforeWork hooks in sequence.
func RunBeforeWorkHooks(ctx context.Context, hooks []Hook, stepID int64, kind string, argsBytes []byte) ([]byte, error) {
	result := argsBytes
	for _, hook := range hooks {
		var err error
		result, err = hook.BeforeWork(ctx, stepID, kind, result)
		if err != nil {
			return nil, &tributarytype.HookError{
				HookName: getHookName(hook),
				Phase:    "BeforeWork",
				StepID:   stepID,
				Err:      err,
			}
		}
	}
	return result, nil
}

// RunAfterWorkHooks runs all AfterWork hooks in sequence.
func RunAfterWorkHooks(ctx context.Context, hooks []Hook, stepID int64, result []byte, err error) ([]byte, error) {
	currentResult := result
	for _, hook := range hooks {
		var hookErr error
		currentResult, hookErr = hook.AfterWork(ctx, stepID, currentResult, err)
		if hookErr != nil {
			return nil, &tributarytype.HookError{
				HookName: getHookName(hook),
				Phase:    "AfterWork",
				StepID:   stepID,
				Err:      hookErr,
			}
		}
	}
	return currentResult, nil
}

// TODO: add a required Kind string on the Hook interface so you don't need this
// getHookName attempts to get a readable name for the hook.
// This uses reflection to get the type name.
func getHookName(hook Hook) string {
	// Simple implementation - could be enhanced with reflection
	switch hook.(type) {
	case *EncryptHook:
		return "EncryptHook"
	case *LoggingHook:
		return "LoggingHook"
	case *MetricsHook:
		return "MetricsHook"
	default:
		return "UnknownHook"
	}
}
