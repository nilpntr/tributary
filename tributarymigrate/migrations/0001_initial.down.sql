-- Rollback initial schema for Tributary workflow system

-- Drop trigger and function
DROP TRIGGER IF EXISTS trigger_step_state_change ON steps;
DROP FUNCTION IF EXISTS notify_step_state_change();

--bun:split

-- Drop tables in reverse dependency order
DROP TABLE IF EXISTS step_dependencies;
DROP TABLE IF EXISTS steps;
DROP TABLE IF EXISTS workflow_executions;