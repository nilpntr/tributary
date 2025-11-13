-- Drop the trigger first (required before dropping the function)
DROP TRIGGER IF EXISTS trigger_step_state_change ON steps;

--bun:split

-- Drop the function
DROP FUNCTION IF EXISTS notify_step_state_change();