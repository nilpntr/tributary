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

// NumberProcessorArgs processes a number and outputs a formatted string
type NumberProcessorArgs struct {
	Number int `json:"number"`
}

func (NumberProcessorArgs) Kind() string { return "number_processor" }

func (NumberProcessorArgs) InsertOpts() tributary.StepInsertOpts {
	return tributary.StepInsertOpts{
		MaxAttempts: 2,
		Priority:    3, // Higher priority to run these first
	}
}

type NumberProcessorWorker struct {
	tributary.WorkerDefaults[NumberProcessorArgs]
}

func (w *NumberProcessorWorker) Work(ctx context.Context, step *tributary.Step[NumberProcessorArgs]) error {
	// Simulate some processing time
	time.Sleep(time.Duration(100+step.Args.Number*50) * time.Millisecond)

	message := fmt.Sprintf("I ran as number %d", step.Args.Number)
	fmt.Printf("[PROCESSOR-%d] %s\n", step.Args.Number, message)

	// Store result for the aggregator
	result := ProcessorResult{
		Number:  step.Args.Number,
		Message: message,
	}
	resultBytes, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("failed to marshal result: %w", err)
	}
	step.Result = resultBytes

	return nil
}

// ProcessorResult represents the output of a number processor
type ProcessorResult struct {
	Number  int    `json:"number"`
	Message string `json:"message"`
}

func (ProcessorResult) Kind() string { return "processor_result" }

// AggregatorArgs collects all processor results
type AggregatorArgs struct {
	ProcessorCount int `json:"processor_count"`
}

func (AggregatorArgs) Kind() string { return "aggregator" }

type AggregatorWorker struct {
	tributary.WorkerDefaults[AggregatorArgs]
}

func (w *AggregatorWorker) Work(ctx context.Context, step *tributary.Step[AggregatorArgs]) error {
	fmt.Printf("[AGGREGATOR] Collecting results from %d processors...\n", step.Args.ProcessorCount)

	if step.DependencyResults == nil {
		return fmt.Errorf("no dependency results available")
	}

	var allMessages []string

	// Collect results from all processor steps
	for i := 0; i < step.Args.ProcessorCount; i++ {
		taskName := fmt.Sprintf("process_%d", i)
		if resultBytes, ok := step.DependencyResults[taskName]; ok && resultBytes != nil {
			var result ProcessorResult
			if err := json.Unmarshal(resultBytes, &result); err != nil {
				return fmt.Errorf("failed to unmarshal result from %s: %w", taskName, err)
			}
			allMessages = append(allMessages, result.Message)
		} else {
			return fmt.Errorf("missing result from processor %d", i)
		}
	}

	// Create final aggregated message
	finalMessage := "Aggregated results: "
	for i, msg := range allMessages {
		if i > 0 {
			finalMessage += " | "
		}
		finalMessage += msg
	}

	fmt.Printf("[AGGREGATOR] ✅ %s\n", finalMessage)

	// Store the final result
	aggregatedResult := AggregatedResult{
		ProcessorCount: step.Args.ProcessorCount,
		FinalMessage:   finalMessage,
		AllMessages:    allMessages,
	}
	resultBytes, err := json.Marshal(aggregatedResult)
	if err != nil {
		return fmt.Errorf("failed to marshal aggregated result: %w", err)
	}
	step.Result = resultBytes

	return nil
}

// AggregatedResult represents the final output
type AggregatedResult struct {
	ProcessorCount int      `json:"processor_count"`
	FinalMessage   string   `json:"final_message"`
	AllMessages    []string `json:"all_messages"`
}

func (AggregatedResult) Kind() string { return "aggregated_result" }

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
	tributary.AddWorker[NumberProcessorArgs](workers, &NumberProcessorWorker{})
	tributary.AddWorker[AggregatorArgs](workers, &AggregatorWorker{})

	logger, err := tributarylog.NewDevelopmentZapLogger()
	if err != nil {
		log.Fatalf("Unable to create development zap logger: %v", err)
	}

	// Create client
	client, err := tributary.NewClient(pool, &tributary.Config{
		Queues: map[string]tributary.QueueConfig{
			"default": {MaxWorkers: 10},
		},
		Workers: workers,
		Logger:  logger,
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

	// Create and execute the parallel aggregation workflow
	fmt.Println("\n=== Creating Parallel Aggregation Workflow ===\n")

	processorCount := 7
	workflow := client.NewWorkflow(&tributary.WorkflowOpts{
		Name: "parallel_aggregation",
	})

	// Add multiple processor tasks that will run in parallel
	processorTaskNames := make([]string, processorCount)
	for i := 0; i < processorCount; i++ {
		taskName := fmt.Sprintf("process_%d", i)
		processorTaskNames[i] = taskName

		workflow.AddTask(taskName, NumberProcessorArgs{
			Number: i,
		}, &tributary.WorkflowTaskOpts{
			InsertOpts: &tributary.InsertOpts{
				// Add some jitter to make the parallel execution more visible
				ScheduledAt: time.Now().Add(time.Duration(i*100) * time.Millisecond),
			},
		})
	}

	// Add aggregator task that depends on all processor tasks
	workflow.AddTask("aggregator", AggregatorArgs{
		ProcessorCount: processorCount,
	}, &tributary.WorkflowTaskOpts{
		DependsOn: processorTaskNames, // Depends on all processor tasks
		InsertOpts: &tributary.InsertOpts{
			Priority: 1, // Lower priority, runs after processors
		},
	})

	// Execute the workflow
	workflowExecutionID, err := workflow.Execute(ctx)
	if err != nil {
		log.Fatalf("Failed to execute workflow: %v\n", err)
	}

	fmt.Printf("Parallel aggregation workflow created with ID: %d\n", workflowExecutionID)
	fmt.Printf("Watch %d processor steps run in parallel, then aggregate!\n", processorCount)
	fmt.Println("Expected: All processors run → aggregator collects and combines results\n")

	// Wait for workflow to complete or interrupt
	fmt.Println("Workflow running... Press Ctrl+C to stop\n")

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Wait a bit for workflow to complete
	select {
	case <-sigCh:
		fmt.Println("\nShutting down...")
	case <-time.After(20 * time.Second):
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
