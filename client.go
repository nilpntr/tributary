package tributary

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nilpntr/tributary/tributaryhook"
)

// Client is the main Tributary client for executing workflows and steps.
type Client struct {
	config *Config
	pool   *pgxpool.Pool

	// State management
	started bool
	mu      sync.Mutex
	wg      sync.WaitGroup
	cancel  context.CancelFunc
	ctx     context.Context

	// Worker management
	workerCh chan struct{} // signals workers to check for new steps
}

// NewClient creates a new Tributary client with the given configuration.
func NewClient(pool *pgxpool.Pool, config *Config) (*Client, error) {
	if config == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}

	if pool == nil {
		return nil, fmt.Errorf("pool cannot be nil")
	}

	// Set defaults and validate
	config.SetDefaults()
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &Client{
		config:   config,
		pool:     pool,
		ctx:      ctx,
		cancel:   cancel,
		workerCh: make(chan struct{}, 1),
	}, nil
}

// Start starts the client and begins processing steps.
// It blocks until the context is cancelled or an error occurs.
func (c *Client) Start(ctx context.Context) error {
	c.mu.Lock()
	if c.started {
		c.mu.Unlock()
		return ErrClientAlreadyStarted
	}
	c.started = true
	c.mu.Unlock()

	// Start notification listener for PostgreSQL LISTEN/NOTIFY
	c.startNotificationListener(ctx)

	// Start workers for each queue
	for queueName, queueConfig := range c.config.Queues {
		for i := 0; i < queueConfig.NumWorkers; i++ {
			c.wg.Add(1)
			go c.runWorker(ctx, queueName)
		}
	}

	// Wait for context cancellation
	<-ctx.Done()

	// Signal workers to stop
	c.cancel()

	// Wait for all workers to finish
	c.wg.Wait()

	c.mu.Lock()
	c.started = false
	c.mu.Unlock()

	return nil
}

// Stop gracefully stops the client.
func (c *Client) Stop(ctx context.Context) error {
	c.cancel()

	// Wait for all workers to finish with timeout
	done := make(chan struct{})
	go func() {
		c.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// createWorkflowExecution creates a new workflow execution record.
func (c *Client) createWorkflowExecution(ctx context.Context, workflowName string) (int64, error) {
	var id int64
	err := c.pool.QueryRow(ctx, `
		INSERT INTO workflow_executions (workflow_name, state)
		VALUES ($1, 'running')
		RETURNING id
	`, workflowName).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("failed to create workflow execution: %w", err)
	}
	return id, nil
}

// InsertMany inserts multiple steps into the database.
func (c *Client) InsertMany(ctx context.Context, params []InsertManyParams) ([]int64, error) {
	return c.InsertManyTx(ctx, nil, params)
}

// InsertManyTx inserts multiple steps into the database within a transaction.
func (c *Client) InsertManyTx(ctx context.Context, tx pgx.Tx, params []InsertManyParams) ([]int64, error) {
	if len(params) == 0 {
		return []int64{}, nil
	}

	var err error
	if tx == nil {
		// Start a new transaction if one wasn't provided
		tx, err = c.pool.Begin(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to begin transaction: %w", err)
		}
		defer tx.Rollback(ctx)
	}

	stepIDs := make([]int64, 0, len(params))
	taskNameToStepID := make(map[string]int64)

	// First pass: insert all steps
	for _, param := range params {
		// Serialize args to JSON
		argsBytes, err := json.Marshal(param.Args)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal args: %w", err)
		}

		// Run BeforeInsert hooks
		argsBytes, err = runBeforeInsertHooks(ctx, c.config.Hooks, param.Args, argsBytes)
		if err != nil {
			return nil, fmt.Errorf("BeforeInsert hook failed: %w", err)
		}

		// Apply default insert options
		opts := param.InsertOpts
		if opts == nil {
			opts = &InsertOpts{}
		}

		// Check if step args provide their own insert options
		if provider, ok := param.Args.(StepInsertOptsProvider); ok {
			stepOpts := provider.InsertOpts()
			// Step-specific options override parameter options, but not explicit values
			if opts.MaxAttempts == 0 && stepOpts.MaxAttempts > 0 {
				opts.MaxAttempts = stepOpts.MaxAttempts
			}
			if opts.Priority == 0 && stepOpts.Priority > 0 {
				opts.Priority = stepOpts.Priority
			}
			if opts.Queue == "" && stepOpts.Queue != "" {
				opts.Queue = stepOpts.Queue
			}
			if opts.Timeout == 0 && stepOpts.Timeout > 0 {
				opts.Timeout = stepOpts.Timeout
			}
		}

		// Apply global defaults
		if opts.MaxAttempts == 0 {
			opts.MaxAttempts = c.config.MaxAttempts
		}
		if opts.Queue == "" {
			opts.Queue = "default"
		}
		if opts.Priority == 0 {
			opts.Priority = 1
		}
		scheduledAt := opts.ScheduledAt
		if scheduledAt.IsZero() {
			scheduledAt = time.Now()
		}

		var timeoutSeconds *int
		if opts.Timeout > 0 {
			seconds := int(opts.Timeout.Seconds())
			timeoutSeconds = &seconds
		}

		var stepID int64
		err = tx.QueryRow(ctx, `
			INSERT INTO steps (
				workflow_execution_id, task_name, kind, args,
				queue, priority, max_attempts, timeout_seconds, scheduled_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
			RETURNING id
		`, param.WorkflowExecutionID, param.TaskName, param.Args.Kind(), argsBytes,
			opts.Queue, opts.Priority, opts.MaxAttempts, timeoutSeconds, scheduledAt).Scan(&stepID)
		if err != nil {
			return nil, fmt.Errorf("failed to insert step: %w", err)
		}

		stepIDs = append(stepIDs, stepID)
		taskNameToStepID[param.TaskName] = stepID

		// Run AfterInsert hooks
		tributaryhook.RunAfterInsertHooks(ctx, c.config.Hooks, stepID, param.Args)
	}

	// Second pass: insert dependencies
	for i, param := range params {
		stepID := stepIDs[i]
		for _, depTaskName := range param.DependsOn {
			depStepID, ok := taskNameToStepID[depTaskName]
			if !ok {
				return nil, fmt.Errorf("dependency %q not found for step %q", depTaskName, param.TaskName)
			}

			_, err = tx.Exec(ctx, `
				INSERT INTO step_dependencies (step_id, depends_on_step_id, workflow_execution_id)
				VALUES ($1, $2, $3)
			`, stepID, depStepID, param.WorkflowExecutionID)
			if err != nil {
				return nil, fmt.Errorf("failed to insert dependency: %w", err)
			}
		}
	}

	// Commit if we started the transaction
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Signal workers that new steps are available
	c.notifyWorkers()

	return stepIDs, nil
}

// NewWorkflow creates a new workflow with the given options.
func (c *Client) NewWorkflow(opts *WorkflowOpts) *Workflow {
	return NewWorkflow(c, opts)
}

// CreateWorkflowExecution creates a new workflow execution record.
// This implements the WorkflowClient interface from tributarytype.
func (c *Client) CreateWorkflowExecution(ctx context.Context, name string) (int64, error) {
	return c.createWorkflowExecution(ctx, name)
}
