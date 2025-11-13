package tributary

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"runtime/debug"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/nilpntr/tributary/tributarytype"
)

// runWorker is the main worker loop for executing steps from a queue.
func (c *Client) runWorker(ctx context.Context, queueName string) {
	defer c.wg.Done()

	ticker := time.NewTicker(c.config.FetchPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-c.ctx.Done():
			// Client is shutting down
			return
		case <-c.workerCh:
			// New steps available, try to fetch
			c.fetchAndExecute(ctx, queueName)
		case <-ticker.C:
			// Poll periodically
			c.fetchAndExecute(ctx, queueName)
		}
	}
}

// fetchAndExecute fetches available steps from the queue and executes them.
func (c *Client) fetchAndExecute(ctx context.Context, queueName string) {
	// Fetch steps that are ready to execute (dependencies satisfied)
	steps, err := c.fetchAvailableSteps(ctx, queueName, 10)
	if err != nil {
		// Log error but continue
		c.config.Logger.Error("Error fetching steps", Field{Key: "error", Value: err}, Field{Key: "queue", Value: queueName})
		return
	}

	if len(steps) == 0 {
		// No steps available, wait for cooldown
		time.Sleep(c.config.FetchCooldown)
		return
	}

	// Execute each step
	for _, stepRow := range steps {
		if ctx.Err() != nil {
			return
		}
		c.executeStep(ctx, stepRow)
	}
}

// stepRow represents a step fetched from the database.
type stepRow struct {
	ID                  int64
	WorkflowExecutionID *int64
	TaskName            *string
	State               string
	Attempt             int
	MaxAttempts         int
	Kind                string
	Args                []byte
	Result              []byte
	Queue               string
	Priority            int
	TimeoutSeconds      *int
	ScheduledAt         time.Time
	AttemptedAt         *time.Time
	FinalizedAt         *time.Time
	CreatedAt           time.Time
}

