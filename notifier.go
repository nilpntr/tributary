package tributary

import (
	"context"
	"fmt"
	"time"
)

// startNotificationListener starts a goroutine that listens for PostgreSQL notifications
// and signals workers when new steps are available.
func (c *Client) startNotificationListener(ctx context.Context) {
	c.wg.Add(1)
	go c.runNotificationListener(ctx)
}

// runNotificationListener is the main loop for listening to PostgreSQL notifications.
func (c *Client) runNotificationListener(ctx context.Context) {
	defer c.wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		case <-c.ctx.Done():
			return
		default:
			// Attempt to listen for notifications
			if err := c.listenForNotifications(ctx); err != nil {
				c.config.Logger.Warn("Notification listener error, reconnecting in 5s", Field{Key: "error", Value: err})
				time.Sleep(5 * time.Second)
			}
		}
	}
}

// listenForNotifications establishes a connection and listens for PostgreSQL NOTIFY events.
func (c *Client) listenForNotifications(ctx context.Context) error {
	// Acquire a dedicated connection for LISTEN
	conn, err := c.pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("failed to acquire connection: %w", err)
	}
	defer conn.Release()

	// Subscribe to the notification channels
	_, err = conn.Exec(ctx, "LISTEN "+ChannelStepAvailable)
	if err != nil {
		return fmt.Errorf("failed to LISTEN: %w", err)
	}

	c.config.Logger.Info("PostgreSQL LISTEN active",
		Field{Key: "channel_available", Value: ChannelStepAvailable},
		Field{Key: "channel_completed", Value: ChannelStepCompleted})

	// Also listen for step completion events
	_, err = conn.Exec(ctx, "LISTEN "+ChannelStepCompleted)
	if err != nil {
		return fmt.Errorf("failed to LISTEN: %w", err)
	}

	// Wait for notifications
	// Use a short timeout to periodically check for context cancellation
	for {
		// Check for shutdown first
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-c.ctx.Done():
			return c.ctx.Err()
		default:
		}

		// Wait for notification with a timeout
		notificationCtx, cancel := context.WithTimeout(ctx, 1*time.Second)
		notification, err := conn.Conn().WaitForNotification(notificationCtx)
		cancel() // Always cancel to free resources

		if err != nil {
			// Check if context was cancelled
			if ctx.Err() != nil || c.ctx.Err() != nil {
				return nil
			}
			// Timeout is expected - continue
			continue
		}

		// Notification received - wake up workers
		if notification.Channel == ChannelStepAvailable || notification.Channel == ChannelStepCompleted {
			c.notifyWorkers()
		}
	}
}

// notifyWorkers signals local workers to check for new steps.
// This is called both from LISTEN/NOTIFY and from local insertions.
func (c *Client) notifyWorkers() {
	select {
	case c.workerCh <- struct{}{}:
	default:
		// Channel already has a pending signal
	}
}
