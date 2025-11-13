# Tributary

A type-safe DAG workflow library for Go. Build complex workflows with steps that can depend on each other, pass data between steps, and execute in parallel when possible.

## Features

- **Type-safe workflows** with compile-time guarantees
- **DAG-based dependencies** - steps execute when their dependencies complete
- **Parallel execution** - steps with no blocking dependencies run concurrently
- **Automatic retries** with exponential backoff
- **Result passing** - steps can access outputs from their dependencies
- **Flexible hooks** - encryption, logging, metrics via composable hooks
- **PostgreSQL-backed** - reliable persistence with pgx
- **Graceful shutdown** - in-flight steps complete before shutdown

## Installation

```bash
go get github.com/nilpntr/tributary
```

## Quick Start

### 1. Define Your Step Types

```go
type CreateUserArgs struct {
    Email string `json:"email"`
}

func (CreateUserArgs) Kind() string { return "create_user" }

type CreateUserWorker struct {
    tributary.WorkerDefaults[CreateUserArgs]
}

func (w *CreateUserWorker) Work(ctx context.Context, step *tributary.Step[CreateUserArgs]) error {
    // Your business logic here
    fmt.Printf("Creating user: %s\n", step.Args.Email)

    // Store result for dependent steps
    result, _ := json.Marshal(map[string]interface{}{
        "user_id": 12345,
    })
    step.Result = result

    return nil
}
```

### 2. Configure Per-Step Options

Steps can provide their own execution options by implementing the `StepInsertOptsProvider` interface:

```go
func (ActivateUserArgs) InsertOpts() tributary.StepInsertOpts {
    return tributary.StepInsertOpts{
        MaxAttempts: 3,               // Retry up to 3 times
        Timeout:     5 * time.Minute, // 5 minute timeout per attempt
        Queue:       "critical",      // Use high-priority queue
        Priority:    10,              // Higher priority than default
    }
}
```

### 3. Register Workers and Start Client

```go
workers := tributary.NewWorkers()
tributary.AddWorker(workers, &CreateUserWorker{})
tributary.AddWorker(workers, &ActivateUserWorker{})

client, err := tributary.NewClient(pool, &tributary.Config{
    Queues: map[string]tributary.QueueConfig{
        "default": {MaxWorkers: 10},
    },
    Workers: workers,
})

go client.Start(ctx)
```

### 4. Build and Execute Workflows

```go
workflow := client.NewWorkflow(&tributary.WorkflowOpts{
    Name: "user_signup",
    ScheduledAt: time.Now().Add(1 * time.Hour), // Start workflow in 1 hour
})

// Add steps with dependencies
workflow.AddTask("create_user", CreateUserArgs{
    Email: "alice@example.com",
}, nil)

workflow.AddTask("activate_user", ActivateUserArgs{},
    &tributary.WorkflowTaskOpts{
        DependsOn: []string{"create_user"}, // depends on create_user
        InsertOpts: &tributary.InsertOpts{
            Priority: 5, // Higher priority for this step
        },
    })

workflow.AddTask("send_email", SendEmailArgs{},
    &tributary.WorkflowTaskOpts{
        DependsOn: []string{"activate_user"}, // depends on activate_user
    })

// Execute workflow
workflowExecutionID, err := workflow.Execute(ctx)
if err != nil {
    log.Fatalf("Failed to execute workflow: %v", err)
}
```

## Passing Data Between Steps

Steps can access results from their dependencies:

```go
func (w *SendEmailWorker) Work(ctx context.Context, step *tributary.Step[SendEmailArgs]) error {
    // Get result from create_user step
    createUserResult := step.GetDependencyResult("create_user")
    userID := int(createUserResult["user_id"].(float64))

    // Use the data
    fmt.Printf("Sending email to user %d\n", userID)
    return nil
}
```

## Encryption Hook

Protect sensitive step arguments with encryption:

```go
import "github.com/nilpntr/tributary"

// Generate or load a 32-byte key
var key [32]byte
copy(key[:], []byte("your-32-byte-encryption-key-here"))

client, err := tributary.NewClient(pool, &tributary.Config{
    Hooks: []tributary.Hook{
        tributary.NewEncryptHook(tributary.NewSecretboxEncryptor(key)),
    },
    // ... other config
})
```

Arguments are automatically encrypted before storage and decrypted before execution.

## Parallel Execution

Steps with no dependencies or whose dependencies are satisfied run in parallel:

```go
createUser := workflow.Add("create_user", CreateUserArgs{...}, nil, nil)

// These both depend only on create_user, so they run in parallel
workflow.Add("send_email", SendEmailArgs{...}, nil,
    &tributary.WorkflowTaskOpts{Deps: []string{createUser.Name}})
workflow.Add("send_sms", SendSMSArgs{...}, nil,
    &tributary.WorkflowTaskOpts{Deps: []string{createUser.Name}})
```

## Database Setup

