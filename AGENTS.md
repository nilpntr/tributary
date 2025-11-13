# Tributary AI Agent Guide

This guide is specifically written for AI agents to understand and effectively use the Tributary workflow library.

## Overview

Tributary is a type-safe DAG (Directed Acyclic Graph) workflow library for Go that uses PostgreSQL for persistence. It enables building complex workflows where steps can depend on each other, pass data between steps, and execute in parallel.

## Key Concepts for AI Agents

### 1. Step-Centric Design
Every unit of work is a "step" with:
- **Args**: Input data (must implement `StepArgs` interface)
- **Worker**: Code that processes the args
- **Result**: Output data for dependent steps
- **Dependencies**: Other steps this step waits for

### 2. Type Safety
- All step arguments and results are strongly typed
- Use Go generics: `Step[T StepArgs]`
- Workers are typed: `WorkerDefaults[MyStepArgs]`
- No runtime type assertions needed

### 3. Three Configuration Levels (Priority Order)
1. **Step-level** (highest): `InsertOpts()` method on step args
2. **Task-level** (medium): `WorkflowTaskOpts.InsertOpts`
3. **Global** (lowest): Client configuration

## Essential Patterns for AI Agents

### Pattern 1: Simple Step Definition
```go
// 1. Define arguments
type ProcessDataArgs struct {
    Input string `json:"input"`
}
func (ProcessDataArgs) Kind() string { return "process_data" }

// 2. Optional: Step-specific configuration
func (ProcessDataArgs) InsertOpts() tributary.StepInsertOpts {
    return tributary.StepInsertOpts{
        MaxAttempts: 3,
        Timeout:     5 * time.Minute,
        Priority:    5,
    }
}

// 3. Implement worker
type ProcessDataWorker struct {
    tributary.WorkerDefaults[ProcessDataArgs]
}

func (w *ProcessDataWorker) Work(ctx context.Context, step *tributary.Step[ProcessDataArgs]) error {
    // Your logic here
    result := fmt.Sprintf("Processed: %s", step.Args.Input)

    // Store result for dependent steps
    step.Result = []byte(result)
    return nil
}
```

### Pattern 2: Result Passing Between Steps
```go
// Define result type
type UserResult struct {
    UserID int64 `json:"user_id"`
    Email  string `json:"email"`
}
func (UserResult) Kind() string { return "user_result" }

// Producer step
func (w *CreateUserWorker) Work(ctx context.Context, step *tributary.Step[CreateUserArgs]) error {
    userID := 12345 // Your logic

    result := UserResult{
        UserID: userID,
        Email:  step.Args.Email,
    }
    resultBytes, _ := json.Marshal(result)
    step.Result = resultBytes
    return nil
}

// Consumer step
func (w *SendEmailWorker) Work(ctx context.Context, step *tributary.Step[SendEmailArgs]) error {
    // Access dependency result
    if userBytes, ok := step.DependencyResults["create_user"]; ok {
        var userResult UserResult
        json.Unmarshal(userBytes, &userResult)
        // Use userResult.UserID, userResult.Email
    }
    return nil
}
```

### Pattern 3: Workflow Building
```go
// Create workflow
workflow := client.NewWorkflow(&tributary.WorkflowOpts{
    Name: "data_pipeline",
    ScheduledAt: time.Now().Add(1 * time.Hour), // Optional delay
})

// Add independent steps (run in parallel)
workflow.AddTask("extract_data", ExtractArgs{Source: "db1"}, nil)
workflow.AddTask("validate_data", ValidateArgs{Rules: []string{"required"}}, nil)

// Add dependent step
workflow.AddTask("transform_data", TransformArgs{},
    &tributary.WorkflowTaskOpts{
        DependsOn: []string{"extract_data", "validate_data"},
        InsertOpts: &tributary.InsertOpts{
            Priority: 10, // High priority
        },
    })

// Execute
workflowID, err := workflow.Execute(ctx)
```

## Common AI Agent Use Cases

### 1. Data Pipeline
```go
// Extract → Transform → Load pattern
workflow.AddTask("extract", ExtractArgs{}, nil)
workflow.AddTask("transform", TransformArgs{},
    &tributary.WorkflowTaskOpts{DependsOn: []string{"extract"}})
workflow.AddTask("load", LoadArgs{},
    &tributary.WorkflowTaskOpts{DependsOn: []string{"transform"}})
```

