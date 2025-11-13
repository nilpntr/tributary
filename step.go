package tributary

import (
	"encoding/json"
	"time"

	"github.com/nilpntr/tributary/tributarytype"
)

// StepArgs is the interface that all step argument types must implement.
// It provides a Kind method that returns a unique identifier for the step type.
type StepArgs interface {
	Kind() string
}

// StepInsertOptsProvider is an optional interface that step arguments can implement
// to provide custom insert options (MaxAttempts, Priority, Queue, Timeout).
type StepInsertOptsProvider interface {
	InsertOpts() StepInsertOpts
}

// ResultType is the interface that all step result types must implement.
// It provides a Kind method that returns a unique identifier for the result type.
// This enables type-safe dependency result access.
type ResultType interface {
	Kind() string
}

// StepInsertOpts contains options for inserting individual steps.
// Used when steps implement StepInsertOptsProvider.
type StepInsertOpts struct {
	// MaxAttempts is the maximum number of times the step will be attempted.
	// Defaults to client config MaxAttempts if not specified.
	MaxAttempts int

	// Priority is the priority of the step. Higher priority steps are executed first.
	// Defaults to 1 if not specified.
	Priority int

	// Queue is the queue to insert the step into.
	// Defaults to "default" if not specified.
	Queue string

	// Timeout is the maximum duration the step is allowed to run.
	// If the step exceeds this duration, it will be cancelled and retried.
	Timeout time.Duration
}

// InsertOpts contains options for inserting a step or workflow.
// This is used in workflow building and step insertion.
type InsertOpts struct {
	// MaxAttempts is the maximum number of times the step will be attempted.
	// Defaults to 25 if not specified.
	MaxAttempts int

	// Priority is the priority of the step. Higher priority steps are executed first.
	// Defaults to 1 if not specified.
	Priority int

	// Queue is the queue to insert the step into.
	// Defaults to "default" if not specified.
	Queue string

	// Timeout is the maximum duration the step is allowed to run.
	// If the step exceeds this duration, it will be cancelled and retried.
	Timeout time.Duration

	// ScheduledAt is the time at which the step should be executed.
	// If not specified, the step will be executed immediately.
	ScheduledAt time.Time
}

// Step represents a step in a workflow with its current state and metadata.
type Step[T StepArgs] struct {
	// ID is the unique identifier for the step.
	ID int64

	// WorkflowExecutionID is the ID of the workflow execution this step belongs to.
	WorkflowExecutionID *int64

	// TaskName is the name of the task within the workflow (e.g., "create_user").
	TaskName *string

	// State is the current state of the step.
	State tributarytype.StepState

	// Attempt is the number of times this step has been attempted.
	Attempt int

	// MaxAttempts is the maximum number of times this step will be attempted.
	MaxAttempts int

	// Kind is the type identifier for this step (from StepArgs.Kind()).
	Kind string

	// Args contains the arguments for this step.
	Args T

	// ArgsRaw contains the raw bytes of the arguments (potentially encrypted).
	ArgsRaw []byte

	// Result contains the output of the step execution (for dependent steps).
	Result []byte

	// Errors contains error messages from previous attempts.
	Errors []string

	// Queue is the queue this step belongs to.
	Queue string

	// Priority is the execution priority of this step.
	Priority int

	// Timeout is the maximum duration the step is allowed to run.
	Timeout time.Duration

	// ScheduledAt is the time at which the step should be executed.
	ScheduledAt time.Time

	// AttemptedAt is the time at which the step was last attempted.
	AttemptedAt *time.Time

	// FinalizedAt is the time at which the step reached a final state.
	FinalizedAt *time.Time

	// CreatedAt is the time at which the step was created.
	CreatedAt time.Time

	// DependencyResults stores the results from parent steps
	DependencyResults map[string][]byte
}

// Re-export StepState constants for convenience
const (
	StepStateAvailable = tributarytype.StepStateAvailable
	StepStateRunning   = tributarytype.StepStateRunning
	StepStateCompleted = tributarytype.StepStateCompleted
	StepStateCancelled = tributarytype.StepStateCancelled
	StepStateDiscarded = tributarytype.StepStateDiscarded
)

// GetResultTyped retrieves a typed result from a specific dependency step by its task name.
// This is a helper function to provide type-safe access to dependency results.
func GetResultTyped[R ResultType](s *Step[StepArgs], taskName string) *R {
	if s.DependencyResults == nil {
		return nil
	}

	resultBytes, ok := s.DependencyResults[taskName]
	if !ok || resultBytes == nil {
		return nil
	}

	var result R
	if err := json.Unmarshal(resultBytes, &result); err != nil {
		return nil
	}

	return &result
}

// GetAllResultsTyped retrieves all results of a specific type from all dependency steps.
// This is useful for scenarios where multiple steps of the same type contribute results.
func GetAllResultsTyped[R ResultType](s *Step[StepArgs]) []R {
	if s.DependencyResults == nil {
		return nil
	}

	var results []R
	var sampleResult R
	targetKind := sampleResult.Kind()

	for _, resultBytes := range s.DependencyResults {
		if resultBytes == nil {
			continue
		}

		// Try to unmarshal to check if it's the right type
		var result R
		if err := json.Unmarshal(resultBytes, &result); err != nil {
			continue
		}

		// Verify the kind matches
		if result.Kind() == targetKind {
			results = append(results, result)
		}
	}

	return results
}

// GetResultsByPatternTyped retrieves results from dependency steps whose task names match a pattern.
// The pattern is matched using simple string contains logic.
func GetResultsByPatternTyped[R ResultType](s *Step[StepArgs], pattern string) []R {
	if s.DependencyResults == nil {
		return nil
	}

	var results []R
	var sampleResult R
	targetKind := sampleResult.Kind()

	for taskName, resultBytes := range s.DependencyResults {
		if resultBytes == nil {
			continue
		}

		// Check if task name contains the pattern
		if !containsPattern(taskName, pattern) {
			continue
		}

		// Try to unmarshal to check if it's the right type
		var result R
		if err := json.Unmarshal(resultBytes, &result); err != nil {
			continue
		}

		// Verify the kind matches
		if result.Kind() == targetKind {
			results = append(results, result)
		}
	}

	return results
}

// setDependencyResults sets the dependency results for this step.
// This is called internally by the execution engine.
func (s *Step[T]) setDependencyResults(results map[string][]byte) {
	s.DependencyResults = results
}

// containsPattern checks if a string contains a pattern.
// This is a simple implementation - could be enhanced with regex or glob patterns.
func containsPattern(s, pattern string) bool {
	// Simple contains check for now
	// Could be enhanced with more sophisticated pattern matching
	return len(pattern) == 0 || (len(s) > 0 && contains(s, pattern))
}

// contains is a helper function for string contains check
func contains(s, substr string) bool {
	return len(substr) == 0 || (len(s) >= len(substr) && indexOfString(s, substr) >= 0)
}

// indexOfString finds the index of a substring in a string
func indexOfString(s, substr string) int {
	if len(substr) == 0 {
		return 0
	}
	if len(s) == 0 {
		return -1
	}

	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
