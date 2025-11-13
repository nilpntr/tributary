package tributary

import (
	"context"
	"fmt"
	"reflect"
	"time"
)

// Worker is the interface that step workers must implement.
// Workers define the logic for executing a specific type of step.
type Worker[T StepArgs] interface {
	// Work executes the step with the given arguments.
	// It should return an error if the step fails.
	Work(ctx context.Context, step *Step[T]) error
}

// WorkerDefaults provides default implementations for worker methods.
// Embed this in your worker struct to get sensible defaults.
type WorkerDefaults[T StepArgs] struct{}

// Timeout returns the default timeout for the worker (no timeout).
// Override this method to provide a custom timeout.
func (WorkerDefaults[T]) Timeout() time.Duration {
	return 0
}

// Workers is a registry of step workers.
type Workers struct {
	workers map[string]workerWrapper
}

// workerWrapper wraps a worker with its reflection type information.
type workerWrapper struct {
	worker   interface{}
	argsType reflect.Type
	workFunc func(ctx context.Context, step interface{}) error
}

// NewWorkers creates a new worker registry.
func NewWorkers() *Workers {
	return &Workers{
		workers: make(map[string]workerWrapper),
	}
}

// Get retrieves a worker wrapper by kind for backward compatibility.
func (w *Workers) Get(kind string) (interface{}, bool) {
	wrapper, ok := w.workers[kind]
	if !ok {
		return nil, false
	}
	return wrapper, true
}

// AddWorker registers a worker for a specific step type.
// The worker's step type is determined by the generic type parameter.
// Panics if the worker is invalid or already registered.
func AddWorker[T StepArgs](workers *Workers, worker Worker[T]) {
	var zero T
	kind := zero.Kind()

	if kind == "" {
		panic("step args Kind() returned empty string")
	}

	if _, exists := workers.workers[kind]; exists {
		panic(fmt.Sprintf("worker for kind %q already registered", kind))
	}

	// Get the type of T for later instantiation
	argsType := reflect.TypeOf(zero)

	// Create a wrapper function that handles type conversion
	workFunc := func(ctx context.Context, step interface{}) error {
		// We receive a *Step[StepArgs] from the executor
		genericStep, ok := step.(*Step[StepArgs])
		if !ok {
			return fmt.Errorf("invalid step type for worker %s", kind)
		}

		// Convert the generic step to a typed Step[T]
		// The Args field should already be of type T due to how we unmarshal
		typedArgs, ok := genericStep.Args.(T)
		if !ok {
			return fmt.Errorf("invalid args type for worker %s: expected %T, got %T", kind, zero, genericStep.Args)
		}

		// Create a properly typed Step[T]
		typedStep := &Step[T]{
			ID:                  genericStep.ID,
			WorkflowExecutionID: genericStep.WorkflowExecutionID,
			TaskName:            genericStep.TaskName,
			State:               genericStep.State,
			Attempt:             genericStep.Attempt,
			MaxAttempts:         genericStep.MaxAttempts,
			Kind:                genericStep.Kind,
			Args:                typedArgs,
			ArgsRaw:             genericStep.ArgsRaw,
			Result:              genericStep.Result,
			Errors:              genericStep.Errors,
			Queue:               genericStep.Queue,
			Priority:            genericStep.Priority,
			Timeout:             genericStep.Timeout,
			ScheduledAt:         genericStep.ScheduledAt,
			AttemptedAt:         genericStep.AttemptedAt,
			FinalizedAt:         genericStep.FinalizedAt,
			CreatedAt:           genericStep.CreatedAt,
		}
		typedStep.setDependencyResults(genericStep.DependencyResults)

		// Execute the worker
		err := worker.Work(ctx, typedStep)

		// Copy the result back to the generic step so it can be saved
		genericStep.Result = typedStep.Result

		return err
	}

	workers.workers[kind] = workerWrapper{
		worker:   worker,
		argsType: argsType,
		workFunc: workFunc,
	}
}

// GetWrapper retrieves a worker wrapper by its kind.
// Returns nil if no worker is registered for the given kind.
func (w *Workers) GetWrapper(kind string) (workerWrapper, bool) {
	wrapper, ok := w.workers[kind]
	return wrapper, ok
}

// GetWorker retrieves a worker by its kind.
func (w *Workers) GetWorker(kind string) (Worker[StepArgs], bool) {
	_, ok := w.workers[kind]
	if !ok {
		return nil, false
	}
	// This is a simplified implementation - in practice would need proper type conversion
	return nil, false // TODO: Implement proper worker extraction
}

// RegisterWorker registers a worker with the given kind.
func (w *Workers) RegisterWorker(kind string, worker Worker[StepArgs]) {
	// This is a simplified implementation - in practice would need proper registration
	// TODO: Implement proper worker registration
}

// ListKinds returns a list of all registered worker kinds.
func (w *Workers) ListKinds() []string {
	kinds := make([]string, 0, len(w.workers))
	for kind := range w.workers {
		kinds = append(kinds, kind)
	}
	return kinds
}

// Has checks if a worker is registered for the given kind.
func (w *Workers) Has(kind string) bool {
	_, ok := w.workers[kind]
	return ok
}

// Kinds returns a list of all registered worker kinds.
func (w *Workers) Kinds() []string {
	kinds := make([]string, 0, len(w.workers))
	for kind := range w.workers {
		kinds = append(kinds, kind)
	}
	return kinds
}