### 2. Fan-Out/Fan-In
```go
// One step → Multiple parallel steps → One aggregator
workflow.AddTask("prepare", PrepareArgs{}, nil)

// Fan out - parallel processing
for i := 0; i < 5; i++ {
    workflow.AddTask(fmt.Sprintf("process_%d", i), ProcessArgs{ID: i},
        &tributary.WorkflowTaskOpts{DependsOn: []string{"prepare"}})
}

// Fan in - aggregation
dependsOn := []string{"process_0", "process_1", "process_2", "process_3", "process_4"}
workflow.AddTask("aggregate", AggregateArgs{},
    &tributary.WorkflowTaskOpts{DependsOn: dependsOn})
```

### 3. Conditional Execution
```go
// Use step results to determine next steps
type DecisionArgs struct {
    Condition string `json:"condition"`
}

func (w *DecisionWorker) Work(ctx context.Context, step *tributary.Step[DecisionArgs]) error {
    if step.Args.Condition == "process_a" {
        // Set result to indicate path A
        step.Result = []byte(`{"path": "a"}`)
    } else {
        step.Result = []byte(`{"path": "b"}`)
    }
    return nil
}

// Later steps check the decision result
func (w *ProcessAWorker) Work(ctx context.Context, step *tributary.Step[ProcessAArgs]) error {
    if decisionBytes, ok := step.DependencyResults["decision"]; ok {
        var decision map[string]string
        json.Unmarshal(decisionBytes, &decision)
        if decision["path"] != "a" {
            return nil // Skip this step
        }
    }
    // Process path A
    return nil
}
```

## Configuration Guidelines for AI Agents

### When to Use Each Configuration Level

**Step-Level (`InsertOpts()` method):**
- Step-type specific requirements (e.g., ML steps need more retries)
- Consistent across all instances of this step type
```go
func (TrainModelArgs) InsertOpts() tributary.StepInsertOpts {
    return tributary.StepInsertOpts{
        MaxAttempts: 5,    // ML training can be flaky
        Timeout: 30 * time.Minute,
        Queue: "gpu",      // Needs GPU queue
    }
}
```

**Task-Level (`WorkflowTaskOpts.InsertOpts`):**
- Workflow-specific overrides
- One-off adjustments
```go
workflow.AddTask("urgent_cleanup", CleanupArgs{},
    &tributary.WorkflowTaskOpts{
        InsertOpts: &tributary.InsertOpts{
            Priority: 20,  // Override step's default priority
        },
    })
```

**Global (Client config):**
- Organization-wide defaults
- Infrastructure limitations
```go
client, err := tributary.NewClient(pool, &tributary.Config{
    MaxAttempts: 3,              // Default retry count
    RetryBackoffBase: 2 * time.Second,
    FetchPollInterval: 500 * time.Millisecond,
})
```

## Error Handling Best Practices

### 1. Retryable vs Non-Retryable Errors
```go
func (w *APICallWorker) Work(ctx context.Context, step *tributary.Step[APICallArgs]) error {
    resp, err := http.Get(step.Args.URL)
    if err != nil {
        // Network errors are retryable
        return fmt.Errorf("network error: %w", err)
    }

    if resp.StatusCode == 400 {
        // Bad request is not retryable - don't return error
        // Log and continue, or set error result
        step.Result = []byte(`{"error": "bad_request"}`)
        return nil
    }

    if resp.StatusCode >= 500 {
        // Server errors are retryable
        return fmt.Errorf("server error: %d", resp.StatusCode)
    }

    return nil
}
```

### 2. Partial Failure Handling
```go
type BatchResult struct {
    Successful []string `json:"successful"`
    Failed     []string `json:"failed"`
}

func (w *BatchWorker) Work(ctx context.Context, step *tributary.Step[BatchArgs]) error {
    result := BatchResult{}

    for _, item := range step.Args.Items {
        if err := processItem(item); err != nil {
            result.Failed = append(result.Failed, item)
        } else {
            result.Successful = append(result.Successful, item)
        }
    }

    resultBytes, _ := json.Marshal(result)
    step.Result = resultBytes

    // Don't fail the step, let dependent steps decide
    return nil
}
```

## Security Considerations

### 1. Using Encryption
```go
// Generate key: openssl rand -hex 32
encryptionKey, _ := hex.DecodeString("your-hex-key")
var key [32]byte
copy(key[:], encryptionKey)

client, err := tributary.NewClient(pool, &tributary.Config{
    Hooks: []tributary.Hook{
        tributary.NewEncryptHook(tributary.NewSecretboxEncryptor(key)),
    },
    // ... other config
})
```

