-- Initial schema for Tributary workflow system

-- Workflow executions table
CREATE TABLE workflow_executions (
    id BIGSERIAL PRIMARY KEY,
    workflow_name TEXT NOT NULL,
    state TEXT NOT NULL DEFAULT 'running', -- running, completed, failed
    metadata JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    finalized_at TIMESTAMPTZ
);

CREATE INDEX idx_workflow_executions_state ON workflow_executions(state);
CREATE INDEX idx_workflow_executions_created_at ON workflow_executions(created_at);

--bun:split

-- Steps table
CREATE TABLE steps (
    id BIGSERIAL PRIMARY KEY,
    workflow_execution_id BIGINT REFERENCES workflow_executions(id) ON DELETE CASCADE,
    task_name TEXT, -- name within workflow (e.g., "create_user")
    state TEXT NOT NULL DEFAULT 'available', -- available, running, completed, cancelled, discarded
    attempt SMALLINT NOT NULL DEFAULT 0,
    max_attempts SMALLINT NOT NULL DEFAULT 25,
    kind TEXT NOT NULL, -- StepArgs.Kind()
    args BYTEA NOT NULL, -- JSON-encoded args (potentially encrypted)
    result BYTEA, -- Step output for dependent steps
    errors TEXT[], -- Error messages from each attempt
    queue TEXT NOT NULL DEFAULT 'default',
    priority SMALLINT NOT NULL DEFAULT 1,
    timeout_seconds INTEGER,
    scheduled_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    attempted_at TIMESTAMPTZ,
    finalized_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_steps_state ON steps(state);
CREATE INDEX idx_steps_kind ON steps(kind);
CREATE INDEX idx_steps_queue ON steps(queue);
CREATE INDEX idx_steps_scheduled_at ON steps(scheduled_at) WHERE state = 'available';
CREATE INDEX idx_steps_workflow_execution_id ON steps(workflow_execution_id);
CREATE INDEX idx_steps_priority_scheduled_at ON steps(priority, scheduled_at) WHERE state = 'available';

--bun:split

-- Step dependencies (DAG edges)
CREATE TABLE step_dependencies (
    id BIGSERIAL PRIMARY KEY,
    step_id BIGINT NOT NULL REFERENCES steps(id) ON DELETE CASCADE,
    depends_on_step_id BIGINT NOT NULL REFERENCES steps(id) ON DELETE CASCADE,
    workflow_execution_id BIGINT NOT NULL REFERENCES workflow_executions(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(step_id, depends_on_step_id)
);

CREATE INDEX idx_step_dependencies_step_id ON step_dependencies(step_id);
CREATE INDEX idx_step_dependencies_depends_on ON step_dependencies(depends_on_step_id);
CREATE INDEX idx_step_dependencies_workflow_execution_id ON step_dependencies(workflow_execution_id);