// fetchAvailableSteps fetches steps that are ready to execute and atomically marks them as running.
// A step is ready if:
// 1. It's in 'available' state
// 2. Its scheduled_at time has passed
// 3. All its dependencies are completed
func (c *Client) fetchAvailableSteps(ctx context.Context, queueName string, limit int) ([]stepRow, error) {
	// Use a transaction to atomically fetch and mark steps as running
	tx, err := c.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	// First, fetch available steps with row-level locking
	rows, err := tx.Query(ctx, `
		SELECT
			s.id, s.workflow_execution_id, s.task_name, s.state, s.attempt, s.max_attempts,
			s.kind, s.args, s.result, s.queue, s.priority, s.timeout_seconds,
			s.scheduled_at, s.attempted_at, s.finalized_at, s.created_at
		FROM steps s
		WHERE s.state = 'available'
			AND s.queue = $1
			AND s.scheduled_at <= NOW()
			AND NOT EXISTS (
				SELECT 1
				FROM step_dependencies d
				JOIN steps parent ON d.depends_on_step_id = parent.id
				WHERE d.step_id = s.id
					AND parent.state != 'completed'
			)
		ORDER BY s.priority DESC, s.scheduled_at ASC
		LIMIT $2
		FOR UPDATE SKIP LOCKED
	`, queueName, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var steps []stepRow
	var stepIDs []int64
	for rows.Next() {
		var step stepRow
		err := rows.Scan(
			&step.ID, &step.WorkflowExecutionID, &step.TaskName, &step.State, &step.Attempt, &step.MaxAttempts,
			&step.Kind, &step.Args, &step.Result, &step.Queue, &step.Priority, &step.TimeoutSeconds,
			&step.ScheduledAt, &step.AttemptedAt, &step.FinalizedAt, &step.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		steps = append(steps, step)
		stepIDs = append(stepIDs, step.ID)
	}
	rows.Close()

	if err := rows.Err(); err != nil {
		return nil, err
	}

	// If we found steps, atomically mark them as running within the same transaction
	if len(stepIDs) > 0 {
		// Convert slice to PostgreSQL array format
		_, err = tx.Exec(ctx, `
			UPDATE steps
			SET state = 'running', attempted_at = NOW()
			WHERE id = ANY($1)
		`, stepIDs)
		if err != nil {
			return nil, err
		}

		// Update the local step objects to reflect the new state
		for i := range steps {
			steps[i].State = "running"
			now := time.Now()
			steps[i].AttemptedAt = &now
		}
	}

	// Commit the transaction to release locks and apply changes
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return steps, nil
}

// executeStep executes a single step.
// Note: The step is already marked as 'running' by fetchAvailableSteps.
func (c *Client) executeStep(ctx context.Context, stepRow stepRow) {
	// Get worker for this step kind
	workerInterface, ok := c.config.Workers.Get(stepRow.Kind)
	if !ok {
		c.markStepFailed(ctx, stepRow, fmt.Errorf("worker not found for kind %q", stepRow.Kind))
		return
	}

	// Type assert to workerWrapper
	workerWrapped, ok := workerInterface.(workerWrapper)
	if !ok {
		c.markStepFailed(ctx, stepRow, fmt.Errorf("invalid worker type for kind %q", stepRow.Kind))
		return
	}

	// Run BeforeWork hooks to decrypt/transform args
	argsBytes := stepRow.Args
	argsBytes, err := runBeforeWorkHooks(ctx, c.config.Hooks, stepRow.ID, stepRow.Kind, argsBytes)
	if err != nil {
		c.markStepFailed(ctx, stepRow, fmt.Errorf("BeforeWork hook failed: %w", err))
		return
	}

	// Unmarshal args
	argsValue := reflect.New(workerWrapped.argsType).Interface()
	if err := json.Unmarshal(argsBytes, argsValue); err != nil {
		c.markStepFailed(ctx, stepRow, fmt.Errorf("failed to unmarshal args: %w", err))
		return
	}

	// Fetch dependency results if this step has dependencies
	depResults, err := c.fetchDependencyResults(ctx, stepRow.ID)
	if err != nil {
		c.markStepFailed(ctx, stepRow, fmt.Errorf("failed to fetch dependency results: %w", err))
		return
	}

	// Create the Step object
	stepPtr := c.createStepFromRow(stepRow, argsValue, depResults)

	// Create context with timeout if specified
	workCtx := ctx
	if stepRow.TimeoutSeconds != nil && *stepRow.TimeoutSeconds > 0 {
		var cancel context.CancelFunc
		workCtx, cancel = context.WithTimeout(ctx, time.Duration(*stepRow.TimeoutSeconds)*time.Second)
		defer cancel()
	}

	// Execute the worker with panic recovery
	workErr := c.executeStepSafely(workCtx, stepRow.ID, stepRow.Kind, workerWrapped.workFunc, stepPtr)

	if workErr != nil {
		c.markStepFailed(ctx, stepRow, workErr)
		return
	}

	// Get result from step if any
	var resultBytes []byte
	stepValue := reflect.ValueOf(stepPtr).Elem()
	resultField := stepValue.FieldByName("Result")
	if resultField.IsValid() && resultField.Len() > 0 {
		resultBytes = resultField.Bytes()
	}

	// Run AfterWork hooks
	resultBytes, err = runAfterWorkHooks(ctx, c.config.Hooks, stepRow.ID, resultBytes, nil)
	if err != nil {
		c.markStepFailed(ctx, stepRow, fmt.Errorf("AfterWork hook failed: %w", err))
		return
	}

	// Mark step as completed
	c.markStepCompleted(ctx, stepRow.ID, stepRow.WorkflowExecutionID, resultBytes)
}

// executeStepSafely executes a worker function with panic recovery.
// If panic recovery is enabled in config, panics are caught and converted to errors.
func (c *Client) executeStepSafely(ctx context.Context, stepID int64, kind string, workFunc func(context.Context, interface{}) error, stepPtr interface{}) (err error) {
	// Check if panic recovery is enabled in config
	// For now, we'll assume it's enabled by default. In the future, this should check c.config.PanicRecovery
	panicRecoveryEnabled := true // TODO: Get from config when Config is updated

	if panicRecoveryEnabled {
		defer func() {
			if r := recover(); r != nil {
				// Capture stack trace
				stackTrace := string(debug.Stack())

				// Create a WorkerPanicError with details
				err = fmt.Errorf("worker panic: %v\nStack: %s", r, stackTrace)

				// Log the panic with structured information
				c.config.Logger.Error("Worker panic recovered", Field{Key: "step_id", Value: stepID}, Field{Key: "kind", Value: kind}, Field{Key: "panic", Value: r}, Field{Key: "stack_trace", Value: stackTrace})
			}
		}()
	}

	// Execute the worker function
	return workFunc(ctx, stepPtr)
}

// createStepFromRow creates a Step object from a stepRow.
// We create a Step[StepArgs] and rely on type assertion in the worker's workFunc.
func (c *Client) createStepFromRow(stepRow stepRow, argsValue interface{}, depResults map[string][]byte) interface{} {
	// Get the dereferenced args value
	argsValueElem := reflect.ValueOf(argsValue).Elem()

	// Create a Step[StepArgs] - the worker's workFunc will handle type assertion to Step[T]
	step := &Step[StepArgs]{
		ID:                  stepRow.ID,
		WorkflowExecutionID: stepRow.WorkflowExecutionID,
		TaskName:            stepRow.TaskName,
		State:               tributarytype.StepState(stepRow.State),
		Attempt:             stepRow.Attempt,
		MaxAttempts:         stepRow.MaxAttempts,
		Kind:                stepRow.Kind,
		Args:                argsValueElem.Interface().(StepArgs),
		ArgsRaw:             stepRow.Args,
		Result:              stepRow.Result,
		Queue:               stepRow.Queue,
		Priority:            stepRow.Priority,
		ScheduledAt:         stepRow.ScheduledAt,
		AttemptedAt:         stepRow.AttemptedAt,
		FinalizedAt:         stepRow.FinalizedAt,
		CreatedAt:           stepRow.CreatedAt,
		DependencyResults:   depResults,
	}

	if stepRow.TimeoutSeconds != nil {
		step.Timeout = time.Duration(*stepRow.TimeoutSeconds) * time.Second
	}

	return step
}

// fetchDependencyResults fetches the results from direct dependencies of a step.
func (c *Client) fetchDependencyResults(ctx context.Context, stepID int64) (map[string][]byte, error) {
	rows, err := c.pool.Query(ctx, `
		SELECT parent.task_name, parent.result
		FROM step_dependencies d
		JOIN steps parent ON d.depends_on_step_id = parent.id
		WHERE d.step_id = $1
	`, stepID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	results := make(map[string][]byte)
	for rows.Next() {
		var taskName *string
		var result []byte
		if err := rows.Scan(&taskName, &result); err != nil {
			return nil, err
		}
		if taskName != nil && result != nil {
			// Decrypt the result using BeforeWork hooks (which handle decryption)
			decryptedResult, err := runBeforeWorkHooks(ctx, c.config.Hooks, 0, "dependency_result", result)
			if err != nil {
				// If decryption fails, log error but continue with raw result
				c.config.Logger.Warn("Failed to decrypt dependency result", Field{Key: "task_name", Value: *taskName}, Field{Key: "error", Value: err})
				decryptedResult = result
			}
			results[*taskName] = decryptedResult
		}
	}

	return results, rows.Err()
}

// markStepRunning marks a step as running.
func (c *Client) markStepRunning(ctx context.Context, stepID int64) error {
	_, err := c.pool.Exec(ctx, `
		UPDATE steps
		SET state = 'running', attempted_at = NOW()
		WHERE id = $1
	`, stepID)
	return err
}

// markStepCompleted marks a step as completed and stores the result.
func (c *Client) markStepCompleted(ctx context.Context, stepID int64, workflowExecutionID *int64, result []byte) error {
	tx, err := c.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// Mark step as completed
	_, err = tx.Exec(ctx, `
		UPDATE steps
		SET state = 'completed', result = $2, finalized_at = NOW()
		WHERE id = $1
	`, stepID, result)
	if err != nil {
		return err
	}

	// Check if workflow is complete
	if workflowExecutionID != nil {
		c.checkWorkflowCompletion(ctx, tx, *workflowExecutionID)
	}

	return tx.Commit(ctx)
}

// markStepFailed marks a step as failed and schedules a retry if attempts remain.
func (c *Client) markStepFailed(ctx context.Context, stepRow stepRow, err error) {
	errorMsg := err.Error()
	nextAttempt := stepRow.Attempt + 1

	if nextAttempt >= stepRow.MaxAttempts {
		// No more attempts, mark as discarded
		_, dbErr := c.pool.Exec(ctx, `
			UPDATE steps
			SET state = 'discarded',
				errors = array_append(errors, $2),
				finalized_at = NOW()
			WHERE id = $1
		`, stepRow.ID, errorMsg)
		if dbErr != nil {
			c.config.Logger.Error("Error marking step as discarded", Field{Key: "step_id", Value: stepRow.ID}, Field{Key: "error", Value: dbErr})
		}

		// Mark workflow as failed if applicable
		if stepRow.WorkflowExecutionID != nil {
			c.markWorkflowFailed(ctx, *stepRow.WorkflowExecutionID)
		}
	} else {
		// Schedule retry with exponential backoff
		backoff := c.config.RetryBackoffBase * time.Duration(1<<uint(nextAttempt-1))
		scheduledAt := time.Now().Add(backoff)

		_, dbErr := c.pool.Exec(ctx, `
			UPDATE steps
			SET state = 'available',
				attempt = $2,
				errors = array_append(errors, $3),
				scheduled_at = $4
			WHERE id = $1
		`, stepRow.ID, nextAttempt, errorMsg, scheduledAt)
		if dbErr != nil {
			c.config.Logger.Error("Error scheduling step retry", Field{Key: "step_id", Value: stepRow.ID}, Field{Key: "attempt", Value: nextAttempt}, Field{Key: "error", Value: dbErr})
		}
	}
}

// checkWorkflowCompletion checks if all steps in a workflow are complete.
func (c *Client) checkWorkflowCompletion(ctx context.Context, tx pgx.Tx, workflowExecutionID int64) {
	var incompleteCount int
	err := tx.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM steps
		WHERE workflow_execution_id = $1
			AND state NOT IN ('completed', 'cancelled', 'discarded')
	`, workflowExecutionID).Scan(&incompleteCount)
	if err != nil {
		c.config.Logger.Error("Error checking workflow completion", Field{Key: "workflow_execution_id", Value: workflowExecutionID}, Field{Key: "error", Value: err})
		return
	}

	if incompleteCount == 0 {
		// Check if any steps were discarded
		var discardedCount int
		err = tx.QueryRow(ctx, `
			SELECT COUNT(*)
			FROM steps
			WHERE workflow_execution_id = $1 AND state = 'discarded'
		`, workflowExecutionID).Scan(&discardedCount)
		if err != nil {
			c.config.Logger.Error("Error checking for discarded steps", Field{Key: "workflow_execution_id", Value: workflowExecutionID}, Field{Key: "error", Value: err})
			return
		}

		state := "completed"
		if discardedCount > 0 {
			state = "failed"
		}

		_, err = tx.Exec(ctx, `
			UPDATE workflow_executions
			SET state = $2, finalized_at = NOW()
			WHERE id = $1
		`, workflowExecutionID, state)
		if err != nil {
			c.config.Logger.Error("Error updating workflow state", Field{Key: "workflow_execution_id", Value: workflowExecutionID}, Field{Key: "state", Value: state}, Field{Key: "error", Value: err})
		}
	}
}

// markWorkflowFailed marks a workflow as failed.
func (c *Client) markWorkflowFailed(ctx context.Context, workflowExecutionID int64) {
	_, err := c.pool.Exec(ctx, `
		UPDATE workflow_executions
		SET state = 'failed', finalized_at = NOW()
		WHERE id = $1
	`, workflowExecutionID)
	if err != nil {
		c.config.Logger.Error("Error marking workflow as failed", Field{Key: "workflow_execution_id", Value: workflowExecutionID}, Field{Key: "error", Value: err})
	}
}