Tributary includes a built-in migration system based on [Bun migrations](https://bun.uptrace.dev/guide/migrations.html). The migration system supports SQL files with `--bun:split` directives and Go migrations.

### Basic Usage

```go
import "github.com/nilpntr/tributary/tributarymigrate"

migrator := tributarymigrate.New(databaseURL, nil)

// Apply all pending migrations
err := migrator.Up()
if err != nil {
    log.Fatalf("Migration failed: %v", err)
}
```

### Migration Management

```go
ctx := context.Background()

// Get migration status
status, err := migrator.Status(ctx)
if err != nil {
    log.Fatalf("Failed to get status: %v", err)
}

for _, migration := range status {
    fmt.Printf("Migration %s: applied=%v\n", migration.Name, migration.Applied)
}

// Apply migrations up to a specific version
err = migrator.UpTo(ctx, "20230101000001_add_users")

// Rollback the last migration group
err = migrator.Rollback(ctx)

// Rollback to a specific version
err = migrator.DownTo(ctx, "20230101000001_add_users")

// Get current version
version, err := migrator.Version(ctx)

// Check for applied migrations that no longer exist
missing, err := migrator.MissingMigrations(ctx)
```

### Creating Migrations

```go
// Create SQL migration files (up and down)
files, err := migrator.CreateSQLMigration(ctx, "add_users_table")
// Creates: 20231113000001_add_users_table.up.sql
//          20231113000001_add_users_table.down.sql

// Create Go migration
file, err := migrator.CreateGoMigration(ctx, "complex_migration",
    tributarymigrate.WithPackageName("migrations"))
// Creates: 20231113000001_complex_migration.go
```

### SQL Migration Format

Tributary supports the Bun migration format with `--bun:split` directives:

```sql
-- 20231113000001_add_users_table.up.sql
CREATE TABLE users (
    id BIGSERIAL PRIMARY KEY,
    email TEXT NOT NULL UNIQUE,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

--bun:split

CREATE INDEX idx_users_email ON users(email);
```

### Automatic Migrations

Enable automatic migrations in your client configuration:

```go
client, err := tributary.NewClient(pool, &tributary.Config{
    // ... other config
    AutoMigrate: true,
    MigrationOptions: &tributary.MigrationOptions{
        DatabaseURL: databaseURL,
    },
})
```

## Configuration Options

### Client Config

```go
&tributary.Config{
    // Queue configurations
    Queues: map[string]tributary.QueueConfig{
        "default": {MaxWorkers: 10},
        "high_priority": {MaxWorkers: 20},
    },

    // Worker registry
    Workers: workers,

    // Lifecycle hooks
    Hooks: []tributary.Hook{
        tributary.NewEncryptHook(encryptor),
    },

    // Polling and retry settings
    FetchCooldown: 100 * time.Millisecond,
    FetchPollInterval: 1 * time.Second,
    MaxAttempts: 25,
    RetryBackoffBase: 1 * time.Second,
}
```

### Insert Options

Tributary supports three levels of configuration that are applied in this priority order:

1. **Step-level options** (highest priority) - via `InsertOpts()` method
2. **Task-level options** (medium priority) - via `WorkflowTaskOpts.InsertOpts`
3. **Global defaults** (lowest priority) - via client config

#### Step-Level Options (for individual step types)
```go
func (MyStepArgs) InsertOpts() tributary.StepInsertOpts {
    return tributary.StepInsertOpts{
        MaxAttempts: 3,              // Retry up to 3 times
        Priority: 10,                // Higher = executed first
        Queue: "high_priority",      // Queue name
        Timeout: 5 * time.Minute,    // Per-attempt timeout
    }
}
```

#### Workflow & Task-Level Options
```go
// Workflow-level scheduling
workflow := client.NewWorkflow(&tributary.WorkflowOpts{
    Name: "user_signup",
    ScheduledAt: time.Now().Add(1 * time.Hour), // Delay entire workflow
})

// Task-level overrides
workflow.AddTask("urgent_step", MyStepArgs{},
    &tributary.WorkflowTaskOpts{
        InsertOpts: &tributary.InsertOpts{
            Priority: 15,                              // Override step priority
            ScheduledAt: time.Now().Add(30 * time.Minute), // Override workflow schedule
        },
    })
```

## Architecture

Tributary workflows are batches of steps with dependency metadata:

1. **Workflow Definition** - Use the builder API to define steps and dependencies
2. **Workflow Preparation** - Validates the DAG (no cycles) and creates a workflow execution record
3. **Step Insertion** - All steps are inserted in a transaction with dependency links
4. **Step Execution** - Workers fetch steps whose dependencies are satisfied and execute them
5. **Result Storage** - Step outputs are stored for dependent steps to access
6. **Workflow Completion** - When all steps complete, the workflow is marked as complete/failed

### Database Schema

- `workflow_executions` - Tracks workflow runs
- `steps` - Individual step instances with state
- `step_dependencies` - DAG edges (which steps depend on which)

### Step States

- `available` - Ready to execute (dependencies satisfied, scheduled time passed)
- `running` - Currently executing
- `completed` - Finished successfully
- `discarded` - Failed after exhausting retries
- `cancelled` - Manually cancelled

## Examples

### User Signup Workflow
`examples/user_signup/main.go` - A complete user signup workflow demonstrating:
- Sequential steps with dependencies
- Result passing between steps
- Per-step configuration

### Retry Logic
`examples/retry_example/main.go` - Demonstrates automatic retry behavior:
- Steps that fail on first attempt
- Exponential backoff between retries
- Successful completion after retry
- Error tracking across attempts

## License

MIT

## Contributing

Contributions welcome! Please open an issue or PR.
