package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nilpntr/tributary"
	"github.com/nilpntr/tributary/tributarylog"
	"github.com/nilpntr/tributary/tributarymigrate"
)

// Step 1: Always succeeds
type StartArgs struct {
	Message string `json:"message"`
}

func (StartArgs) Kind() string { return "start" }

type StartWorker struct {
	tributary.WorkerDefaults[StartArgs]
}

func (w *StartWorker) Work(ctx context.Context, step *tributary.Step[StartArgs]) error {
	fmt.Printf("[START] Attempt %d: %s\n", step.Attempt, step.Args.Message)

	result := StartResult{
		StartedAt: time.Now().Unix(),
	}
	resultBytes, _ := json.Marshal(result)
	step.Result = resultBytes

	return nil
}

// Step 2: Fails on first attempt, succeeds on second
type FlakyArgs struct {
	Operation string `json:"operation"`
}

func (FlakyArgs) Kind() string { return "flaky" }

type FlakyWorker struct {
	tributary.WorkerDefaults[FlakyArgs]
}

func (w *FlakyWorker) Work(ctx context.Context, step *tributary.Step[FlakyArgs]) error {
	fmt.Printf("[FLAKY] Attempt %d/%d: %s\n", step.Attempt, step.MaxAttempts, step.Args.Operation)

	// Fail on first attempt
	if step.Attempt == 0 {
		fmt.Println("[FLAKY] ❌ Simulating failure on first attempt...")
		return fmt.Errorf("simulated failure: network timeout")
	}

	// Succeed on second attempt
	fmt.Println("[FLAKY] ✅ Success on retry!")

	result := FlakyResult{
		Attempts:    step.Attempt + 1,
		CompletedAt: time.Now().Unix(),
	}
	resultBytes, _ := json.Marshal(result)
	step.Result = resultBytes

	return nil
}

// Step 3: Uses data from previous steps
type FinishArgs struct {
	Summary string `json:"summary"`
}

func (FinishArgs) Kind() string { return "finish" }

// Result types for dependency access
type StartResult struct {
	StartedAt int64 `json:"started_at"`
}

func (StartResult) Kind() string { return "start_result" }

type FlakyResult struct {
	Attempts    int   `json:"attempts"`
	CompletedAt int64 `json:"completed_at"`
}

func (FlakyResult) Kind() string { return "flaky_result" }

type FinishWorker struct {
	tributary.WorkerDefaults[FinishArgs]
}

func (w *FinishWorker) Work(ctx context.Context, step *tributary.Step[FinishArgs]) error {
	fmt.Printf("[FINISH] Attempt %d: %s\n", step.Attempt, step.Args.Summary)

	// Get results from previous steps by directly accessing DependencyResults
	if step.DependencyResults != nil {
		if startBytes, ok := step.DependencyResults["start"]; ok && startBytes != nil {
			var startResult StartResult
			if err := json.Unmarshal(startBytes, &startResult); err == nil {
				fmt.Printf("[FINISH] Start step completed at: %v\n", startResult.StartedAt)
			}
		}

		if flakyBytes, ok := step.DependencyResults["flaky"]; ok && flakyBytes != nil {
			var flakyResult FlakyResult
			if err := json.Unmarshal(flakyBytes, &flakyResult); err == nil {
				fmt.Printf("[FINISH] Flaky step took %v attempt(s)\n", flakyResult.Attempts)
			}
		}
	}

	fmt.Println("[FINISH] ✅ Workflow completed successfully!")

	return nil
}

func main() {
	ctx := context.Background()

	// Connect to PostgreSQL
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://postgres:postgres@localhost:5432/tributary?sslmode=disable"
	}

	if err := tributarymigrate.New(dbURL, nil).Up(); err != nil {
		log.Fatalf("failed to migrate DB: %v", err)
	}

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		log.Fatalf("Unable to connect to database: %v\n", err)
	}
	defer pool.Close()

	// Register workers
	workers := tributary.NewWorkers()
	tributary.AddWorker[StartArgs](workers, &StartWorker{})
	tributary.AddWorker[FlakyArgs](workers, &FlakyWorker{})
	tributary.AddWorker[FinishArgs](workers, &FinishWorker{})

	logger, err := tributarylog.NewDevelopmentZapLogger()
	if err != nil {
		log.Fatalf("Unable to create development zap logger: %v", err)
	}

	// Create client
	client, err := tributary.NewClient(pool, &tributary.Config{
		Queues: map[string]tributary.QueueConfig{
			"default": {MaxWorkers: 10},
		},
		Workers:          workers,
		RetryBackoffBase: 2 * time.Second, // 2s base delay for retries
		Logger:           logger,
	})
	if err != nil {
		log.Fatalf("Failed to create client: %v\n", err)
	}

	// Start client in a goroutine
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	go func() {
		if err := client.Start(ctx); err != nil {
			log.Printf("Client stopped: %v\n", err)
		}
	}()

	// Give the client a moment to start
	time.Sleep(100 * time.Millisecond)

	// Create and insert a workflow with a flaky step
	fmt.Println("\n=== Creating Retry Example Workflow ===\n")

	workflow := client.NewWorkflow(&tributary.WorkflowOpts{
		Name: "retry_example",
	})

	// Add tasks with dependencies
	workflow.AddTask("start", StartArgs{
		Message: "Starting workflow with retry example",
	}, nil)

	workflow.AddTask("flaky", FlakyArgs{
		Operation: "processing data",
	}, &tributary.WorkflowTaskOpts{
		InsertOpts: &tributary.InsertOpts{
			MaxAttempts: 5,
		},
		DependsOn: []string{"start"},
	})

	workflow.AddTask("finish", FinishArgs{
		Summary: "wrapping up",
	}, &tributary.WorkflowTaskOpts{
		DependsOn: []string{"start", "flaky"},
	})

	// Execute the workflow
	workflowExecutionID, err := workflow.Execute(ctx)
	if err != nil {
		log.Fatalf("Failed to execute workflow: %v\n", err)
	}

	fmt.Printf("Workflow execution created with ID: %d\n\n", workflowExecutionID)
	fmt.Println("Watch the retry logic in action!")
	fmt.Println("Expected: start → flaky (fails) → wait 2s → flaky (retry succeeds) → finish\n")

	// Wait for workflow to complete or interrupt
	fmt.Println("Workflow running... Press Ctrl+C to stop\n")

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Wait a bit for workflow to complete
	select {
	case <-sigCh:
		fmt.Println("\nShutting down...")
	case <-time.After(15 * time.Second):
		fmt.Println("\nWorkflow completed!")
	}

	// Graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	if err = client.Stop(shutdownCtx); err != nil {
		log.Printf("Error during shutdown: %v\n", err)
	}

	fmt.Println("Goodbye!")
}