### 2. Sensitive Data Handling
```go
type ProcessPaymentArgs struct {
    CardNumber string `json:"card_number"` // Will be encrypted
    Amount     int64  `json:"amount"`
}

// The encryption hook automatically encrypts args before storage
// and decrypts them before execution
```

## Performance Tips for AI Agents

### 1. Queue Organization
```go
client, err := tributary.NewClient(pool, &tributary.Config{
    Queues: map[string]tributary.QueueConfig{
        "default":    {MaxWorkers: 10},
        "cpu_heavy":  {MaxWorkers: 4},   // CPU-bound tasks
        "io_heavy":   {MaxWorkers: 20},  // I/O-bound tasks
        "gpu":        {MaxWorkers: 2},   // GPU tasks
    },
})

// Route steps to appropriate queues
func (CPUIntensiveArgs) InsertOpts() tributary.StepInsertOpts {
    return tributary.StepInsertOpts{
        Queue: "cpu_heavy",
    }
}
```

### 2. Dependency Optimization
```go
// BAD: Linear chain (slow)
// A → B → C → D → E

// GOOD: Parallel where possible
//     B → D
//   /       \
// A           → E
//   \       /
//     C ----

// Only add dependencies when truly needed
workflow.AddTask("prepare_data", PrepareArgs{}, nil)
workflow.AddTask("train_model", TrainArgs{},
    &tributary.WorkflowTaskOpts{DependsOn: []string{"prepare_data"}})
// validate_model can run in parallel with train_model if it doesn't need training results
workflow.AddTask("validate_model", ValidateArgs{},
    &tributary.WorkflowTaskOpts{DependsOn: []string{"prepare_data"}})
```

## Migration Management

```go
// Basic migration
migrator := tributarymigrate.New(databaseURL, nil)
err := migrator.Up()

// Check status
status, err := migrator.Status(ctx)
for _, migration := range status {
    fmt.Printf("Migration %s: applied=%v\n", migration.Name, migration.Applied)
}

// Create new migrations
files, err := migrator.CreateSQLMigration(ctx, "add_user_index")
```

## Common Gotchas for AI Agents

1. **Always implement `Kind()` method** - it's required for all StepArgs
2. **Use proper JSON tags** - args are serialized/deserialized
3. **Handle nil DependencyResults** - check before accessing
4. **Don't block in Work()** - respect context cancellation
5. **Store results as []byte** - use json.Marshal for complex data
6. **Dependencies are by task name** - not by step type

## Example: Complete AI Data Pipeline

```go
// 1. Data ingestion
type IngestArgs struct {
    Source string `json:"source"`
}
func (IngestArgs) Kind() string { return "ingest" }

// 2. Data preprocessing
type PreprocessArgs struct {
    CleaningRules []string `json:"cleaning_rules"`
}
func (PreprocessArgs) Kind() string { return "preprocess" }
func (PreprocessArgs) InsertOpts() tributary.StepInsertOpts {
    return tributary.StepInsertOpts{
        Queue: "cpu_heavy",
        Timeout: 10 * time.Minute,
    }
}

// 3. Model training
type TrainModelArgs struct {
    ModelType string `json:"model_type"`
}
func (TrainModelArgs) Kind() string { return "train_model" }
func (TrainModelArgs) InsertOpts() tributary.StepInsertOpts {
    return tributary.StepInsertOpts{
        Queue: "gpu",
        MaxAttempts: 3,
        Timeout: 60 * time.Minute,
    }
}

// 4. Build workflow
workflow := client.NewWorkflow(&tributary.WorkflowOpts{
    Name: "ai_pipeline",
})

workflow.AddTask("ingest", IngestArgs{Source: "s3://data"}, nil)
workflow.AddTask("preprocess", PreprocessArgs{CleaningRules: []string{"remove_nulls"}},
    &tributary.WorkflowTaskOpts{DependsOn: []string{"ingest"}})
workflow.AddTask("train", TrainModelArgs{ModelType: "transformer"},
    &tributary.WorkflowTaskOpts{DependsOn: []string{"preprocess"}})

workflowID, err := workflow.Execute(ctx)
```

This covers the essential patterns and considerations for AI agents using Tributary effectively.