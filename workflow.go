package tributary

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/nilpntr/tributary/tributarytype"
)

// Re-export WorkflowExecutionState from tributarytype
type WorkflowExecutionState = tributarytype.WorkflowExecutionState

// Re-export constants
const (
	WorkflowExecutionStateRunning   = tributarytype.WorkflowExecutionStateRunning
	WorkflowExecutionStateCompleted = tributarytype.WorkflowExecutionStateCompleted
	WorkflowExecutionStateFailed    = tributarytype.WorkflowExecutionStateFailed
	WorkflowExecutionStateCancelled = tributarytype.WorkflowExecutionStateCancelled
)

// InsertManyParams contains parameters for inserting multiple steps.
type InsertManyParams struct {
	WorkflowExecutionID *int64
	TaskName            string
	Args                StepArgs
	InsertOpts          *InsertOpts
	DependsOn           []string // List of task names this step depends on
}

// WorkflowOpts contains options for workflow execution.
type WorkflowOpts struct {
	Name        string    // Workflow name for identification
	ScheduledAt time.Time // When the workflow should start executing (applied to all initial steps)
}

// WorkflowTaskOpts contains options for individual workflow tasks.
type WorkflowTaskOpts struct {
	InsertOpts *InsertOpts
	DependsOn  []string // Task names this task depends on
}

// WorkflowTask represents a task within a workflow.
type WorkflowTask struct {
	Name string
	Args StepArgs
	Opts *WorkflowTaskOpts
}

// WorkflowResult represents the result of a completed workflow.
type WorkflowResult struct {
	WorkflowExecutionID int64
	State               WorkflowExecutionState
	StepResults         map[string][]byte // Task name -> step result
	CompletedAt         *time.Time
}

// WorkflowExecution represents a workflow execution.
type WorkflowExecution struct {
	ID          int64
	Name        string
	State       WorkflowExecutionState
	CreatedAt   time.Time
	CompletedAt *time.Time
}

// WorkflowClient interface defines the methods needed by the workflow system.
type WorkflowClient interface {
	CreateWorkflowExecution(ctx context.Context, name string) (int64, error)
	InsertManyTx(ctx context.Context, tx pgx.Tx, params []InsertManyParams) ([]int64, error)
}

// Workflow represents a workflow that can be executed.
type Workflow struct {
	client WorkflowClient
	opts   *WorkflowOpts
	tasks  []*WorkflowTask
}

// NewWorkflow creates a new workflow with the given client and options.
func NewWorkflow(client WorkflowClient, opts *WorkflowOpts) *Workflow {
	if opts == nil {
		opts = &WorkflowOpts{}
	}
	return &Workflow{
		client: client,
		opts:   opts,
		tasks:  make([]*WorkflowTask, 0),
	}
}

// AddTask adds a task to the workflow.
func (w *Workflow) AddTask(name string, args StepArgs, opts *WorkflowTaskOpts) *Workflow {
	w.tasks = append(w.tasks, &WorkflowTask{
		Name: name,
		Args: args,
		Opts: opts,
	})
	return w
}

// Execute runs the workflow and returns the workflow execution ID.
func (w *Workflow) Execute(ctx context.Context) (int64, error) {
	// Create workflow execution
	workflowExecutionID, err := w.client.CreateWorkflowExecution(ctx, w.opts.Name)
	if err != nil {
		return 0, err
	}

	// Convert tasks to insert params
	params := make([]InsertManyParams, len(w.tasks))
	for i, task := range w.tasks {
		params[i] = InsertManyParams{
			WorkflowExecutionID: &workflowExecutionID,
			TaskName:            task.Name,
			Args:                task.Args,
			InsertOpts:          nil, // Will use task opts if provided
			DependsOn:           nil, // Will be set from task opts
		}
		if task.Opts != nil {
			params[i].InsertOpts = task.Opts.InsertOpts
			params[i].DependsOn = task.Opts.DependsOn
		}

		// Apply workflow ScheduledAt to initial steps (those without dependencies)
		if !w.opts.ScheduledAt.IsZero() && (task.Opts == nil || len(task.Opts.DependsOn) == 0) {
			if params[i].InsertOpts == nil {
				params[i].InsertOpts = &InsertOpts{}
			}
			// Only set ScheduledAt if not already specified in task opts
			if params[i].InsertOpts.ScheduledAt.IsZero() {
				params[i].InsertOpts.ScheduledAt = w.opts.ScheduledAt
			}
		}
	}

	// Insert all tasks
	_, err = w.client.InsertManyTx(ctx, nil, params)
	if err != nil {
		return 0, err
	}

	return workflowExecutionID, nil
}
