-- Function to notify on step state changes
-- Channel names correspond to constants in channels.go:
-- - ChannelStepAvailable = 'tributary_step_available'
-- - ChannelStepCompleted = 'tributary_step_completed'
CREATE
OR REPLACE FUNCTION notify_step_state_change()
    RETURNS TRIGGER AS $$
BEGIN
    IF
NEW.state = 'available' AND OLD.state IS DISTINCT
        FROM 'available' THEN
        PERFORM pg_notify('tributary_step_available',
                          NEW.id::TEXT);
    ELSIF
NEW.state = 'completed' AND OLD.state IS
        DISTINCT FROM 'completed' THEN
        PERFORM pg_notify('tributary_step_completed',
                          NEW.id::TEXT);
END IF;
RETURN NEW;
END;
$$
LANGUAGE plpgsql;

--bun:split

CREATE TRIGGER trigger_step_state_change
    AFTER INSERT OR
UPDATE ON steps
    FOR EACH ROW
    EXECUTE FUNCTION notify_step_state_change();