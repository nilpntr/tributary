package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nilpntr/tributary"
	"github.com/nilpntr/tributary/tributaryhook"
	"github.com/nilpntr/tributary/tributarylog"
	"github.com/nilpntr/tributary/tributarymigrate"
)

// Define typed result for type-safe dependency access
type CreateUserResult struct {
	UserID int64  `json:"user_id"`
	Email  string `json:"email"`
}

func (CreateUserResult) Kind() string { return "create_user_result" }

// Step 1: Create User
type CreateUserArgs struct {
	Email string `json:"email"`
}

func (CreateUserArgs) Kind() string { return "create_user" }

type CreateUserWorker struct {
	tributary.WorkerDefaults[CreateUserArgs]
}

func (w *CreateUserWorker) Work(ctx context.Context, step *tributary.Step[CreateUserArgs]) error {
	fmt.Printf("Creating user with email: %s\n", step.Args.Email)

	// Simulate user creation
	time.Sleep(100 * time.Millisecond)

	// Store the typed result for dependent steps
	result := CreateUserResult{
		UserID: 12345,
		Email:  step.Args.Email,
	}
	resultBytes, err := json.Marshal(result)
	if err != nil {
		return err
	}
	step.Result = resultBytes

	fmt.Printf("User created with ID: 12345\n")
	return nil
}

// Step 2: Activate User
type ActivateUserArgs struct {
	Reason string `json:"reason"`
}

func (ActivateUserArgs) Kind() string { return "activate_user" }

func (ActivateUserArgs) InsertOpts() tributary.StepInsertOpts {
	return tributary.StepInsertOpts{
		MaxAttempts: 3,               // Retry up to 3 times for activation
		Timeout:     5 * time.Minute, // 5 minute timeout
		Priority:    5,               // Higher priority than default
	}
}

type ActivateUserWorker struct {
	tributary.WorkerDefaults[ActivateUserArgs]
}

func (w *ActivateUserWorker) Work(ctx context.Context, step *tributary.Step[ActivateUserArgs]) error {
	// Get the user ID from the create_user step using type-safe access
	if step.DependencyResults == nil {
		return fmt.Errorf("no dependency results available")
	}

	createUserResultBytes, ok := step.DependencyResults["create_user"]
	if !ok {
		return fmt.Errorf("create_user result not found")
	}

	var createUserResult CreateUserResult
	if err := json.Unmarshal(createUserResultBytes, &createUserResult); err != nil {
		return fmt.Errorf("failed to unmarshal create_user result: %w", err)
	}

	fmt.Printf("Activating user %d (%s) - reason: %s\n", createUserResult.UserID, createUserResult.Email, step.Args.Reason)

	// OPTION: Legacy access (for comparison)
	// legacyResult := step.GetDependencyResult("create_user")
	// userID := int(legacyResult["user_id"].(float64))
	// email := legacyResult["email"].(string)

	// Simulate activation
	time.Sleep(150 * time.Millisecond)

	fmt.Printf("User %d activated successfully\n", createUserResult.UserID)
	return nil
}

// Step 3: Send Welcome Email
type SendWelcomeEmailArgs struct {
	Template string `json:"template"`
}

func (SendWelcomeEmailArgs) Kind() string { return "send_welcome_email" }

type SendWelcomeEmailWorker struct {
	tributary.WorkerDefaults[SendWelcomeEmailArgs]
}

func (w *SendWelcomeEmailWorker) Work(ctx context.Context, step *tributary.Step[SendWelcomeEmailArgs]) error {
	// Type-safe access to create_user result
	if step.DependencyResults == nil {
		return fmt.Errorf("no dependency results available")
	}

	createUserResultBytes, ok := step.DependencyResults["create_user"]
	if !ok {
		return fmt.Errorf("create_user result not found")
	}

	var createUserResult CreateUserResult
	if err := json.Unmarshal(createUserResultBytes, &createUserResult); err != nil {
		return fmt.Errorf("failed to unmarshal create_user result: %w", err)
	}

	fmt.Printf("Sending welcome email to %s (user %d) using template: %s\n",
		createUserResult.Email, createUserResult.UserID, step.Args.Template)

	// Simulate email sending
	time.Sleep(200 * time.Millisecond)

	fmt.Printf("Welcome email sent successfully to %s\n", createUserResult.Email)
	return nil
}

func main() {
	ctx := context.Background()

	// Connect to PostgreSQL
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://postgres:postgres@localhost:5432/tributary?sslmode=disable"
	}

	encryptionKeyHex := "89bad4444b316893ff9b215c8c9a2b617a911aef494918969489272ffb50a1f3"
	encryptionKey, err := hex.DecodeString(encryptionKeyHex)
	if err != nil {
		log.Fatalf("Failed to decode encryption key: %v", err)
	}

	if err = tributarymigrate.New(dbURL, nil).Up(); err != nil {
		log.Fatalf("failed to migrate DB: %v", err)
	}

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		log.Fatalf("Unable to connect to database: %v\n", err)
	}
	defer pool.Close()

	// Register workers
	workers := tributary.NewWorkers()
	tributary.AddWorker[CreateUserArgs](workers, &CreateUserWorker{})
	tributary.AddWorker[ActivateUserArgs](workers, &ActivateUserWorker{})
	tributary.AddWorker[SendWelcomeEmailArgs](workers, &SendWelcomeEmailWorker{})

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
		Hooks: []tributary.Hook{
			tributaryhook.NewEncryptHook(tributaryhook.NewSecretboxEncryptor([32]byte(encryptionKey))),
		},
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

	// Create and insert a user signup workflow
	fmt.Println("\n=== Creating User Signup Workflow ===\n")

	workflow := client.NewWorkflow(&tributary.WorkflowOpts{
		Name: "user_signup",
	})

	// Add tasks with dependencies
	workflow.AddTask("create_user", CreateUserArgs{
		Email: "alice@example.com",
	}, nil)

	workflow.AddTask("activate_user", ActivateUserArgs{
		Reason: "new_signup",
	}, &tributary.WorkflowTaskOpts{
		InsertOpts: &tributary.InsertOpts{
			MaxAttempts: 3,
		},
		DependsOn: []string{"create_user"},
	})

	workflow.AddTask("send_welcome_email", SendWelcomeEmailArgs{
		Template: "welcome_v2",
	}, &tributary.WorkflowTaskOpts{
		DependsOn: []string{"create_user", "activate_user"}, // Depends on both to access create_user result
	})

	// Prepare and insert the workflow
	workflowExecutionID, err := workflow.Execute(ctx)
	if err != nil {
		log.Fatalf("Failed to execute workflow: %v\n", err)
	}

	fmt.Printf("User signup workflow execution created with ID: %d\n\n", workflowExecutionID)

	// Wait for workflow to complete or interrupt
	fmt.Println("Workflow running... Press Ctrl+C to stop\n")

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Wait a bit for workflow to complete
	select {
	case <-sigCh:
		fmt.Println("\nShutting down...")
	case <-time.After(10 * time.Second):
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
